import { LitElement, html, css, svg } from '/static/vendor/lit/lit.js';

// <nottario-arch-canvas
//   .nodes=${...} .edges=${...}
//   .expanded=${Set<string>} .focus=${id|''} .selected=${id|''}
//   .query=${''}
//   @select=${e => ...} @focus-changed=${...} @expand-changed=${...}>
//
// Hand-rolled SVG renderer for the architecture diagram. Layout is
// computed by vendored elkjs (layered + orthogonal); the result is
// persisted to localStorage so reloads with the same (nodes, edges,
// expanded) signature are instant.

class NottarioArchCanvas extends LitElement {
  static properties = {
    nodes: { type: Array },
    edges: { type: Array },
    expanded: { type: Object },
    selected: { type: String },
    focus: { type: String },
    query: { type: String },
    // When set to an edge ID, highlight ONLY that edge (and its
    // two endpoints) — drives the panel-hover-an-edge UX where the
    // right rail wants the canvas to single-out one connection.
    highlightEdge: { type: String, attribute: 'highlight-edge' },

    _viewBox: { state: true },
    _hover: { state: true },
    _animating: { state: true },
  };

  // Layout constants. Centralised so siblings (toolbar) can read them.
  static LEAF_W = 160; // floor — leaves never go below this
  static LEAF_W_MAX = 280; // ceiling — beyond this, the name wraps to 2 lines
  static LEAF_PAD_H = 12; // horizontal padding inside the box (per side)
  static LEAF_H = 72;
  static LEAF_H_WRAP = 86; // height when the name wraps to two lines
  static LABEL_STRIP = 28;
  static CORNER_R = 4;
  static FOCUS_MS = 220;
  static FOCUS_MARGIN = 20;
  // Font specs MUST mirror the .name / .slug CSS rules below. Used
  // for off-screen canvas measurement when sizing leaf boxes.
  static NAME_FONT =
    '600 14px/1.2 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif';
  static SLUG_FONT = '11px/1 ui-monospace, SFMono-Regular, monospace';
  // Grid used purely by the label-placement obstacle map.
  static GRID_CELL = 8;
  static GRID_BUF = 4;

  static styles = css`
    :host {
      display: block;
      box-sizing: border-box;
      position: relative;
      /* Overflow:hidden so the SVG doesn't poke past the host
         rectangle. border-radius is NOT set here — the parent page
         is in charge of rounding the canvas's corners (per-corner,
         e.g. only left in the split layout so the vertical
         separator on the right stays 100% straight). */
      overflow: hidden;
    }
    * { box-sizing: border-box; }

    svg {
      display: block;
      width: 100%;
      height: 100%;
      min-height: 480px;
      background: var(--bg);
      cursor: grab;
      user-select: none;
    }
    svg.dragging { cursor: grabbing; }

    /* Nodes — rx/ry set as ATTRIBUTES on each <rect>, not as CSS.
       CSS rx/ry was added in SVG2 but Firefox (as of 2026) still
       ignores it; only the rx="" / ry="" attributes are universal. */
    .node rect.box {
      stroke: var(--border);
      stroke-width: 1;
    }
    .node.container rect.box { fill: var(--bg-subtle); }
    .node.leaf      rect.box { fill: var(--bg); }
    .node.selected rect.box { stroke: var(--accent); stroke-width: 2; }
    /* Hover/search dim: applied to nodes NOT in the highlighted set. */
    .node.dim { opacity: 0.25; transition: opacity 140ms ease-out; }
    .node       { transition: opacity 140ms ease-out; }
    .node.clickable { cursor: pointer; }

    .kind-chip circle { stroke: none; }
    .kind-chip text {
      font: 600 10.5px/1 ui-monospace, SFMono-Regular, monospace;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      fill: var(--fg-muted);
    }
    .caret {
      font: 600 11px/1 ui-monospace, SFMono-Regular, monospace;
      fill: var(--gray-5);
      pointer-events: none;
    }
    /* Hit overlays — transparent fill, sit ON TOP of the strip content
       so clicks anywhere in the label strip toggle expand. */
    .caret-hit, .strip-hit {
      fill: transparent;
      cursor: pointer;
    }
    .strip-hit:hover ~ text.caret,
    .caret-hit:hover  ~ text.caret { fill: var(--fg); }
    .name {
      font: 600 14px/1.2 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: var(--fg);
    }
    .slug {
      font: 11px/1 ui-monospace, SFMono-Regular, monospace;
      fill: var(--gray-5);
    }

    /* Edges */
    .edge {
      fill: none;
      stroke: var(--fg-muted);
      stroke-width: 1.5;
      transition: opacity 140ms ease-out, stroke 140ms ease-out;
    }
    .edge.selected, .edge.highlight { stroke: var(--accent); stroke-width: 2; }
    .edge.dim { opacity: 0.18; }
    .edge-label rect {
      fill: var(--bg);
      stroke: var(--border);
      stroke-width: 1;
    }
    .edge-label text {
      font: 11px/1 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: var(--fg);
    }
    .edge-label.dim { opacity: 0.18; transition: opacity 140ms ease-out; }
    .edge-label { transition: opacity 140ms ease-out; }

    /* Ancestor breadcrumb strip shown in Focus mode */
    .focus-strip {
      fill: var(--bg-subtle);
      stroke: var(--border);
      stroke-width: 1;
    }
    .focus-strip-text {
      font: 600 11px/1 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: var(--fg);
    }
    .focus-strip-sep { fill: var(--gray-5); }
    .focus-exit {
      font: 600 11px/1 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: var(--accent);
      cursor: pointer;
    }

    .empty-text {
      font: 13px -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: var(--gray-5);
      font-style: italic;
    }
    .laying-out {
      position: absolute;
      inset: 0;
      display: flex;
      align-items: center;
      justify-content: center;
      font: 13px -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      color: var(--gray-5);
      font-style: italic;
      pointer-events: none;
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
    this.highlightEdge = '';
    this._elkLoadPromise = null;
    this._elkCache = null;
    this._elkCacheKey = '';
    this._elkHydrated = false;

    this._viewBox = null; // { x, y, w, h } — null until first layout
    this._hover = '';
    this._animating = false;
    this._dragging = false;
    this._dragOrigin = null;
    this._reducedMotion = false;
    if (typeof window !== 'undefined' && window.matchMedia) {
      this._reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    }
  }

  // ----- Public API for the parent -----

  // Reset pan/zoom to fit the whole layout with margin. Clears the
  // "user interacted" flag so future data arrivals (edges loading
  // late, expand-state changing) keep re-fitting automatically.
  fit() {
    const layout = this._layout();
    if (!layout.roots.length) return;
    this._userInteractedViewBox = false;
    this._animateViewBox({
      x: 0,
      y: 0,
      w: layout.width,
      h: layout.height,
    });
  }

  // Imperative focus setter (also reachable by setting the `focus`
  // property; this is just a convenience).
  focusOn(id) {
    if (this.focus === id) return;
    this.focus = id || '';
    this.dispatchEvent(
      new CustomEvent('focus-changed', {
        detail: { id: this.focus },
        bubbles: true,
        composed: true,
      }),
    );
  }

  // ----- Lifecycle -----

  connectedCallback() {
    super.connectedCallback();
    this._onKeyDown = (e) => {
      if (e.key === 'Escape') {
        if (this.focus) {
          this.focusOn('');
          e.preventDefault();
        }
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
    const layoutChanged = changed.has('nodes') || changed.has('edges') || changed.has('expanded');
    if (layoutChanged) {
      this._cachedRoutes = null; // route cache is keyed on the layout
      this._cachedRoutesKey = '';
      this._elkCacheKey = ''; // ELK cache shares the same lifetime
    }
    // Animate the viewBox toward the focused subtree when focus changes.
    if (changed.has('focus')) {
      this._animateForFocus();
    }
    // (Re)fit viewBox to the current layout. Two cases worth handling:
    //   • First render — no viewBox yet, set it to the layout bounds.
    //   • Data arrived later (nodes loaded before edges, expanded set
    //     changed, …) — the previous fit was over partial data and
    //     no longer matches what's drawn. Re-fit UNLESS the user has
    //     already manually panned/zoomed in this session.
    if (this._viewBox === null || (layoutChanged && !this._userInteractedViewBox)) {
      const layout = this._layout();
      if (layout.roots.length) {
        this._viewBox = { x: 0, y: 0, w: layout.width, h: layout.height };
      }
    }
  }

  // ----- Tree builder (ELK input shape) -----

  _buildTree() {
    const byID = new Map();
    for (const n of this.nodes || []) {
      byID.set(n.id, { node: n, children: [] });
    }
    const roots = [];
    for (const wrapper of byID.values()) {
      const pid = wrapper.node.parent_id;
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
      const rk = rank(a.node.kind) - rank(b.node.kind);
      if (rk !== 0) return rk;
      const rp = (a.node.position ?? 0) - (b.node.position ?? 0);
      if (rp !== 0) return rp;
      return (a.node.name || '').localeCompare(b.node.name || '');
    });
    for (const w of byID.values()) {
      w.children.sort((a, b) => {
        const rp = (a.node.position ?? 0) - (b.node.position ?? 0);
        if (rp !== 0) return rp;
        return (a.node.name || '').localeCompare(b.node.name || '');
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
    return this.expanded?.has?.(w.node.id) ?? false;
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

  // ----- Layout (ELK-only, with localStorage cache) -----

  _layout() {
    // Layout is purely a function of (nodes, edges, expanded). ELK
    // is async; while it computes (cold start) we return an empty
    // placeholder layout so the render shows "Laying out…" instead
    // of a different engine's positions.
    const key = JSON.stringify({
      // Name + slug enter the key because they drive measured leaf
      // sizes — a rename changes the box width, so the cached layout
      // would no longer be valid.
      nodes: (this.nodes || []).map(
        (n) => `${n.id}|${n.parent_id}|${n.name || ''}|${n.slug || ''}`,
      ),
      edges: (this.edges || []).map((e) => e.from_node_id + '>' + e.to_node_id),
      expanded: [...(this.expanded || [])].sort(),
    });
    if (this._elkCache && this._elkCacheKey === key) return this._elkCache;
    // Try to hydrate from localStorage on first miss so reloads are
    // instant when nothing has changed.
    if (!this._elkHydrated) {
      this._elkHydrated = true;
      const cached = this._loadElkCache(key);
      if (cached) {
        this._elkCache = cached;
        this._elkCacheKey = key;
        return cached;
      }
    }
    this._kickElkLayout(key);
    // Stale cache (e.g. expanded changed) is better than nothing while
    // the new ELK result computes.
    if (this._elkCache) return this._elkCache;
    // Cold start. Return an empty placeholder; render() shows a
    // "Laying out…" message.
    return { roots: [], flat: [], byID: new Map(), width: 0, height: 0, _pending: true };
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
      const afterX = curr.x + outDx * cr;
      const afterY = curr.y + outDy * cr;
      d += ` L ${beforeX} ${beforeY}`;
      d += ` Q ${curr.x} ${curr.y} ${afterX} ${afterY}`;
    }
    const last = points[points.length - 1];
    d += ` L ${last.x} ${last.y}`;
    return d;
  }

  // ----- Text measurement (canvas, off-screen) -----
  //
  // ELK cannot measure text; we feed it explicit pixel widths so a
  // long node name like "GitHub Container Registry" gets a wider box
  // instead of overflowing the default LEAF_W. The 2D canvas context
  // is shared across all calls in the page lifecycle.

  static _measureCtx() {
    if (!this.__mctx) {
      this.__mctx = document.createElement('canvas').getContext('2d');
    }
    return this.__mctx;
  }

  static _measureWidth(text, font) {
    if (!text) return 0;
    const ctx = this._measureCtx();
    ctx.font = font;
    return ctx.measureText(text).width;
  }

  // Returns either [singleLine] or [line1, line2] split at the
  // best space character. If there is no good break (one long word
  // wider than maxW), returns [text] and the caller can let it
  // overflow — better than a midword chop.
  static _wrapAtSpace(text, font, maxW) {
    if (this._measureWidth(text, font) <= maxW) return [text];
    let bestIdx = -1;
    const ctx = this._measureCtx();
    ctx.font = font;
    for (let i = 0; i < text.length; i++) {
      if (text[i] === ' ' && ctx.measureText(text.slice(0, i)).width <= maxW) {
        bestIdx = i;
      }
    }
    if (bestIdx <= 0) return [text];
    return [text.slice(0, bestIdx), text.slice(bestIdx + 1)];
  }

  // Compute (width, height) for a leaf box that fits its name and
  // slug. Width is clamped between LEAF_W (floor) and LEAF_W_MAX
  // (ceiling). When the name still overflows the ceiling, the box
  // grows vertically and the renderer wraps the name onto two lines.
  static _leafSize(name, slug) {
    const nameW = this._measureWidth(name || '', this.NAME_FONT);
    const slugW = this._measureWidth(slug || '', this.SLUG_FONT);
    const desired = Math.max(nameW, slugW) + this.LEAF_PAD_H * 2;
    const w = Math.min(Math.max(desired, this.LEAF_W), this.LEAF_W_MAX);
    const innerW = w - this.LEAF_PAD_H * 2;
    const h = nameW > innerW ? this.LEAF_H_WRAP : this.LEAF_H;
    return { w, h };
  }

  // ----- ELK loader / driver -----

  _ensureElkLoaded() {
    if (typeof window !== 'undefined' && window.ELK) return Promise.resolve();
    if (this._elkLoadPromise) return this._elkLoadPromise;
    this._elkLoadPromise = new Promise((resolve, reject) => {
      const s = document.createElement('script');
      s.src = '/static/vendor/elkjs/elk.bundled.js';
      s.async = true;
      s.onload = () => resolve();
      s.onerror = () => reject(new Error('Failed to load elkjs'));
      document.head.appendChild(s);
    });
    return this._elkLoadPromise;
  }

  async _kickElkLayout(targetKey) {
    if (this._elkComputing === targetKey) return;
    this._elkComputing = targetKey;
    try {
      await this._ensureElkLoaded();
      const out = await this._runElkLayout();
      if (this._elkComputing !== targetKey) return; // superseded
      this._elkCache = out;
      this._elkCacheKey = targetKey;
      // Persist for instant reloads. Defensive: ignore quota / JSON errors.
      this._saveElkCache(targetKey, out);
      // Invalidate route cache. Routes were computed against the
      // placeholder layout we rendered while ELK was still working;
      // ELK has now produced its own positions, so the placeholder
      // routes point at the WRONG coordinates and must be discarded
      // before the next render.
      this._cachedRoutes = null;
      this._cachedRoutesKey = '';
      // Re-fit the viewBox to the new layout bounds unless the user
      // has already manually panned/zoomed.
      if (!this._userInteractedViewBox) this._viewBox = null;
      this.requestUpdate();
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('ELK layout failed:', err);
    } finally {
      if (this._elkComputing === targetKey) this._elkComputing = null;
    }
  }

  _saveElkCache(key, layout) {
    if (typeof localStorage === 'undefined') return;
    try {
      const flatSnap = layout.flat.map((w) => ({
        id: w.node.id,
        x: w.x,
        y: w.y,
        w: w.w,
        h: w.h,
        _relX: w._relX,
        _relY: w._relY,
        _isContainer: !!w._isContainer,
        _expanded: !!w._expanded,
      }));
      const routes = (layout._elkRouted || []).map((r) => ({
        d: r.d,
        waypoints: r.waypoints,
        edgeID: r.edge?.id ?? null,
      }));
      const payload = {
        v: 1,
        key,
        width: layout.width,
        height: layout.height,
        flat: flatSnap,
        routes,
      };
      localStorage.setItem('nottario.arch.elk', JSON.stringify(payload));
    } catch (_) {
      /* quota or serialisation; ignore */
    }
  }

  _loadElkCache(key) {
    if (typeof localStorage === 'undefined') return null;
    try {
      const raw = localStorage.getItem('nottario.arch.elk');
      if (!raw) return null;
      const payload = JSON.parse(raw);
      if (payload?.v !== 1 || payload.key !== key) return null;
      const { roots, byID } = this._buildTree();
      // Apply saved positions to the freshly built wrapper tree.
      for (const snap of payload.flat) {
        const w = byID.get(snap.id);
        if (!w) return null; // tree mismatch → cache invalid
        w.x = snap.x;
        w.y = snap.y;
        w.w = snap.w;
        w.h = snap.h;
        w._relX = snap._relX;
        w._relY = snap._relY;
        w._isContainer = snap._isContainer;
        w._expanded = snap._expanded;
      }
      const flat = this._flatten(roots);
      const edgesByID = new Map();
      for (const e of this.edges || []) edgesByID.set(e.id, e);
      const _elkRouted = payload.routes
        .map((r) => ({
          d: r.d,
          waypoints: r.waypoints,
          edge: edgesByID.get(r.edgeID) || null,
        }))
        .filter((r) => r.edge);
      return { roots, flat, byID, width: payload.width, height: payload.height, _elkRouted };
    } catch (_) {
      return null;
    }
  }

  async _runElkLayout() {
    // Build a tree of wrappers (same shape _buildTree produces) so the
    // downstream code can read .children / parent links.
    const { roots, byID } = this._buildTree();
    if (!roots.length) {
      return { roots: [], flat: [], byID, width: 600, height: 480 };
    }
    // Determine which container wrappers are visually expanded; only
    // those expose their children to ELK. Collapsed containers and
    // leaves get a fixed leaf size.
    const isExpanded = (w, depth) => this._isExpanded(w, depth);
    const elkNodeFor = (w, depth = 0) => {
      const hasKids = (w.children?.length || 0) > 0;
      const expanded = hasKids && isExpanded(w, depth);
      const node = {
        id: w.node.id,
        labels: [{ text: w.node.name || '' }],
        layoutOptions: {
          'elk.padding': '[top=44,left=24,bottom=24,right=24]',
        },
      };
      if (!hasKids || !expanded) {
        const sz = NottarioArchCanvas._leafSize(w.node.name, w.node.slug);
        node.width = sz.w;
        node.height = sz.h;
      } else {
        node.children = w.children.map((c) => elkNodeFor(c, depth + 1));
      }
      return node;
    };
    const elkChildren = roots.map((r) => elkNodeFor(r, 0));
    // Edges: ELK wants source/target as node IDs. We pass the raw IDs;
    // ELK handles hierarchy and projection internally for layered.
    const visibleIDs = new Set();
    const collect = (n) => {
      visibleIDs.add(n.id);
      for (const c of n.children || []) collect(c);
    };
    for (const r of elkChildren) collect(r);
    const elkEdges = [];
    for (const e of this.edges || []) {
      // Only include edges whose endpoints are reachable in the layered
      // graph. For collapsed containers, we project the endpoint to its
      // nearest visible ancestor.
      const project = (id) => {
        if (visibleIDs.has(id)) return id;
        let w = byID.get(id);
        while (w && w.node.parent_id) {
          if (visibleIDs.has(w.node.parent_id)) return w.node.parent_id;
          w = byID.get(w.node.parent_id);
        }
        return null;
      };
      const s = project(e.from_node_id);
      const t = project(e.to_node_id);
      if (!s || !t || s === t) continue;
      elkEdges.push({
        id: `${e.from_node_id}__${e.to_node_id}`,
        sources: [s],
        targets: [t],
        _origEdge: e,
      });
    }
    const elkGraph = {
      id: 'root',
      layoutOptions: {
        'elk.algorithm': 'layered',
        'elk.direction': 'DOWN',
        'elk.edgeRouting': 'ORTHOGONAL',
        'elk.hierarchyHandling': 'INCLUDE_CHILDREN',
        'elk.layered.spacing.nodeNodeBetweenLayers': '64',
        'elk.spacing.nodeNode': '32',
        'elk.spacing.edgeNode': '16',
        'elk.spacing.edgeEdge': '14',
        'elk.layered.nodePlacement.bk.fixedAlignment': 'BALANCED',
        'elk.layered.crossingMinimization.semiInteractive': 'true',
      },
      children: elkChildren,
      edges: elkEdges,
    };
    const elk = new window.ELK();
    const result = await elk.layout(elkGraph);
    // Walk the ELK result, copying positions into wrappers (absolute
    // coords for x/y; w/h for width/height; _relX/_relY mirror x/y
    // relative to the parent so _placeChildren-style code still works).
    const applyPositions = (elkNode, parentAbsX = 0, parentAbsY = 0) => {
      const w = byID.get(elkNode.id);
      if (!w) return;
      const absX = parentAbsX + (elkNode.x ?? 0);
      const absY = parentAbsY + (elkNode.y ?? 0);
      w.x = absX;
      w.y = absY;
      w.w = elkNode.width ?? NottarioArchCanvas.LEAF_W;
      w.h = elkNode.height ?? NottarioArchCanvas.LEAF_H;
      w._relX = elkNode.x ?? 0;
      w._relY = elkNode.y ?? 0;
      w._isContainer = (w.children?.length || 0) > 0;
      w._expanded = w._isContainer && (elkNode.children?.length || 0) > 0;
      for (const c of elkNode.children || []) applyPositions(c, absX, absY);
    };
    for (const n of result.children || []) applyPositions(n, 0, 0);
    // Translate ELK edge sections (sequence of {startPoint, bendPoints,
    // endPoint}) into our waypoints + d.
    const routedEdges = [];
    const wByID = new Map(roots.flatMap((r) => this._flatten([r])).map((w) => [w.node.id, w]));
    // Snap an entry/exit segment to be perpendicular to the node face it
    // touches. Without this, ELK occasionally returns a sub-pixel
    // misaligned last bend-point and the arrowhead ends up tilted with
    // a tiny diagonal segment running into the box border.
    const snapTerminalOrthogonal = (wp, terminalNode, end /* 'start'|'end' */) => {
      if (!terminalNode || wp.length < 2) return;
      const idx = end === 'start' ? 0 : wp.length - 1;
      const sideIdx = end === 'start' ? 1 : wp.length - 2;
      const terminal = wp[idx];
      const neighbour = wp[sideIdx];
      const TOL = 1.5;
      let face = null;
      if (Math.abs(terminal.x - terminalNode.x) <= TOL) face = 'left';
      else if (Math.abs(terminal.x - (terminalNode.x + terminalNode.w)) <= TOL) face = 'right';
      else if (Math.abs(terminal.y - terminalNode.y) <= TOL) face = 'top';
      else if (Math.abs(terminal.y - (terminalNode.y + terminalNode.h)) <= TOL) face = 'bottom';
      if (!face) return;
      const vertFace = face === 'top' || face === 'bottom';
      if (vertFace) {
        // Last segment must be vertical → neighbour.x must equal terminal.x.
        if (Math.abs(neighbour.x - terminal.x) > 0.5) {
          // Insert a corner so the final segment becomes vertical.
          if (end === 'start') wp.splice(1, 0, { x: terminal.x, y: neighbour.y });
          else wp.splice(wp.length - 1, 0, { x: terminal.x, y: neighbour.y });
        }
      } else {
        if (Math.abs(neighbour.y - terminal.y) > 0.5) {
          if (end === 'start') wp.splice(1, 0, { x: neighbour.x, y: terminal.y });
          else wp.splice(wp.length - 1, 0, { x: neighbour.x, y: terminal.y });
        }
      }
    };
    for (const ee of result.edges || []) {
      const elkEdgeEntry = elkEdges.find((x) => x.id === ee.id);
      const orig = elkEdgeEntry?._origEdge;
      if (!orig) continue;
      const wp = [];
      const sec = ee.sections?.[0];
      if (!sec) continue;
      let cx = 0,
        cy = 0;
      if (ee.container) {
        const cw = wByID.get(ee.container);
        if (cw) {
          cx = cw.x;
          cy = cw.y;
        }
      }
      wp.push({ x: cx + sec.startPoint.x, y: cy + sec.startPoint.y });
      for (const bp of sec.bendPoints || []) {
        wp.push({ x: cx + bp.x, y: cy + bp.y });
      }
      wp.push({ x: cx + sec.endPoint.x, y: cy + sec.endPoint.y });
      // Force entry/exit segments to be perpendicular to source/target.
      const srcNode = wByID.get(orig.from_node_id) || wByID.get(elkEdgeEntry.sources[0]);
      const tgtNode = wByID.get(orig.to_node_id) || wByID.get(elkEdgeEntry.targets[0]);
      snapTerminalOrthogonal(wp, srcNode, 'start');
      snapTerminalOrthogonal(wp, tgtNode, 'end');
      routedEdges.push({ d: this._pathD(wp), waypoints: wp, edge: orig });
    }
    const flat = this._flatten(roots);
    const out = {
      roots,
      flat,
      byID,
      width: result.width || 600,
      height: result.height || 480,
      _elkRouted: routedEdges,
    };
    return out;
  }

  // ----- Obstacle grid for label placement -----

  _buildObstacles(layout) {
    const cell = NottarioArchCanvas.GRID_CELL;
    const buf = NottarioArchCanvas.GRID_BUF;
    const W = Math.ceil((layout.width + cell * 4) / cell);
    const H = Math.ceil((layout.height + cell * 4) / cell);
    const grid = new Uint8Array(W * H);
    for (const w of layout.flat) {
      const x0 = Math.max(0, Math.floor((w.x - buf) / cell));
      const y0 = Math.max(0, Math.floor((w.y - buf) / cell));
      const x1 = Math.min(W, Math.ceil((w.x + w.w + buf) / cell));
      const y1 = Math.min(H, Math.ceil((w.y + w.h + buf) / cell));
      for (let y = y0; y < y1; y++) {
        for (let x = x0; x < x1; x++) grid[y * W + x] = 1;
      }
    }
    return { grid, W, H, cell, flat: layout.flat };
  }

  // Pick a position for the label pill that does NOT overlap any
  // laid-out node. Four progressively permissive passes:
  //   1. Dense sampling along every segment, looking for free space.
  //   2. Perpendicular offsets up to ±64px above/below the longest
  //      segment (covers sibling-edge gap cases where the path
  //      itself is inside a buffer halo).
  //   3. 2D rectangular scan ±64 around the path midpoint, picking
  //      the closest free position.
  //   4. As a last resort, place at the longest segment midpoint
  //      (worse than nothing, but at least the label exists).
  _labelPosition(waypoints, pillW, pillH, obs) {
    if (waypoints.length < 2) return null;
    let best = null;
    const consider = (mx, my, isHoriz, score) => {
      if (obs && this._pillIntersectsObstacle(mx, my, pillW, pillH, obs)) return;
      if (!best || score > best.score) {
        best = { score, isHoriz, mx, my };
      }
    };
    // Pass 1 — dense along each segment.
    for (let i = 0; i < waypoints.length - 1; i++) {
      const a = waypoints[i];
      const b = waypoints[i + 1];
      const isHoriz = a.y === b.y;
      const len = Math.hypot(b.x - a.x, b.y - a.y);
      if (len < 8) continue;
      const baseScore = (isHoriz ? 1 : 0.4) * len;
      const steps = Math.max(3, Math.floor(len / 10));
      for (let k = 1; k < steps; k++) {
        const t = k / steps;
        const mx = a.x + (b.x - a.x) * t;
        const my = a.y + (b.y - a.y) * t;
        consider(mx, my, isHoriz, baseScore - Math.abs(t - 0.5) * 0.2 * len);
      }
    }
    if (best) return best;

    // Identify the longest segment for the next two fallback passes.
    let longest = null;
    for (let i = 0; i < waypoints.length - 1; i++) {
      const a = waypoints[i];
      const b = waypoints[i + 1];
      const len = Math.hypot(b.x - a.x, b.y - a.y);
      if (!longest || len > longest.len) {
        longest = {
          len,
          isHoriz: a.y === b.y,
          mx: (a.x + b.x) / 2,
          my: (a.y + b.y) / 2,
        };
      }
    }
    if (!longest) return null;

    // Pass 2 — perpendicular offsets, increasing distance.
    for (const off of [12, -12, 18, -18, 24, -24, 36, -36, 48, -48, 64, -64]) {
      const mx = longest.isHoriz ? longest.mx : longest.mx + off;
      const my = longest.isHoriz ? longest.my + off : longest.my;
      if (!this._pillIntersectsObstacle(mx, my, pillW, pillH, obs)) {
        return { score: 0, isHoriz: longest.isHoriz, mx, my };
      }
    }

    // Pass 3 — 2D rectangular scan ±64 around the midpoint. Prefer
    // the closest-to-midpoint free position; ties broken by smaller
    // perpendicular offset relative to the path direction.
    const step = NottarioArchCanvas.GRID_CELL;
    let nearest = null;
    let nearestDist = Infinity;
    for (let dy = -64; dy <= 64; dy += step) {
      for (let dx = -64; dx <= 64; dx += step) {
        const mx = longest.mx + dx;
        const my = longest.my + dy;
        if (this._pillIntersectsObstacle(mx, my, pillW, pillH, obs)) continue;
        const d = Math.hypot(dx, dy);
        if (d < nearestDist) {
          nearestDist = d;
          nearest = { score: -0.5, isHoriz: longest.isHoriz, mx, my };
        }
      }
    }
    if (nearest) return nearest;

    // Pass 4 — give up, return midpoint even though it overlaps.
    return { score: -1, ...longest };
  }

  // True when the label pill of size (pillW × pillH) centred at
  // (mx, my) overlaps any obstacle cell in the grid.
  _pillIntersectsObstacle(mx, my, pillW, pillH, obs) {
    const { grid, W, H, cell } = obs;
    // Use a slight inset so the pill can graze an obstacle's clearance
    // buffer without being rejected; only a real overlap into the
    // obstacle cells counts.
    const inset = 2;
    const x0 = Math.floor((mx - pillW / 2 + inset) / cell);
    const x1 = Math.ceil((mx + pillW / 2 - inset) / cell);
    const y0 = Math.floor((my - pillH / 2 + inset) / cell);
    const y1 = Math.ceil((my + pillH / 2 - inset) / cell);
    for (let y = Math.max(0, y0); y < Math.min(H, y1); y++) {
      for (let x = Math.max(0, x0); x < Math.min(W, x1); x++) {
        if (grid[y * W + x]) return true;
      }
    }
    return false;
  }

  // ----- Interaction helpers -----

  // Set of node ids that should stay full-opacity given the current
  // hover/search/focus state. Returns null when no highlighting is
  // active (everything full opacity).
  _highlightedSet(layout) {
    // External edge-highlight has top priority: only the two
    // endpoints of that one edge stay full-opacity, everything else
    // dims (and only that edge gets the accent stroke).
    if (this.highlightEdge) {
      const ed = (this.edges || []).find((e) => e.id === this.highlightEdge);
      if (ed) return new Set([ed.from_node_id, ed.to_node_id]);
    }
    const q = (this.query || '').trim().toLowerCase();
    if (this._hover) {
      // Hover: hovered node + nodes connected by any edge.
      const set = new Set([this._hover]);
      for (const e of this.edges || []) {
        if (e.from_node_id === this._hover) set.add(e.to_node_id);
        if (e.to_node_id === this._hover) set.add(e.from_node_id);
      }
      return set;
    }
    if (q) {
      const set = new Set();
      for (const w of layout.flat) {
        const n = w.node;
        if ((n.name || '').toLowerCase().includes(q) || (n.slug || '').toLowerCase().includes(q)) {
          set.add(n.id);
        }
      }
      return set;
    }
    if (this.focus) {
      // Focus mode: the focused subtree.
      const set = new Set();
      const collect = (w) => {
        set.add(w.node.id);
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
      else this._animating = false;
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
      x: w.x - m,
      y: w.y - m,
      w: w.w + m * 2,
      h: w.h + m * 2,
    });
  }

  // ----- Events -----

  _emitSelect(id) {
    this.selected = id;
    this.dispatchEvent(
      new CustomEvent('select', {
        detail: { id },
        bubbles: true,
        composed: true,
      }),
    );
  }
  _emitExpandChanged() {
    this.dispatchEvent(
      new CustomEvent('expand-changed', {
        detail: { expanded: [...this.expanded] },
        bubbles: true,
        composed: true,
      }),
    );
  }

  _onNodeClick(e, w) {
    e.stopPropagation();
    // Collapsed containers: any click on the box also toggles
    // expand (the entire box IS the header at this state — there's
    // no separate body to select without expanding). For expanded
    // containers, the label strip handles toggling and body clicks
    // fall through here to just select. For leaves, just select.
    if (w._isContainer && !w._expanded) {
      const ex = new Set(this.expanded || []);
      ex.add(w.node.id);
      this.expanded = ex;
      this._emitExpandChanged();
    }
    this._emitSelect(w.node.id);
  }
  _onNodeDblClick(e, w) {
    e.stopPropagation();
    this.focusOn(w.node.id);
  }
  _onCaretClick(e, w) {
    e.stopPropagation();
    const ex = new Set(this.expanded || []);
    if (ex.has(w.node.id)) ex.delete(w.node.id);
    else ex.add(w.node.id);
    this.expanded = ex;
    this._emitExpandChanged();
  }
  _onNodeEnter(w) {
    this._hover = w.node.id;
  }
  _onNodeLeave() {
    this._hover = '';
  }

  // Keyboard activation on a focused node. Enter / Space select;
  // Right / Down on a collapsed container expand it; Left / Up on
  // an expanded container collapse it.
  _onNodeKey(e, w) {
    switch (e.key) {
      case 'Enter':
      case ' ':
        e.preventDefault();
        this._emitSelect(w.node.id);
        return;
      case 'ArrowRight':
      case 'ArrowDown':
        if (w._isContainer && !w._expanded) {
          e.preventDefault();
          const ex = new Set(this.expanded || []);
          ex.add(w.node.id);
          this.expanded = ex;
          this._emitExpandChanged();
        }
        return;
      case 'ArrowLeft':
      case 'ArrowUp':
        if (w._isContainer && w._expanded) {
          e.preventDefault();
          const ex = new Set(this.expanded || []);
          ex.delete(w.node.id);
          this.expanded = ex;
          this._emitExpandChanged();
        }
        return;
    }
  }

  // Pan: pointerdown on the SVG background. Pointermove translates
  // the viewBox by an inverted delta (so dragging right scrolls the
  // content right relative to the user).
  _onSvgPointerDown(e) {
    if (e.button !== 0) return;
    // Only pan if the click hit the background (not a node).
    const path = e.composedPath?.() || [];
    if (path.some((el) => el?.classList?.contains?.('node'))) return;
    this._dragging = true;
    this._dragOrigin = { x: e.clientX, y: e.clientY, vb: { ...this._viewBox } };
    e.currentTarget.setPointerCapture?.(e.pointerId);
  }
  _onSvgPointerMove(e) {
    if (!this._dragging || !this._dragOrigin) return;
    const svgEl = e.currentTarget;
    const rect = svgEl.getBoundingClientRect();
    // SVG uses preserveAspectRatio="xMidYMid meet": the content is
    // scaled by the SAME factor on both axes (the larger of the two
    // viewBox/rect ratios). Using separate scaleX/scaleY here would
    // make the axis with the slack feel faster than the other —
    // exactly the bug the user reported. One scale, both axes.
    const scale = Math.max(this._viewBox.w / rect.width, this._viewBox.h / rect.height);
    const dx = (e.clientX - this._dragOrigin.x) * scale;
    const dy = (e.clientY - this._dragOrigin.y) * scale;
    this._viewBox = {
      ...this._dragOrigin.vb,
      x: this._dragOrigin.vb.x - dx,
      y: this._dragOrigin.vb.y - dy,
    };
    this._userInteractedViewBox = true;
    this.requestUpdate();
  }
  _onSvgPointerUp(e) {
    if (!this._dragging) return;
    this._dragging = false;
    this._dragOrigin = null;
    e.currentTarget.releasePointerCapture?.(e.pointerId);
  }
  // Wheel: ctrl/cmd + wheel zooms (anchor at cursor). Without a
  // modifier, the wheel pans the viewBox in both axes at a 1:1
  // CSS-pixel ratio so trackpad two-finger scroll feels like
  // grabbing the canvas. Browsers normalise DOM_DELTA_LINE /
  // DOM_DELTA_PAGE so we scale those back to px.
  _onSvgWheel(e) {
    const svgEl = e.currentTarget;
    const rect = svgEl.getBoundingClientRect();
    if (e.ctrlKey || e.metaKey) {
      e.preventDefault();
      const cx = (e.clientX - rect.left) / rect.width;
      const cy = (e.clientY - rect.top) / rect.height;
      // Scale exponentially with deltaY so trackpad pinch (many tiny
      // events) and mouse-wheel-with-modifier (few large events) feel
      // equivalent. A wheel notch (~100px) gives ~1.65x, a pinch tick
      // (~2px) gives ~1.01x.
      const factor = Math.exp(e.deltaY * 0.005);
      const newW = this._viewBox.w * factor;
      const newH = this._viewBox.h * factor;
      this._viewBox = {
        x: this._viewBox.x + (this._viewBox.w - newW) * cx,
        y: this._viewBox.y + (this._viewBox.h - newH) * cy,
        w: newW,
        h: newH,
      };
      this._userInteractedViewBox = true;
      this.requestUpdate();
      return;
    }
    // Pan. Convert delta to viewBox units so movement matches the
    // user's gesture regardless of current zoom.
    e.preventDefault();
    const lineH = 16; // approximate px per wheel "line"
    const k = e.deltaMode === 1 ? lineH : e.deltaMode === 2 ? rect.height : 1;
    // Same unified scale as drag (see _onSvgPointerMove). With
    // preserveAspectRatio="meet", both axes share one factor.
    const scale = Math.max(this._viewBox.w / rect.width, this._viewBox.h / rect.height);
    this._viewBox = {
      ...this._viewBox,
      x: this._viewBox.x + e.deltaX * k * scale,
      y: this._viewBox.y + e.deltaY * k * scale,
    };
    this._userInteractedViewBox = true;
    this.requestUpdate();
  }

  // ----- Render -----

  _renderNode(w, dim) {
    const n = w.node;
    const cls = [
      'node',
      w._isContainer ? 'container' : 'leaf',
      this.selected === n.id ? 'selected' : '',
      'clickable',
      dim ? 'dim' : '',
    ]
      .filter(Boolean)
      .join(' ');
    const dot = kindDotColor(n.kind);
    const kindLabel = (n.kind || '').toLowerCase();
    const caret = w._isContainer ? (w._expanded ? '▾' : '▸') : null;
    const showAsContainer = w._isContainer && w._expanded;

    if (showAsContainer) {
      return svg`
        <g class=${cls} transform=${`translate(${w.x},${w.y})`}
           role="button"
           tabindex=${this.selected === n.id ? '0' : '-1'}
           aria-label=${`${kindLabel || 'node'} ${n.name}`}
           @click=${(e) => this._onNodeClick(e, w)}
           @dblclick=${(e) => this._onNodeDblClick(e, w)}
           @keydown=${(e) => this._onNodeKey(e, w)}
           @mouseenter=${() => this._onNodeEnter(w)}
           @mouseleave=${() => this._onNodeLeave()}>
          <rect class="box" x="0" y="0" width=${w.w} height=${w.h} rx="8" ry="8"></rect>
          <line x1="0" y1=${NottarioArchCanvas.LABEL_STRIP} x2=${w.w} y2=${NottarioArchCanvas.LABEL_STRIP}
                style="stroke: var(--gray-2)" stroke-width="1"></line>
          <g class="kind-chip" transform="translate(10,10)">
            <circle cx="3" cy="6" r="3" style=${`fill: ${dot}`}></circle>
            <text x="10" y="9">${kindLabel}</text>
          </g>
          <text class="name" x=${w.w / 2} y="18" text-anchor="middle">${n.name}</text>
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
    const hint = w._isContainer && !w._expanded ? `${w.children.length} inside` : n.slug || '';
    return svg`
      <g class=${cls} transform=${`translate(${w.x},${w.y})`}
         @click=${(e) => this._onNodeClick(e, w)}
         @dblclick=${(e) => this._onNodeDblClick(e, w)}
         @mouseenter=${() => this._onNodeEnter(w)}
         @mouseleave=${() => this._onNodeLeave()}>
        <rect class="box" x="0" y="0" width=${w.w} height=${w.h} rx="8" ry="8"></rect>
        <g class="kind-chip" transform="translate(10,10)">
          <circle cx="3" cy="6" r="3" style=${`fill: ${dot}`}></circle>
          <text x="10" y="9">${kindLabel}</text>
        </g>
        ${
          w._isContainer
            ? svg`
          <text class="caret" x=${w.w - 16} y="18" text-anchor="middle">${caret}</text>
          <rect class="caret-hit" x=${w.w - 32} y="0" width="32" height="28"
                @click=${(e) => this._onCaretClick(e, w)}></rect>
        `
            : null
        }
        ${this._renderLeafName(n.name, w.w, w.h, hint)}
      </g>
    `;
  }

  // Render the leaf's name and slug. If the name fits in the box's
  // inner width, draw it on a single line (the original layout). If
  // the name was measured wider than the cap, the box already grew
  // vertically — split the name at the last good space and render
  // two <text> lines, with the slug below.
  _renderLeafName(name, w, h, hint) {
    const innerW = w - NottarioArchCanvas.LEAF_PAD_H * 2;
    const lines = NottarioArchCanvas._wrapAtSpace(name || '', NottarioArchCanvas.NAME_FONT, innerW);
    if (lines.length === 1) {
      return svg`
        <text class="name" x=${w / 2} y=${h / 2 - 2}
              text-anchor="middle">${lines[0]}</text>
        ${
          hint
            ? svg`
          <text class="slug" x=${w / 2} y=${h / 2 + 16}
                text-anchor="middle">${hint}</text>
        `
            : null
        }
      `;
    }
    // Two lines: shift the name block up so the slug still fits.
    return svg`
      <text class="name" x=${w / 2} y=${h / 2 - 10}
            text-anchor="middle">${lines[0]}</text>
      <text class="name" x=${w / 2} y=${h / 2 + 6}
            text-anchor="middle">${lines[1]}</text>
      ${
        hint
          ? svg`
        <text class="slug" x=${w / 2} y=${h / 2 + 22}
              text-anchor="middle">${hint}</text>
      `
          : null
      }
    `;
  }

  _renderEdge(routed, dim, highlight) {
    if (!routed) return null;
    const e = routed.edge;
    const isSelected = this.selected === e.from_node_id || this.selected === e.to_node_id;
    const cls = [
      'edge',
      isSelected ? 'selected' : '',
      highlight ? 'highlight' : '',
      dim ? 'dim' : '',
    ]
      .filter(Boolean)
      .join(' ');
    const lastWP = routed.waypoints[routed.waypoints.length - 1];
    const prevWP = routed.waypoints[routed.waypoints.length - 2];
    // ELK occasionally returns waypoint x/y as floats whose endpoints
    // differ by ~1e-14 (e.g. 139.9999999999... vs 140), enough for
    // Math.sign to report ±1 instead of 0 on the would-be-zero axis.
    // That false direction value tilts the arrowhead. We snap to
    // 0 below a 0.5-px threshold so axis-aligned segments stay so.
    const rawDX = lastWP.x - prevWP.x;
    const rawDY = lastWP.y - prevWP.y;
    const dx = Math.abs(rawDX) < 0.5 ? 0 : Math.sign(rawDX);
    const dy = Math.abs(rawDY) < 0.5 ? 0 : Math.sign(rawDY);
    const aSize = 7;
    // The line should end at the BASE of the arrowhead, not at its
    // tip — otherwise the path and the triangle overlap and the line
    // pokes through the arrow. Build a shortened copy of the
    // waypoints whose final point is pulled back by aSize, then run
    // it through _pathD so corner-rounding still works.
    const shortened = routed.waypoints.slice();
    const li = shortened.length - 1;
    shortened[li] = {
      x: lastWP.x - dx * aSize,
      y: lastWP.y - dy * aSize,
    };
    const dPath = this._pathD(shortened);
    let arrow = '';
    if (dx !== 0) {
      const baseX = lastWP.x - dx * aSize;
      const yTop = lastWP.y - aSize / 2;
      const yBot = lastWP.y + aSize / 2;
      arrow = `M ${baseX} ${yTop} L ${lastWP.x} ${lastWP.y} L ${baseX} ${yBot} Z`;
    } else {
      const baseY = lastWP.y - dy * aSize;
      const xLeft = lastWP.x - aSize / 2;
      const xRight = lastWP.x + aSize / 2;
      arrow = `M ${xLeft} ${baseY} L ${lastWP.x} ${lastWP.y} L ${xRight} ${baseY} Z`;
    }
    const stroke = isSelected || highlight ? 'var(--accent)' : 'var(--fg-muted)';
    return svg`
      <g>
        <path class=${cls} d=${dPath} style=${`stroke: ${stroke}`}></path>
        <path class=${cls} d=${arrow} style=${`fill: ${stroke}; stroke: none`}></path>
      </g>
    `;
  }

  _renderEdgeLabel(routed, dim, obs) {
    if (!routed || !routed.edge.label) return null;
    const label = routed.edge.label;
    const textWidth = Math.max(24, label.length * 6.5);
    const pillW = textWidth + 10;
    const pillH = 18;
    const pos = this._labelPosition(routed.waypoints, pillW, pillH, obs);
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
    // Cold start (ELK still computing, no cache to hydrate from).
    if (layout._pending) {
      return html`
        <svg viewBox="0 0 600 240" xmlns="http://www.w3.org/2000/svg"></svg>
        <div class="laying-out">Laying out…</div>
      `;
    }
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
    let routedEdges, obstacles;
    if (
      this._cachedRoutes &&
      this._cachedRoutesKey === this._elkCacheKey &&
      this._cachedRoutes.forLayout === layout
    ) {
      ({ routedEdges, obstacles } = this._cachedRoutes);
    } else {
      routedEdges = layout._elkRouted || [];
      // Keep a lightweight obstacle map purely so the label
      // placement code (`_renderEdgeLabel`) can avoid printing pills
      // over nodes.
      obstacles = this._buildObstacles(layout);
      this._cachedRoutes = { routedEdges, obstacles, forLayout: layout };
      this._cachedRoutesKey = this._elkCacheKey;
    }

    // Highlighted set: external edge highlight > hover > query >
    // focus subtree. null = everything full.
    const hi = this._highlightedSet(layout);
    const isDim = (id) => hi !== null && !hi.has(id);
    const isHighlightedEdge = (e) => {
      if (this.highlightEdge) return e.id === this.highlightEdge;
      return hi !== null && hi.has(e.from_node_id) && hi.has(e.to_node_id);
    };
    const isAccentedEdge = (e) =>
      (this._hover && (e.from_node_id === this._hover || e.to_node_id === this._hover)) ||
      (this.highlightEdge && e.id === this.highlightEdge);

    const containers = layout.flat.filter((w) => w._isContainer && w._expanded);
    const leavesAndCollapsed = layout.flat.filter((w) => !w._isContainer || !w._expanded);

    const dragCls = this._dragging ? 'dragging' : '';

    return html`
      <svg viewBox=${`${vb.x} ${vb.y} ${vb.w} ${vb.h}`}
           class=${dragCls}
           xmlns="http://www.w3.org/2000/svg"
           preserveAspectRatio="xMidYMid meet"
           role="application"
           aria-label="Architecture diagram"
           tabindex="0"
           @pointerdown=${(e) => this._onSvgPointerDown(e)}
           @pointermove=${(e) => this._onSvgPointerMove(e)}
           @pointerup=${(e) => this._onSvgPointerUp(e)}
           @pointercancel=${(e) => this._onSvgPointerUp(e)}
           @wheel=${(e) => this._onSvgWheel(e)}>
        ${containers.map((w) => this._renderNode(w, isDim(w.node.id)))}
        ${routedEdges.map((r) =>
          this._renderEdge(r, hi !== null && !isHighlightedEdge(r.edge), isAccentedEdge(r.edge)),
        )}
        ${leavesAndCollapsed.map((w) => this._renderNode(w, isDim(w.node.id)))}
        ${routedEdges.map((r) =>
          this._renderEdgeLabel(r, hi !== null && !isHighlightedEdge(r.edge), obstacles),
        )}
      </svg>
    `;
  }
}

function kindDotColor(kind) {
  switch ((kind || '').toLowerCase()) {
    case 'system':
      return 'var(--accent)';
    case 'service':
      return 'var(--success)';
    case 'module':
      return 'var(--role-design)';
    case 'external':
      return 'var(--kind-external)';
    case 'data':
      return 'var(--warning)';
    case 'queue':
      return 'var(--danger)';
    default:
      return 'var(--fg-muted)';
  }
}

customElements.define('nottario-arch-canvas', NottarioArchCanvas);
