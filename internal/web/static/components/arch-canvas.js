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
    // When set to an edge ID, highlight ONLY that edge (and its
    // two endpoints) — drives the panel-hover-an-edge UX where the
    // right rail wants the canvas to single-out one connection.
    highlightEdge: { type: String, attribute: 'highlight-edge' },
    // 'custom' uses our hand-rolled Sugiyama+channel router.
    // 'elk'    uses vendored elkjs (layered+orthogonal) via _elkLayout.
    engine: { type: String },

    _viewBox: { state: true },
    _hover:   { state: true },
    _animating: { state: true },
  };

  // Layout constants. Centralised so siblings (toolbar) can read them.
  static LEAF_W      = 160;
  static LEAF_H      = 72;
  static LABEL_STRIP = 28;
  static PAD         = 24;
  // Horizontal gap between sibling boxes in the same Sugiyama layer.
  // Needs to be wide enough for an arrow + an arrowhead + a label
  // pill to fit between two boxes when an edge enters/leaves
  // sideways. 32px ≈ 16px clear stroke + 8px arrowhead overhang
  // + 8px breathing room.
  static GAP         = 32;
  // Vertical gap between Sugiyama layers. Needs to be at least
  // 2 × stub + N × cell where N is the maximum number of edges
  // crossing this gap (each edge needs its own horizontal track
  // when paths overlap). 96 = 48 (stubs) + 48 (six 8-px tracks).
  // TODO: this is still ad-hoc; the right fix is per-channel
  // track allocation so each crossing edge gets a guaranteed
  // unique y inside the gap instead of relying on A*'s
  // congestion penalty.
  static LAYER_GAP   = 96;
  static CANVAS_PAD  = 24;
  // Constant pitch (in px) between parallel edge tracks AND between
  // face anchors on the same node side. Using a constant — rather than
  // dividing the available space by (N+1) — makes parallel edges sit
  // at predictable, equal distances regardless of how many share the
  // channel or the face.
  static TRACK_PITCH = 14;
  static CORNER_R    = 4;
  static FOCUS_MS    = 220;
  static FOCUS_MARGIN = 20;

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
    this.highlightEdge = '';
    this.engine = 'custom';
    this._elkLoadPromise = null;
    this._elkCache = null;
    this._elkCacheKey = '';

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

  // Reset pan/zoom to fit the whole layout with margin. Clears the
  // "user interacted" flag so future data arrivals (edges loading
  // late, expand-state changing) keep re-fitting automatically.
  fit() {
    const layout = this._layout();
    if (!layout.roots.length) return;
    this._userInteractedViewBox = false;
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
    const layoutChanged = changed.has('nodes') || changed.has('edges') || changed.has('expanded') || changed.has('engine');
    if (layoutChanged) {
      this._cachedLayoutKey = '';   // invalidate layout cache
      this._cachedRoutes = null;    // route cache is keyed on the layout
      this._cachedRoutesKey = '';
      this._elkCacheKey = '';       // ELK cache shares the same lifetime
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

  // ----- Sugiyama compound layout (v4) -----
  //
  // Each "level" (the synthetic root level and each expanded
  // container's children) gets the classic Sugiyama framework:
  //   1. Layer assignment via longest-path on sibling-resolved edges.
  //   2. Crossing minimisation via barycentre heuristic.
  //   3. Coordinate assignment within layers (left-to-right packed).
  //   4. Recursive: containers size to fit their children's sub-layout.
  //
  // Layers go TOP-DOWN (source layer at top, sinks at bottom). For
  // software architecture this matches the convention "callers above
  // callees", e.g. Web UI above Backend, Backend above PostgreSQL.

  // For each global edge, resolve both endpoints to their ancestor
  // child of `parentID` (null means top-level: roots). Returns the
  // list of (srcID, dstID) tuples at this level.
  _nodeByID(id) {
    if (!this._nodeIndex || this._nodeIndex.size !== (this.nodes || []).length) {
      this._nodeIndex = new Map((this.nodes || []).map(n => [n.ID, n]));
    }
    return this._nodeIndex.get(id) || null;
  }

  _levelEdges(children, parentID) {
    const childIDs = new Set(children.map(c => c.node.ID));
    // Walk up the parent chain until we hit a node that IS one of
    // the children at this level. If we exhaust the chain without
    // finding one, the node isn't in any child's subtree at this
    // level so the edge doesn't contribute here.
    const ancestorAtLevel = (id) => {
      let cur = this._nodeByID(id);
      while (cur) {
        if (childIDs.has(cur.ID)) return cur.ID;
        if (!cur.ParentID) return null;
        cur = this._nodeByID(cur.ParentID);
      }
      return null;
    };
    const seen = new Set();
    const out = [];
    for (const e of this.edges || []) {
      const a = ancestorAtLevel(e.FromNodeID);
      const b = ancestorAtLevel(e.ToNodeID);
      if (!a || !b || a === b) continue;
      const k = a + '>' + b;
      if (seen.has(k)) continue;
      seen.add(k);
      out.push([a, b]);
    }
    return out;
  }

  // Longest-path layer assignment. Returns layers[][] (each layer is
  // an ordered array of wrappers from `children`).
  _assignLayers(children, edges) {
    const childByID = new Map(children.map(c => [c.node.ID, c]));
    const inDeg = new Map();
    const adj   = new Map();
    for (const c of children) {
      inDeg.set(c.node.ID, 0);
      adj.set(c.node.ID, []);
    }
    for (const [src, dst] of edges) {
      if (!childByID.has(src) || !childByID.has(dst)) continue;
      adj.get(src).push(dst);
      inDeg.set(dst, inDeg.get(dst) + 1);
    }
    const layer = new Map();
    for (const c of children) layer.set(c.node.ID, 0);
    // Kahn-style topological walk, propagating longest path.
    const remaining = new Map(inDeg);
    const queue = children.filter(c => remaining.get(c.node.ID) === 0).map(c => c.node.ID);
    while (queue.length) {
      const id = queue.shift();
      for (const next of adj.get(id)) {
        layer.set(next, Math.max(layer.get(next), layer.get(id) + 1));
        remaining.set(next, remaining.get(next) - 1);
        if (remaining.get(next) === 0) queue.push(next);
      }
    }
    // Group children into layers, preserving the children array's
    // tie-break order (smart-child-ordering pre-pass + kind rank).
    const maxL = Math.max(0, ...layer.values());
    const layers = Array.from({ length: maxL + 1 }, () => []);
    for (const c of children) layers[layer.get(c.node.ID)].push(c);
    return layers;
  }

  // Barycentre crossing minimisation. 4 sweeps alternating direction.
  _orderInLayers(layers, edges) {
    if (layers.length <= 1) return;
    const allIDs = new Set();
    for (const layer of layers) for (const c of layer) allIDs.add(c.node.ID);
    const adjOut = new Map();
    const adjIn  = new Map();
    for (const id of allIDs) { adjOut.set(id, []); adjIn.set(id, []); }
    for (const [src, dst] of edges) {
      if (!allIDs.has(src) || !allIDs.has(dst)) continue;
      adjOut.get(src).push(dst);
      adjIn.get(dst).push(src);
    }
    for (let iter = 0; iter < 4; iter++) {
      // Down sweep: order each layer by avg of predecessor positions.
      for (let i = 1; i < layers.length; i++) {
        const prevPos = new Map();
        layers[i - 1].forEach((c, idx) => prevPos.set(c.node.ID, idx));
        const sc = (c) => {
          const ins = adjIn.get(c.node.ID);
          if (!ins.length) return 1e9; // stable end
          let s = 0;
          for (const p of ins) s += prevPos.get(p) ?? 0;
          return s / ins.length;
        };
        layers[i].sort((a, b) => sc(a) - sc(b));
      }
      // Up sweep: order each layer by avg of successor positions.
      for (let i = layers.length - 2; i >= 0; i--) {
        const nextPos = new Map();
        layers[i + 1].forEach((c, idx) => nextPos.set(c.node.ID, idx));
        const sc = (c) => {
          const outs = adjOut.get(c.node.ID);
          if (!outs.length) return 1e9;
          let s = 0;
          for (const o of outs) s += nextPos.get(o) ?? 0;
          return s / outs.length;
        };
        layers[i].sort((a, b) => sc(a) - sc(b));
      }
    }
  }

  // Pack a single level (layers) into a bounding box, returning
  // per-cell relative coordinates and the totals. Layers stacked
  // top-down; nodes within a layer left-to-right. When a single
  // layer holds many nodes (e.g. a level with few inter-child edges,
  // so Kahn put them all on layer 0), it's reshaped into a roughly
  // square grid so the parent doesn't blow out horizontally. The
  // wrapped sub-rows use the same `G` gap as siblings within a
  // layer (NOT the bigger `LG`) so they read as one logical group.
  // Edge density between children of the current level; the grid
  // wrap is only useful when there are very few inter-child edges
  // (otherwise the regular Sugiyama layers already break a wide
  // group up into multiple short rows). The caller stashes the
  // current level's edge count in this._levelEdgeDensity before
  // calling _packLevel; we fall back to "many edges" so the wrap
  // never fires when the caller forgot to set it.
  _packLevel(layers) {
    const G  = NottarioArchCanvas.GAP;
    const LG = NottarioArchCanvas.LAYER_GAP;
    const totalNodes = layers.reduce((s, l) => s + l.length, 0);
    const sparseEdges = (this._levelEdgeDensity ?? Infinity) * 2 < totalNodes;
    const WRAP_THRESHOLD = sparseEdges ? 5 : Infinity;
    const gridX = [];
    const gridY = []; // gridY[li] is now an array (one Y per cell)
    let curY = 0;
    let maxW = 0;
    // Phase C adaptive gap: this._adaptiveLayerGap[parentID][li] holds
    // an override LAYER_GAP for the channel BELOW layer `li` of the
    // current parent. _phaseCExpand pre-computes these from edge counts.
    const adaptiveLG = this._currentAdaptiveLG || null;
    for (let li = 0; li < layers.length; li++) {
      const layer = layers[li];
      const n = layer.length;
      const wrap = n >= WRAP_THRESHOLD;
      const cols = wrap ? Math.max(WRAP_THRESHOLD - 1, Math.ceil(Math.sqrt(n))) : n;
      const xs = new Array(n);
      const ys = new Array(n);
      let subRowY = curY;
      let subRowX = 0;
      let rowMaxH = 0;
      let subRowStart = 0;
      let layerMaxW = 0;
      for (let i = 0; i < n; i++) {
        const c = layer[i];
        if (i > subRowStart && (i - subRowStart) >= cols) {
          const rowW = Math.max(0, subRowX - G);
          if (rowW > layerMaxW) layerMaxW = rowW;
          subRowY += rowMaxH + G;
          subRowX = 0;
          rowMaxH = 0;
          subRowStart = i;
        }
        xs[i] = subRowX;
        ys[i] = subRowY;
        subRowX += c.w + G;
        if (c.h > rowMaxH) rowMaxH = c.h;
      }
      const lastRowW = Math.max(0, subRowX - G);
      if (lastRowW > layerMaxW) layerMaxW = lastRowW;
      const layerHeight = (subRowY + rowMaxH) - curY;
      if (layerMaxW > maxW) maxW = layerMaxW;
      gridX.push(xs);
      gridY.push(ys);
      const thisLG = adaptiveLG?.[li] ?? LG;
      curY += layerHeight + thisLG;
    }
    return {
      gridX,
      gridY,
      totalW: maxW,
      totalH: Math.max(0, curY - LG),
    };
  }

  // Recursive Sugiyama on a container wrapper. Computes children's
  // sub-layout first (bottom-up sizing), then layouts THIS container's
  // direct children in layers. Sets w.w / w.h to the container's full
  // bbox and stores relative child positions in c._relX / c._relY.
  _packSugiyama(w, depth = 0) {
    if (!w.children.length) {
      // Phase C: hub-aware sizing. A leaf widens (or grows tall) when
      // _phaseCExpand decided one of its faces needs more room for
      // parallel tracks. _minW/_minH default to LEAF_W/LEAF_H.
      w.w = Math.max(NottarioArchCanvas.LEAF_W, w._minW || 0);
      w.h = Math.max(NottarioArchCanvas.LEAF_H, w._minH || 0);
      w._isContainer = false;
      w._expanded = false;
      return;
    }
    w._isContainer = true;
    w._expanded = this._isExpanded(w, depth);
    if (!w._expanded) {
      w.w = NottarioArchCanvas.LEAF_W;
      w.h = NottarioArchCanvas.LEAF_H;
      return;
    }
    // Recurse first so children have their w/h.
    for (const child of w.children) this._packSugiyama(child, depth + 1);
    const edges = this._levelEdges(w.children, w.node.ID);
    const layers = this._assignLayers(w.children, edges);
    this._orderInLayers(layers, edges);
    this._levelEdgeDensity = edges.length;
    // Phase C adaptive gap: per-container override stashed by
    // _phaseCExpand. _packLevel reads `_currentAdaptiveLG` directly.
    const prevALG = this._currentAdaptiveLG;
    this._currentAdaptiveLG = this._allAdaptiveLG?.get(w.node.ID) || null;
    const placed = this._packLevel(layers);
    this._currentAdaptiveLG = prevALG;
    for (let li = 0; li < layers.length; li++) {
      for (let i = 0; i < layers[li].length; i++) {
        const c = layers[li][i];
        c._relX = placed.gridX[li][i];
        c._relY = placed.gridY[li][i];
      }
    }
    const pad = NottarioArchCanvas.PAD;
    w.w = placed.totalW + pad * 2;
    w.h = placed.totalH + pad * 2 + NottarioArchCanvas.LABEL_STRIP;
  }

  // Place children at their absolute positions based on relative
  // offsets and parent's absolute (x, y). Recursive.
  _placeChildren(w) {
    if (!w._isContainer || !w._expanded) return;
    const pad = NottarioArchCanvas.PAD;
    const startX = w.x + pad;
    const startY = w.y + NottarioArchCanvas.LABEL_STRIP + pad;
    for (const c of w.children) {
      c.x = startX + (c._relX ?? 0);
      c.y = startY + (c._relY ?? 0);
      this._placeChildren(c);
    }
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

  // Reorder each non-root container's children so children with
  // external edges going east end up east in the grid, west-going go
  // west, etc. The score is a signed integer:
  //   - Positive → child's external edges point east (higher
  //     top-level rank than the child's own).
  //   - Negative → external edges point west.
  // Top-level lane order is left untouched (it's already sorted by
  // kind rank in `_buildTree`).
  _orderChildrenByEdges(roots) {
    // Per-node descendant set (including self).
    const desc = new Map();
    const buildDesc = (w) => {
      const set = new Set([w.node.ID]);
      for (const c of w.children) {
        buildDesc(c);
        for (const id of desc.get(c.node.ID)) set.add(id);
      }
      desc.set(w.node.ID, set);
    };
    for (const r of roots) buildDesc(r);

    // Top-level rank per top-level node (the lane order). Then a
    // node-id → top-level-id index so we can score any node.
    const rootRankByID = new Map();
    roots.forEach((r, i) => rootRankByID.set(r.node.ID, i));
    const topAnc = new Map();
    for (const r of roots) {
      for (const id of desc.get(r.node.ID)) topAnc.set(id, r.node.ID);
    }

    // For each non-root container, sort its children by external-edge
    // horizontal preference.
    const order = (container) => {
      if (!container.children.length) return;
      const scores = new Map();
      for (const child of container.children) {
        const childSet = desc.get(child.node.ID);
        let s = 0;
        for (const e of this.edges || []) {
          const fromIn = childSet.has(e.FromNodeID);
          const toIn   = childSet.has(e.ToNodeID);
          if (!fromIn && !toIn)  continue;
          if (fromIn && toIn)    continue; // loop inside this child
          const otherID = fromIn ? e.ToNodeID : e.FromNodeID;
          const otherRank = rootRankByID.get(topAnc.get(otherID)) ?? 1;
          const childRank = rootRankByID.get(topAnc.get(child.node.ID)) ?? 1;
          s += (otherRank - childRank);
        }
        scores.set(child.node.ID, s);
      }
      container.children.sort((a, b) => {
        const sa = scores.get(a.node.ID) ?? 0;
        const sb = scores.get(b.node.ID) ?? 0;
        if (sa !== sb) return sa - sb;
        const rp = (a.node.Position ?? 0) - (b.node.Position ?? 0);
        if (rp !== 0) return rp;
        return (a.node.Name || '').localeCompare(b.node.Name || '');
      });
      for (const c of container.children) order(c);
    };
    for (const r of roots) order(r);
  }

  _layout() {
    // Memoise — layout is purely a function of (engine, nodes, edges, expanded).
    const key = JSON.stringify({
      engine: this.engine || 'custom',
      nodes: (this.nodes || []).map(n => n.ID + '|' + n.ParentID),
      edges: (this.edges || []).map(e => e.FromNodeID + '>' + e.ToNodeID),
      expanded: [...(this.expanded || [])].sort(),
    });
    // ELK is async; we keep its output in `_elkCache` and reuse it.
    // When the cache key matches, return the ELK-computed layout. When
    // it doesn't, fire the async computation (re-renders happen via
    // requestUpdate at completion) and fall through to the hand-rolled
    // layout as a placeholder so the first paint isn't empty.
    const engine = this.engine || 'custom';
    if (engine === 'elk') {
      if (this._elkCache && this._elkCacheKey === key) return this._elkCache;
      this._kickElkLayout(key);
    }
    if (engine === 'sugiyama') {
      if (this._sugCache && this._sugCacheKey === key) return this._sugCache;
      const out = this._runSugiyamaLayout();
      if (out) {
        this._sugCache = out;
        this._sugCacheKey = key;
        // Sugiyama re-computed → route cache (computed against the
        // placeholder layout, if any) must be discarded.
        this._cachedRoutes = null;
        this._cachedRoutesKey = '';
        return out;
      }
    }
    if (this._cachedLayoutKey === key && this._cachedLayout) {
      return this._cachedLayout;
    }
    // Snapshot positions BEFORE recomputing so we can animate to the
    // new positions when state changes (expand/collapse). On the very
    // first layout there's no prior snapshot, so no animation fires.
    const prevPositions = new Map();
    if (this._cachedLayout) {
      for (const w of this._cachedLayout.flat) {
        prevPositions.set(w.node.ID, { x: w.x, y: w.y, w: w.w, h: w.h });
      }
    }
    const { roots, byID } = this._buildTree();
    if (!roots.length) {
      const layout = { roots: [], flat: [], byID, width: 600, height: 480 };
      this._cachedLayout = layout;
      this._cachedLayoutKey = key;
      return layout;
    }
    // Smart child ordering — kind-rank tie-break so the Sugiyama
    // crossing-min has a sensible starting permutation.
    this._orderChildrenByEdges(roots);
    // Size each root (and recursively size descendants) via Sugiyama.
    for (const r of roots) this._packSugiyama(r, 0);
    // Top-level lane: also Sugiyama. Roots stacked top-down by the
    // longest-path layer of cross-root edges.
    const rootEdges = this._levelEdges(roots, null);
    const rootLayers = this._assignLayers(roots, rootEdges);
    this._orderInLayers(rootLayers, rootEdges);
    this._levelEdgeDensity = rootEdges.length;
    const rootPlace = this._packLevel(rootLayers);
    const pad = NottarioArchCanvas.CANVAS_PAD;
    for (let li = 0; li < rootLayers.length; li++) {
      for (let i = 0; i < rootLayers[li].length; i++) {
        const r = rootLayers[li][i];
        r.x = pad + rootPlace.gridX[li][i];
        r.y = pad + rootPlace.gridY[li][i];
        this._placeChildren(r);
      }
    }
    const layout = {
      roots,
      flat: this._flatten(roots),
      byID,
      width:  rootPlace.totalW + pad * 2,
      height: rootPlace.totalH + pad * 2,
    };
    // Phase B — global barycenter reorder. Sugiyama already minimises
    // intra-container crossings; this pass goes further by including
    // edges that LEAVE the container (e.g. an Identity→GitHub edge
    // moves Identity toward GitHub's x within Backend), iterated until
    // no row order changes. Capped at 5 iterations as a safety belt.
    for (let iter = 0; iter < 5; iter++) {
      if (!this._globalBarycenterPass(layout, this.edges || [])) break;
    }
    // Phase C — hub-aware expansion. Count edges per face; for each
    // overloaded face widen/extend the node so all anchors fit at
    // TRACK_PITCH. Also count edges per inter-row channel and grow
    // LAYER_GAP if a channel needs more parallel tracks than the
    // default. If anything expanded, rebuild the layout once.
    if (this._phaseCExpand(layout, roots, byID, rootPlace, pad)) {
      // After expansion, _packSugiyama + _packLevel + _placeChildren
      // ran inside _phaseCExpand. layout.width/height refreshed.
    }
    // Kick reflow animation if we had a prior layout. The animation
    // mutates the wrappers in-place each frame so subsequent renders
    // (from requestUpdate) see the interpolated positions.
    if (prevPositions.size && !this._reducedMotion) {
      this._kickReflowAnimation(prevPositions, layout);
    }
    return layout;
  }

  // ----- Faithful Sugiyama layout engine -----
  // Synchronous (no Promise) so it fits into the existing render flow.
  // Calls into `internal/web/static/layout/sugiyama/index.js`, which
  // implements the published algorithms phase by phase with invariants.
  _runSugiyamaLayout() {
    try {
      if (!this._sugModule) {
        // Lazy load the ES module on first use. Cache the namespace so
        // subsequent calls are synchronous.
        if (!this._sugModulePromise) {
          this._sugModulePromise = import('/static/layout/sugiyama/index.js').then(m => {
            this._sugModule = m;
            this.requestUpdate();
          }).catch(err => {
            // eslint-disable-next-line no-console
            console.warn('Failed to load Sugiyama module:', err);
          });
        }
        return null;
      }
      const { roots, byID } = this._buildTree();
      if (!roots.length) {
        return { roots: [], flat: [], byID, width: 600, height: 480 };
      }
      // Convert our tree to plain nodes/edges for the module.
      const moduleNodes = [];
      const visit = (w) => {
        moduleNodes.push({
          id: w.node.ID,
          parentID: w.node.ParentID || null,
          kind: w.node.Kind,
          name: w.node.Name,
          w: NottarioArchCanvas.LEAF_W,
          h: NottarioArchCanvas.LEAF_H,
        });
        for (const c of w.children || []) visit(c);
      };
      for (const r of roots) visit(r);
      const moduleEdges = (this.edges || []).map(e => ({
        id: e.FromNodeID + '__' + e.ToNodeID,
        src: e.FromNodeID,
        tgt: e.ToNodeID,
        _orig: e,
      }));
      const expanded = new Set(this.expanded || []);
      const result = this._sugModule.layout(
        { nodes: moduleNodes, edges: moduleEdges, expanded },
        { debug: false });
      // Apply positions to the wrapper tree.
      for (const w of byID.values()) {
        const p = result.positions.get(w.node.ID);
        if (!p) continue;
        w.x = p.x; w.y = p.y; w.w = p.w; w.h = p.h;
        w._isContainer = (w.children?.length || 0) > 0;
        w._expanded = w._isContainer && expanded.has(w.node.ID);
      }
      const flat = this._flatten(roots);
      // Translate routes (using original edge objects) into our format.
      const routedEdges = result.routes.map(r => ({
        d: this._pathD(r.waypoints),
        waypoints: r.waypoints,
        edge: r.edge,
      }));
      return {
        roots, flat, byID,
        width:  result.width  || 600,
        height: result.height || 480,
        _elkRouted: routedEdges, // reuse the same field name; render() reads it
      };
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('Sugiyama layout failed, falling back to custom:', err);
      return null;
    }
  }

  // ----- ELK-based layout engine -----
  //
  // Calls vendored elkjs to produce positions + edge waypoints. The
  // result is shaped to look identical to a hand-rolled `_layout()`
  // result (roots[], flat[], byID, width, height with wrappers carrying
  // x/y/w/h and _relX/_relY) so all downstream rendering, animation,
  // focus, hover, and edge label code keeps working unchanged.

  _ensureElkLoaded() {
    if (typeof window !== 'undefined' && window.ELK) return Promise.resolve();
    if (this._elkLoadPromise) return this._elkLoadPromise;
    this._elkLoadPromise = new Promise((resolve, reject) => {
      const s = document.createElement('script');
      s.src = '/static/vendor/elkjs/elk.bundled.js';
      s.async = true;
      s.onload  = () => resolve();
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
      // Invalidate route cache. Routes were computed against the
      // placeholder layout we rendered while ELK was still working;
      // ELK has now produced its own positions, so the placeholder
      // routes point at the WRONG coordinates and must be discarded
      // before the next render.
      this._cachedRoutes = null;
      this._cachedRoutesKey = '';
      this.requestUpdate();
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('ELK layout failed, falling back to custom:', err);
    } finally {
      if (this._elkComputing === targetKey) this._elkComputing = null;
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
        id: w.node.ID,
        labels: [{ text: w.node.Name || '' }],
        layoutOptions: {
          'elk.padding': '[top=44,left=24,bottom=24,right=24]',
        },
      };
      if (!hasKids || !expanded) {
        node.width  = NottarioArchCanvas.LEAF_W;
        node.height = NottarioArchCanvas.LEAF_H;
      } else {
        node.children = w.children.map(c => elkNodeFor(c, depth + 1));
      }
      return node;
    };
    const elkChildren = roots.map(r => elkNodeFor(r, 0));
    // Edges: ELK wants source/target as node IDs. We pass the raw IDs;
    // ELK handles hierarchy and projection internally for layered.
    const visibleIDs = new Set();
    const collect = (n) => {
      visibleIDs.add(n.id);
      for (const c of (n.children || [])) collect(c);
    };
    for (const r of elkChildren) collect(r);
    const elkEdges = [];
    for (const e of (this.edges || [])) {
      // Only include edges whose endpoints are reachable in the layered
      // graph. For collapsed containers, we project the endpoint to its
      // nearest visible ancestor.
      const project = (id) => {
        if (visibleIDs.has(id)) return id;
        let w = byID.get(id);
        while (w && w.node.ParentID) {
          if (visibleIDs.has(w.node.ParentID)) return w.node.ParentID;
          w = byID.get(w.node.ParentID);
        }
        return null;
      };
      const s = project(e.FromNodeID);
      const t = project(e.ToNodeID);
      if (!s || !t || s === t) continue;
      elkEdges.push({
        id: `${e.FromNodeID}__${e.ToNodeID}`,
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
      w.w = elkNode.width  ?? NottarioArchCanvas.LEAF_W;
      w.h = elkNode.height ?? NottarioArchCanvas.LEAF_H;
      w._relX = elkNode.x ?? 0;
      w._relY = elkNode.y ?? 0;
      w._isContainer = (w.children?.length || 0) > 0;
      w._expanded = w._isContainer && (elkNode.children?.length || 0) > 0;
      for (const c of (elkNode.children || [])) applyPositions(c, absX, absY);
    };
    for (const n of result.children || []) applyPositions(n, 0, 0);
    // Translate ELK edge sections (sequence of {startPoint, bendPoints,
    // endPoint}) into our waypoints + d.
    const routedEdges = [];
    const wByID = new Map(roots.flatMap(r => this._flatten([r])).map(w => [w.node.ID, w]));
    // Snap an entry/exit segment to be perpendicular to the node face it
    // touches. Without this, ELK occasionally returns a sub-pixel
    // misaligned last bend-point and the arrowhead ends up tilted with
    // a tiny diagonal segment running into the box border.
    const snapTerminalOrthogonal = (wp, terminalNode, end /* 'start'|'end' */) => {
      if (!terminalNode || wp.length < 2) return;
      const idx     = end === 'start' ? 0 : wp.length - 1;
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
      const vertFace = (face === 'top' || face === 'bottom');
      if (vertFace) {
        // Last segment must be vertical → neighbour.x must equal terminal.x.
        if (Math.abs(neighbour.x - terminal.x) > 0.5) {
          // Insert a corner so the final segment becomes vertical.
          if (end === 'start') wp.splice(1, 0, { x: terminal.x, y: neighbour.y });
          else                 wp.splice(wp.length - 1, 0, { x: terminal.x, y: neighbour.y });
        }
      } else {
        if (Math.abs(neighbour.y - terminal.y) > 0.5) {
          if (end === 'start') wp.splice(1, 0, { x: neighbour.x, y: terminal.y });
          else                 wp.splice(wp.length - 1, 0, { x: neighbour.x, y: terminal.y });
        }
      }
    };
    for (const ee of (result.edges || [])) {
      const elkEdgeEntry = elkEdges.find(x => x.id === ee.id);
      const orig = elkEdgeEntry?._origEdge;
      if (!orig) continue;
      const wp = [];
      const sec = ee.sections?.[0];
      if (!sec) continue;
      let cx = 0, cy = 0;
      if (ee.container) {
        const cw = wByID.get(ee.container);
        if (cw) { cx = cw.x; cy = cw.y; }
      }
      wp.push({ x: cx + sec.startPoint.x, y: cy + sec.startPoint.y });
      for (const bp of (sec.bendPoints || [])) {
        wp.push({ x: cx + bp.x, y: cy + bp.y });
      }
      wp.push({ x: cx + sec.endPoint.x, y: cy + sec.endPoint.y });
      // Force entry/exit segments to be perpendicular to source/target.
      const srcNode = wByID.get(orig.FromNodeID) || wByID.get(elkEdgeEntry.sources[0]);
      const tgtNode = wByID.get(orig.ToNodeID)   || wByID.get(elkEdgeEntry.targets[0]);
      snapTerminalOrthogonal(wp, srcNode, 'start');
      snapTerminalOrthogonal(wp, tgtNode, 'end');
      routedEdges.push({ d: this._pathD(wp), waypoints: wp, edge: orig });
    }
    const flat = this._flatten(roots);
    const out = {
      roots,
      flat,
      byID,
      width:  (result.width  || 600),
      height: (result.height || 480),
      _elkRouted: routedEdges,
    };
    return out;
  }

  // Phase C — hub-aware node and channel expansion. Two responsibilities:
  //
  //   • Face load: count how many edges touch each (node, side). If a
  //     face needs more anchors than fit at TRACK_PITCH (with corner
  //     margins), set _minW/_minH on the LEAF so a fresh _packSugiyama
  //     gives it room.
  //   • Channel load: count edges per inter-layer channel within each
  //     container. If a channel needs more parallel tracks than the
  //     default LAYER_GAP can hold, stash an adaptive LAYER_GAP for
  //     that specific channel; _packLevel reads `_currentAdaptiveLG`
  //     to apply per-layer gap overrides during the re-pack.
  //
  // Returns true when at least one expansion fired (caller may rebuild
  // the layout once more if it likes; here we already do it inline).
  _phaseCExpand(layout, roots, byID, rootPlace, pad) {
    const PITCH = NottarioArchCanvas.TRACK_PITCH;
    const MARGIN = 16;
    const STUB = NottarioArchCanvas.STUB_CELLS * NottarioArchCanvas.GRID_CELL;
    const LG = NottarioArchCanvas.LAYER_GAP;

    // ---- 1. Face load ----
    const faceCount = new Map();
    for (const e of (this.edges || [])) {
      const src = byID.get(e.FromNodeID);
      const tgt = byID.get(e.ToNodeID);
      if (!src || !tgt) continue;
      if (typeof src.x !== 'number' || typeof tgt.x !== 'number') continue;
      const [sSide, tSide] = this._chooseSides(src, tgt);
      const sK = src.node.ID + '|' + sSide;
      const tK = tgt.node.ID + '|' + tSide;
      faceCount.set(sK, (faceCount.get(sK) || 0) + 1);
      faceCount.set(tK, (faceCount.get(tK) || 0) + 1);
    }
    let anyExpand = false;
    for (const [key, n] of faceCount) {
      if (n <= 1) continue;
      const [nodeID, side] = key.split('|');
      const w = byID.get(nodeID);
      if (!w) continue;
      if (w._isContainer && w._expanded) continue; // resize handled by children
      const required = (n - 1) * PITCH + 2 * MARGIN;
      if (side === 'top' || side === 'bottom') {
        if ((w._minW || 0) < required) { w._minW = required; anyExpand = true; }
      } else {
        if ((w._minH || 0) < required) { w._minH = required; anyExpand = true; }
      }
    }

    // ---- 2. Channel load (inter-layer gaps per container) ----
    const chanCount = new Map(); // key: parentID|liBelow → count
    for (const e of (this.edges || [])) {
      const src = byID.get(e.FromNodeID);
      const tgt = byID.get(e.ToNodeID);
      if (!src || !tgt) continue;
      if (typeof src.x !== 'number' || typeof tgt.x !== 'number') continue;
      const [sSide, tSide] = this._chooseSides(src, tgt);
      const sV = sSide === 'top' || sSide === 'bottom';
      const tV = tSide === 'top' || tSide === 'bottom';
      if (!sV || !tV) continue;
      // Identify the inter-layer y the edge crosses through. Use the
      // midpoint between source bottom and target top.
      const sFY = sSide === 'bottom' ? src.y + src.h : src.y;
      const tFY = tSide === 'top'    ? tgt.y         : tgt.y + tgt.h;
      const midY = (sFY + tFY) / 2;
      // Find a container whose children straddle midY (the channel
      // lives in that container).
      for (const w of layout.flat) {
        if (!w._isContainer || !w._expanded) continue;
        if (midY <= w.y || midY >= w.y + w.h) continue;
        // Walk the children; identify the row pair the midY sits in.
        const ys = [...new Set(w.children.map(c => Math.round(c.y)))].sort((a, b) => a - b);
        for (let i = 0; i < ys.length - 1; i++) {
          const rowTop = ys[i];
          const rowBot = ys[i + 1];
          // Row block i ends at top of row i+1: gap channel is
          // (rowTop + LEAF_H, rowTop + LEAF_H + LG).
          // Use child heights instead of LEAF_H to be robust.
          const childH = w.children.find(c => Math.round(c.y) === rowTop)?.h
                       || NottarioArchCanvas.LEAF_H;
          const channelTop = rowTop + childH;
          const channelBot = rowBot;
          if (midY >= channelTop && midY <= channelBot) {
            const key = w.node.ID + '|' + i;
            chanCount.set(key, (chanCount.get(key) || 0) + 1);
          }
        }
        break;
      }
    }
    const adaptiveLG = new Map(); // containerID → { liBelow: gapPx }
    for (const [key, n] of chanCount) {
      if (n <= 1) continue;
      const [parentID, liStr] = key.split('|');
      const li = +liStr;
      const required = (n - 1) * PITCH + 2 * STUB + 2 * MARGIN;
      if (required > LG) {
        if (!adaptiveLG.has(parentID)) adaptiveLG.set(parentID, {});
        adaptiveLG.get(parentID)[li] = required;
        anyExpand = true;
      }
    }

    if (!anyExpand) return false;

    // ---- 3. Re-pack the layout with new sizes / gaps ----
    // _allAdaptiveLG is consulted INSIDE _packSugiyama, per container.
    this._allAdaptiveLG = adaptiveLG;
    for (const r of roots) this._packSugiyama(r, 0);
    this._allAdaptiveLG = null;

    // Re-pack the root level.
    const rootEdges = this._levelEdges(roots, null);
    const rootLayers = this._assignLayers(roots, rootEdges);
    this._orderInLayers(rootLayers, rootEdges);
    this._levelEdgeDensity = rootEdges.length;
    const newRootPlace = this._packLevel(rootLayers);
    for (let li = 0; li < rootLayers.length; li++) {
      for (let i = 0; i < rootLayers[li].length; i++) {
        const r = rootLayers[li][i];
        r.x = pad + newRootPlace.gridX[li][i];
        r.y = pad + newRootPlace.gridY[li][i];
        this._placeChildren(r);
      }
    }
    layout.width  = newRootPlace.totalW + pad * 2;
    layout.height = newRootPlace.totalH + pad * 2;
    return true;
  }

  // Phase B — single barycenter pass across the whole layout. For each
  // expanded container, group its children by row, then for each row
  // reorder by the average x-coordinate of every node each child is
  // connected to (in absolute coords, so an edge into an external
  // ancestor like GitHub counts toward the source-side child's
  // ideal x). Returns true if any row order changed; the caller
  // re-iterates until stable. Sibling-only barycenter (the kind Sugiyama
  // already does via _orderInLayers) is captured here too because the
  // sibling's centre is just another connected x.
  _globalBarycenterPass(layout, edges) {
    const absPos = new Map();
    for (const w of layout.flat) {
      if (typeof w.x === 'number') {
        absPos.set(w.node.ID, { cx: w.x + w.w / 2, cy: w.y + w.h / 2 });
      }
    }
    const descCache = new Map();
    const collectDesc = (w) => {
      if (descCache.has(w.node.ID)) return descCache.get(w.node.ID);
      const set = new Set([w.node.ID]);
      if (w.children) for (const c of w.children) {
        for (const id of collectDesc(c)) set.add(id);
      }
      descCache.set(w.node.ID, set);
      return set;
    };
    let anyChange = false;
    const containers = layout.flat.filter(w =>
      w._isContainer && w._expanded && (w.children?.length || 0) > 1);
    const G = NottarioArchCanvas.GAP;
    for (const c of containers) {
      const rows = new Map();
      for (const child of c.children) {
        const yKey = Math.round(child._relY ?? 0);
        if (!rows.has(yKey)) rows.set(yKey, []);
        rows.get(yKey).push(child);
      }
      for (const row of rows.values()) {
        if (row.length <= 1) continue;
        for (const child of row) {
          const descs = collectDesc(child);
          let sum = 0, n = 0;
          for (const e of edges) {
            const inSrc = descs.has(e.FromNodeID);
            const inDst = descs.has(e.ToNodeID);
            if (inSrc === inDst) continue; // both inside or both outside → skip
            const otherID = inSrc ? e.ToNodeID : e.FromNodeID;
            const p = absPos.get(otherID);
            if (!p) continue;
            sum += p.cx; n++;
          }
          child._idealX = n > 0 ? (sum / n) : (child.x + child.w / 2);
        }
        const oldOrder = row.map(c => c.node.ID).join(',');
        row.sort((a, b) => a._idealX - b._idealX);
        const newOrder = row.map(c => c.node.ID).join(',');
        if (oldOrder !== newOrder) {
          anyChange = true;
          // Reassign _relX within the row (preserve total row width).
          let cur = row[0]._relX; // anchor at leftmost original x
          for (const child of row) {
            child._relX = cur;
            cur += child.w + G;
          }
        }
      }
    }
    if (anyChange) {
      // Recompute absolute positions so the next iteration sees the
      // new geometry. roots' x/y are already set; placeChildren walks
      // down using _relX/_relY.
      for (const r of layout.roots) this._placeChildren(r);
    }
    return anyChange;
  }

  // Animate every wrapper's x/y/w/h from its previous position to
  // its newly computed target over 400ms ease-out-quart. Cancels any
  // in-flight animation; chains feel smooth because the prev snapshot
  // is whatever positions the wrappers held at the moment of the
  // state change (mid-animation included).
  _kickReflowAnimation(prevPositions, layout) {
    if (this._reflowRaf != null) cancelAnimationFrame(this._reflowRaf);
    // Capture target positions so we can lerp.
    const targets = new Map();
    for (const w of layout.flat) {
      targets.set(w.node.ID, { x: w.x, y: w.y, w: w.w, h: w.h });
    }
    // Snap wrappers to prev positions so the first render of this
    // state shows the OLD layout, then animate forward.
    for (const w of layout.flat) {
      const p = prevPositions.get(w.node.ID);
      if (p) { w.x = p.x; w.y = p.y; w.w = p.w; w.h = p.h; }
      // Nodes that didn't exist in prev (newly visible due to expand)
      // start at their target position so they don't fly in from 0,0.
    }
    const start = performance.now();
    const dur = 400;
    const ease = (t) => 1 - Math.pow(1 - t, 4);
    const tick = (now) => {
      const t = Math.min(1, (now - start) / dur);
      const e = ease(t);
      for (const w of layout.flat) {
        const p = prevPositions.get(w.node.ID);
        const tg = targets.get(w.node.ID);
        if (!tg) continue;
        if (!p) { w.x = tg.x; w.y = tg.y; w.w = tg.w; w.h = tg.h; continue; }
        w.x = p.x + (tg.x - p.x) * e;
        w.y = p.y + (tg.y - p.y) * e;
        w.w = p.w + (tg.w - p.w) * e;
        w.h = p.h + (tg.h - p.h) * e;
      }
      this.requestUpdate();
      if (t < 1) this._reflowRaf = requestAnimationFrame(tick);
      else        this._reflowRaf = null;
    };
    this._reflowRaf = requestAnimationFrame(tick);
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

  // ----- Channel router (Phase A: deterministic gap-track routing) -----
  //
  // For each edge:
  //   1. Choose source/target face based on the geometric relation
  //      between the two boxes.
  //   2. Spread anchors along the face so N edges sharing a face don't
  //      stack on the same coordinate.
  //   3. Detect obstacles between source and target. If a non-ancestor
  //      box would be crossed, route via the closest "column strip" or
  //      "row strip" of free space (5-bend detour). Otherwise use a
  //      2-bend mid-track path.
  //   4. Allocate a unique track per shared channel so parallel edges
  //      don't overlap.
  //   5. Assemble waypoints. Done.
  //
  // No A*, no congestion penalty, no fallback chain.
  _routeChannels(edges, byID, layout) {
    const STUB = NottarioArchCanvas.STUB_CELLS * NottarioArchCanvas.GRID_CELL;

    // ---- 1. Face planning ----
    const planned = [];
    for (const edge of (edges || [])) {
      const src = byID.get(edge.FromNodeID);
      const tgt = byID.get(edge.ToNodeID);
      if (!src || !tgt) continue;
      if (typeof src.x !== 'number' || typeof tgt.x !== 'number') continue;
      const [sSide, tSide] = this._chooseSides(src, tgt);
      planned.push({ edge, src, tgt, sSide, tSide, sFrac: 0.5, tFrac: 0.5 });
    }

    // ---- 2. Spread anchors along each (node, face) bucket ----
    const buckets = new Map();
    const push = (k, e, end) => {
      if (!buckets.has(k)) buckets.set(k, []);
      buckets.get(k).push({ entry: e, end });
    };
    for (const p of planned) {
      push(`${p.src.node.ID}|${p.sSide}`, p, 's');
      push(`${p.tgt.node.ID}|${p.tSide}`, p, 't');
    }
    const PITCH = NottarioArchCanvas.TRACK_PITCH;
    for (const [key, list] of buckets) {
      const side = key.split('|')[1];
      const perpOf = (it) => {
        const other = it.end === 's' ? it.entry.tgt : it.entry.src;
        if (side === 'top' || side === 'bottom') return other.x + other.w / 2;
        return other.y + other.h / 2;
      };
      list.sort((a, b) => perpOf(a) - perpOf(b));
      const n = list.length;
      // Constant-pitch face spread: anchors sit at `PITCH` px apart,
      // centred on the node face. With node face length L and N
      // anchors, the spread occupies (N-1)·PITCH px of L. We clamp
      // each anchor's frac to [0.05, 0.95] so the stub stays inside
      // the rounded corner radius; if the bundle exceeds the face
      // length the clamp collapses ties (Phase C will detect this
      // and grow the node).
      list.forEach((it, i) => {
        const self = it.end === 's' ? it.entry.src : it.entry.tgt;
        const L = (side === 'top' || side === 'bottom') ? self.w : self.h;
        const center = 0.5;
        const offsetPx = (i - (n - 1) / 2) * PITCH;
        let frac = center + offsetPx / L;
        if (frac < 0.05) frac = 0.05;
        if (frac > 0.95) frac = 0.95;
        if (it.end === 's') it.entry.sFrac = frac;
        else                it.entry.tFrac = frac;
      });
    }

    // ---- 3a. Per-edge: anchor + stub geometry ----
    for (const p of planned) {
      p.sAnchor = this._faceAnchorPx(p.src, p.sSide, p.sFrac);
      p.tAnchor = this._faceAnchorPx(p.tgt, p.tSide, p.tFrac);
      p.sStub   = this._stubAnchorPx(p.src, p.sSide, p.sFrac, STUB);
      p.tStub   = this._stubAnchorPx(p.tgt, p.tSide, p.tFrac, STUB);
    }

    // ---- 3b. Obstacle detection helpers ----
    const ancestorsSet = (w) => {
      const out = new Set();
      let curID = w?.node?.ParentID;
      while (curID) {
        const a = byID.get(curID);
        if (!a) break;
        out.add(a);
        curID = a.node.ParentID;
      }
      return out;
    };
    // Returns true if [y0, y1] at x crosses any non-exempt node's
    // rect (ignoring source/target/ancestors).
    const verticalBlocked = (x, y0, y1, exempt) => {
      const lo = Math.min(y0, y1), hi = Math.max(y0, y1);
      for (const w of layout.flat) {
        if (exempt.has(w)) continue;
        if (x > w.x + 2 && x < w.x + w.w - 2 && lo < w.y + w.h - 2 && hi > w.y + 2) return w;
      }
      return null;
    };
    const horizontalBlocked = (y, x0, x1, exempt) => {
      const lo = Math.min(x0, x1), hi = Math.max(x0, x1);
      for (const w of layout.flat) {
        if (exempt.has(w)) continue;
        if (y > w.y + 2 && y < w.y + w.h - 2 && lo < w.x + w.w - 2 && hi > w.x + 2) return w;
      }
      return null;
    };

    // ---- 4. Track allocation per channel ----
    // For each "vertical-vertical" edge (bottom→top or top→bottom)
    // sharing a (srcStubY, tgtStubY) bucket, allocate unique midY.
    // For "horizontal-horizontal" similarly with midX. For mixed (L)
    // edges, no shared track — they're each unique.
    const vvGroups = new Map(); // key: "vv|y0|y1" → [planEntries]
    const hhGroups = new Map();
    for (const p of planned) {
      const sV = p.sSide === 'top' || p.sSide === 'bottom';
      const tV = p.tSide === 'top' || p.tSide === 'bottom';
      if (sV && tV) {
        p.bend = 'vv';
        const k = `vv|${Math.round(p.sStub.y)}|${Math.round(p.tStub.y)}`;
        if (!vvGroups.has(k)) vvGroups.set(k, []);
        vvGroups.get(k).push(p);
      } else if (!sV && !tV) {
        p.bend = 'hh';
        const k = `hh|${Math.round(p.sStub.x)}|${Math.round(p.tStub.x)}`;
        if (!hhGroups.has(k)) hhGroups.set(k, []);
        hhGroups.get(k).push(p);
      } else {
        p.bend = 'L';
      }
    }
    // Constant-pitch channel tracks: parallel edges sit `PITCH` px
    // apart, centred on the midpoint between the two stubs. The
    // bundle thus occupies (N-1)·PITCH px of channel height/width
    // regardless of N — visually neighbouring channels look uniform.
    for (const list of vvGroups.values()) {
      list.sort((a, b) =>
        ((a.sStub.x + a.tStub.x) / 2) - ((b.sStub.x + b.tStub.x) / 2));
      const n = list.length;
      const sY = list[0].sStub.y, tY = list[0].tStub.y;
      const center = (sY + tY) / 2;
      list.forEach((p, i) => {
        const offset = (i - (n - 1) / 2) * PITCH;
        p.midY = center + offset;
      });
    }
    for (const list of hhGroups.values()) {
      list.sort((a, b) =>
        ((a.sStub.y + a.tStub.y) / 2) - ((b.sStub.y + b.tStub.y) / 2));
      const n = list.length;
      const sX = list[0].sStub.x, tX = list[0].tStub.x;
      const center = (sX + tX) / 2;
      list.forEach((p, i) => {
        const offset = (i - (n - 1) / 2) * PITCH;
        p.midX = center + offset;
      });
    }

    // ---- 5. Waypoint assembly per edge ----
    // 5a. For each edge: try the canonical short path. If it crosses
    //     a non-exempt box, switch to a column-strip / row-strip
    //     detour (5-bend).
    const routed = [];
    for (const p of planned) {
      const wp = this._assembleEdgePath(p, layout, ancestorsSet,
                                        verticalBlocked, horizontalBlocked);
      if (wp && wp.length >= 2) {
        routed.push({ d: '', waypoints: wp, edge: p.edge });
      }
    }
    // 5b. Global track separation. The per-channel allocation above
    //     only sees edges sharing the same (srcStubY, tgtStubY) bucket;
    //     edges from different buckets whose detour columns/rows happen
    //     to collide are NOT separated until this pass. We shift the
    //     middle (non-face-anchored) verticals & horizontals apart by
    //     TRACK_PITCH so parallel detours sit next to each other rather
    //     than on top of each other.
    this._separateOverlappingSegments(routed);
    for (const r of routed) r.d = this._pathD(r.waypoints);
    return { routed, planned };
  }

  _separateOverlappingSegments(routed) {
    const PITCH = NottarioArchCanvas.TRACK_PITCH;
    const TOL = 4;
    const OVERLAP_MIN = 6;
    // Helper: pick segments where both endpoints can move freely without
    // detaching from a face anchor. The face anchor lives at waypoints[0]
    // and waypoints[length-1]; any vertical at the same x as either of
    // those (or any horizontal at the same y) feeds the stub and must
    // stay put.
    const collectV = () => {
      const out = [];
      for (const r of routed) {
        const wp = r.waypoints;
        const firstX = wp[0].x, lastX = wp[wp.length - 1].x;
        for (let i = 0; i < wp.length - 1; i++) {
          const a = wp[i], b = wp[i + 1];
          if (Math.abs(a.x - b.x) >= 0.5 || Math.abs(a.y - b.y) <= 1) continue;
          if (Math.abs(a.x - firstX) < 0.5 || Math.abs(a.x - lastX) < 0.5) continue;
          out.push({ r, i, x: a.x, y0: Math.min(a.y, b.y), y1: Math.max(a.y, b.y) });
        }
      }
      return out;
    };
    const collectH = () => {
      const out = [];
      for (const r of routed) {
        const wp = r.waypoints;
        const firstY = wp[0].y, lastY = wp[wp.length - 1].y;
        for (let i = 0; i < wp.length - 1; i++) {
          const a = wp[i], b = wp[i + 1];
          if (Math.abs(a.y - b.y) >= 0.5 || Math.abs(a.x - b.x) <= 1) continue;
          if (Math.abs(a.y - firstY) < 0.5 || Math.abs(a.y - lastY) < 0.5) continue;
          out.push({ r, i, y: a.y, x0: Math.min(a.x, b.x), x1: Math.max(a.x, b.x) });
        }
      }
      return out;
    };
    const groupAndSpread = (segs, axis) => {
      const co = axis === 'v' ? 'x' : 'y';
      const lo = axis === 'v' ? 'y0' : 'x0';
      const hi = axis === 'v' ? 'y1' : 'x1';
      segs.sort((a, b) => a[co] - b[co]);
      let i = 0;
      while (i < segs.length) {
        let j = i + 1;
        while (j < segs.length && segs[j][co] - segs[i][co] < TOL) j++;
        const group = segs.slice(i, j);
        if (group.length > 1) {
          let conflict = false;
          for (let a = 0; a < group.length && !conflict; a++) {
            for (let b = a + 1; b < group.length && !conflict; b++) {
              const ov = Math.min(group[a][hi], group[b][hi]) -
                         Math.max(group[a][lo], group[b][lo]);
              if (ov > OVERLAP_MIN) conflict = true;
            }
          }
          if (conflict) {
            group.sort((a, b) => a[lo] - b[lo]);
            const center = group.reduce((s, g) => s + g[co], 0) / group.length;
            group.forEach((g, k) => {
              const newCo = center + (k - (group.length - 1) / 2) * PITCH;
              g.r.waypoints[g.i][co] = newCo;
              g.r.waypoints[g.i + 1][co] = newCo;
            });
          }
        }
        i = j;
      }
    };
    // Two iterations: first verticals, then horizontals, then
    // verticals again so any new vertical collision created by the
    // horizontal pass is also resolved.
    groupAndSpread(collectV(), 'v');
    groupAndSpread(collectH(), 'h');
    groupAndSpread(collectV(), 'v');
  }

  // Build the actual waypoint list for one planned edge, retrying with
  // a detour if the straight mid-path would slice through a box.
  _assembleEdgePath(p, layout, ancestorsOf, vBlock, hBlock) {
    const exempt = new Set([p.src, p.tgt, ...ancestorsOf(p.src), ...ancestorsOf(p.tgt)]);
    const wp = [p.sAnchor, p.sStub];

    if (p.bend === 'vv') {
      const sameX = Math.abs(p.sStub.x - p.tStub.x) < 0.5;
      const sVertCross = sameX
        ? vBlock(p.sStub.x, p.sStub.y, p.tStub.y, exempt)
        : vBlock(p.sStub.x, p.sStub.y, p.midY, exempt);
      const horizCross = sameX ? null
        : hBlock(p.midY, Math.min(p.sStub.x, p.tStub.x),
                          Math.max(p.sStub.x, p.tStub.x), exempt);
      const tVertCross = sameX ? null
        : vBlock(p.tStub.x, p.midY, p.tStub.y, exempt);
      const clean = !sVertCross && !horizCross && !tVertCross;
      if (clean) {
        if (!sameX) {
          wp.push({ x: p.sStub.x, y: p.midY });
          wp.push({ x: p.tStub.x, y: p.midY });
        }
      } else {
        // 4-bend detour. Hops at sStub.y and tStub.y stay inside the
        // row strips just outside the source and target rows (which
        // are guaranteed clear of the row contents the stub already
        // cleared). Vertical hop runs through the chosen column strip
        // for the whole span.
        const detourX = this._findColumnDetour(p, layout, exempt, vBlock, hBlock);
        if (detourX != null) {
          wp.push({ x: detourX, y: p.sStub.y });
          wp.push({ x: detourX, y: p.tStub.y });
        } else if (!sameX) {
          // No clean detour; emit canonical (may cross — surfaces the
          // case for layout-tuning later).
          wp.push({ x: p.sStub.x, y: p.midY });
          wp.push({ x: p.tStub.x, y: p.midY });
        }
      }
    } else if (p.bend === 'hh') {
      const sameY = Math.abs(p.sStub.y - p.tStub.y) < 0.5;
      const sHorizCross = sameY
        ? hBlock(p.sStub.y, p.sStub.x, p.tStub.x, exempt)
        : hBlock(p.sStub.y, p.sStub.x, p.midX, exempt);
      const vertCross = sameY ? null
        : vBlock(p.midX, Math.min(p.sStub.y, p.tStub.y),
                          Math.max(p.sStub.y, p.tStub.y), exempt);
      const tHorizCross = sameY ? null
        : hBlock(p.tStub.y, p.midX, p.tStub.x, exempt);
      const clean = !sHorizCross && !vertCross && !tHorizCross;
      if (clean) {
        if (!sameY) {
          wp.push({ x: p.midX, y: p.sStub.y });
          wp.push({ x: p.midX, y: p.tStub.y });
        }
      } else {
        const detourY = this._findRowDetour(p, layout, exempt, vBlock, hBlock);
        if (detourY != null) {
          wp.push({ x: p.sStub.x, y: detourY });
          wp.push({ x: p.tStub.x, y: detourY });
        } else if (!sameY) {
          wp.push({ x: p.midX, y: p.sStub.y });
          wp.push({ x: p.midX, y: p.tStub.y });
        }
      }
    } else {
      const sV = p.sSide === 'top' || p.sSide === 'bottom';
      if (sV) wp.push({ x: p.sStub.x, y: p.tStub.y });
      else    wp.push({ x: p.tStub.x, y: p.sStub.y });
    }

    wp.push(p.tStub);
    wp.push(p.tAnchor);
    return this._simplifyOrtho(wp);
  }

  // Pick a vertical column x where a top-to-bottom run from sStub.y
  // to tStub.y clears all non-exempt boxes, AND the two horizontal
  // hops at sStub.y and tStub.y (between source/target x and the
  // chosen column) are also clear. Candidates are the natural gaps
  // between adjacent node columns (left edge − pad, right edge + pad).
  _findColumnDetour(p, layout, exempt, vBlock, hBlock) {
    const PAD = 12;
    const candidates = new Set();
    for (const w of layout.flat) {
      candidates.add(w.x - PAD);
      candidates.add(w.x + w.w + PAD);
    }
    let best = null, bestCost = Infinity;
    const targetX = (p.sStub.x + p.tStub.x) / 2;
    for (const x of candidates) {
      if (vBlock(x, p.sStub.y, p.tStub.y, exempt)) continue;
      if (hBlock(p.sStub.y, Math.min(p.sStub.x, x), Math.max(p.sStub.x, x), exempt)) continue;
      if (hBlock(p.tStub.y, Math.min(p.tStub.x, x), Math.max(p.tStub.x, x), exempt)) continue;
      const cost = Math.abs(x - targetX);
      if (cost < bestCost) { bestCost = cost; best = x; }
    }
    return best;
  }

  _findRowDetour(p, layout, exempt, vBlock, hBlock) {
    const PAD = 12;
    const candidates = new Set();
    for (const w of layout.flat) {
      candidates.add(w.y - PAD);
      candidates.add(w.y + w.h + PAD);
    }
    let best = null, bestCost = Infinity;
    const targetY = (p.sStub.y + p.tStub.y) / 2;
    for (const y of candidates) {
      if (hBlock(y, p.sStub.x, p.tStub.x, exempt)) continue;
      if (vBlock(p.sStub.x, Math.min(p.sStub.y, y), Math.max(p.sStub.y, y), exempt)) continue;
      if (vBlock(p.tStub.x, Math.min(p.tStub.y, y), Math.max(p.tStub.y, y), exempt)) continue;
      const cost = Math.abs(y - targetY);
      if (cost < bestCost) { bestCost = cost; best = y; }
    }
    return best;
  }

  // ----- Legacy A* router (kept for fallback compatibility; unused) -----

  // Build a coarse 8px grid over the laid-out canvas where EVERY
  // node (leaf, collapsed container, AND expanded container) is a
  // blocked cell (with a 4px buffer). Edges that legitimately need
  // to cross an expanded container's interior — because the source
  // or target is nested inside it — get a per-edge exemption in A*
  // for the ancestor chain; everything else (sibling boxes, unrelated
  // expanded containers) acts as a hard wall, so edges always rotate
  // around visible boxes instead of slicing under them.
  static GRID_CELL  = 8;
  static GRID_BUF   = 4;

  _buildObstacles(layout) {
    const cell = NottarioArchCanvas.GRID_CELL;
    const buf  = NottarioArchCanvas.GRID_BUF;
    const W = Math.ceil((layout.width  + cell * 4) / cell);
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

  // Walk up the parent chain, returning every visible ancestor wrapper
  // (skipping the wrapper itself). Used by edge routing to exempt the
  // containers an edge legitimately needs to traverse from the global
  // "everything blocks" obstacle set.
  _ancestorWrappers(w, byID) {
    const out = [];
    let curID = w?.node?.ParentID;
    while (curID) {
      const parent = byID.get(curID);
      if (!parent) break;
      if (typeof parent.x === 'number') out.push(parent);
      curID = parent.node.ParentID;
    }
    return out;
  }

  // True iff the grid cell (cx, cy) intersects the node's rect plus
  // buffer halo. Same predicate `_buildObstacles` uses to mark cells.
  _cellTouchesNode(cx, cy, node, obs) {
    const cell = obs.cell;
    const buf  = NottarioArchCanvas.GRID_BUF;
    const px = cx * cell;
    const py = cy * cell;
    return px + cell > node.x - buf && px < node.x + node.w + buf &&
           py + cell > node.y - buf && py < node.y + node.h + buf;
  }

  // Pick the grid cell adjacent to a node's chosen exit face. `frac`
  // ∈ (0,1) picks the position along the face: 0.5 = centre, smaller
  // values closer to the top/left, larger values closer to the
  // bottom/right. Used by the edge-spread pass so two edges sharing
  // a face don't overlap. Returns the outer-face cell even when it
  // lies in another node's buffer; A* exempts those buffer-only
  // regions for the source and target.
  // Number of cells the A* anchor sits OUT from the face. Larger
  // values give a longer guaranteed straight stub before the first
  // corner, so an edge never appears to ride along the box outline:
  // it leaves perpendicular for STUB_CELLS × GRID_CELL pixels first,
  // and only then is A* allowed to bend. 5 cells = 40px at
  // GRID_CELL=8 — wider than the label-pill padding so the stub
  // breathes past the label before turning.
  static STUB_CELLS  = 5;

  _anchorCell(node, side, obs, frac = 0.5, offsetCells = NottarioArchCanvas.STUB_CELLS) {
    const { cell, W, H } = obs;
    const fx = node.x + node.w * frac;
    const fy = node.y + node.h * frac;
    const off = cell * offsetCells;
    let ax, ay;
    switch (side) {
      case 'right':  ax = node.x + node.w + off;       ay = fy; break;
      case 'left':   ax = node.x - off;                ay = fy; break;
      case 'bottom': ax = fx; ay = node.y + node.h + off;      break;
      case 'top':    ax = fx; ay = node.y - off;               break;
      default:       ax = node.x + node.w / 2; ay = node.y + node.h / 2;
    }
    const gx = Math.round(ax / cell);
    const gy = Math.round(ay / cell);
    if (gx < 0 || gx >= W || gy < 0 || gy >= H) return null;
    return { x: gx, y: gy, side };
  }

  // Pixel coords of the stub anchor — the point STUB_CELLS cells
  // outside the face, on the same row/column as the face anchor.
  // The rendered path goes (face) → straight to stub → A* route
  // → stub → (face). The straight segments at each end guarantee
  // the corner happens well away from the arrowhead.
  _stubAnchorPx(node, side, frac = 0.5, stubLen = NottarioArchCanvas.GRID_CELL * NottarioArchCanvas.STUB_CELLS) {
    const fx = node.x + node.w * frac;
    const fy = node.y + node.h * frac;
    switch (side) {
      case 'right':  return { x: node.x + node.w + stubLen, y: fy };
      case 'left':   return { x: node.x - stubLen,           y: fy };
      case 'bottom': return { x: fx, y: node.y + node.h + stubLen };
      case 'top':    return { x: fx, y: node.y - stubLen };
      default:       return { x: node.x + node.w / 2, y: node.y + node.h / 2 };
    }
  }

  // True when the given cell is inside the BUFFER-ONLY halo of `node`
  // (the 4px clearance around its real rect, but not inside the rect
  // itself). Used by A* to exempt the source/target buffers so two
  // adjacent siblings can still route into each other.
  _inNodeBufferOnly(gx, gy, node, obs) {
    const cell = obs.cell;
    const px = gx * cell;
    const py = gy * cell;
    // The actual node rect (no buffer) — A* must always treat this
    // as an obstacle, even for source/target. Containers when
    // expanded only block their label strip.
    const innerTopH = (node._isContainer && node._expanded)
      ? NottarioArchCanvas.LABEL_STRIP : node.h;
    const insideRect =
      px + cell > node.x && px < node.x + node.w &&
      py + cell > node.y && py < node.y + innerTopH;
    if (insideRect) return false;
    // The buffer-extended rect (the same area marked obstacle in the
    // global grid).
    const buf = NottarioArchCanvas.GRID_BUF;
    const insideBuffer =
      px + cell > node.x - buf && px < node.x + node.w + buf &&
      py + cell > node.y - buf && py < node.y + innerTopH + buf;
    return insideBuffer;
  }

  // 4-connected A* over the obstacle grid with a small direction-change
  // penalty so paths prefer long straight runs. Source and target
  // nodes' buffer-only halos are exempted so the path can start/end
  // even when the anchor cell falls inside an adjacent neighbour's
  // buffer (sibling pair case).
  //
  // `congestion` is an optional Uint8Array (one byte per cell, 0 =
  // free, >0 = previously used by another routed edge). It adds a
  // soft cost so subsequent edges prefer fresh corridors — the
  // standard orthogonal-connector-routing approach (Wybrow et al.,
  // Graph Drawing 2009) used by ELK and similar libraries to keep
  // parallel edges from bundling onto a single track.
  _astar(obs, start, goal, srcNode, tgtNode, congestion = null, ancestorRects = null) {
    const { grid, W, H, flat } = obs;
    const idx = (x, y) => y * W + x;
    const heur = (x, y) => Math.abs(goal.x - x) + Math.abs(goal.y - y);
    // Stub corridor: cells in the source/target node's buffer halo are
    // ONLY passable along the anchor's perpendicular column (= the
    // straight stub from face to start/goal). Without this constraint,
    // A* could glide along the entire buffer halo of the node and the
    // edge would appear to ride along its own container's border.
    const STUB = NottarioArchCanvas.STUB_CELLS;
    const inStubColumn = (x, y, node, anchor, side) => {
      if (!node || !side || !anchor) return false;
      if (side === 'bottom' || side === 'top') {
        return x === anchor.x;
      }
      return y === anchor.y;
    };
    // Build the set of nodes whose blocking must be IGNORED for this
    // edge. ONLY ancestor containers are exempt — the edge legitimately
    // traverses their interior on its way out/in. Source and target
    // themselves stay in the obstacle set, with one exception handled
    // by `_inNodeBufferOnly` below: the buffer halo around their chosen
    // face is passable so the stub anchor can sit just outside it.
    // (Earlier this code exempted src/tgt entirely; that made A*
    // happily route THROUGH the source box because its interior was
    // marked "free", producing visible U-turns where an edge dipped
    // into its own container before circling back out.)
    const exempt = new Set(ancestorRects || []);
    const nonExempt = flat ? flat.filter(w => !exempt.has(w)) : null;
    const isBlocked = (x, y) => {
      if (x === goal.x && y === goal.y) return false;
      if (x === start.x && y === start.y) return false;
      if (!grid[idx(x, y)]) return false;
      // Source/target buffer halo is passable, BUT only along the
      // stub column (perpendicular axis from the chosen face anchor).
      // Cells in the buffer halo that lie off-axis stay blocked so A*
      // can't slide along the node's own border before turning.
      if (srcNode && this._inNodeBufferOnly(x, y, srcNode, obs)
          && inStubColumn(x, y, srcNode, start, start.side)) return false;
      if (tgtNode && this._inNodeBufferOnly(x, y, tgtNode, obs)
          && inStubColumn(x, y, tgtNode, goal, goal.side)) return false;
      if (nonExempt) {
        for (const w of nonExempt) {
          if (this._cellTouchesNode(x, y, w, obs)) return true;
        }
        return false;
      }
      return true;
    };
    const open = new Map();
    const closed = new Set();
    const skey = `${start.x},${start.y}`;
    open.set(skey, { x: start.x, y: start.y, g: 0, h: heur(start.x, start.y), parent: null, dir: null });
    const dirs = [[1, 0], [-1, 0], [0, 1], [0, -1]];
    while (open.size > 0) {
      let best = null, bestKey = '';
      for (const [k, v] of open) {
        if (!best || (v.g + v.h) < (best.g + best.h)) { best = v; bestKey = k; }
      }
      if (best.x === goal.x && best.y === goal.y) {
        const path = [];
        let cur = best;
        while (cur) { path.push([cur.x, cur.y]); cur = cur.parent; }
        return path.reverse();
      }
      open.delete(bestKey);
      closed.add(bestKey);
      for (const [dx, dy] of dirs) {
        const nx = best.x + dx, ny = best.y + dy;
        if (nx < 0 || nx >= W || ny < 0 || ny >= H) continue;
        const k = `${nx},${ny}`;
        if (closed.has(k)) continue;
        if (isBlocked(nx, ny)) continue;
        const turn = best.dir && (best.dir[0] !== dx || best.dir[1] !== dy);
        let g = best.g + 1 + (turn ? 2 : 0);
        // Congestion penalty. A cell already used by another routed
        // edge adds cost; if the new step matches the direction of
        // the existing segment (same axis), the penalty is harsher
        // so parallel routes get pushed onto a different track.
        if (congestion) {
          const c = congestion[idx(nx, ny)];
          if (c) {
            // Heavy same-axis penalty forces parallel routes to
            // commit to a different track for the whole channel
            // rather than partially-overlap with a 1-cell zig-zag.
            // Same-axis bumped to 200 so any longer detour the
            // grid can offer beats coalescing onto an already-used
            // track; perpendicular crossings (different bit) are
            // unavoidable in compound layouts so they stay cheap.
            const sameAxis = dx !== 0 ? (c & 1) : (c & 2);
            g += sameAxis ? 200 : 4;
          }
        }
        const existing = open.get(k);
        if (existing && existing.g <= g) continue;
        open.set(k, { x: nx, y: ny, g, h: heur(nx, ny), parent: best, dir: [dx, dy] });
      }
    }
    return null;
  }

  // Mark every cell traversed by `cells` in the congestion map. Bit
  // 0 = a horizontal segment passed through; bit 1 = a vertical one.
  // Adjacent cells perpendicular to each segment also get a light
  // mark so the next edge is pushed at least one track away.
  _markCongestion(cells, obs, congestion) {
    if (!cells || cells.length < 2) return;
    const { W, H } = obs;
    const idx = (x, y) => y * W + x;
    for (let i = 0; i < cells.length - 1; i++) {
      const [ax, ay] = cells[i];
      const [bx, by] = cells[i + 1];
      const horiz = ay === by;
      const bit = horiz ? 1 : 2;
      const otherBit = horiz ? 2 : 1;
      const sx = Math.min(ax, bx), ex = Math.max(ax, bx);
      const sy = Math.min(ay, by), ey = Math.max(ay, by);
      for (let y = sy; y <= ey; y++) {
        for (let x = sx; x <= ex; x++) {
          congestion[idx(x, y)] |= bit;
          // Halo cells get the OPPOSITE-axis bit instead of the
          // travelled-axis bit. Effect in A*: a parallel route in
          // the next cell over reads "no same-axis flag" and pays
          // nothing, so adjacent tracks pack tight; only a crossing
          // (perpendicular usage) at the halo pays the cross-cost.
          if (horiz) {
            if (y - 1 >= 0) congestion[idx(x, y - 1)] |= otherBit;
            if (y + 1 < H)  congestion[idx(x, y + 1)] |= otherBit;
          } else {
            if (x - 1 >= 0) congestion[idx(x - 1, y)] |= otherBit;
            if (x + 1 < W)  congestion[idx(x + 1, y)] |= otherBit;
          }
        }
      }
    }
  }

  // Drop collinear waypoints between two segments so the simplified
  // path keeps only the corner cells.
  _simplifyOrtho(points) {
    if (points.length < 3) return points;
    // Pass 1 — drop exact duplicates (consecutive points at the same
    // coords introduce zero-length corner curves in the rendered path).
    const dedup = [points[0]];
    for (let i = 1; i < points.length; i++) {
      const a = dedup[dedup.length - 1];
      const b = points[i];
      if (a.x === b.x && a.y === b.y) continue;
      dedup.push(b);
    }
    if (dedup.length < 3) return dedup;
    // Pass 2 — drop collinear interior points so each path bend
    // appears exactly once.
    const out = [dedup[0]];
    for (let i = 1; i < dedup.length - 1; i++) {
      const a = out[out.length - 1];
      const b = dedup[i];
      const c = dedup[i + 1];
      const colinear =
        (a.x === b.x && b.x === c.x) ||
        (a.y === b.y && b.y === c.y);
      if (!colinear) out.push(b);
    }
    out.push(dedup[dedup.length - 1]);
    return out;
  }

  // Decide the source and target exit faces based on relative position
  // (same logic as `_anchors`) and return them as side identifiers.
  // Decide the exit/entry faces. Two cases:
  //
  //   1. Same Sugiyama row (vertical bounding-boxes overlap, but
  //      horizontal don't) → sideways arrows. tCx > sCx ⇒ source
  //      exits right, target enters left.
  //
  //   2. Anything else (different rows, no overlap at all, or one
  //      contains the other) → top/bottom arrows. This is the
  //      Sugiyama convention: cross-layer edges flow vertically
  //      through the layer gap, never wrap around the side of the
  //      target. The previous fallback (dominant centre-to-centre
  //      axis) was picking horizontal sides for diagonally-placed
  //      nodes, which forced A* into U-turns when the source's
  //      right-stub overshot the target's left-stub (or vice-versa).
  _chooseSides(src, tgt) {
    const sCx = src.x + src.w / 2;
    const sCy = src.y + src.h / 2;
    const tCx = tgt.x + tgt.w / 2;
    const tCy = tgt.y + tgt.h / 2;
    const vOverlap = Math.min(src.y + src.h, tgt.y + tgt.h)
                   > Math.max(src.y, tgt.y);
    const hOverlap = Math.min(src.x + src.w, tgt.x + tgt.w)
                   > Math.max(src.x, tgt.x);
    if (vOverlap && !hOverlap) {
      return tCx > sCx ? ['right', 'left'] : ['left', 'right'];
    }
    return tCy > sCy ? ['bottom', 'top'] : ['top', 'bottom'];
  }

  // Convert a precise face anchor (pixel coords) to the exact pixel
  // start/end the rendered path should use. We snap A*'s grid path
  // back to the canvas-resolution coordinates while preserving the
  // anchor's center-of-face origin so the arrow lands cleanly.
  _faceAnchorPx(node, side, frac = 0.5) {
    const fx = node.x + node.w * frac;
    const fy = node.y + node.h * frac;
    switch (side) {
      case 'right':  return { x: node.x + node.w,  y: fy };
      case 'left':   return { x: node.x,           y: fy };
      case 'bottom': return { x: fx,               y: node.y + node.h };
      case 'top':    return { x: fx,               y: node.y };
      default:       return { x: node.x + node.w / 2, y: node.y + node.h / 2 };
    }
  }

  // Plan per-edge anchor fractions so that two edges sharing a face
  // on the same node don't stack on top of each other. For each
  // (node, side) bucket the edges, sort them by the OTHER endpoint's
  // perpendicular coordinate (so crossings minimise), and distribute
  // them along the face at fractions 1/(N+1), 2/(N+1), …
  _planAnchors(edges, byID) {
    const planned = edges.map((edge) => {
      const src = byID.get(edge.FromNodeID);
      const tgt = byID.get(edge.ToNodeID);
      if (!src || !tgt) return null;
      if (typeof src.x !== 'number' || typeof tgt.x !== 'number') return null;
      const [sSide, tSide] = this._chooseSides(src, tgt);
      return { edge, src, tgt, sSide, tSide, sFrac: 0.5, tFrac: 0.5 };
    });
    // Face-load redistribution. A node with many outgoing/incoming
    // edges on one face piles them all into a narrow strip; the
    // _planAnchors spread step then squeezes them into thin tracks
    // and A* has nowhere to send them. Reroute the edges whose
    // OTHER endpoint sits clearly off-axis from the original face
    // through the perpendicular face instead. Example: MCP server
    // has 7 outgoing-to-bottom edges; the 5 whose targets are
    // strictly right of MCP exit via the RIGHT face instead, leaving
    // 2 on BOTTOM for the targets directly below.
    // Face-load threshold for the redistribute step. Each face can
    // host ~5 parallel tracks before the routing geometry gets too
    // tight; tighten this further only when the diagram is wider on
    // average, since redistributing prematurely steers edges into
    // adjacent faces whose clearance may be even worse.
    const MAX_PER_FACE = 5;
    const countOn = (nodeID, side) => planned.reduce((n, p) =>
      n + (p && ((p.src.node.ID === nodeID && p.sSide === side) ||
                 (p.tgt.node.ID === nodeID && p.tSide === side)) ? 1 : 0), 0);
    const rerouteEnd = (p, end) => {
      // end === 's' → adjust the SOURCE face; 't' → target face.
      const side = end === 's' ? p.sSide : p.tSide;
      if (side === 'bottom' || side === 'top') {
        const self = end === 's' ? p.src : p.tgt;
        const other = end === 's' ? p.tgt : p.src;
        const oCx = other.x + other.w / 2;
        const right = oCx > self.x + self.w;
        const left  = oCx < self.x;
        if (!right && !left) return false; // target is roughly above/below
        const newSide = right ? 'right' : 'left';
        if (end === 's') {
          p.sSide = newSide;
          // Target side flips to whichever vertical face is closer.
          const otherCy = other.y + other.h / 2;
          p.tSide = otherCy < self.y + self.h / 2 ? 'bottom' : 'top';
        } else {
          p.tSide = newSide;
          const selfCy = self.y + self.h / 2;
          const otherCy = other.y + other.h / 2;
          p.sSide = otherCy < selfCy ? 'top' : 'bottom';
        }
        return true;
      }
      if (side === 'left' || side === 'right') {
        const self = end === 's' ? p.src : p.tgt;
        const other = end === 's' ? p.tgt : p.src;
        const oCy = other.y + other.h / 2;
        const below = oCy > self.y + self.h;
        const above = oCy < self.y;
        if (!below && !above) return false;
        const newSide = below ? 'bottom' : 'top';
        if (end === 's') {
          p.sSide = newSide;
          const otherCx = other.x + other.w / 2;
          p.tSide = otherCx < self.x + self.w / 2 ? 'right' : 'left';
        } else {
          p.tSide = newSide;
          const selfCx = self.x + self.w / 2;
          const otherCx = other.x + other.w / 2;
          p.sSide = otherCx < selfCx ? 'left' : 'right';
        }
        return true;
      }
      return false;
    };
    // Walk faces sorted by overload first so the biggest jams get
    // relieved before any chain-reaction redistribution.
    let safety = 8; // bounded relax — cycles theoretically impossible but be safe.
    while (safety--) {
      let worst = null;
      const counts = new Map();
      for (const p of planned) {
        if (!p) continue;
        for (const [nodeID, side] of [[p.src.node.ID, p.sSide], [p.tgt.node.ID, p.tSide]]) {
          const k = nodeID + '|' + side;
          counts.set(k, (counts.get(k) || 0) + 1);
          if (counts.get(k) > (worst?.count || MAX_PER_FACE)) {
            worst = { key: k, nodeID, side, count: counts.get(k) };
          }
        }
      }
      if (!worst) break;
      // Move OFF-AXIS edges from the worst face. Sort by how far
      // off-axis the other endpoint sits; reroute the most-off-axis
      // ones first until the face count drops at/below MAX_PER_FACE.
      const candidates = planned
        .map((p, i) => ({ p, i }))
        .filter(({ p }) => p &&
          ((p.src.node.ID === worst.nodeID && p.sSide === worst.side) ||
           (p.tgt.node.ID === worst.nodeID && p.tSide === worst.side)))
        .map(({ p, i }) => {
          const isS = p.src.node.ID === worst.nodeID && p.sSide === worst.side;
          const self = isS ? p.src : p.tgt;
          const other = isS ? p.tgt : p.src;
          const offAxis = (worst.side === 'top' || worst.side === 'bottom')
            ? Math.abs((other.x + other.w/2) - (self.x + self.w/2))
            : Math.abs((other.y + other.h/2) - (self.y + self.h/2));
          return { p, i, end: isS ? 's' : 't', offAxis };
        })
        .sort((a, b) => b.offAxis - a.offAxis);
      let moved = 0;
      for (const c of candidates) {
        if (counts.get(worst.key) - moved <= MAX_PER_FACE) break;
        if (rerouteEnd(c.p, c.end)) moved++;
      }
      if (moved === 0) break; // nothing left to redistribute
    }
    // From here on, source side / target side are stable.
    const buckets = new Map(); // key → array of { entry, end:'s'|'t' }
    const push = (key, entry, end) => {
      if (!buckets.has(key)) buckets.set(key, []);
      buckets.get(key).push({ entry, end });
    };
    for (const p of planned) {
      if (!p) continue;
      push(`${p.src.node.ID}|${p.sSide}`, p, 's');
      push(`${p.tgt.node.ID}|${p.tSide}`, p, 't');
    }
    for (const [key, list] of buckets) {
      // Sort by the other endpoint's perpendicular coord so the
      // ordering along the face matches the ordering of destinations.
      const side = key.split('|')[1];
      const perpOf = (item) => {
        const other = item.end === 's' ? item.entry.tgt : item.entry.src;
        // Horizontal faces (top/bottom) → distribute along x.
        if (side === 'top' || side === 'bottom') {
          return other.x + other.w / 2;
        }
        // Vertical faces (left/right) → distribute along y.
        return other.y + other.h / 2;
      };
      list.sort((a, b) => perpOf(a) - perpOf(b));
      const n = list.length;
      list.forEach((item, i) => {
        let frac = (i + 1) / (n + 1);
        // Jitter the exact-centre frac slightly. A source with an odd
        // count places its middle anchor at frac=0.5 (= node centre).
        // A single-incoming target's anchor is also frac=0.5. When two
        // such nodes happen to be vertically center-aligned (the
        // common Sugiyama symmetry), their vertical segments coincide
        // in x and overlap visually. 5%-of-face nudge is invisible
        // but breaks the alignment.
        if (n > 1 && Math.abs(frac - 0.5) < 1e-6) frac = 0.55;
        if (item.end === 's') item.entry.sFrac = frac;
        else                  item.entry.tFrac = frac;
      });
    }
    return planned;
  }

  // Channel-routing pass for cross-layer edges (vertical sides on
  // both ends). Two filters:
  //
  //   - eligible: source/target must use vertical faces (bottom→top
  //     or top→bottom).
  //   - clearChannel: no OTHER laid-out node may sit fully inside
  //     the y-range between source's exit face and target's entry
  //     face. This is what catches cross-multi-layer edges (e.g.
  //     a layer-0 node going to a layer-2 node, whose channel
  //     contains layer-1 nodes). Those edges fall through to A*
  //     so the route can go AROUND the intermediate boxes instead
  //     of straight through them.
  //
  // Within each (srcY, tgtY) bucket, edges are sorted by midpoint x
  // (avg of source-anchor and target-anchor) and assigned a unique
  // track y. The midpoint sort packs adjacent-in-x edges into
  // adjacent tracks, which reduces visual crossings vs. sorting by
  // target x alone (which clusters opposite-direction edges into
  // neighbouring tracks and forces crossings).
  _routeTracked(plans, layoutFlat, byID) {
    const result = new Map();
    const groups = new Map();
    const eligible = (p) =>
      (p.sSide === 'bottom' && p.tSide === 'top') ||
      (p.sSide === 'top' && p.tSide === 'bottom');
    // Mirror the A* router's "ancestor exemption" rule: a straight
    // track may cross an expanded container only if that container
    // is an ancestor of source or target (i.e. the edge legitimately
    // enters/leaves it). Unrelated expanded containers count as
    // obstacles and force this edge to fall through to A* routing.
    const isAncestor = (cont, leaf) => {
      let cur = leaf?.node?.ParentID;
      while (cur) {
        if (cur === cont.node.ID) return true;
        const w = byID?.get(cur);
        if (!w) break;
        cur = w.node.ParentID;
      }
      return false;
    };
    const clearChannel = (p, srcY, tgtY) => {
      const top = Math.min(srcY, tgtY);
      const bot = Math.max(srcY, tgtY);
      for (const w of layoutFlat) {
        if (w === p.src || w === p.tgt) continue;
        if (w._isContainer && w._expanded
            && (isAncestor(w, p.src) || isAncestor(w, p.tgt))) {
          continue; // legitimately routed through; not an obstacle
        }
        if (w.y > top && w.y + w.h < bot) return false;
      }
      return true;
    };
    plans.forEach((p, i) => {
      if (!p || !eligible(p)) return;
      const srcA = this._faceAnchorPx(p.src, p.sSide, p.sFrac);
      const tgtA = this._faceAnchorPx(p.tgt, p.tSide, p.tFrac);
      if (!clearChannel(p, srcA.y, tgtA.y)) return;
      const key = `${srcA.y}|${tgtA.y}`;
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push({ idx: i, plan: p, srcA, tgtA });
    });
    for (const [key, group] of groups) {
      group.sort((a, b) =>
        (a.srcA.x + a.tgtA.x) - (b.srcA.x + b.tgtA.x)
      );
      const [srcY, tgtY] = key.split('|').map(Number);
      const dy = tgtY - srcY;
      const n = group.length;
      group.forEach((entry, i) => {
        const t = (i + 1) / (n + 1);
        const trackY = srcY + dy * t;
        const waypoints = [
          entry.srcA,
          { x: entry.srcA.x, y: trackY },
          { x: entry.tgtA.x, y: trackY },
          entry.tgtA,
        ];
        result.set(entry.idx, {
          d: this._pathD(waypoints),
          waypoints,
          edge: entry.plan.edge,
        });
      });
    }
    return result;
  }

  // Top-level edge router. Tries A* first; falls back to the v1
  // L-router when A* fails (no path or anchor cell blocked). When
  // a congestion map is provided, it is consulted (and updated) so
  // edges routed later don't reuse the same corridors.
  _routeEdgeBest(plan, byID, obs, congestion = null) {
    if (!plan) return null;
    let { edge, src, tgt, sSide, tSide, sFrac, tFrac } = plan;
    let sCell = this._anchorCell(src, sSide, obs, sFrac);
    let tCell = this._anchorCell(tgt, tSide, obs, tFrac);
    if (!sCell || !tCell) return this._routeEdge(edge, byID);
    // Ancestor exemption: an edge from inside container A to outside
    // (or to inside container B) must be allowed to traverse A's
    // interior on its way out, and B's interior on its way in. Every
    // OTHER expanded container stays a hard obstacle, so the edge
    // can't slice under unrelated boxes.
    const ancestorRects = [
      ...this._ancestorWrappers(src, byID),
      ...this._ancestorWrappers(tgt, byID),
    ];
    let cells = this._astar(obs, sCell, tCell, src, tgt, congestion, ancestorRects);
    if (!cells) {
      // A* couldn't find a path with the planned faces (likely the
      // redistribute step picked a face whose stub anchor falls
      // inside an adjacent box's buffer halo). Retry on the natural
      // bottom/top axis that _chooseSides would have picked from
      // scratch — that pair almost always has clearance because the
      // node was originally laid out as a Sugiyama layer.
      const [fbS, fbT] = this._chooseSides(src, tgt);
      if (fbS !== sSide || fbT !== tSide) {
        const sCell2 = this._anchorCell(src, fbS, obs, sFrac);
        const tCell2 = this._anchorCell(tgt, fbT, obs, tFrac);
        if (sCell2 && tCell2) {
          cells = this._astar(obs, sCell2, tCell2, src, tgt, congestion, ancestorRects);
          if (cells) {
            // Patch the plan in place so downstream code (path
            // assembly, label placement) reads the actual faces used.
            plan.sSide = sSide = fbS;
            plan.tSide = tSide = fbT;
            sCell = sCell2; tCell = tCell2;
          }
        }
      }
    }
    if (cells && congestion) this._markCongestion(cells, obs, congestion);
    if (!cells || cells.length < 2) return this._routeEdge(edge, byID);
    const cell = obs.cell;
    // Convert grid cells to pixel waypoints (center of each cell).
    // Replace the first and last waypoints with the EXACT face anchor
    // pixel positions so the path visually touches the node edge.
    const pxPath = cells.map(([x, y]) => ({ x: x * cell, y: y * cell }));
    const sAnchor = this._faceAnchorPx(src, sSide, sFrac);
    const tAnchor = this._faceAnchorPx(tgt, tSide, tFrac);
    // Snap the FULL initial straight run (and the full trailing run)
    // to the face anchor's perpendicular coord. The face anchor is
    // computed from frac × node.w (sub-pixel exact), but A* operates
    // on an 8-px grid so sCell/tCell are rounded. When A* moves in
    // the face-direction axis for several cells, every cell in that
    // run shares the same (rounded) perpendicular coord — we want
    // ALL of them aligned to the exact face-anchor perpendicular so
    // the rendered stub-plus-run is one perfectly straight segment.
    // The previous code only snapped pxPath[0]/[last], which left
    // pxPath[1..] at the rounded grid x and produced a sub-cell
    // diagonal kink between the snapped first cell and A*'s second.
    if (pxPath.length >= 1) {
      const isHorizS = (sSide === 'left' || sSide === 'right');
      const origS = isHorizS ? pxPath[0].y : pxPath[0].x;
      const wantS = isHorizS ? sAnchor.y : sAnchor.x;
      for (let i = 0; i < pxPath.length; i++) {
        const wp = pxPath[i];
        const cur = isHorizS ? wp.y : wp.x;
        if (Math.abs(cur - origS) >= cell / 2) break;
        if (isHorizS) wp.y = wantS; else wp.x = wantS;
      }
      const isHorizT = (tSide === 'left' || tSide === 'right');
      const li = pxPath.length - 1;
      const origT = isHorizT ? pxPath[li].y : pxPath[li].x;
      const wantT = isHorizT ? tAnchor.y : tAnchor.x;
      for (let i = li; i >= 0; i--) {
        const wp = pxPath[i];
        const cur = isHorizT ? wp.y : wp.x;
        if (Math.abs(cur - origT) >= cell / 2) break;
        if (isHorizT) wp.y = wantT; else wp.x = wantT;
      }
    }
    pxPath.unshift(sAnchor);
    pxPath.push(tAnchor);
    const waypoints = this._simplifyOrtho(pxPath);
    return { d: this._pathD(waypoints), waypoints, edge };
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
    const x1 = Math.ceil ((mx + pillW / 2 - inset) / cell);
    const y0 = Math.floor((my - pillH / 2 + inset) / cell);
    const y1 = Math.ceil ((my + pillH / 2 - inset) / cell);
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
      const ed = (this.edges || []).find(e => e.ID === this.highlightEdge);
      if (ed) return new Set([ed.FromNodeID, ed.ToNodeID]);
    }
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
    // Collapsed containers: any click on the box also toggles
    // expand (the entire box IS the header at this state — there's
    // no separate body to select without expanding). For expanded
    // containers, the label strip handles toggling and body clicks
    // fall through here to just select. For leaves, just select.
    if (w._isContainer && !w._expanded) {
      const ex = new Set(this.expanded || []);
      ex.add(w.node.ID);
      this.expanded = ex;
      this._emitExpandChanged();
    }
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

  // Keyboard activation on a focused node. Enter / Space select;
  // Right / Down on a collapsed container expand it; Left / Up on
  // an expanded container collapse it.
  _onNodeKey(e, w) {
    switch (e.key) {
      case 'Enter':
      case ' ':
        e.preventDefault();
        this._emitSelect(w.node.ID);
        return;
      case 'ArrowRight':
      case 'ArrowDown':
        if (w._isContainer && !w._expanded) {
          e.preventDefault();
          const ex = new Set(this.expanded || []);
          ex.add(w.node.ID);
          this.expanded = ex;
          this._emitExpandChanged();
        }
        return;
      case 'ArrowLeft':
      case 'ArrowUp':
        if (w._isContainer && w._expanded) {
          e.preventDefault();
          const ex = new Set(this.expanded || []);
          ex.delete(w.node.ID);
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
    if (path.some(el => el?.classList?.contains?.('node'))) return;
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
    const scale = Math.max(this._viewBox.w / rect.width,
                           this._viewBox.h / rect.height);
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
      const factor = e.deltaY < 0 ? 0.88 : 1.12;
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
    const k = e.deltaMode === 1 ? lineH
            : e.deltaMode === 2 ? rect.height
            : 1;
    // Same unified scale as drag (see _onSvgPointerMove). With
    // preserveAspectRatio="meet", both axes share one factor.
    const scale = Math.max(this._viewBox.w / rect.width,
                           this._viewBox.h / rect.height);
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
           role="button"
           tabindex=${this.selected === n.ID ? '0' : '-1'}
           aria-label=${`${kindLabel || 'node'} ${n.Name}`}
           @click=${(e) => this._onNodeClick(e, w)}
           @dblclick=${(e) => this._onNodeDblClick(e, w)}
           @keydown=${(e) => this._onNodeKey(e, w)}
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
        <path class=${cls} d=${dPath}></path>
        <path class=${cls} d=${arrow} fill=${stroke} stroke="none"></path>
      </g>
    `;
  }

  _renderEdgeLabel(routed, dim, obs) {
    if (!routed || !routed.edge.Label) return null;
    const label = routed.edge.Label;
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
    // Edge routing (obstacles + plan + A*) is deterministic given
    // (layout + edges). Cache by the same layout key as _layout itself
    // — without this, every hover-driven re-render replays every
    // edge through A*, which produces a visible ~1s lag on dense
    // diagrams.
    // Disable the route cache while the reflow animation is interpolating
    // wrapper positions: every frame mutates w.x/w.y, so a frozen route
    // set drawn for one snapshot would float over boxes that have moved
    // since. Once `_reflowRaf` clears at animation end, the next render
    // re-enables the cache and reuses the static routes.
    const animating = this._reflowRaf != null;
    let routedEdges, anchorPlan, wByID, obstacles;
    // Two checks: key matches (no graph change) AND the cached routes
    // were computed against THIS exact layout object. The second check
    // catches the case where the async ELK layout completes after a
    // placeholder layout was already routed — same cache key, different
    // node positions.
    if (!animating
        && this._cachedRoutes
        && this._cachedRoutesKey === this._cachedLayoutKey
        && this._cachedRoutes.forLayout === layout) {
      ({ routedEdges, anchorPlan, wByID, obstacles } = this._cachedRoutes);
    } else {
      wByID = new Map(layout.flat.map(w => [w.node.ID, w]));
      // ELK produces routedEdges directly inside _layout(); reuse them.
      // For the custom engine, fall through to the channel router.
      if (layout._elkRouted) {
        routedEdges = layout._elkRouted;
        anchorPlan  = [];
      } else {
        const channelRouted = this._routeChannels(this.edges || [], wByID, layout);
        routedEdges = channelRouted.routed;
        anchorPlan  = channelRouted.planned;
      }
      // Keep a lightweight obstacle map purely so the existing label
      // placement code (`_renderEdgeLabel`) can avoid printing pills
      // over nodes. The router itself no longer reads it.
      obstacles = this._buildObstacles(layout);
      if (!animating) {
        this._cachedRoutes = { routedEdges, anchorPlan, wByID, obstacles, forLayout: layout };
        this._cachedRoutesKey = this._cachedLayoutKey;
      }
    }

    // Highlighted set: external edge highlight > hover > query >
    // focus subtree. null = everything full.
    const hi = this._highlightedSet(layout);
    const isDim = (id) => hi !== null && !hi.has(id);
    const isHighlightedEdge = (e) => {
      if (this.highlightEdge) return e.ID === this.highlightEdge;
      return hi !== null && hi.has(e.FromNodeID) && hi.has(e.ToNodeID);
    };
    const isAccentedEdge = (e) =>
      (this._hover && (e.FromNodeID === this._hover || e.ToNodeID === this._hover)) ||
      (this.highlightEdge && e.ID === this.highlightEdge);

    const containers = layout.flat.filter(w => w._isContainer && w._expanded);
    const leavesAndCollapsed = layout.flat.filter(w => !w._isContainer || !w._expanded);

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
        ${containers.map(w => this._renderNode(w, isDim(w.node.ID)))}
        ${routedEdges.map(r => this._renderEdge(r,
          hi !== null && !isHighlightedEdge(r.edge),
          isAccentedEdge(r.edge)))}
        ${leavesAndCollapsed.map(w => this._renderNode(w, isDim(w.node.ID)))}
        ${routedEdges.map(r => this._renderEdgeLabel(r,
          hi !== null && !isHighlightedEdge(r.edge),
          obstacles))}
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
