import { LitElement, html, css, svg } from '/static/vendor/lit/lit.js';

// <nottario-arch-canvas
//   .nodes=${...} .edges=${...}
//   .expanded=${Set<string>} .focus=${id|''} .selected=${id|''}
//   .query=${''}
//   @select=${e => ...} @focus-changed=${...} @expand-changed=${...}>
//
// Hand-rolled SVG renderer for the architecture diagram. Replaces the
// dagre-driven `arch-graph.js` with containment layout: every node
// renders nested INSIDE its parent. The user never loses the parent
// when they explore deeper.
//
// Children A+B+C of feature f9a7a488:
//   A: containment layout (Layers 1+2).
//   B: orthogonal edge routing + label pills.
//   C: interactions — click, expand/collapse, hover-dim, search,
//      pan, zoom (ctrl+wheel + pinch), Fit, focus mode (smooth zoom
//      with viewBox transition 220ms ease-out-quart, ancestors fade,
//      siblings dim).
//
// State the canvas owns internally (no parent wiring needed):
//   - pan/zoom: free panning and zooming. Reset via `fit()`.
//   - hover: highlights edges + connected nodes; rest dims.
//   - viewBox animation: when `focus` changes, animates from current
//     viewBox to the focused subtree's bounding-box + 20px margin.
//
// State the canvas REFLECTS via properties (parent owns):
//   - `selected` (id): single click selection. Re-emit `select` event
//     so the parent can update the right-rail detail panel.
//   - `expanded` (Set): collapsible containers; the parent persists
//     this to the URL hash. Caret click emits `expand-changed`.
//   - `focus` (id): focus-mode target. Double-click emits
//     `focus-changed`. Esc clears focus.
//   - `query` (string): substring filter; matches stay full opacity,
//     non-matches dim. Parent owns the search input.

class NottarioArchCanvas extends LitElement {
  static properties = {
    nodes:    { type: Array },
    edges:    { type: Array },
    expanded: { type: Object },
    selected: { type: String },
    focus:    { type: String },
    query:    { type: String },

    _viewBox: { state: true },
    _hover:   { state: true },
    _animating: { state: true },
  };

  // Layout constants. Centralised so siblings (toolbar) can read them.
  static LEAF_W      = 160;
  static LEAF_H      = 72;
  static LABEL_STRIP = 28;
  static PAD         = 24;
  static GAP         = 16;
  static CANVAS_PAD  = 24;
  static CORNER_R    = 4;
  static FOCUS_MS    = 220;
  static FOCUS_MARGIN = 20;

  static styles = css`
    :host {
      display: block;
      box-sizing: border-box;
      position: relative;
      /* Clip the SVG content to the host's rounded corners. The parent
         page sets the border + border-radius; without overflow:hidden
         the SVG's white background pokes into the rounded corners and
         covers them. inherit picks up whatever radius the parent set. */
      border-radius: inherit;
      overflow: hidden;
    }
    * { box-sizing: border-box; }

    svg {
      display: block;
      width: 100%;
      height: 100%;
      min-height: 480px;
      background: #ffffff;
      cursor: grab;
      user-select: none;
    }
    svg.dragging { cursor: grabbing; }

    /* Nodes — rx/ry set as ATTRIBUTES on each <rect>, not as CSS.
       CSS rx/ry was added in SVG2 but Firefox (as of 2026) still
       ignores it; only the rx="" / ry="" attributes are universal. */
    .node rect.box {
      stroke: #d1d9e0;
      stroke-width: 1;
    }
    .node.container rect.box { fill: #f6f8fa; }
    .node.leaf      rect.box { fill: #ffffff; }
    .node.selected rect.box { stroke: #0969da; stroke-width: 2; }
    /* Hover/search dim: applied to nodes NOT in the highlighted set. */
    .node.dim { opacity: 0.25; transition: opacity 140ms ease-out; }
    .node       { transition: opacity 140ms ease-out; }
    .node.clickable { cursor: pointer; }

    .kind-chip circle { stroke: none; }
    .kind-chip text {
      font: 600 10.5px/1 ui-monospace, SFMono-Regular, monospace;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      fill: #59636e;
    }
    .caret {
      font: 600 11px/1 ui-monospace, SFMono-Regular, monospace;
      fill: #8b949e;
      pointer-events: none;
    }
    /* Hit overlays — transparent fill, sit ON TOP of the strip content
       so clicks anywhere in the label strip toggle expand. */
    .caret-hit, .strip-hit {
      fill: transparent;
      cursor: pointer;
    }
    .strip-hit:hover ~ text.caret,
    .caret-hit:hover  ~ text.caret { fill: #1f2328; }
    .name {
      font: 600 14px/1.2 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: #1f2328;
    }
    .slug {
      font: 11px/1 ui-monospace, SFMono-Regular, monospace;
      fill: #8b949e;
    }

    /* Edges */
    .edge {
      fill: none;
      stroke: #59636e;
      stroke-width: 1.5;
      transition: opacity 140ms ease-out, stroke 140ms ease-out;
    }
    .edge.selected, .edge.highlight { stroke: #0969da; stroke-width: 2; }
    .edge.dim { opacity: 0.18; }
    .edge-label rect {
      fill: #ffffff;
      stroke: #d1d9e0;
      stroke-width: 1;
    }
    .edge-label text {
      font: 11px/1 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: #1f2328;
    }
    .edge-label.dim { opacity: 0.18; transition: opacity 140ms ease-out; }
    .edge-label { transition: opacity 140ms ease-out; }

    /* Ancestor breadcrumb strip shown in Focus mode */
    .focus-strip {
      fill: #f6f8fa;
      stroke: #d1d9e0;
      stroke-width: 1;
    }
    .focus-strip-text {
      font: 600 11px/1 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: #1f2328;
    }
    .focus-strip-sep { fill: #8b949e; }
    .focus-exit {
      font: 600 11px/1 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: #0969da;
      cursor: pointer;
    }

    .empty-text {
      font: 13px -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: #8b949e;
      font-style: italic;
    }
  `;

  constructor() {
    super();
    this.nodes = [];
    this.edges = [];
    this.expanded = new Set();
    this.selected = '';
    this.focus = '';
    this.query = '';

    this._viewBox = null;         // { x, y, w, h } — null until first layout
    this._hover = '';
    this._animating = false;
    this._dragging = false;
    this._dragOrigin = null;
    this._cachedLayout = null;     // memoise layout between renders
    this._cachedLayoutKey = '';
    this._reducedMotion = false;
    if (typeof window !== 'undefined' && window.matchMedia) {
      this._reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    }
  }

  // ----- Public API for the parent -----

  // Reset pan/zoom to fit the whole layout with margin.
  fit() {
    const layout = this._layout();
    if (!layout.roots.length) return;
    this._animateViewBox({
      x: 0, y: 0, w: layout.width, h: layout.height,
    });
  }

  // Imperative focus setter (also reachable by setting the `focus`
  // property; this is just a convenience).
  focusOn(id) {
    if (this.focus === id) return;
    this.focus = id || '';
    this.dispatchEvent(new CustomEvent('focus-changed', {
      detail: { id: this.focus }, bubbles: true, composed: true,
    }));
  }

  // ----- Lifecycle -----

  connectedCallback() {
    super.connectedCallback();
    this._onKeyDown = (e) => {
      if (e.key === 'Escape') {
        if (this.focus) { this.focusOn(''); e.preventDefault(); }
      }
    };
    window.addEventListener('keydown', this._onKeyDown);
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    window.removeEventListener('keydown', this._onKeyDown);
  }

  updated(changed) {
    super.updated?.(changed);
    // Layout changes when nodes/edges/expanded change.
    if (changed.has('nodes') || changed.has('edges') || changed.has('expanded')) {
      this._cachedLayoutKey = ''; // invalidate cache
    }
    // Animate the viewBox toward the focused subtree when focus changes.
    if (changed.has('focus')) {
      this._animateForFocus();
    }
    // First render: initial viewBox is the full layout fit.
    if (this._viewBox === null) {
      const layout = this._layout();
      if (layout.roots.length) {
        this._viewBox = { x: 0, y: 0, w: layout.width, h: layout.height };
      }
    }
  }

  // ----- Tree + Layout (same as B; refactored to memoise) -----

  _buildTree() {
    const byID = new Map();
    for (const n of this.nodes || []) {
      byID.set(n.ID, { node: n, children: [] });
    }
    const roots = [];
    for (const wrapper of byID.values()) {
      const pid = wrapper.node.ParentID;
      if (pid && byID.has(pid)) {
        byID.get(pid).children.push(wrapper);
      } else {
        roots.push(wrapper);
      }
    }
    const rank = (k) => {
      if (k === 'system') return 0;
      if (k === 'external') return 2;
      return 1;
    };
    roots.sort((a, b) => {
      const rk = rank(a.node.Kind) - rank(b.node.Kind);
      if (rk !== 0) return rk;
      const rp = (a.node.Position ?? 0) - (b.node.Position ?? 0);
      if (rp !== 0) return rp;
      return (a.node.Name || '').localeCompare(b.node.Name || '');
    });
    for (const w of byID.values()) {
      w.children.sort((a, b) => {
        const rp = (a.node.Position ?? 0) - (b.node.Position ?? 0);
        if (rp !== 0) return rp;
        return (a.node.Name || '').localeCompare(b.node.Name || '');
      });
    }
    return { roots, byID };
  }

  // A container is rendered as collapsed if it's NOT in `expanded`.
  // Collapsed containers show as a single bigger leaf (the children
  // aren't laid out at all). Roots default to expanded.
  _isExpanded(w, depth) {
    if (!w.children.length) return false;
    if (depth === 0) return true; // roots always show their immediate children
    return this.expanded?.has?.(w.node.ID) ?? false;
  }

  _packSize(w, depth = 0) {
    if (!w.children.length) {
      w.w = NottarioArchCanvas.LEAF_W;
      w.h = NottarioArchCanvas.LEAF_H;
      w._isContainer = false;
      w._expanded = false;
      return;
    }
    w._isContainer = true;
    w._expanded = this._isExpanded(w, depth);
    if (!w._expanded) {
      // Collapsed container renders as a leaf with a child-count hint.
      w.w = NottarioArchCanvas.LEAF_W;
      w.h = NottarioArchCanvas.LEAF_H;
      return;
    }
    for (const child of w.children) this._packSize(child, depth + 1);
    const cols = Math.max(1, Math.ceil(Math.sqrt(w.children.length)));
    const rows = Math.ceil(w.children.length / cols);
    const colW = new Array(cols).fill(0);
    const rowH = new Array(rows).fill(0);
    w.children.forEach((c, i) => {
      const r = Math.floor(i / cols);
      const cc = i % cols;
      if (c.w > colW[cc]) colW[cc] = c.w;
      if (c.h > rowH[r])  rowH[r]  = c.h;
    });
    const gap = NottarioArchCanvas.GAP;
    const pad = NottarioArchCanvas.PAD;
    const interiorW = colW.reduce((s, x) => s + x, 0) + gap * (cols - 1);
    const interiorH = rowH.reduce((s, x) => s + x, 0) + gap * (rows - 1);
    w.w = interiorW + pad * 2;
    w.h = interiorH + pad * 2 + NottarioArchCanvas.LABEL_STRIP;
    w._grid = { cols, rows, colW, rowH };
  }

  _placeChildren(w) {
    if (!w._isContainer || !w._expanded) return;
    const { cols, rows, colW, rowH } = w._grid;
    const gap = NottarioArchCanvas.GAP;
    const pad = NottarioArchCanvas.PAD;
    const startX = w.x + pad;
    const startY = w.y + NottarioArchCanvas.LABEL_STRIP + pad;
    const colX = new Array(cols).fill(0);
    for (let i = 1; i < cols; i++) colX[i] = colX[i - 1] + colW[i - 1] + gap;
    const rowY = new Array(rows).fill(0);
    for (let i = 1; i < rows; i++) rowY[i] = rowY[i - 1] + rowH[i - 1] + gap;
    w.children.forEach((c, i) => {
      const r = Math.floor(i / cols);
      const cc = i % cols;
      c.x = startX + colX[cc] + (colW[cc] - c.w) / 2;
      c.y = startY + rowY[r];
      this._placeChildren(c);
    });
  }

  _flatten(roots) {
    const out = [];
    const walk = (w) => {
      out.push(w);
      if (w._isContainer && w._expanded) {
        for (const c of w.children) walk(c);
      }
    };
    for (const r of roots) walk(r);
    return out;
  }

  _layout() {
    // Memoise — layout is purely a function of (nodes, edges, expanded).
    const key = JSON.stringify({
      nodes: (this.nodes || []).map(n => n.ID + '|' + n.ParentID),
      expanded: [...(this.expanded || [])].sort(),
    });
    if (this._cachedLayoutKey === key && this._cachedLayout) {
      return this._cachedLayout;
    }
    const { roots, byID } = this._buildTree();
    if (!roots.length) {
      const layout = { roots: [], flat: [], byID, width: 600, height: 480 };
      this._cachedLayout = layout;
      this._cachedLayoutKey = key;
      return layout;
    }
    for (const r of roots) this._packSize(r, 0);
    const gap = NottarioArchCanvas.GAP;
    const pad = NottarioArchCanvas.CANVAS_PAD;
    let cursorX = pad;
    let maxH = 0;
    for (const r of roots) {
      r.x = cursorX;
      r.y = pad;
      cursorX += r.w + gap;
      if (r.h > maxH) maxH = r.h;
      this._placeChildren(r);
    }
    const layout = {
      roots,
      flat: this._flatten(roots),
      byID,
      width:  cursorX - gap + pad,
      height: maxH + pad * 2,
    };
    this._cachedLayout = layout;
    this._cachedLayoutKey = key;
    return layout;
  }

  // ----- Edge routing (from child B) -----

  _anchors(src, tgt) {
    const sCx = src.x + src.w / 2;
    const sCy = src.y + src.h / 2;
    const tCx = tgt.x + tgt.w / 2;
    const tCy = tgt.y + tgt.h / 2;
    const dx = tCx - sCx;
    const dy = tCy - sCy;
    if (Math.abs(dx) >= Math.abs(dy)) {
      if (dx > 0) {
        return [
          { x: src.x + src.w, y: sCy, dir: 'h' },
          { x: tgt.x,         y: tCy, dir: 'h' },
        ];
      }
      return [
        { x: src.x,           y: sCy, dir: 'h' },
        { x: tgt.x + tgt.w,   y: tCy, dir: 'h' },
      ];
    }
    if (dy > 0) {
      return [
        { x: sCx, y: src.y + src.h, dir: 'v' },
        { x: tCx, y: tgt.y,         dir: 'v' },
      ];
    }
    return [
      { x: sCx, y: src.y,           dir: 'v' },
      { x: tCx, y: tgt.y + tgt.h,   dir: 'v' },
    ];
  }
  _waypoints(s, t) {
    if (s.dir === 'h' && t.dir === 'h') {
      const midX = (s.x + t.x) / 2;
      return [{ x: s.x, y: s.y }, { x: midX, y: s.y }, { x: midX, y: t.y }, { x: t.x, y: t.y }];
    }
    if (s.dir === 'v' && t.dir === 'v') {
      const midY = (s.y + t.y) / 2;
      return [{ x: s.x, y: s.y }, { x: s.x, y: midY }, { x: t.x, y: midY }, { x: t.x, y: t.y }];
    }
    if (s.dir === 'h') return [{ x: s.x, y: s.y }, { x: t.x, y: s.y }, { x: t.x, y: t.y }];
    return [{ x: s.x, y: s.y }, { x: s.x, y: t.y }, { x: t.x, y: t.y }];
  }
  _pathD(points) {
    if (points.length < 2) return '';
    const r = NottarioArchCanvas.CORNER_R;
    let d = `M ${points[0].x} ${points[0].y}`;
    for (let i = 1; i < points.length - 1; i++) {
      const prev = points[i - 1];
      const curr = points[i];
      const next = points[i + 1];
      const inDx = Math.sign(curr.x - prev.x);
      const inDy = Math.sign(curr.y - prev.y);
      const outDx = Math.sign(next.x - curr.x);
      const outDy = Math.sign(next.y - curr.y);
      const inLen = Math.hypot(curr.x - prev.x, curr.y - prev.y);
      const outLen = Math.hypot(next.x - curr.x, next.y - curr.y);
      const cr = Math.min(r, inLen / 2, outLen / 2);
      const beforeX = curr.x - inDx * cr;
      const beforeY = curr.y - inDy * cr;
      const afterX  = curr.x + outDx * cr;
      const afterY  = curr.y + outDy * cr;
      d += ` L ${beforeX} ${beforeY}`;
      d += ` Q ${curr.x} ${curr.y} ${afterX} ${afterY}`;
    }
    const last = points[points.length - 1];
    d += ` L ${last.x} ${last.y}`;
    return d;
  }
  _routeEdge(edge, byID) {
    const src = byID.get(edge.FromNodeID);
    const tgt = byID.get(edge.ToNodeID);
    if (!src || !tgt) return null;
    if (typeof src.x !== 'number' || typeof tgt.x !== 'number') return null;
    const [sA, tA] = this._anchors(src, tgt);
    const waypoints = this._waypoints(sA, tA);
    return { d: this._pathD(waypoints), waypoints, edge };
  }
  _labelPosition(waypoints) {
    let best = null;
    for (let i = 0; i < waypoints.length - 1; i++) {
      const a = waypoints[i];
      const b = waypoints[i + 1];
      const isHoriz = a.y === b.y;
      const len = Math.hypot(b.x - a.x, b.y - a.y);
      const score = (isHoriz ? 1 : 0.4) * len;
      if (!best || score > best.score) {
        best = { score, len, isHoriz, mx: (a.x + b.x) / 2, my: (a.y + b.y) / 2 };
      }
    }
    return best;
  }

  // ----- Interaction helpers -----

  // Set of node ids that should stay full-opacity given the current
  // hover/search/focus state. Returns null when no highlighting is
  // active (everything full opacity).
  _highlightedSet(layout) {
    const q = (this.query || '').trim().toLowerCase();
    if (this._hover) {
      // Hover: hovered node + nodes connected by any edge.
      const set = new Set([this._hover]);
      for (const e of this.edges || []) {
        if (e.FromNodeID === this._hover) set.add(e.ToNodeID);
        if (e.ToNodeID   === this._hover) set.add(e.FromNodeID);
      }
      return set;
    }
    if (q) {
      const set = new Set();
      for (const w of layout.flat) {
        const n = w.node;
        if ((n.Name || '').toLowerCase().includes(q) ||
            (n.Slug || '').toLowerCase().includes(q)) {
          set.add(n.ID);
        }
      }
      return set;
    }
    if (this.focus) {
      // Focus mode: the focused subtree.
      const set = new Set();
      const collect = (w) => {
        set.add(w.node.ID);
        if (w.children) for (const c of w.children) collect(c);
      };
      const w = layout.byID.get(this.focus);
      if (w) collect(w);
      return set;
    }
    return null;
  }

  // Animate the SVG viewBox toward target over FOCUS_MS, easing
  // out-quart. prefers-reduced-motion → snap. No-op when target
  // matches current viewBox within a px.
  _animateViewBox(target) {
    if (!this._viewBox) {
      this._viewBox = { ...target };
      return;
    }
    const cur = { ...this._viewBox };
    const dx = Math.abs(target.x - cur.x);
    const dy = Math.abs(target.y - cur.y);
    const dw = Math.abs(target.w - cur.w);
    const dh = Math.abs(target.h - cur.h);
    if (dx + dy + dw + dh < 1) return;
    if (this._reducedMotion) {
      this._viewBox = { ...target };
      this.requestUpdate();
      return;
    }
    const start = performance.now();
    const dur = NottarioArchCanvas.FOCUS_MS;
    this._animating = true;
    const ease = (t) => 1 - Math.pow(1 - t, 4); // out-quart
    const step = (now) => {
      const t = Math.min(1, (now - start) / dur);
      const e = ease(t);
      this._viewBox = {
        x: cur.x + (target.x - cur.x) * e,
        y: cur.y + (target.y - cur.y) * e,
        w: cur.w + (target.w - cur.w) * e,
        h: cur.h + (target.h - cur.h) * e,
      };
      this.requestUpdate();
      if (t < 1) requestAnimationFrame(step);
      else      this._animating = false;
    };
    requestAnimationFrame(step);
  }

  _animateForFocus() {
    const layout = this._layout();
    if (!this.focus) {
      // Unfocus → fit the whole layout.
      this._animateViewBox({ x: 0, y: 0, w: layout.width, h: layout.height });
      return;
    }
    const w = layout.byID.get(this.focus);
    if (!w || typeof w.x !== 'number') return;
    const m = NottarioArchCanvas.FOCUS_MARGIN;
    this._animateViewBox({
      x: w.x - m, y: w.y - m, w: w.w + m * 2, h: w.h + m * 2,
    });
  }

  // ----- Events -----

  _emitSelect(id) {
    this.selected = id;
    this.dispatchEvent(new CustomEvent('select', {
      detail: { id }, bubbles: true, composed: true,
    }));
  }
  _emitExpandChanged() {
    this.dispatchEvent(new CustomEvent('expand-changed', {
      detail: { expanded: [...this.expanded] }, bubbles: true, composed: true,
    }));
  }

  _onNodeClick(e, w) {
    e.stopPropagation();
    this._emitSelect(w.node.ID);
  }
  _onNodeDblClick(e, w) {
    e.stopPropagation();
    this.focusOn(w.node.ID);
  }
  _onCaretClick(e, w) {
    e.stopPropagation();
    const ex = new Set(this.expanded || []);
    if (ex.has(w.node.ID)) ex.delete(w.node.ID);
    else                  ex.add(w.node.ID);
    this.expanded = ex;
    this._emitExpandChanged();
  }
  _onNodeEnter(w) { this._hover = w.node.ID; }
  _onNodeLeave()  { this._hover = ''; }

  // Pan: pointerdown on the SVG background. Pointermove translates
  // the viewBox by an inverted delta (so dragging right scrolls the
  // content right relative to the user).
  _onSvgPointerDown(e) {
    if (e.button !== 0) return;
    // Only pan if the click hit the background (not a node).
    const path = e.composedPath?.() || [];
    if (path.some(el => el?.classList?.contains?.('node'))) return;
    this._dragging = true;
    this._dragOrigin = { x: e.clientX, y: e.clientY, vb: { ...this._viewBox } };
    e.currentTarget.setPointerCapture?.(e.pointerId);
  }
  _onSvgPointerMove(e) {
    if (!this._dragging || !this._dragOrigin) return;
    const svgEl = e.currentTarget;
    const rect = svgEl.getBoundingClientRect();
    const scaleX = this._viewBox.w / rect.width;
    const scaleY = this._viewBox.h / rect.height;
    const dx = (e.clientX - this._dragOrigin.x) * scaleX;
    const dy = (e.clientY - this._dragOrigin.y) * scaleY;
    this._viewBox = {
      ...this._dragOrigin.vb,
      x: this._dragOrigin.vb.x - dx,
      y: this._dragOrigin.vb.y - dy,
    };
    this.requestUpdate();
  }
  _onSvgPointerUp(e) {
    if (!this._dragging) return;
    this._dragging = false;
    this._dragOrigin = null;
    e.currentTarget.releasePointerCapture?.(e.pointerId);
  }
  // Zoom: ctrl/cmd + wheel (or pinch on trackpad which the browser
  // delivers as a ctrl-wheel event). Anchor at cursor.
  _onSvgWheel(e) {
    if (!(e.ctrlKey || e.metaKey)) return;
    e.preventDefault();
    const svgEl = e.currentTarget;
    const rect = svgEl.getBoundingClientRect();
    const cx = (e.clientX - rect.left) / rect.width;
    const cy = (e.clientY - rect.top) / rect.height;
    const factor = e.deltaY < 0 ? 0.88 : 1.12;
    const newW = this._viewBox.w * factor;
    const newH = this._viewBox.h * factor;
    // Keep the cursor anchored: x' = x + (oldW - newW) * cx
    this._viewBox = {
      x: this._viewBox.x + (this._viewBox.w - newW) * cx,
      y: this._viewBox.y + (this._viewBox.h - newH) * cy,
      w: newW,
      h: newH,
    };
    this.requestUpdate();
  }

  // ----- Render -----

  _renderNode(w, dim) {
    const n = w.node;
    const cls = [
      'node',
      w._isContainer ? 'container' : 'leaf',
      this.selected === n.ID ? 'selected' : '',
      'clickable',
      dim ? 'dim' : '',
    ].filter(Boolean).join(' ');
    const dot = kindDotColor(n.Kind);
    const kindLabel = (n.Kind || '').toLowerCase();
    const caret = w._isContainer
      ? (w._expanded ? '▾' : '▸')
      : null;
    const showAsContainer = w._isContainer && w._expanded;

    if (showAsContainer) {
      return svg`
        <g class=${cls} transform=${`translate(${w.x},${w.y})`}
           @click=${(e) => this._onNodeClick(e, w)}
           @dblclick=${(e) => this._onNodeDblClick(e, w)}
           @mouseenter=${() => this._onNodeEnter(w)}
           @mouseleave=${() => this._onNodeLeave()}>
          <rect class="box" x="0" y="0" width=${w.w} height=${w.h} rx="8" ry="8"></rect>
          <line x1="0" y1=${NottarioArchCanvas.LABEL_STRIP} x2=${w.w} y2=${NottarioArchCanvas.LABEL_STRIP}
                stroke="#eaeef2" stroke-width="1"></line>
          <g class="kind-chip" transform="translate(10,10)">
            <circle cx="3" cy="6" r="3" fill=${dot}></circle>
            <text x="10" y="9">${kindLabel}</text>
          </g>
          <text class="name" x=${w.w / 2} y="18" text-anchor="middle">${n.Name}</text>
          ${n.Slug ? svg`
            <text class="slug" x=${w.w / 2} y="46"
                  text-anchor="middle">${n.Slug}</text>
          ` : null}
          <!-- Caret glyph (visual hint only; pointer-events: none). -->
          <text class="caret" x=${w.w - 16} y="18" text-anchor="middle">${caret}</text>
          <!-- Whole header strip catches clicks for expand/collapse.
               Rendered LAST so it sits on top of the strip content. -->
          <rect class="strip-hit" x="0" y="0" width=${w.w} height=${NottarioArchCanvas.LABEL_STRIP}
                @click=${(e) => this._onCaretClick(e, w)}></rect>
        </g>
      `;
    }
    // Collapsed container OR leaf
    const hint = (w._isContainer && !w._expanded)
      ? `${w.children.length} inside`
      : (n.Slug || '');
    return svg`
      <g class=${cls} transform=${`translate(${w.x},${w.y})`}
         @click=${(e) => this._onNodeClick(e, w)}
         @dblclick=${(e) => this._onNodeDblClick(e, w)}
         @mouseenter=${() => this._onNodeEnter(w)}
         @mouseleave=${() => this._onNodeLeave()}>
        <rect class="box" x="0" y="0" width=${w.w} height=${w.h} rx="8" ry="8"></rect>
        <g class="kind-chip" transform="translate(10,10)">
          <circle cx="3" cy="6" r="3" fill=${dot}></circle>
          <text x="10" y="9">${kindLabel}</text>
        </g>
        ${w._isContainer ? svg`
          <text class="caret" x=${w.w - 16} y="18" text-anchor="middle">${caret}</text>
          <rect class="caret-hit" x=${w.w - 32} y="0" width="32" height="28"
                @click=${(e) => this._onCaretClick(e, w)}></rect>
        ` : null}
        <text class="name" x=${w.w / 2} y=${w.h / 2 - 2} text-anchor="middle">${n.Name}</text>
        ${hint ? svg`
          <text class="slug" x=${w.w / 2} y=${w.h / 2 + 16}
                text-anchor="middle">${hint}</text>
        ` : null}
      </g>
    `;
  }

  _renderEdge(routed, dim, highlight) {
    if (!routed) return null;
    const e = routed.edge;
    const isSelected = this.selected === e.FromNodeID || this.selected === e.ToNodeID;
    const cls = [
      'edge',
      isSelected ? 'selected' : '',
      highlight ? 'highlight' : '',
      dim ? 'dim' : '',
    ].filter(Boolean).join(' ');
    const lastWP = routed.waypoints[routed.waypoints.length - 1];
    const prevWP = routed.waypoints[routed.waypoints.length - 2];
    const dx = Math.sign(lastWP.x - prevWP.x);
    const dy = Math.sign(lastWP.y - prevWP.y);
    const aSize = 7;
    let arrow = '';
    if (dx !== 0) {
      const baseX = lastWP.x - dx * aSize;
      const yTop  = lastWP.y - aSize / 2;
      const yBot  = lastWP.y + aSize / 2;
      arrow = `M ${baseX} ${yTop} L ${lastWP.x} ${lastWP.y} L ${baseX} ${yBot} Z`;
    } else {
      const baseY = lastWP.y - dy * aSize;
      const xLeft = lastWP.x - aSize / 2;
      const xRight = lastWP.x + aSize / 2;
      arrow = `M ${xLeft} ${baseY} L ${lastWP.x} ${lastWP.y} L ${xRight} ${baseY} Z`;
    }
    const stroke = (isSelected || highlight) ? '#0969da' : '#59636e';
    return svg`
      <g>
        <path class=${cls} d=${routed.d}></path>
        <path class=${cls} d=${arrow} fill=${stroke} stroke="none"></path>
      </g>
    `;
  }

  _renderEdgeLabel(routed, dim) {
    if (!routed || !routed.edge.Label) return null;
    const label = routed.edge.Label;
    const textWidth = Math.max(24, label.length * 6.5);
    const pillW = textWidth + 10;
    const pillH = 18;
    const pos = this._labelPosition(routed.waypoints);
    if (!pos) return null;
    const x = pos.mx - pillW / 2;
    const y = pos.my - pillH / 2;
    return svg`
      <g class=${'edge-label' + (dim ? ' dim' : '')}>
        <rect x=${x} y=${y} width=${pillW} height=${pillH} rx="4" ry="4"></rect>
        <text x=${pos.mx} y=${pos.my + 4} text-anchor="middle">${label}</text>
      </g>
    `;
  }

  render() {
    const layout = this._layout();
    if (!layout.roots.length) {
      return html`
        <svg viewBox="0 0 600 240" xmlns="http://www.w3.org/2000/svg">
          <text class="empty-text" x="300" y="120" text-anchor="middle">
            No architecture nodes yet.
          </text>
        </svg>
      `;
    }
    const vb = this._viewBox || { x: 0, y: 0, w: layout.width, h: layout.height };
    const wByID = new Map(layout.flat.map(w => [w.node.ID, w]));
    const routedEdges = (this.edges || [])
      .map(e => this._routeEdge(e, wByID))
      .filter(Boolean);

    // Highlighted set: hover > query > focus subtree. null = everything full.
    const hi = this._highlightedSet(layout);
    const isDim = (id) => hi !== null && !hi.has(id);
    const isHighlightedEdge = (e) => hi !== null && hi.has(e.FromNodeID) && hi.has(e.ToNodeID);

    const containers = layout.flat.filter(w => w._isContainer && w._expanded);
    const leavesAndCollapsed = layout.flat.filter(w => !w._isContainer || !w._expanded);

    const dragCls = this._dragging ? 'dragging' : '';

    return html`
      <svg viewBox=${`${vb.x} ${vb.y} ${vb.w} ${vb.h}`}
           class=${dragCls}
           xmlns="http://www.w3.org/2000/svg"
           preserveAspectRatio="xMidYMid meet"
           @pointerdown=${(e) => this._onSvgPointerDown(e)}
           @pointermove=${(e) => this._onSvgPointerMove(e)}
           @pointerup=${(e) => this._onSvgPointerUp(e)}
           @pointercancel=${(e) => this._onSvgPointerUp(e)}
           @wheel=${(e) => this._onSvgWheel(e)}>
        ${containers.map(w => this._renderNode(w, isDim(w.node.ID)))}
        ${routedEdges.map(r => this._renderEdge(r,
          hi !== null && !isHighlightedEdge(r.edge),
          this._hover && (r.edge.FromNodeID === this._hover || r.edge.ToNodeID === this._hover)))}
        ${leavesAndCollapsed.map(w => this._renderNode(w, isDim(w.node.ID)))}
        ${routedEdges.map(r => this._renderEdgeLabel(r,
          hi !== null && !isHighlightedEdge(r.edge)))}
      </svg>
    `;
  }
}

function kindDotColor(kind) {
  switch ((kind || '').toLowerCase()) {
    case 'system':   return '#0969da';
    case 'service':  return '#1f883d';
    case 'module':   return '#8250df';
    case 'external': return '#bc4c00';
    case 'data':     return '#9a6700';
    case 'queue':    return '#cf222e';
    default:         return '#59636e';
  }
}

customElements.define('nottario-arch-canvas', NottarioArchCanvas);
