import { LitElement, html, css, svg } from '/static/vendor/lit/lit.js';

// <nottario-arch-canvas .nodes=${...} .edges=${...} .expanded=${set} .selected=${id}>
//
// Hand-rolled SVG renderer for the architecture diagram. Replaces the
// dagre-driven `arch-graph.js` with a containment layout: every node
// is rendered nested INSIDE its parent, so the parent IS a labeled
// container around its children. The user never loses the parent when
// they explore deeper.
//
// Children A+B of feature f9a7a488 (containment layout + edge routing).
// Interactions and focus mode land in child C; detail panel in D.
//
// Layout contract:
//   - Leaf node = 160×72 (rounded 8px, hairline #d1d9e0 border, white).
//   - Container = 28px label strip on top + 24px interior padding
//     around a square-ish grid of its children.
//   - Top-level lane = roots laid out left→right, `system` first,
//     `external` last, rest in stored order.
//   - Edges = orthogonal Manhattan routing with 4px rounded corners,
//     1.5px hairline #59636e (blue when endpoint selected), label on
//     a white pill on the longest segment, 6px triangle arrowhead.
//   - Z-order: edges paint BEFORE nodes so node boxes always sit on
//     top of any line that would otherwise cross them.

class NottarioArchCanvas extends LitElement {
  static properties = {
    nodes:    { type: Array },
    edges:    { type: Array },
    expanded: { type: Object }, // Set<string> of node ids that are expanded
    selected: { type: String },
  };

  // Layout constants. Centralised so child C (interactions) can read
  // them without re-deriving.
  static LEAF_W      = 160;
  static LEAF_H      = 72;
  static LABEL_STRIP = 28;
  static PAD         = 24;
  static GAP         = 16;
  static CANVAS_PAD  = 24;
  static CORNER_R    = 4;

  static styles = css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }

    svg {
      display: block;
      width: 100%;
      height: 100%;
      min-height: 480px;
      background: #ffffff;
    }

    .node rect.box {
      stroke: #d1d9e0;
      stroke-width: 1;
      rx: 8;
      ry: 8;
    }
    .node.container rect.box { fill: #f6f8fa; }
    .node.leaf      rect.box { fill: #ffffff; }
    .node.selected rect.box {
      stroke: #0969da;
      stroke-width: 2;
    }

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
    }
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
    }
    .edge.selected { stroke: #0969da; stroke-width: 2; }
    .edge-label rect {
      fill: #ffffff;
      stroke: #d1d9e0;
      stroke-width: 1;
      rx: 4;
      ry: 4;
    }
    .edge-label text {
      font: 11px/1 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: #1f2328;
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
  }

  // ----- Tree construction -----

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

  // ----- Layout -----

  _packSize(w) {
    if (!w.children.length) {
      w.w = NottarioArchCanvas.LEAF_W;
      w.h = NottarioArchCanvas.LEAF_H;
      w._isContainer = false;
      return;
    }
    w._isContainer = true;
    for (const child of w.children) this._packSize(child);
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
    if (!w._isContainer) return;
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

  // Walk the laid-out tree and produce a flat array of nodes with
  // absolute coordinates. Used to render edges as a separate flat
  // layer BENEATH the nodes, and to look up endpoints when routing.
  _flatten(roots) {
    const out = [];
    const walk = (w) => {
      out.push(w);
      for (const c of w.children) walk(c);
    };
    for (const r of roots) walk(r);
    return out;
  }

  _layout() {
    const { roots, byID } = this._buildTree();
    if (!roots.length) {
      return { roots: [], flat: [], byID, width: 600, height: 480 };
    }
    for (const r of roots) this._packSize(r);
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
    return {
      roots,
      flat: this._flatten(roots),
      byID,
      width:  cursorX - gap + pad,
      height: maxH + pad * 2,
    };
  }

  // ----- Edge routing -----

  // Decide which side of each box the edge exits/enters from, based
  // on the relative position of the two boxes. Returns absolute
  // (x, y) anchor points + a direction vector for the first segment.
  _anchors(src, tgt) {
    const sCx = src.x + src.w / 2;
    const sCy = src.y + src.h / 2;
    const tCx = tgt.x + tgt.w / 2;
    const tCy = tgt.y + tgt.h / 2;
    const dx = tCx - sCx;
    const dy = tCy - sCy;
    if (Math.abs(dx) >= Math.abs(dy)) {
      // Mostly horizontal: exit/enter on left/right faces.
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
    // Mostly vertical: exit/enter on top/bottom faces.
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

  // Build an L-shape Manhattan path from src-anchor to tgt-anchor.
  // For horizontal anchors: go halfway in X, vertical bend, into tgt.
  // For vertical anchors: same idea rotated.
  _waypoints(s, t) {
    if (s.dir === 'h' && t.dir === 'h') {
      const midX = (s.x + t.x) / 2;
      return [
        { x: s.x, y: s.y },
        { x: midX, y: s.y },
        { x: midX, y: t.y },
        { x: t.x, y: t.y },
      ];
    }
    if (s.dir === 'v' && t.dir === 'v') {
      const midY = (s.y + t.y) / 2;
      return [
        { x: s.x, y: s.y },
        { x: s.x, y: midY },
        { x: t.x, y: midY },
        { x: t.x, y: t.y },
      ];
    }
    // Mixed direction: simple L through the corner.
    if (s.dir === 'h') {
      return [
        { x: s.x, y: s.y },
        { x: t.x, y: s.y },
        { x: t.x, y: t.y },
      ];
    }
    return [
      { x: s.x, y: s.y },
      { x: s.x, y: t.y },
      { x: t.x, y: t.y },
    ];
  }

  // Convert waypoints into an SVG `d` path with rounded corners.
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
      // Shorter of (r, half of incoming/outgoing segment) so two close
      // bends don't overlap.
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

  // Place the edge label on the longest segment. Prefer horizontal
  // segments because the label reads horizontally — vertical segments
  // are tolerated only if all horizontal candidates are shorter than
  // the label fits.
  _labelPosition(waypoints, labelWidth) {
    let best = null;
    for (let i = 0; i < waypoints.length - 1; i++) {
      const a = waypoints[i];
      const b = waypoints[i + 1];
      const isHoriz = a.y === b.y;
      const len = Math.hypot(b.x - a.x, b.y - a.y);
      const score = (isHoriz ? 1 : 0.4) * len; // bias horizontal
      if (!best || score > best.score) {
        best = { score, len, isHoriz, mx: (a.x + b.x) / 2, my: (a.y + b.y) / 2 };
      }
    }
    return best;
  }

  // ----- Render -----

  _renderNode(w) {
    const n = w.node;
    const cls = [
      'node',
      w._isContainer ? 'container' : 'leaf',
      this.selected === n.ID ? 'selected' : '',
    ].filter(Boolean).join(' ');
    const dot = kindDotColor(n.Kind);
    const kindLabel = (n.Kind || '').toLowerCase();

    if (w._isContainer) {
      const labelY = NottarioArchCanvas.LABEL_STRIP;
      return svg`
        <g class=${cls} transform=${`translate(${w.x},${w.y})`}>
          <rect class="box" x="0" y="0" width=${w.w} height=${w.h}></rect>
          <line x1="0" y1=${labelY} x2=${w.w} y2=${labelY}
                stroke="#eaeef2" stroke-width="1"></line>
          <g class="kind-chip" transform="translate(10,10)">
            <circle cx="3" cy="6" r="3" fill=${dot}></circle>
            <text x="10" y="9">${kindLabel}</text>
          </g>
          <text class="caret" x=${w.w - 12} y="14" text-anchor="end">▾</text>
          <text class="name" x=${w.w / 2} y="18" text-anchor="middle">${n.Name}</text>
          ${n.Slug ? svg`
            <text class="slug" x=${w.w / 2} y="46"
                  text-anchor="middle">${n.Slug}</text>
          ` : null}
        </g>
      `;
    }
    return svg`
      <g class=${cls} transform=${`translate(${w.x},${w.y})`}>
        <rect class="box" x="0" y="0" width=${w.w} height=${w.h}></rect>
        <g class="kind-chip" transform="translate(10,10)">
          <circle cx="3" cy="6" r="3" fill=${dot}></circle>
          <text x="10" y="9">${kindLabel}</text>
        </g>
        <text class="name" x=${w.w / 2} y=${w.h / 2 - 2} text-anchor="middle">${n.Name}</text>
        ${n.Slug ? svg`
          <text class="slug" x=${w.w / 2} y=${w.h / 2 + 16}
                text-anchor="middle">${n.Slug}</text>
        ` : null}
      </g>
    `;
  }

  _renderEdge(routed) {
    if (!routed) return null;
    const e = routed.edge;
    const isSelected = this.selected === e.FromNodeID || this.selected === e.ToNodeID;
    const cls = 'edge' + (isSelected ? ' selected' : '');
    const lastWP = routed.waypoints[routed.waypoints.length - 1];
    const prevWP = routed.waypoints[routed.waypoints.length - 2];
    // Arrowhead: a small triangle pointing along the last segment's direction.
    const dx = Math.sign(lastWP.x - prevWP.x);
    const dy = Math.sign(lastWP.y - prevWP.y);
    const aSize = 7;
    let arrow = '';
    if (dx !== 0) {
      // Horizontal entry
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
    const stroke = isSelected ? '#0969da' : '#59636e';
    return svg`
      <g>
        <path class=${cls} d=${routed.d}></path>
        <path class=${cls} d=${arrow} fill=${stroke} stroke="none"></path>
      </g>
    `;
  }

  _renderEdgeLabel(routed) {
    if (!routed || !routed.edge.Label) return null;
    const label = routed.edge.Label;
    // Rough text width estimate (no font metrics in SVG land):
    // 11px sans, ~6.5px per character for mixed casing.
    const textWidth = Math.max(24, label.length * 6.5);
    const pillW = textWidth + 10;
    const pillH = 18;
    const pos = this._labelPosition(routed.waypoints, pillW);
    if (!pos) return null;
    const x = pos.mx - pillW / 2;
    const y = pos.my - pillH / 2;
    return svg`
      <g class="edge-label">
        <rect x=${x} y=${y} width=${pillW} height=${pillH}></rect>
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
    // Resolve edges to (source, target) wrappers from the layout.
    const wByID = new Map(layout.flat.map(w => [w.node.ID, w]));
    const routedEdges = (this.edges || [])
      .map(e => this._routeEdge(e, wByID))
      .filter(Boolean);

    // Z-order:
    //   1. Container shells (so containers form the visual context).
    //   2. Edges (paint above container interiors).
    //   3. Leaves (so edges never visually cross a leaf node).
    //   4. Labels on top so the pill is always readable.
    const containers = layout.flat.filter(w => w._isContainer);
    const leaves     = layout.flat.filter(w => !w._isContainer);
    return html`
      <svg viewBox=${`0 0 ${layout.width} ${layout.height}`}
           xmlns="http://www.w3.org/2000/svg"
           preserveAspectRatio="xMidYMid meet">
        ${containers.map(w => this._renderNode(w))}
        ${routedEdges.map(r => this._renderEdge(r))}
        ${leaves.map(w => this._renderNode(w))}
        ${routedEdges.map(r => this._renderEdgeLabel(r))}
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
