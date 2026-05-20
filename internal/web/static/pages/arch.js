import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';
import { buttonStyles } from '/static/components/buttons.js';
import '/static/components/page-header.js';
import '/static/components/segmented-control.js';
import './arch-graph.js';

class NottarioArchPage extends LitElement {
  static properties = {
    me: { type: Object },
    projectId: { type: String },
    // 'diagram' (default, was 'graph') or 'tree'. Driven by URL.
    view: { type: String },
    project: { state: true },
    kinds: { state: true },
    rootNodes: { state: true },
    selectedSlug: { state: true },
    selectedDetail: { state: true },
    expanded: { state: true },
    childrenCache: { state: true },
    error: { state: true },
  };

  static styles = [buttonStyles, css`
    :host { display: block; }
    .header h2 { margin: 0; }
    .header .muted { color: #59636e; }
    .layout {
      display: grid;
      grid-template-columns: 340px 1fr;
      gap: 16px;
      min-height: 70vh;
    }
    .sidebar, .reader {
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      padding: 12px;
      overflow: auto;
      max-height: 80vh;
    }
    .tree { list-style: none; padding-left: 0; margin: 0; }
    .tree ul { list-style: none; padding-left: 14px; margin: 0; }
    .node {
      display: flex;
      align-items: center;
      gap: 4px;
      padding: 3px 4px;
      border-radius: 4px;
      cursor: pointer;
      font-size: 13px;
    }
    .node:hover { background: #f6f8fa; }
    .node.active { background: #ddf4ff; color: #0969da; }
    .toggle {
      width: 14px;
      text-align: center;
      color: #59636e;
      user-select: none;
      font-family: ui-monospace, monospace;
    }
    .toggle.empty { visibility: hidden; }
    .kind-dot {
      display: inline-block;
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: #999;
    }
    .kind-label {
      font-size: 10px;
      color: #59636e;
      text-transform: uppercase;
      margin-left: 4px;
    }
    .reader header {
      display: flex;
      align-items: baseline;
      gap: 8px;
      border-bottom: 1px solid #eaeef2;
      padding-bottom: 8px;
      margin-bottom: 12px;
    }
    .reader header h3 { margin: 0; font-size: 18px; }
    .reader .meta { color: #59636e; font-size: 12px; }
    .reader .meta-grid {
      display: grid;
      grid-template-columns: max-content 1fr;
      gap: 4px 16px;
      font-size: 13px;
      margin-bottom: 12px;
    }
    .reader .meta-grid > div:nth-child(odd) {
      color: #59636e;
    }
    .reader .section { margin-top: 16px; }
    .reader .section h4 {
      margin: 0 0 6px 0;
      font-size: 12px;
      text-transform: uppercase;
      color: #59636e;
    }
    .reader pre.body {
      white-space: pre-wrap;
      background: #f6f8fa;
      border-radius: 6px;
      padding: 12px;
      font-size: 13px;
      margin: 0;
    }
    .reader .edge-line {
      padding: 4px 8px;
      border-radius: 4px;
      font-size: 13px;
      font-family: ui-monospace, monospace;
    }
    .reader .edge-line:hover { background: #f6f8fa; cursor: pointer; }
    .arrow { color: #59636e; margin: 0 4px; }
    .kind-pill {
      display: inline-block;
      padding: 0 6px;
      border-radius: 2em;
      font-size: 11px;
      background: #eaeef2;
      color: #1f2328;
      margin-left: 4px;
    }
    .empty {
      padding: 40px;
      text-align: center;
      color: #59636e;
    }
    .error {
      color: #cf222e;
      background: #ffebe9;
      padding: 8px 12px;
      border-radius: 6px;
      margin-bottom: 8px;
    }
    code { font-size: 12px; }
  `];

  constructor() {
    super();
    this.view = 'diagram';
    this.project = null;
    this.kinds = [];
    this.rootNodes = null;
    this.selectedSlug = null;
    this.selectedDetail = null;
    this.expanded = {};
    this.childrenCache = {};
    this.error = '';
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
    if (slug) this.select(slug);
  }

  _subscribe() {
    this._unsub?.();
    if (!this.projectId) return;
    this._unsub = subscribe(this.projectId, (ev) => {
      if (!ev.type?.startsWith('arch.')) return;
      this.load();
      if (this.selectedSlug) {
        // refresh the open node detail
        fetch(`/api/projects/${this.projectId}/arch/nodes/${encodeURIComponent(this.selectedSlug)}`)
          .then(r => r.ok ? r.json() : null)
          .then(d => { if (d) this.selectedDetail = d; });
      }
    });
  }

  async load() {
    if (!this.projectId) return;
    try {
      const [pr, kr, nr] = await Promise.all([
        fetch(`/api/projects/${this.projectId}`),
        fetch(`/api/projects/${this.projectId}/arch/kinds`),
        fetch(`/api/projects/${this.projectId}/arch/nodes?root_only=true`),
      ]);
      if (!pr.ok) throw new Error('project not found');
      this.project = await pr.json();
      this.kinds = (await kr.json()).kinds || [];
      this.rootNodes = (await nr.json()).nodes || [];
    } catch (e) { this.error = e.message; }
  }

  kindByKey(key) { return this.kinds.find(k => k.Key === key); }

  back() { window.nottarioNavigate('/'); }

  async toggle(slug) {
    const isOpen = !!this.expanded[slug];
    if (isOpen) {
      this.expanded = { ...this.expanded, [slug]: false };
      return;
    }
    if (!this.childrenCache[slug]) {
      const r = await fetch(
        `/api/projects/${this.projectId}/arch/nodes?parent_slug=${encodeURIComponent(slug)}`
      );
      if (r.ok) {
        const j = await r.json();
        this.childrenCache = { ...this.childrenCache, [slug]: j.nodes || [] };
      }
    }
    this.expanded = { ...this.expanded, [slug]: true };
  }

  async select(slug) {
    this.selectedSlug = slug;
    try {
      const r = await fetch(`/api/projects/${this.projectId}/arch/nodes/${encodeURIComponent(slug)}`);
      if (!r.ok) throw new Error('node not found');
      this.selectedDetail = await r.json();
    } catch (e) { this.error = e.message; }
  }

  renderNode(n) {
    const open = !!this.expanded[n.Slug];
    const children = this.childrenCache[n.Slug];
    const kind = this.kindByKey(n.Kind);
    return html`
      <li>
        <div class=${`node ${this.selectedSlug === n.Slug ? 'active' : ''}`}>
          <span class="toggle" @click=${(e) => { e.stopPropagation(); this.toggle(n.Slug); }}>
            ${open ? '▾' : '▸'}
          </span>
          <span class="kind-dot" style=${`background: ${kind?.Color || '#999'}`} title=${kind?.Label || n.Kind}></span>
          <span @click=${() => this.select(n.Slug)}>${n.Name}</span>
          <span class="kind-label">${n.Kind}</span>
        </div>
        ${open && children && children.length ? html`
          <ul>${children.map(c => this.renderNode(c))}</ul>
        ` : null}
        ${open && children && !children.length ? html`
          <ul><li class="muted" style="padding-left:20px;font-size:12px">no children</li></ul>
        ` : null}
      </li>
    `;
  }

  renderSidebar() {
    if (this.rootNodes === null) return html`<div class="sidebar">Loading…</div>`;
    return html`
      <div class="sidebar">
        <div style="display:flex;gap:8px;margin-bottom:8px;align-items:center">
          <strong>Architecture</strong>
          <span class="muted" style="margin-left:auto;font-size:11px">read only</span>
        </div>
        ${this.rootNodes.length === 0 ? html`
          <p class="muted">No architecture defined yet. Ask an agent to start with <code>nottario.arch.upsert_node</code>.</p>
        ` : html`
          <ul class="tree">
            ${this.rootNodes.map(n => this.renderNode(n))}
          </ul>
        `}
      </div>
    `;
  }

  renderReader() {
    if (!this.selectedDetail) {
      return html`<div class="reader empty">Select a node on the left.</div>`;
    }
    const { node, children, edges, links } = this.selectedDetail;
    const kind = this.kindByKey(node.Kind);
    const incoming = edges.filter(e => e.ToSlug === node.Slug);
    const outgoing = edges.filter(e => e.FromSlug === node.Slug);
    return html`
      <div class="reader">
        <header>
          <h3>${node.Name}</h3>
          <span class="kind-pill" style=${`background: ${kind?.Color || '#eaeef2'}1a; color: ${kind?.Color || '#1f2328'}`}>${kind?.Label || node.Kind}</span>
          <div style="flex:1"></div>
          <span class="meta"><code>${node.Slug}</code></span>
        </header>
        <div class="meta-grid">
          ${node.LinkedRepo ? html`<div>Repo</div><div><code>${node.LinkedRepo}</code></div>` : null}
          ${node.LinkedPath ? html`<div>Path</div><div><code>${node.LinkedPath}</code></div>` : null}
          ${Object.keys(node.Metadata || {}).length ? html`
            <div>Metadata</div>
            <div><code>${JSON.stringify(node.Metadata)}</code></div>
          ` : null}
        </div>
        ${node.DescriptionMD ? html`
          <div class="section">
            <h4>Description</h4>
            <pre class="body">${node.DescriptionMD}</pre>
          </div>
        ` : null}
        ${children && children.length ? html`
          <div class="section">
            <h4>Children (${children.length})</h4>
            ${children.map(c => html`
              <div class="edge-line" @click=${() => this.select(c.Slug)}>
                <span class="kind-dot" style=${`background: ${this.kindByKey(c.Kind)?.Color || '#999'}`}></span>
                ${c.Name} <span class="kind-label">${c.Kind}</span>
              </div>
            `)}
          </div>
        ` : null}
        ${outgoing.length ? html`
          <div class="section">
            <h4>Outgoing edges (${outgoing.length})</h4>
            ${outgoing.map(e => html`
              <div class="edge-line">
                <strong>${e.Kind}</strong>
                <span class="arrow">→</span>
                <span @click=${() => this.select(e.ToSlug)} style="color:#0969da;cursor:pointer">${e.ToName}</span>
                ${e.Label ? html` <span class="muted">— ${e.Label}</span>` : null}
              </div>
            `)}
          </div>
        ` : null}
        ${incoming.length ? html`
          <div class="section">
            <h4>Incoming edges (${incoming.length})</h4>
            ${incoming.map(e => html`
              <div class="edge-line">
                <span @click=${() => this.select(e.FromSlug)} style="color:#0969da;cursor:pointer">${e.FromName}</span>
                <span class="arrow">→</span>
                <strong>${e.Kind}</strong>
                ${e.Label ? html` <span class="muted">— ${e.Label}</span>` : null}
              </div>
            `)}
          </div>
        ` : null}
        ${links && links.length ? html`
          <div class="section">
            <h4>Linked items (${links.length})</h4>
            ${links.map(l => html`
              <div class="edge-line">
                <strong>${l.LinkType}</strong>: <code>${l.TargetID}</code>
              </div>
            `)}
          </div>
        ` : null}
      </div>
    `;
  }

  render() {
    if (!this.project) return html`<p>Loading…</p>`;
    return html`
      <nottario-page-header title="Architecture">
        <nottario-segmented-control slot="switcher"
          .options=${[
            { value: 'diagram', label: 'Diagram' },
            { value: 'tree',    label: 'Tree'    },
          ]}
          .value=${this.view === 'tree' ? 'tree' : 'diagram'}
          @change=${(e) => window.nottarioNavigate(
            `/projects/${this.projectId}/arch/${e.detail.value}`)}>
        </nottario-segmented-control>
      </nottario-page-header>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      ${this.view === 'diagram'
        ? html`<nottario-arch-graph .projectId=${this.projectId}></nottario-arch-graph>`
        : html`<div class="layout">${this.renderSidebar()}${this.renderReader()}</div>`}
    `;
  }
}

customElements.define('nottario-arch-page', NottarioArchPage);
