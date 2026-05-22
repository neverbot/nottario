import { LitElement, html, css, svg } from '/static/vendor/lit/lit.js';

// <nottario-arch-canvas .nodes=${...} .edges=${...} .expanded=${set} .selected=${id}>
//
// Hand-rolled SVG renderer for the architecture diagram. Replaces the
// dagre-driven `arch-graph.js` with a containment layout: every node
// is rendered nested INSIDE its parent, so the parent IS a labeled
// container around its children. The user never loses the parent when
// they explore deeper.
//
// This is the FOUNDATION (child A of feature f9a7a488) — Layers 1+2
// only: containment pack + top-level lane. No edge routing, no
// interactions, no focus mode. Each lands in a sibling child task.
//
// Layout contract:
//   - Leaf node = 160×72 (rounded 8px, hairline #d1d9e0 border, white).
//   - Container = 28px label strip on top + 24px interior padding
//     around a square-ish grid of its children.
//   - Top-level lane = roots laid out left→right, `system` first,
//     `external` last, rest in stored order.
//   - Grid columns = ceil(sqrt(children.length)) so containers stay
//     roughly square.
//
// All dimensions match the design tokens locked in task 5236da63.
class NottarioArchCanvas extends LitElement {
  static properties = {
    nodes:    { type: Array },
    edges:    { type: Array }, // unused in this iteration; placeholder
    expanded: { type: Object }, // Set<string> of node ids that are expanded
    selected: { type: String },
  };

  // Layout constants. Centralised so child B (edge routing) and child
  // C (interactions) can read them without re-deriving.
  static LEAF_W      = 160;
  static LEAF_H      = 72;
  static LABEL_STRIP = 28;
  static PAD         = 24;
  static GAP         = 16;
  static CANVAS_PAD  = 24; // outer margin around the whole layout

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

    /* Node rectangles: container vs leaf differ only by fill. */
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

    /* Kind chip — coloured dot + label, top-left of every node. */
    .kind-chip circle { stroke: none; }
    .kind-chip text {
      font: 600 10.5px/1 ui-monospace, SFMono-Regular, monospace;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      fill: #59636e;
    }

    /* Caret on containers (top-right). Always pointing down for now;
       interactions in child C will swap the glyph + handle clicks. */
    .caret {
      font: 600 11px/1 ui-monospace, SFMono-Regular, monospace;
      fill: #8b949e;
    }

    /* Name + slug. Containers show them in their label strip; leaves
       show them centered. */
    .name {
      font: 600 14px/1.2 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
      fill: #1f2328;
    }
    .slug {
      font: 11px/1 ui-monospace, SFMono-Regular, monospace;
      fill: #8b949e;
    }

    /* Empty state */
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

  // ----- Layout -----

  // Build a tree from the flat nodes array. Every node gets a children
  // array (possibly empty). Roots are the nodes without parent_id.
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
    // Top-level lane order: `system` kind first, `external` last,
    // rest in their stored Position then Name.
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
    return roots;
  }

  // Layer 1: containment pack. Recursive bottom-up sizing.
  // Sets w/h on every wrapper. Containers are laid out as a
  // ceil(sqrt(children.length))-column grid; total size is the grid
  // bounding box + interior padding + the 28px top label strip.
  _packSize(w) {
    if (!w.children.length) {
      w.w = NottarioArchCanvas.LEAF_W;
      w.h = NottarioArchCanvas.LEAF_H;
      w._isContainer = false;
      return;
    }
    w._isContainer = true;
    // Size children first.
    for (const child of w.children) this._packSize(child);
    const cols = Math.max(1, Math.ceil(Math.sqrt(w.children.length)));
    const rows = Math.ceil(w.children.length / cols);
    // Row heights and column widths: take the max of the cells assigned
    // to that row/col in row-major order.
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

  // Layer 1b: place children within their parent (recursive). The
  // root caller positions roots in the top-level lane.
  _placeChildren(w) {
    if (!w._isContainer) return;
    const { cols, rows, colW, rowH } = w._grid;
    const gap = NottarioArchCanvas.GAP;
    const pad = NottarioArchCanvas.PAD;
    const startX = w.x + pad;
    const startY = w.y + NottarioArchCanvas.LABEL_STRIP + pad;
    // Pre-compute column x offsets and row y offsets from cumulative
    // colW/rowH so children align in their cells.
    const colX = new Array(cols).fill(0);
    for (let i = 1; i < cols; i++) colX[i] = colX[i - 1] + colW[i - 1] + gap;
    const rowY = new Array(rows).fill(0);
    for (let i = 1; i < rows; i++) rowY[i] = rowY[i - 1] + rowH[i - 1] + gap;
    w.children.forEach((c, i) => {
      const r = Math.floor(i / cols);
      const cc = i % cols;
      // Center the child inside its (colW, rowH) cell, top-aligned to
      // the row baseline so unequal-size children still feel orderly.
      c.x = startX + colX[cc] + (colW[cc] - c.w) / 2;
      c.y = startY + rowY[r];
      this._placeChildren(c);
    });
  }

  // Layer 2: top-level lane. Roots are laid out left→right with GAP
  // between them. Total canvas size is the bounding box + CANVAS_PAD
  // on each side.
  _layout() {
    const roots = this._buildTree();
    if (!roots.length) return { roots: [], width: 600, height: 480 };
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
      width:  cursorX - gap + pad,
      height: maxH + pad * 2,
    };
  }

  // ----- Render -----

  // Recursively render a node + (if container) its children.
  _renderNode(w) {
    const n = w.node;
    const cls = [
      'node',
      w._isContainer ? 'container' : 'leaf',
      this.selected === n.ID ? 'selected' : '',
    ].filter(Boolean).join(' ');

    // Kind chip dot colour comes from the project-defined kind palette
    // (n.KindColor would be the eventual field; today we fall back to
    // a muted default per kind name).
    const dot = kindDotColor(n.Kind);
    const kindLabel = (n.Kind || '').toLowerCase();

    // Container content: label strip with kind chip + name + slug + caret.
    // Leaf content: same elements but centered vertically inside the box.
    if (w._isContainer) {
      const labelY = NottarioArchCanvas.LABEL_STRIP;
      return svg`
        <g class=${cls} transform=${`translate(${w.x},${w.y})`}>
          <rect class="box" x="0" y="0" width=${w.w} height=${w.h}></rect>
          <line x1="0" y1=${labelY} x2=${w.w} y2=${labelY}
                stroke="#eaeef2" stroke-width="1"></line>
          <!-- Kind chip top-left -->
          <g class="kind-chip" transform="translate(10,10)">
            <circle cx="3" cy="6" r="3" fill=${dot}></circle>
            <text x="10" y="9">${kindLabel}</text>
          </g>
          <!-- Caret top-right (static for now; child C wires the click) -->
          <text class="caret" x=${w.w - 12} y="14" text-anchor="end">▾</text>
          <!-- Name + slug centered in the strip -->
          <text class="name" x=${w.w / 2} y="18" text-anchor="middle">${n.Name}</text>
          ${n.Slug ? svg`
            <text class="slug" x=${w.w / 2} y="46"
                  text-anchor="middle">${n.Slug}</text>
          ` : null}
          ${w.children.map(c => this._renderNode(c))}
        </g>
      `;
    }
    // Leaf
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
    return html`
      <svg viewBox=${`0 0 ${layout.width} ${layout.height}`}
           xmlns="http://www.w3.org/2000/svg"
           preserveAspectRatio="xMidYMid meet">
        ${layout.roots.map(r => this._renderNode(r))}
      </svg>
    `;
  }
}

// Default colours per common kind. Projects can override via their
// own per-kind palette once that lands (the eventual `node.KindColor`
// field). These swatches are deliberately muted, OKLCH-tinted neutrals
// paired with the GitHub-blue accent — never loud saturated colours.
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
