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
  // Horizontal gap between sibling boxes in the same Sugiyama layer.
  // Needs to be wide enough for an arrow + an arrowhead + a label
  // pill to fit between two boxes when an edge enters/leaves
  // sideways. 32px ≈ 16px clear stroke + 8px arrowhead overhang
  // + 8px breathing room.
  static GAP         = 32;
  // Vertical gap between Sugiyama layers. Larger than the within-
  // layer GAP so edges and their pill labels have real breathing
  // room. 48px ≈ 18px label + ~30px for the arrow + arrowhead.
  static LAYER_GAP   = 48;
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
  // top-down; nodes within a layer left-to-right.
  _packLevel(layers) {
    const G  = NottarioArchCanvas.GAP;
    const LG = NottarioArchCanvas.LAYER_GAP;
    const gridX = [];
    const gridY = [];
    let curY = 0;
    let maxW = 0;
    for (let li = 0; li < layers.length; li++) {
      const layer = layers[li];
      let curX = 0;
      const rowH = layer.length ? Math.max(...layer.map(c => c.h)) : 0;
      const xs = [];
      for (const c of layer) {
        xs.push(curX);
        curX += c.w + G;
      }
      const rowW = Math.max(0, curX - G);
      if (rowW > maxW) maxW = rowW;
      gridX.push(xs);
      gridY.push(curY);
      // Use the bigger vertical gap between layers so edges + label
      // pills fit; siblings within a layer still use G.
      curY += rowH + LG;
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
      w.w = NottarioArchCanvas.LEAF_W;
      w.h = NottarioArchCanvas.LEAF_H;
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
    const placed = this._packLevel(layers);
    for (let li = 0; li < layers.length; li++) {
      for (let i = 0; i < layers[li].length; i++) {
        const c = layers[li][i];
        c._relX = placed.gridX[li][i];
        c._relY = placed.gridY[li];
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
    // Memoise — layout is purely a function of (nodes, edges, expanded).
    const key = JSON.stringify({
      nodes: (this.nodes || []).map(n => n.ID + '|' + n.ParentID),
      edges: (this.edges || []).map(e => e.FromNodeID + '>' + e.ToNodeID),
      expanded: [...(this.expanded || [])].sort(),
    });
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
    const rootPlace = this._packLevel(rootLayers);
    const pad = NottarioArchCanvas.CANVAS_PAD;
    for (let li = 0; li < rootLayers.length; li++) {
      for (let i = 0; i < rootLayers[li].length; i++) {
        const r = rootLayers[li][i];
        r.x = pad + rootPlace.gridX[li][i];
        r.y = pad + rootPlace.gridY[li];
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
    this._cachedLayout = layout;
    this._cachedLayoutKey = key;
    // Kick reflow animation if we had a prior layout. The animation
    // mutates the wrappers in-place each frame so subsequent renders
    // (from requestUpdate) see the interpolated positions.
    if (prevPositions.size && !this._reducedMotion) {
      this._kickReflowAnimation(prevPositions, layout);
    }
    return layout;
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

  // ----- A* obstacle-avoiding router (layout v2) -----

  // Build a coarse 8px grid over the laid-out canvas where each leaf
  // and each collapsed container is a blocked cell (with a 4px buffer).
  // Expanded containers DON'T block — only their label strip (top
  // 28px) does, so edges can route through the inner padding of a
  // container as long as they don't cross a real child.
  static GRID_CELL  = 8;
  static GRID_BUF   = 4;

  _buildObstacles(layout) {
    const cell = NottarioArchCanvas.GRID_CELL;
    const buf  = NottarioArchCanvas.GRID_BUF;
    const W = Math.ceil((layout.width  + cell * 4) / cell);
    const H = Math.ceil((layout.height + cell * 4) / cell);
    const grid = new Uint8Array(W * H);
    for (const w of layout.flat) {
      // Containers when expanded only block their label strip; their
      // interior is "passable" so edges can use the inner padding.
      let topH = w.h;
      if (w._isContainer && w._expanded) topH = NottarioArchCanvas.LABEL_STRIP;
      const x0 = Math.max(0, Math.floor((w.x - buf) / cell));
      const y0 = Math.max(0, Math.floor((w.y - buf) / cell));
      const x1 = Math.min(W, Math.ceil((w.x + w.w + buf) / cell));
      const y1 = Math.min(H, Math.ceil((w.y + topH + buf) / cell));
      for (let y = y0; y < y1; y++) {
        for (let x = x0; x < x1; x++) grid[y * W + x] = 1;
      }
    }
    return { grid, W, H, cell };
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
  // corner, so the arrowhead doesn't look like it merges with the
  // bend. 3 cells = 24px at GRID_CELL=8.
  static STUB_CELLS  = 3;

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
  _astar(obs, start, goal, srcNode, tgtNode, congestion = null) {
    const { grid, W, H } = obs;
    const idx = (x, y) => y * W + x;
    const heur = (x, y) => Math.abs(goal.x - x) + Math.abs(goal.y - y);
    const isBlocked = (x, y) => {
      if (x === goal.x && y === goal.y) return false;
      if (x === start.x && y === start.y) return false;
      if (!grid[idx(x, y)]) return false;
      if (srcNode && this._inNodeBufferOnly(x, y, srcNode, obs)) return false;
      if (tgtNode && this._inNodeBufferOnly(x, y, tgtNode, obs)) return false;
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
            // rather than partially-overlap with a 1-cell zig-zag
            // (which is what a small penalty produces).
            const sameAxis = dx !== 0 ? (c & 1) : (c & 2);
            g += sameAxis ? 20 : 4;
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
      const sx = Math.min(ax, bx), ex = Math.max(ax, bx);
      const sy = Math.min(ay, by), ey = Math.max(ay, by);
      for (let y = sy; y <= ey; y++) {
        for (let x = sx; x <= ex; x++) {
          congestion[idx(x, y)] |= bit;
          // Soft 1-cell halo on the perpendicular axis.
          if (horiz) {
            if (y - 1 >= 0) congestion[idx(x, y - 1)] |= bit;
            if (y + 1 < H)  congestion[idx(x, y + 1)] |= bit;
          } else {
            if (x - 1 >= 0) congestion[idx(x - 1, y)] |= bit;
            if (x + 1 < W)  congestion[idx(x + 1, y)] |= bit;
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
        const frac = (i + 1) / (n + 1);
        if (item.end === 's') item.entry.sFrac = frac;
        else                  item.entry.tFrac = frac;
      });
    }
    return planned;
  }

  // Top-level edge router. Tries A* first; falls back to the v1
  // L-router when A* fails (no path or anchor cell blocked). When
  // a congestion map is provided, it is consulted (and updated) so
  // edges routed later don't reuse the same corridors.
  _routeEdgeBest(plan, byID, obs, congestion = null) {
    if (!plan) return null;
    const { edge, src, tgt, sSide, tSide, sFrac, tFrac } = plan;
    const sCell = this._anchorCell(src, sSide, obs, sFrac);
    const tCell = this._anchorCell(tgt, tSide, obs, tFrac);
    if (!sCell || !tCell) return this._routeEdge(edge, byID);
    const cells = this._astar(obs, sCell, tCell, src, tgt, congestion);
    if (cells && congestion) this._markCongestion(cells, obs, congestion);
    if (!cells || cells.length < 2) return this._routeEdge(edge, byID);
    const cell = obs.cell;
    // Convert grid cells to pixel waypoints (center of each cell).
    // Replace the first and last waypoints with the EXACT face anchor
    // pixel positions so the path visually touches the node edge.
    const pxPath = cells.map(([x, y]) => ({ x: x * cell, y: y * cell }));
    const sAnchor = this._faceAnchorPx(src, sSide, sFrac);
    const tAnchor = this._faceAnchorPx(tgt, tSide, tFrac);
    // Snap the first/last A* cells' FACE-PERPENDICULAR axis to match
    // the face anchor. The cells are already STUB_CELLS×GRID_CELL
    // away from the face along the face-normal axis, so after the
    // snap, face → first-cell is a straight perpendicular segment.
    // (Previous code also overrode pxPath[0]/[last] to a separate
    // "stub" pixel — that interacted badly with the second-waypoint
    // alignment when A*'s first step was perpendicular to the face,
    // creating sub-cell duplicate points that survived simplify and
    // distorted the geometry. Snapping in place avoids it.)
    if (pxPath.length >= 1) {
      if (sSide === 'left' || sSide === 'right') pxPath[0].y = sAnchor.y;
      else                                        pxPath[0].x = sAnchor.x;
      const li = pxPath.length - 1;
      if (tSide === 'left' || tSide === 'right') pxPath[li].y = tAnchor.y;
      else                                        pxPath[li].x = tAnchor.x;
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
    const wByID = new Map(layout.flat.map(w => [w.node.ID, w]));
    // Build the obstacle grid once per render, then route each edge
    // via A*. Falls back to the L-router on per-edge failures (anchor
    // blocked, no path found, etc).
    const obstacles = this._buildObstacles(layout);
    const anchorPlan = this._planAnchors(this.edges || [], wByID);
    // Sequential routing with a shared congestion grid. Edges are
    // routed shortest-first so tightly-constrained edges claim the
    // direct corridor; longer edges are pushed onto parallel tracks
    // by the congestion penalty inside A*.
    const congestion = new Uint8Array(obstacles.W * obstacles.H);
    const order = anchorPlan
      .map((p, i) => ({ p, i }))
      .filter(o => o.p)
      .sort((a, b) => {
        const da = Math.hypot(a.p.tgt.x - a.p.src.x, a.p.tgt.y - a.p.src.y);
        const db = Math.hypot(b.p.tgt.x - b.p.src.x, b.p.tgt.y - b.p.src.y);
        return da - db;
      });
    const routedById = new Map();
    for (const { p, i } of order) {
      const r = this._routeEdgeBest(p, wByID, obstacles, congestion);
      if (r) routedById.set(i, r);
    }
    const routedEdges = anchorPlan
      .map((_, i) => routedById.get(i))
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
