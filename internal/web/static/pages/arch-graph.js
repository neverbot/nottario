import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';
import '/static/components/markdown.js';
import '/static/components/arch-canvas.js';
import '/static/components/task-chip.js';
import '/static/components/search-input.js';

// <nottario-arch-graph> renders the architecture diagram for a project.
// Layout is computed by elkjs; rendering, interaction and the right-
// rail detail panel are this surface's job (delegated to
// <nottario-arch-canvas> for the SVG canvas itself).

class NottarioArchGraph extends LitElement {
  static properties = {
    projectId:        { type: String },
    kinds:            { state: true },
    allNodes:         { state: true }, // slug-keyed map
    allEdges:         { state: true },
    selectedSlug:     { state: true },
    selectedDetail:   { state: true },
    error:            { state: true },

    _expanded: { state: true }, // Set<id>
    _focus:    { state: true }, // id
    _query:    { state: true }, // string
    _hoveredEdgeID: { state: true }, // edge id, drives canvas highlight
  };

  static styles = css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }

    .toolbar {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-bottom: 12px;
      font-size: 13px;
    }
    .toolbar .spacer { flex: 1; }
    .btn {
      padding: 4px 12px;
      font: inherit;
      font-size: 12px;
      background: #ffffff;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      cursor: pointer;
      color: #1f2328;
    }
    .btn:hover { background: #f6f8fa; border-color: #afb8c1; }
    .btn.on { background: #0969da; color: #fff; border-color: #0969da; }
    .btn.on:hover { background: #0860c4; border-color: #0860c4; }

    .split {
      display: grid;
      grid-template-columns: 1fr 320px;
      gap: 0;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      overflow: hidden;
      min-height: 70vh;
      background: #ffffff;
    }
    nottario-arch-canvas {
      display: block;
      border-right: 1px solid #d1d9e0;
      /* HEIGHT, not min-height. With min-height alone, the inner SVG
         (width:100% height:100% with a viewBox) auto-grows to keep
         the viewBox's aspect ratio, dragging the host's height
         along. The fit() result then renders at a much larger pixel
         size than the visible viewport, which reads as "zoomed in"
         on first paint. A fixed viewport-relative height caps the
         host, the SVG fills it, and preserveAspectRatio="meet" does
         the visible fitting. */
      height: 70vh;
      /* Round only the LEFT corners so the right-side separator
         (border-right above) stays a perfectly vertical line. */
      border-top-left-radius: 7px;
      border-bottom-left-radius: 7px;
    }

    .panel {
      padding: 18px 20px 24px;
      overflow: auto;
      max-height: 70vh;
      font-size: 13px;
      color: #1f2328;
    }
    .panel .empty-pane {
      color: #8b949e;
      font-style: italic;
      margin: 12px 0 0;
    }
    .head {
      display: flex;
      flex-direction: column;
      gap: 4px;
      margin-bottom: 4px;
    }
    .head h2 {
      margin: 0;
      font-size: 18px;
      font-weight: 600;
      line-height: 1.25;
      color: #1f2328;
    }
    .kind-chip {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      font: 600 10.5px/1 ui-monospace, SFMono-Regular, monospace;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      color: #59636e;
    }
    .kind-chip .dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      display: inline-block;
    }
    .crumb {
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 11px;
      color: #8b949e;
      margin-top: 6px;
      display: flex;
      flex-wrap: wrap;
      gap: 4px;
      align-items: center;
    }
    .crumb a { color: #59636e; cursor: pointer; }
    .crumb a:hover { color: #0969da; }
    .crumb .sep { color: #d0d7de; }

    .meta-row {
      margin-top: 10px;
      display: flex;
      align-items: baseline;
      gap: 8px;
      font-size: 12px;
    }
    .meta-row .lbl {
      color: #8b949e;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      font-size: 10px;
      font-weight: 600;
    }
    .meta-row code {
      font-size: 12px;
      color: #1f2328;
      background: #f6f8fa;
      padding: 1px 6px;
      border-radius: 4px;
    }

    .section { margin-top: 18px; }
    .section .eyebrow {
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.06em;
      color: #8b949e;
      font-weight: 600;
      margin: 0 0 8px;
    }
    .section .empty {
      font-size: 13px;
      color: #8b949e;
      font-style: italic;
      margin: 0;
    }

    .edges {
      display: flex;
      flex-direction: column;
      gap: 6px;
    }
    .edge-chip {
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
    .edge-chip:hover {
      border-color: #0969da;
      background: #ddf4ff;
    }
    .edge-chip .lbl { font-weight: 500; }
    .edge-chip .from, .edge-chip .to {
      color: #59636e;
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .tasks { display: flex; flex-direction: column; gap: 4px; }

    .error { color: #cf222e; margin-bottom: 8px; font-size: 13px; }
  `;

  constructor() {
    super();
    this.kinds = [];
    this.allNodes = null;
    this.allEdges = [];
    this.selectedSlug = null;
    this.selectedDetail = null;
    this.error = '';
    this._expanded = new Set();
    this._focus = '';
    this._query = '';
    this._hoveredEdgeID = '';
  }

  // ----- Lifecycle -----

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

  // Persist + restore expanded/focus + selected from the URL hash.
  // Hash shape: #node=slug&expand=id1,id2&focus=id
  _applyHash() {
    const h = new URLSearchParams((window.location.hash || '#').slice(1));
    const exp = h.get('expand');
    this._expanded = new Set(exp ? exp.split(',').filter(Boolean) : []);
    this._focus = h.get('focus') || '';
    const slug = h.get('node');
    if (slug && this.allNodes && this.allNodes[slug]) {
      this.selectedSlug = slug;
      this.loadDetail(slug);
    }
  }
  _writeHash() {
    const parts = [];
    if (this.selectedSlug) parts.push('node=' + encodeURIComponent(this.selectedSlug));
    if (this._expanded && this._expanded.size) {
      parts.push('expand=' + [...this._expanded].join(','));
    }
    if (this._focus) parts.push('focus=' + this._focus);
    const hash = parts.join('&');
    history.replaceState(null, '', '#' + hash + window.location.search);
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

  // ----- Data -----

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
    } catch (e) {
      this.error = e.message;
    }
  }

  async loadDetail(slug) {
    try {
      const r = await fetch(`/api/projects/${this.projectId}/arch/nodes/${encodeURIComponent(slug)}`);
      if (!r.ok) throw new Error('failed to load node');
      this.selectedDetail = await r.json();
    } catch (e) {
      this.error = e.message;
    }
  }

  // ----- Helpers -----

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

  _kindColor(k) {
    switch ((k || '').toLowerCase()) {
      case 'system':   return '#0969da';
      case 'service':  return '#1f883d';
      case 'module':   return '#8250df';
      case 'external': return '#bc4c00';
      case 'data':     return '#9a6700';
      case 'queue':    return '#cf222e';
      default:         return '#59636e';
    }
  }

  // ----- Event handlers -----

  _onCanvasSelect(e) {
    const id = e.detail.id;
    const n = this._nodeByID(id);
    if (!n) { this.selectedSlug = null; this._writeHash(); return; }
    this.selectedSlug = n.Slug;
    this.loadDetail(n.Slug);
    this._writeHash();
  }
  _onExpandChanged(e) {
    this._expanded = new Set(e.detail.expanded);
    this._writeHash();
  }
  _onFocusChanged(e) {
    this._focus = e.detail.id || '';
    this._writeHash();
  }

  // ----- Render -----

  render() {
    if (this.allNodes === null) return html`<p>Loading…</p>`;
    const selID = this._selectedID();
    const sel = selID ? this._nodeByID(selID) : null;
    const inEdges = sel ? (this.allEdges || []).filter(e => e.ToNodeID === sel.ID) : [];
    const outEdges = sel ? (this.allEdges || []).filter(e => e.FromNodeID === sel.ID) : [];
    const ancestors = sel ? this._ancestorChain(sel.ID) : [];

    return html`
      ${this.error ? html`<div class="error">${this.error}</div>` : null}

      <div class="toolbar">
        <div class="spacer"></div>
        <nottario-search-input
          placeholder="Filter nodes…"
          .value=${this._query}
          @input=${(e) => { this._query = e.detail.value; }}
          @clear=${() => { this._query = ''; }}
          style="width:240px"></nottario-search-input>
        <button class="btn" @click=${() => this._archCanvas()?.fit()}>Fit</button>
      </div>

      <div class="split">
        <nottario-arch-canvas
          .nodes=${this._nodesArr()}
          .edges=${this.allEdges || []}
          .expanded=${this._expanded}
          .focus=${this._focus}
          .selected=${selID}
          .query=${this._query}
          .highlightEdge=${this._hoveredEdgeID}
          @select=${(e) => this._onCanvasSelect(e)}
          @expand-changed=${(e) => this._onExpandChanged(e)}
          @focus-changed=${(e) => this._onFocusChanged(e)}>
        </nottario-arch-canvas>

        ${this._renderPanel(sel, ancestors, inEdges, outEdges)}
      </div>
    `;
  }

  _archCanvas() {
    return this.shadowRoot?.querySelector('nottario-arch-canvas');
  }

  _renderPanel(sel, ancestors, inEdges, outEdges) {
    if (!sel) {
      return html`
        <aside class="panel">
          <p class="empty-pane">Select a node to see its detail.</p>
        </aside>
      `;
    }
    const detail = this.selectedDetail;
    return html`
      <aside class="panel">
        <header class="head">
          <div class="kind-chip">
            <span class="dot" style=${`background:${this._kindColor(sel.Kind)}`}></span>
            <span class="lbl">${(sel.Kind || '').toLowerCase()}</span>
          </div>
          <h2>${sel.Name}</h2>
        </header>
        <div class="crumb">
          ${ancestors.map((a, i) => html`
            ${i > 0 ? html`<span class="sep">/</span>` : null}
            <a @click=${() => this._onCanvasSelect({ detail: { id: a.ID } })}>${a.Slug}</a>
          `)}
        </div>
        ${sel.LinkedRepo ? html`
          <div class="meta-row">
            <span class="lbl">repo</span>
            <code>${sel.LinkedRepo}${sel.LinkedPath ? '/' + sel.LinkedPath : ''}</code>
          </div>
        ` : null}
        ${sel.DescriptionMD ? html`
          <section class="section">
            <nottario-markdown
              project-id=${this.projectId}
              .source=${sel.DescriptionMD}></nottario-markdown>
          </section>
        ` : null}

        <section class="section">
          <h4 class="eyebrow">Incoming edges</h4>
          ${inEdges.length === 0
            ? html`<p class="empty">No incoming edges.</p>`
            : html`<div class="edges">
                ${inEdges.map(e => html`
                  <a class="edge-chip"
                     @click=${() => this._onCanvasSelect({ detail: { id: e.FromNodeID } })}
                     @mouseenter=${() => { this._hoveredEdgeID = e.ID; }}
                     @mouseleave=${() => { this._hoveredEdgeID = ''; }}>
                    <span class="lbl">${e.Label || e.Kind || 'connects'}</span>
                    <span class="from">← ${this._nodeByID(e.FromNodeID)?.Name || '?'}</span>
                  </a>
                `)}
              </div>`}
        </section>

        <section class="section">
          <h4 class="eyebrow">Outgoing edges</h4>
          ${outEdges.length === 0
            ? html`<p class="empty">No outgoing edges.</p>`
            : html`<div class="edges">
                ${outEdges.map(e => html`
                  <a class="edge-chip"
                     @click=${() => this._onCanvasSelect({ detail: { id: e.ToNodeID } })}
                     @mouseenter=${() => { this._hoveredEdgeID = e.ID; }}
                     @mouseleave=${() => { this._hoveredEdgeID = ''; }}>
                    <span class="lbl">${e.Label || e.Kind || 'connects'}</span>
                    <span class="to">→ ${this._nodeByID(e.ToNodeID)?.Name || '?'}</span>
                  </a>
                `)}
              </div>`}
        </section>

        ${detail?.LinkedTasks?.length ? html`
          <section class="section">
            <h4 class="eyebrow">Linked tasks</h4>
            <div class="tasks">
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
