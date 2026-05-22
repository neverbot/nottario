import { LitElement, html, css, svg } from '/static/vendor/lit/lit.js';
import dagre from '/static/vendor/dagre/dagre.js';
import { subscribe } from '/static/realtime.js';
import '/static/components/markdown.js';
import '/static/components/arch-canvas.js';
import '/static/components/task-chip.js';
import '/static/components/search-input.js';

// <nottario-arch-graph> renders the architecture diagram as boxes
// with arrows. Navigation is by drill-down: the view shows the
// children of a single parent (or every root node when no parent is
// selected). Click a node to recentre on it, or use the breadcrumb
// to go back up. Edges shown are only those whose both endpoints
// live at the current level — a deliberate v1 simplification so the
// reader is never surprised by where an arrow goes.

class NottarioArchGraph extends LitElement {
  static properties = {
    projectId: { type: String },
    kinds: { state: true },
    allNodes: { state: true },         // full list, indexed by slug
    allEdges: { state: true },         // full list
    currentParentSlug: { state: true }, // null = top-level
    selectedSlug: { state: true },
    selectedDetail: { state: true },
    viewBox: { state: true },           // { x, y, w, h }
    layout: { state: true },            // { nodes: [...], edges: [...] }
    cycleEdgeIds: { state: true },      // Set of edge uuids
    error: { state: true },
  };

  static styles = css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }

    /* ---- Next view (feature f9a7a488) ---- */
    .next-root { display: flex; flex-direction: column; gap: 12px; }
    .next-toolbar {
      display: flex;
      align-items: center;
      gap: 8px;
      font-size: 13px;
    }
    .next-toolbar .spacer { flex: 1; }
    .next-toolbar a.legacy-link {
      color: #59636e;
      cursor: pointer;
      font-size: 12px;
    }
    .next-toolbar a.legacy-link:hover { color: #1f2328; }
    .next-btn {
      padding: 4px 12px;
      font: inherit;
      font-size: 12px;
      background: #ffffff;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      cursor: pointer;
      color: #1f2328;
    }
    .next-btn:hover { background: #f6f8fa; border-color: #afb8c1; }

    .next-split {
      display: grid;
      grid-template-columns: 1fr 320px;
      gap: 0;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      overflow: hidden;
      min-height: 70vh;
      background: #ffffff;
    }
    .next-canvas {
      display: block;
      border-right: 1px solid #d1d9e0;
      min-height: 70vh;
    }

    .next-panel {
      padding: 18px 20px 24px;
      overflow: auto;
      max-height: 70vh;
      font-size: 13px;
      color: #1f2328;
    }
    .next-panel .next-empty {
      color: #8b949e;
      font-style: italic;
      margin: 12px 0 0;
    }
    .next-head {
      display: flex;
      flex-direction: column;
      gap: 4px;
      margin-bottom: 4px;
    }
    .next-head h2 {
      margin: 0;
      font-size: 18px;
      font-weight: 600;
      line-height: 1.25;
      color: #1f2328;
    }
    .next-kind-chip {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      font: 600 10.5px/1 ui-monospace, SFMono-Regular, monospace;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      color: #59636e;
    }
    .next-kind-chip .dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      display: inline-block;
    }
    .next-crumb {
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 11px;
      color: #8b949e;
      margin-top: 6px;
      display: flex;
      flex-wrap: wrap;
      gap: 4px;
      align-items: center;
    }
    .next-crumb a { color: #59636e; cursor: pointer; }
    .next-crumb a:hover { color: #0969da; }
    .next-crumb .sep { color: #d0d7de; }

    .next-meta-row {
      margin-top: 10px;
      display: flex;
      align-items: baseline;
      gap: 8px;
      font-size: 12px;
    }
    .next-meta-row .lbl {
      color: #8b949e;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      font-size: 10px;
      font-weight: 600;
    }
    .next-meta-row code {
      font-size: 12px;
      color: #1f2328;
      background: #f6f8fa;
      padding: 1px 6px;
      border-radius: 4px;
    }

    .next-section { margin-top: 18px; }
    .next-section .eyebrow {
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.06em;
      color: #8b949e;
      font-weight: 600;
      margin: 0 0 8px;
    }
    .next-section .empty {
      font-size: 13px;
      color: #8b949e;
      font-style: italic;
      margin: 0;
    }

    .next-edges {
      display: flex;
      flex-direction: column;
      gap: 6px;
    }
    .next-edge-chip {
      display: flex;
      align-items: baseline;
      gap: 8px;
      padding: 6px 10px;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      background: #f6f8fa;
      cursor: pointer;
      font-size: 12px;
      color: #1f2328;
      text-decoration: none;
    }
    .next-edge-chip:hover {
      border-color: #0969da;
      background: #ddf4ff;
    }
    .next-edge-chip .lbl { font-weight: 500; }
    .next-edge-chip .from, .next-edge-chip .to {
      color: #59636e;
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .next-tasks {
      display: flex;
      flex-direction: column;
      gap: 4px;
    }

    /* ---- Legacy view (dagre) styles below ---- */
    .header { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
    .breadcrumb { display: flex; gap: 4px; align-items: center; font-size: 13px; }
    .breadcrumb a {
      color: #0969da;
      cursor: pointer;
    }
    .breadcrumb a:hover { text-decoration: underline; }
    .breadcrumb .sep { color: #59636e; }
    .stage {
      position: relative;
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      overflow: hidden;
      width: 100%;
      height: 70vh;
    }
    svg { display: block; width: 100%; height: 100%; cursor: grab; }
    svg.dragging { cursor: grabbing; }
    .empty {
      position: absolute;
      inset: 0;
      display: flex;
      align-items: center;
      justify-content: center;
      color: #59636e;
      pointer-events: none;
      font-size: 14px;
    }
    .node-rect {
      fill: #fff;
      stroke: #afb8c1;
      stroke-width: 1.5;
      cursor: pointer;
    }
    .node-rect:hover { stroke: #0969da; }
    .node-rect.selected { stroke: #0969da; stroke-width: 2.5; }
    .node-rect.cyclic { stroke: #cf222e; }
    .node-kind {
      fill: #fff;
      font-size: 11px;
      font-family: ui-monospace, SFMono-Regular, monospace;
    }
    .node-kind-bg {
      stroke: none;
    }
    .node-name {
      fill: #1f2328;
      font-size: 13px;
      font-weight: 600;
      pointer-events: none;
    }
    .node-children-hint {
      fill: #59636e;
      font-size: 11px;
      pointer-events: none;
    }
    .edge-path {
      fill: none;
      stroke: #59636e;
      stroke-width: 1.5;
    }
    .edge-path.cyclic { stroke: #cf222e; stroke-dasharray: 4 3; }
    .edge-label {
      fill: #57606a;
      font-size: 11px;
      pointer-events: none;
    }
    .edge-label-bg {
      fill: #fff;
    }
    .panel {
      position: absolute;
      top: 12px;
      right: 12px;
      width: 320px;
      max-height: calc(100% - 24px);
      overflow: auto;
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      padding: 12px;
      box-shadow: 0 4px 12px rgba(0,0,0,0.08);
      font-size: 13px;
    }
    .panel header {
      display: flex;
      align-items: baseline;
      gap: 8px;
      margin-bottom: 8px;
      border-bottom: 1px solid #eaeef2;
      padding-bottom: 6px;
    }
    .panel header h4 { margin: 0; font-size: 15px; }
    .panel .close { margin-left: auto; cursor: pointer; color: #59636e; }
    .panel .section { margin-top: 8px; }
    .panel .section .label {
      font-size: 11px;
      text-transform: uppercase;
      color: #59636e;
    }
    .panel pre {
      background: #f6f8fa;
      padding: 8px;
      border-radius: 6px;
      white-space: pre-wrap;
      font-size: 12px;
      margin: 4px 0;
    }
    .panel a.slug { color: #0969da; cursor: pointer; }
    .panel a.slug:hover { text-decoration: underline; }
    .panel .pill {
      display: inline-block;
      padding: 0 6px;
      border-radius: 2em;
      font-size: 11px;
      background: #eaeef2;
    }
    .error {
      color: #cf222e;
      background: #ffebe9;
      padding: 8px 12px;
      border-radius: 6px;
      margin-bottom: 8px;
    }
    .hint {
      position: absolute;
      bottom: 8px;
      left: 12px;
      color: #59636e;
      font-size: 11px;
      pointer-events: none;
    }
  `;

  constructor() {
    super();
    this.kinds = [];
    this.allNodes = null;
    this.allEdges = [];
    this.currentParentSlug = null;
    this.selectedSlug = null;
    this.selectedDetail = null;
    this.viewBox = { x: 0, y: 0, w: 800, h: 600 };
    this.layout = { nodes: [], edges: [] };
    this.cycleEdgeIds = new Set();
    this.error = '';
    this._dragging = null;
  }

  connectedCallback() {
    super.connectedCallback();
    this.load().then(() => this._applyHash());
    this._subscribe();
    this._hashHandler = () => this._applyHash();
    window.addEventListener('hashchange', this._hashHandler);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unsub?.();
    window.removeEventListener('hashchange', this._hashHandler);
  }

  updated(c) {
    if (c.has('projectId')) {
      this.load().then(() => this._applyHash());
      this._subscribe();
    }
  }

  _applyHash() {
    const h = new URLSearchParams(window.location.hash.slice(1));
    const slug = h.get('node');
    if (!slug || !this.allNodes) return;
    const node = this.allNodes[slug];
    if (!node) return;
    // Drill the view into the node's parent so the target is visible
    // at the current level. Root nodes need null (the Roots view).
    let parentSlug = null;
    if (node.ParentID) {
      const parent = Object.values(this.allNodes).find(n => n.ID === node.ParentID);
      parentSlug = parent?.Slug || null;
    }
    if (this.currentParentSlug !== parentSlug) {
      this.currentParentSlug = parentSlug;
      this.recomputeLayout();
    }
    this.selectedSlug = slug;
    this.loadDetail(slug);
  }

  _subscribe() {
    this._unsub?.();
    if (!this.projectId) return;
    this._unsub = subscribe(this.projectId, (ev) => {
      if (ev.type !== 'realtime.reconnected' && !ev.type?.startsWith('arch.')) return;
      this.load();
      if (this.selectedSlug) this.loadDetail(this.selectedSlug);
    });
  }

  async load() {
    if (!this.projectId) return;
    try {
      const [kr, nr, er] = await Promise.all([
        fetch(`/api/projects/${this.projectId}/arch/kinds`),
        fetch(`/api/projects/${this.projectId}/arch/nodes`),
        fetch(`/api/projects/${this.projectId}/arch/edges`),
      ]);
      this.kinds = (await kr.json()).kinds || [];
      const nodes = (await nr.json()).nodes || [];
      const map = {};
      for (const n of nodes) map[n.Slug] = n;
      this.allNodes = map;
      this.allEdges = (await er.json()).edges || [];
      this.detectCycles();
      this.recomputeLayout();
    } catch (e) {
      this.error = e.message;
    }
  }

  kindByKey(key) { return this.kinds.find(k => k.Key === key); }

  // The set of nodes shown at the current level.
  visibleNodes() {
    if (!this.allNodes) return [];
    const out = [];
    const parentID = this.currentParentSlug
      ? this.allNodes[this.currentParentSlug]?.ID
      : null;
    for (const n of Object.values(this.allNodes)) {
      const nParent = n.ParentID;
      if (parentID === null && !nParent) out.push(n);
      else if (parentID && nParent === parentID) out.push(n);
    }
    return out.sort((a, b) => a.Position - b.Position || a.Slug.localeCompare(b.Slug));
  }

  // The set of edges drawn at the current level. Endpoints living
  // deeper in the tree are *promoted* to their nearest visible
  // ancestor, so the root view still shows meaningful arrows when
  // the underlying edges live between descendants.
  visibleEdges(visibleNodes) {
    const visibleByID = new Map(visibleNodes.map(n => [n.ID, n]));
    const byID = new Map();
    for (const n of Object.values(this.allNodes || {})) byID.set(n.ID, n);

    const promote = (id) => {
      // walk up parent_id until we find a visible ancestor; null if none
      let cur = byID.get(id);
      while (cur) {
        if (visibleByID.has(cur.ID)) return cur.ID;
        if (!cur.ParentID) return null;
        cur = byID.get(cur.ParentID);
      }
      return null;
    };

    const out = [];
    for (const e of this.allEdges) {
      const fromID = promote(e.FromNodeID);
      const toID = promote(e.ToNodeID);
      if (!fromID || !toID || fromID === toID) continue;
      // Substitute the visible endpoints so layout & rendering use them.
      out.push({
        ...e,
        FromNodeID: fromID,
        ToNodeID: toID,
        FromSlug: byID.get(fromID).Slug,
        FromName: byID.get(fromID).Name,
        ToSlug: byID.get(toID).Slug,
        ToName: byID.get(toID).Name,
        _promoted: fromID !== e.FromNodeID || toID !== e.ToNodeID,
      });
    }
    return out;
  }

  // hasChildren reports whether a node has any direct children.
  hasChildren(node) {
    return Object.values(this.allNodes || {}).some(n => n.ParentID === node.ID);
  }

  recomputeLayout() {
    const visible = this.visibleNodes();
    const edges = this.visibleEdges(visible);
    if (visible.length === 0) {
      this.layout = { nodes: [], edges: [] };
      return;
    }

    const g = new dagre.graphlib.Graph({ multigraph: true });
    g.setGraph({
      rankdir: 'LR',
      nodesep: 40,
      ranksep: 80,
      marginx: 24,
      marginy: 24,
    });
    g.setDefaultEdgeLabel(() => ({}));

    const nodeW = 180, nodeH = 60;
    for (const n of visible) {
      g.setNode(n.Slug, { width: nodeW, height: nodeH });
    }
    for (const e of edges) {
      const fromSlug = visible.find(v => v.ID === e.FromNodeID)?.Slug;
      const toSlug = visible.find(v => v.ID === e.ToNodeID)?.Slug;
      if (fromSlug && toSlug) {
        g.setEdge(fromSlug, toSlug, { kind: e.Kind, label: e.Label, id: e.ID }, `${e.ID}`);
      }
    }
    dagre.layout(g);

    const laidNodes = visible.map(n => {
      const gn = g.node(n.Slug);
      return {
        node: n,
        x: gn.x,
        y: gn.y,
        w: gn.width,
        h: gn.height,
      };
    });
    const laidEdges = edges.map(e => {
      const ge = g.edge(
        visible.find(v => v.ID === e.FromNodeID).Slug,
        visible.find(v => v.ID === e.ToNodeID).Slug,
        `${e.ID}`,
      );
      return {
        edge: e,
        points: ge.points,
      };
    });

    // Compute viewBox from extents.
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const ln of laidNodes) {
      minX = Math.min(minX, ln.x - ln.w / 2);
      minY = Math.min(minY, ln.y - ln.h / 2);
      maxX = Math.max(maxX, ln.x + ln.w / 2);
      maxY = Math.max(maxY, ln.y + ln.h / 2);
    }
    const pad = 32;
    this.layout = { nodes: laidNodes, edges: laidEdges };
    this.viewBox = {
      x: minX - pad,
      y: minY - pad,
      w: Math.max(400, maxX - minX + pad * 2),
      h: Math.max(300, maxY - minY + pad * 2),
    };
  }

  // detectCycles marks edges that participate in a directed cycle.
  // Uses a depth-first search with three colours (white, grey, black);
  // any edge to a grey node is a back-edge → it's part of a cycle.
  detectCycles() {
    const adj = new Map();   // nodeID -> [{ edgeID, toID }]
    for (const n of Object.values(this.allNodes || {})) adj.set(n.ID, []);
    for (const e of this.allEdges) {
      if (adj.has(e.FromNodeID)) {
        adj.get(e.FromNodeID).push({ edgeID: e.ID, toID: e.ToNodeID });
      }
    }
    const colour = new Map(); // 0 white, 1 grey, 2 black
    const cyc = new Set();
    const dfs = (u) => {
      colour.set(u, 1);
      for (const { edgeID, toID } of adj.get(u) || []) {
        const c = colour.get(toID) || 0;
        if (c === 1) {
          cyc.add(edgeID);
        } else if (c === 0) {
          dfs(toID);
        }
      }
      colour.set(u, 2);
    };
    for (const id of adj.keys()) if ((colour.get(id) || 0) === 0) dfs(id);
    this.cycleEdgeIds = cyc;
  }

  drillInto(slug) {
    this.currentParentSlug = slug;
    this.selectedSlug = null;
    this.selectedDetail = null;
    this.recomputeLayout();
  }

  popTo(slug) {
    this.currentParentSlug = slug;
    this.selectedSlug = null;
    this.selectedDetail = null;
    this.recomputeLayout();
  }

  breadcrumb() {
    if (!this.currentParentSlug) return [{ slug: null, name: 'Roots' }];
    const path = [];
    let s = this.currentParentSlug;
    while (s) {
      const n = this.allNodes[s];
      if (!n) break;
      path.unshift({ slug: n.Slug, name: n.Name });
      if (!n.ParentID) break;
      const parent = Object.values(this.allNodes).find(x => x.ID === n.ParentID);
      s = parent?.Slug;
    }
    return [{ slug: null, name: 'Roots' }, ...path];
  }

  // --- node click handling ---
  onNodeClick(slug) {
    if (this.selectedSlug === slug) {
      // double action: drill into if it has children, else just close detail.
      const n = this.allNodes[slug];
      if (n && this.hasChildren(n)) {
        this.drillInto(slug);
        return;
      }
      this.selectedSlug = null;
      this.selectedDetail = null;
      return;
    }
    this.selectedSlug = slug;
    this.loadDetail(slug);
  }

  async loadDetail(slug) {
    try {
      const r = await fetch(`/api/projects/${this.projectId}/arch/nodes/${encodeURIComponent(slug)}`);
      if (!r.ok) throw new Error('failed');
      this.selectedDetail = await r.json();
    } catch (e) {
      this.error = e.message;
    }
  }

  // --- pan/zoom ---
  onSvgPointerDown(e) {
    if (e.target.closest('.node-rect')) return;
    this._dragging = { x: e.clientX, y: e.clientY, vb: { ...this.viewBox } };
    e.currentTarget.classList.add('dragging');
  }

  onSvgPointerMove(e) {
    if (!this._dragging) return;
    const dx = e.clientX - this._dragging.x;
    const dy = e.clientY - this._dragging.y;
    const rect = e.currentTarget.getBoundingClientRect();
    const scaleX = this._dragging.vb.w / rect.width;
    const scaleY = this._dragging.vb.h / rect.height;
    this.viewBox = {
      x: this._dragging.vb.x - dx * scaleX,
      y: this._dragging.vb.y - dy * scaleY,
      w: this._dragging.vb.w,
      h: this._dragging.vb.h,
    };
  }

  onSvgPointerUp(e) {
    this._dragging = null;
    e.currentTarget.classList.remove('dragging');
  }

  onSvgWheel(e) {
    e.preventDefault();
    const rect = e.currentTarget.getBoundingClientRect();
    const cx = this.viewBox.x + (e.clientX - rect.left) * (this.viewBox.w / rect.width);
    const cy = this.viewBox.y + (e.clientY - rect.top) * (this.viewBox.h / rect.height);
    const factor = e.deltaY > 0 ? 1.15 : 1 / 1.15;
    const w = this.viewBox.w * factor;
    const h = this.viewBox.h * factor;
    this.viewBox = {
      x: cx - (cx - this.viewBox.x) * factor,
      y: cy - (cy - this.viewBox.y) * factor,
      w,
      h,
    };
  }

  fitView() {
    this.recomputeLayout();
  }

  // --- render helpers ---

  renderEdge(le) {
    const p = le.points;
    if (!p || !p.length) return null;
    let d = `M${p[0].x},${p[0].y}`;
    for (let i = 1; i < p.length; i++) d += ` L${p[i].x},${p[i].y}`;
    const cyclic = this.cycleEdgeIds.has(le.edge.ID);
    // mid-point for label
    const mid = p[Math.floor(p.length / 2)];
    return svg`
      <g>
        <path class=${`edge-path ${cyclic ? 'cyclic' : ''}`}
              d=${d} marker-end="url(#arrowhead)"></path>
        ${le.edge.Label || le.edge.Kind ? svg`
          <text class="edge-label" x=${mid.x} y=${mid.y - 6} text-anchor="middle">
            ${le.edge.Label || le.edge.Kind}
          </text>
        ` : null}
      </g>
    `;
  }

  renderNode(ln) {
    const n = ln.node;
    const kind = this.kindByKey(n.Kind);
    const color = kind?.Color || '#888';
    const x = ln.x - ln.w / 2;
    const y = ln.y - ln.h / 2;
    const selected = this.selectedSlug === n.Slug;
    const cyclic = false; // node-level cycle indicator could be added later
    const expandable = this.hasChildren(n);
    return svg`
      <g @click=${(e) => { e.stopPropagation(); this.onNodeClick(n.Slug); }}>
        <rect class=${`node-rect ${selected ? 'selected' : ''} ${cyclic ? 'cyclic' : ''}`}
              x=${x} y=${y} width=${ln.w} height=${ln.h} rx="8" ry="8"></rect>
        <rect class="node-kind-bg" x=${x} y=${y} width=${ln.w} height="16"
              fill=${color} opacity="0.9"></rect>
        <text class="node-kind" x=${x + 8} y=${y + 12}>${n.Kind}${expandable ? '  ▾' : ''}</text>
        <text class="node-name" x=${ln.x} y=${y + 36} text-anchor="middle">
          ${n.Name}
        </text>
        <text class="node-children-hint" x=${ln.x} y=${y + ln.h - 10} text-anchor="middle">
          ${n.Slug}
        </text>
      </g>
    `;
  }

  renderPanel() {
    if (!this.selectedDetail) return null;
    const { node, children, edges, links } = this.selectedDetail;
    const incoming = edges.filter(e => e.ToSlug === node.Slug);
    const outgoing = edges.filter(e => e.FromSlug === node.Slug);
    const kind = this.kindByKey(node.Kind);
    return html`
      <div class="panel">
        <header>
          <h4>${node.Name}</h4>
          <span class="pill" style=${`background: ${kind?.Color || '#eaeef2'}1a`}>${kind?.Label || node.Kind}</span>
          <span class="close" @click=${() => { this.selectedSlug = null; this.selectedDetail = null; }}>✕</span>
        </header>
        <div class="section">
          <div class="label">slug</div>
          <code>${node.Slug}</code>
        </div>
        ${node.LinkedRepo ? html`<div class="section"><div class="label">repo</div><code>${node.LinkedRepo}${node.LinkedPath ? ' · ' + node.LinkedPath : ''}</code></div>` : null}
        ${Object.keys(node.Metadata || {}).length ? html`
          <div class="section">
            <div class="label">metadata</div>
            <pre>${JSON.stringify(node.Metadata, null, 2)}</pre>
          </div>
        ` : null}
        ${node.DescriptionMD ? html`
          <div class="section">
            <div class="label">description</div>
            <nottario-markdown
              project-id=${this.projectId}
              .source=${node.DescriptionMD}></nottario-markdown>
          </div>
        ` : null}
        ${children && children.length ? html`
          <div class="section">
            <div class="label">children (${children.length})</div>
            ${children.map(c => html`<div><a class="slug" @click=${() => this.drillInto(c.Slug)}>${c.Name}</a></div>`)}
            <button @click=${() => this.drillInto(node.Slug)} style="margin-top:6px">Drill into ${node.Slug}</button>
          </div>
        ` : null}
        ${outgoing.length ? html`
          <div class="section">
            <div class="label">outgoing</div>
            ${outgoing.map(e => html`<div><strong>${e.Kind}</strong> → <a class="slug" @click=${() => this.onNodeClick(e.ToSlug)}>${e.ToName}</a> ${e.Label ? html`<span style="color:#57606a">— ${e.Label}</span>` : null}</div>`)}
          </div>
        ` : null}
        ${incoming.length ? html`
          <div class="section">
            <div class="label">incoming</div>
            ${incoming.map(e => html`<div><a class="slug" @click=${() => this.onNodeClick(e.FromSlug)}>${e.FromName}</a> → <strong>${e.Kind}</strong> ${e.Label ? html`<span style="color:#57606a">— ${e.Label}</span>` : null}</div>`)}
          </div>
        ` : null}
        ${links && links.length ? html`
          <div class="section">
            <div class="label">links</div>
            ${links.map(l => html`<div><strong>${l.LinkType}</strong> · <code>${l.TargetID}</code></div>`)}
          </div>
        ` : null}
      </div>
    `;
  }

  render() {
    if (this.allNodes === null) return html`<p>Loading…</p>`;
    // Opt-in preview of the redesigned hand-rolled canvas (feature
    // f9a7a488). Visit /arch/diagram?next to see the new layout +
    // toolbar + right-rail detail panel.
    // Drops cleanly when child E retires the legacy view entirely.
    if (typeof window !== 'undefined' &&
        new URLSearchParams(window.location.search).get('next') !== null) {
      return this._renderNext();
    }
    const crumbs = this.breadcrumb();
    const vb = this.viewBox;
    const vbStr = `${vb.x} ${vb.y} ${vb.w} ${vb.h}`;
    const hasNothing = this.layout.nodes.length === 0;
    return html`
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      <div class="header">
        <div class="breadcrumb">
          ${crumbs.map((c, i) => html`
            ${i > 0 ? html`<span class="sep">/</span>` : null}
            <a @click=${() => this.popTo(c.slug)}>${c.name}</a>
          `)}
        </div>
        <div style="flex:1"></div>
        <button @click=${() => this.fitView()}>Fit</button>
      </div>
      <div class="stage">
        <svg viewBox=${vbStr}
             @pointerdown=${(e) => this.onSvgPointerDown(e)}
             @pointermove=${(e) => this.onSvgPointerMove(e)}
             @pointerup=${(e) => this.onSvgPointerUp(e)}
             @pointerleave=${(e) => this.onSvgPointerUp(e)}
             @wheel=${(e) => this.onSvgWheel(e)}>
          <defs>
            <marker id="arrowhead" viewBox="0 0 10 10" refX="9" refY="5"
                    markerWidth="8" markerHeight="8" orient="auto-start-reverse">
              <path d="M 0 0 L 10 5 L 0 10 z" fill="#59636e"></path>
            </marker>
          </defs>
          ${this.layout.edges.map(le => this.renderEdge(le))}
          ${this.layout.nodes.map(ln => this.renderNode(ln))}
        </svg>
        ${hasNothing ? html`<div class="empty">No nodes at this level.</div>` : null}
        ${this.renderPanel()}
        <div class="hint">scroll to zoom · drag background to pan · click box to inspect · click ▾ box to drill in</div>
      </div>
    `;
  }

  // ----- "Next" view (feature f9a7a488 children A-D) -----

  // Read-only convenience getters that walk the loaded nodes.
  _nodesArr() {
    return this.allNodes ? Object.values(this.allNodes) : [];
  }
  _nodeByID(id) {
    if (!this.allNodes) return null;
    return Object.values(this.allNodes).find(n => n.ID === id) || null;
  }
  _selectedID() {
    if (!this.selectedSlug || !this.allNodes) return '';
    const n = this.allNodes[this.selectedSlug];
    return n ? n.ID : '';
  }

  // Compute the path from root to the selected node so the detail
  // panel can render the slug breadcrumb (system / backend / auth).
  _ancestorChain(id) {
    if (!id) return [];
    const chain = [];
    let cur = this._nodeByID(id);
    let guard = 0;
    while (cur && guard++ < 32) {
      chain.unshift(cur);
      if (!cur.ParentID) break;
      cur = this._nodeByID(cur.ParentID);
    }
    return chain;
  }

  _onCanvasSelect(e) {
    const id = e.detail.id;
    const n = this._nodeByID(id);
    if (!n) { this.selectedSlug = null; return; }
    this.selectedSlug = n.Slug;
    this.loadDetail(n.Slug);
  }
  _onCanvasExpandChanged(e) {
    this._nextExpanded = new Set(e.detail.expanded);
    this._writeNextHash();
  }
  _onCanvasFocusChanged(e) {
    this._nextFocus = e.detail.id || '';
    this._writeNextHash();
  }

  _writeNextHash() {
    if (typeof window === 'undefined') return;
    const parts = [];
    if (this._nextExpanded && this._nextExpanded.size) {
      parts.push('expand=' + [...this._nextExpanded].join(','));
    }
    if (this._nextFocus) parts.push('focus=' + this._nextFocus);
    const hash = parts.join('&');
    history.replaceState(null, '', '#' + hash + window.location.search);
  }

  _renderNext() {
    if (!this._nextInited) {
      this._nextInited = true;
      // Initial expanded/focus from URL hash.
      const h = new URLSearchParams((window.location.hash || '#').slice(1));
      const exp = h.get('expand');
      this._nextExpanded = new Set(exp ? exp.split(',').filter(Boolean) : []);
      this._nextFocus = h.get('focus') || '';
      this._nextQuery = '';
    }
    const nodesArr = this._nodesArr();
    const selectedID = this._selectedID();
    const sel = selectedID ? this._nodeByID(selectedID) : null;
    const inEdges = sel ? (this.allEdges || []).filter(e => e.ToNodeID === sel.ID) : [];
    const outEdges = sel ? (this.allEdges || []).filter(e => e.FromNodeID === sel.ID) : [];
    const ancestors = sel ? this._ancestorChain(sel.ID) : [];

    return html`
      <div class="next-root">
        <div class="next-toolbar">
          <a class="legacy-link" @click=${() => { window.location.search = ''; }}>← Legacy view</a>
          <div class="spacer"></div>
          <nottario-search-input
            placeholder="Filter nodes…"
            .value=${this._nextQuery}
            @input=${(e) => { this._nextQuery = e.detail.value; this.requestUpdate(); }}
            @clear=${() => { this._nextQuery = ''; this.requestUpdate(); }}
            style="width:240px"></nottario-search-input>
          <button class="next-btn" @click=${() => this._archCanvas()?.fit()}>Fit</button>
        </div>

        <div class="next-split">
          <nottario-arch-canvas
              class="next-canvas"
              .nodes=${nodesArr}
              .edges=${this.allEdges || []}
              .expanded=${this._nextExpanded}
              .focus=${this._nextFocus}
              .selected=${selectedID}
              .query=${this._nextQuery}
              @select=${(e) => this._onCanvasSelect(e)}
              @expand-changed=${(e) => this._onCanvasExpandChanged(e)}
              @focus-changed=${(e) => this._onCanvasFocusChanged(e)}>
          </nottario-arch-canvas>

          ${this._renderNextPanel(sel, ancestors, inEdges, outEdges)}
        </div>
      </div>
    `;
  }

  _archCanvas() {
    return this.shadowRoot?.querySelector('nottario-arch-canvas');
  }

  _renderNextPanel(sel, ancestors, inEdges, outEdges) {
    if (!sel) {
      return html`
        <aside class="next-panel">
          <p class="next-empty">Select a node to see its detail.</p>
        </aside>
      `;
    }
    const detail = this.selectedDetail;
    const kindColor = (() => {
      switch ((sel.Kind || '').toLowerCase()) {
        case 'system':   return '#0969da';
        case 'service':  return '#1f883d';
        case 'module':   return '#8250df';
        case 'external': return '#bc4c00';
        case 'data':     return '#9a6700';
        case 'queue':    return '#cf222e';
        default:         return '#59636e';
      }
    })();
    return html`
      <aside class="next-panel">
        <header class="next-head">
          <div class="next-kind-chip">
            <span class="dot" style=${`background:${kindColor}`}></span>
            <span class="lbl">${(sel.Kind || '').toLowerCase()}</span>
          </div>
          <h2>${sel.Name}</h2>
        </header>
        <div class="next-crumb">
          ${ancestors.map((a, i) => html`
            ${i > 0 ? html`<span class="sep">/</span>` : null}
            <a @click=${() => this._onCanvasSelect({ detail: { id: a.ID } })}>${a.Slug}</a>
          `)}
        </div>
        ${sel.LinkedRepo ? html`
          <div class="next-meta-row">
            <span class="lbl">repo</span>
            <code>${sel.LinkedRepo}${sel.LinkedPath ? '/' + sel.LinkedPath : ''}</code>
          </div>
        ` : null}
        ${sel.DescriptionMD ? html`
          <section class="next-section">
            <nottario-markdown
              project-id=${this.projectId}
              .source=${sel.DescriptionMD}></nottario-markdown>
          </section>
        ` : null}

        <section class="next-section">
          <h4 class="eyebrow">Incoming edges</h4>
          ${inEdges.length === 0
            ? html`<p class="empty">No incoming edges.</p>`
            : html`<div class="next-edges">
                ${inEdges.map(e => html`
                  <a class="next-edge-chip"
                     @click=${() => this._onCanvasSelect({ detail: { id: e.FromNodeID } })}>
                    <span class="lbl">${e.Label || e.Kind || 'connects'}</span>
                    <span class="from">← ${this._nodeByID(e.FromNodeID)?.Name || '?'}</span>
                  </a>
                `)}
              </div>`}
        </section>

        <section class="next-section">
          <h4 class="eyebrow">Outgoing edges</h4>
          ${outEdges.length === 0
            ? html`<p class="empty">No outgoing edges.</p>`
            : html`<div class="next-edges">
                ${outEdges.map(e => html`
                  <a class="next-edge-chip"
                     @click=${() => this._onCanvasSelect({ detail: { id: e.ToNodeID } })}>
                    <span class="lbl">${e.Label || e.Kind || 'connects'}</span>
                    <span class="to">→ ${this._nodeByID(e.ToNodeID)?.Name || '?'}</span>
                  </a>
                `)}
              </div>`}
        </section>

        ${detail?.LinkedTasks?.length ? html`
          <section class="next-section">
            <h4 class="eyebrow">Linked tasks</h4>
            <div class="next-tasks">
              ${detail.LinkedTasks.map(t => html`
                <nottario-task-chip
                  project-id=${this.projectId}
                  .task=${t}></nottario-task-chip>
              `)}
            </div>
          </section>
        ` : null}
      </aside>
    `;
  }
}

customElements.define('nottario-arch-graph', NottarioArchGraph);
