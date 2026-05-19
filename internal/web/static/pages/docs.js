import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';

class NottarioDocsPage extends LitElement {
  static properties = {
    me: { type: Object },
    projectId: { type: String },
    project: { state: true },
    summaries: { state: true },
    selected: { state: true },        // currently open document (full body)
    editing: { state: true },          // true if in edit mode
    draft: { state: true },            // textarea content while editing
    creating: { state: true },         // true if the user is creating a new doc
    newPath: { state: true },
    error: { state: true },
    info: { state: true },
    search: { state: true },           // current search query
    hits: { state: true },
  };

  static styles = css`
    :host { display: block; }
    .layout {
      display: grid;
      grid-template-columns: 280px 1fr;
      gap: 16px;
      min-height: 70vh;
    }
    .sidebar {
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      padding: 12px;
      overflow: auto;
      max-height: 80vh;
    }
    .sidebar h3 {
      font-size: 12px;
      text-transform: uppercase;
      color: #59636e;
      margin: 8px 0 4px 0;
    }
    .sidebar .actions {
      display: flex;
      gap: 6px;
      margin-bottom: 8px;
    }
    .sidebar input.search {
      width: 100%;
      margin-bottom: 8px;
    }
    .tree {
      list-style: none;
      padding-left: 0;
      margin: 0;
    }
    .tree li {
      padding: 2px 4px;
      cursor: pointer;
      border-radius: 4px;
      font-size: 13px;
    }
    .tree li:hover { background: #f6f8fa; }
    .tree li.active { background: #ddf4ff; color: #0969da; }
    .tree .kind {
      font-size: 10px;
      color: #59636e;
      text-transform: uppercase;
      margin-left: 4px;
    }
    .reader {
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      padding: 16px;
      overflow: auto;
      max-height: 80vh;
    }
    .reader header {
      display: flex;
      align-items: baseline;
      gap: 8px;
      border-bottom: 1px solid #eaeef2;
      padding-bottom: 12px;
      margin-bottom: 12px;
    }
    .reader header h2 { margin: 0; font-size: 18px; }
    .reader header .meta { margin-left: auto; color: #59636e; font-size: 12px; }
    .reader header .spacer { flex: 1; }
    .reader pre {
      white-space: pre-wrap;
      word-break: break-word;
      background: #f6f8fa;
      padding: 12px;
      border-radius: 6px;
      font-size: 12px;
    }
    .reader textarea {
      width: 100%;
      min-height: 50vh;
      font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
      font-size: 13px;
      line-height: 1.5;
    }
    .field { margin-top: 8px; }
    .field label { font-weight: 500; font-size: 12px; display: block; margin-bottom: 4px; }
    .empty { color: #59636e; padding: 40px; text-align: center; }
    .actions-row { display: flex; gap: 8px; margin-top: 12px; }
    .error { color: #cf222e; background: #ffebe9; padding: 8px 12px; border-radius: 6px; margin-bottom: 8px; }
    .info  { color: #1f883d; background: #ddf4d1; padding: 8px 12px; border-radius: 6px; margin-bottom: 8px; }
    .badge {
      display: inline-block;
      padding: 1px 7px;
      border-radius: 2em;
      font-size: 11px;
      border: 1px solid #d1d9e0;
    }
    .badge.skill   { background: #ddf4ff; color: #0969da; border-color: #8ec0ff; }
    .badge.context { background: #f6f8fa; color: #1f2328; }
    .badge.note    { background: #fff8c5; color: #7d4e00; border-color: #d4a72c; }
    .group { margin-bottom: 8px; }
    .group-title { font-size: 11px; color: #57606a; padding-left: 4px; }
  `;

  constructor() {
    super();
    this.project = null;
    this.summaries = null;
    this.selected = null;
    this.editing = false;
    this.draft = '';
    this.creating = false;
    this.newPath = '';
    this.error = '';
    this.info = '';
    this.search = '';
    this.hits = null;
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
    const path = h.get('path');
    if (path) this.open(path);
  }

  _subscribe() {
    this._unsub?.();
    if (!this.projectId) return;
    this._unsub = subscribe(this.projectId, (ev) => {
      if (!ev.type?.startsWith('doc.')) return;
      this.load();
      // Refresh the open document if it was the one that changed.
      if (this.selected && ev.path === this.selected.Path) {
        this.open(this.selected.Path);
      }
    });
  }

  async load() {
    if (!this.projectId) return;
    try {
      const [pr, lr] = await Promise.all([
        fetch(`/api/projects/${this.projectId}`),
        fetch(`/api/docs?scope=project&project_id=${this.projectId}`),
      ]);
      if (!pr.ok) throw new Error('project not found');
      this.project = await pr.json();
      this.summaries = (await lr.json()).documents || [];
    } catch (e) {
      this.error = e.message;
    }
  }

  async open(path) {
    this.error = '';
    this.info = '';
    this.editing = false;
    try {
      const r = await fetch(
        `/api/docs/read?scope=project&project_id=${this.projectId}&path=${encodeURIComponent(path)}`
      );
      if (!r.ok) throw new Error('read failed');
      this.selected = await r.json();
    } catch (e) {
      this.error = e.message;
    }
  }

  startCreate() {
    this.creating = true;
    this.newPath = '';
    this.draft = '---\ntitle: New document\nkind: context\n---\n\n# New document\n\nBody...';
    this.error = '';
    this.info = '';
  }

  cancelCreate() {
    this.creating = false;
    this.draft = '';
    this.newPath = '';
  }

  async saveNew() {
    const path = this.newPath.trim();
    if (!path) { this.error = 'path required'; return; }
    try {
      const r = await fetch('/api/docs/write', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          scope: 'project',
          project_id: this.projectId,
          path,
          content_md: this.draft,
          expected_version: 0,
        }),
      });
      if (!r.ok) throw new Error((await r.json()).error || 'failed');
      const doc = await r.json();
      this.creating = false;
      this.info = `Created ${doc.Path}`;
      await this.load();
      await this.open(doc.Path);
    } catch (e) { this.error = e.message; }
  }

  startEdit() {
    if (!this.selected) return;
    this.editing = true;
    // Reconstruct frontmatter + body for editing.
    const fm = this.selected.Frontmatter || {};
    let draft = '';
    if (Object.keys(fm).length) {
      const lines = ['---'];
      for (const [k, v] of Object.entries(fm)) {
        if (Array.isArray(v)) {
          lines.push(`${k}: [${v.map(x => JSON.stringify(x)).join(', ')}]`);
        } else if (typeof v === 'string') {
          lines.push(`${k}: ${v}`);
        } else {
          lines.push(`${k}: ${JSON.stringify(v)}`);
        }
      }
      lines.push('---');
      draft = lines.join('\n') + '\n\n' + (this.selected.ContentMD || '');
    } else {
      draft = this.selected.ContentMD || '';
    }
    this.draft = draft;
  }

  async saveEdit() {
    try {
      const r = await fetch('/api/docs/write', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          scope: 'project',
          project_id: this.projectId,
          path: this.selected.Path,
          content_md: this.draft,
          expected_version: this.selected.CurrentVersion,
        }),
      });
      if (r.status === 409) {
        throw new Error('Someone else edited this document. Refresh and retry.');
      }
      if (!r.ok) throw new Error((await r.json()).error || 'failed');
      const doc = await r.json();
      this.info = `Saved (version ${doc.CurrentVersion}).`;
      this.editing = false;
      this.selected = doc;
      await this.load();
    } catch (e) { this.error = e.message; }
  }

  async del() {
    if (!confirm(`Delete ${this.selected.Path}?`)) return;
    try {
      const r = await fetch('/api/docs/delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          scope: 'project',
          project_id: this.projectId,
          path: this.selected.Path,
          message: 'deleted via web ui',
        }),
      });
      if (!r.ok) throw new Error('delete failed');
      this.info = `Deleted ${this.selected.Path}`;
      this.selected = null;
      await this.load();
    } catch (e) { this.error = e.message; }
  }

  async runSearch() {
    if (!this.search.trim()) {
      this.hits = null;
      return;
    }
    try {
      const r = await fetch(
        `/api/docs/search?scope=project&project_id=${this.projectId}&q=${encodeURIComponent(this.search)}`
      );
      if (!r.ok) throw new Error((await r.json()).error || 'failed');
      this.hits = (await r.json()).hits || [];
    } catch (e) {
      this.error = e.message;
      this.hits = [];
    }
  }

  back() { window.nottarioNavigate('/'); }

  groupByKind() {
    const out = { skill: [], context: [], note: [] };
    for (const s of this.summaries || []) {
      out[s.Kind]?.push(s) ?? out.context.push(s);
    }
    return out;
  }

  render() {
    if (!this.project) return html`<p>Loading…</p>`;
    return html`
      <div style="display:flex; align-items:baseline; gap:16px; margin-bottom:16px;">
        <button @click=${() => this.back()}>← Back</button>
        <h2 style="margin:0">${this.project.Name}</h2>
        <span class="muted">docs</span>
      </div>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      ${this.info ? html`<div class="info">${this.info}</div>` : null}
      <div class="layout">
        ${this.renderSidebar()}
        ${this.renderReader()}
      </div>
    `;
  }

  renderSidebar() {
    if (this.summaries === null) return html`<div class="sidebar">Loading…</div>`;
    const groups = this.groupByKind();
    return html`
      <div class="sidebar">
        <div class="actions">
          <button class="primary" style="flex:1" @click=${() => this.startCreate()}>+ New doc</button>
        </div>
        <input class="search" placeholder="Search…" .value=${this.search}
          @input=${(e) => { this.search = e.target.value; }}
          @keydown=${(e) => { if (e.key === 'Enter') this.runSearch(); }}>
        ${this.hits !== null ? this.renderHits() : html`
          ${['skill', 'context', 'note'].map(kind => {
            const items = groups[kind];
            if (!items || !items.length) return null;
            return html`
              <div class="group">
                <div class="group-title">${kind.toUpperCase()}</div>
                <ul class="tree">
                  ${items.map(s => html`
                    <li class=${this.selected?.Path === s.Path ? 'active' : ''}
                        @click=${() => this.open(s.Path)}>
                      ${s.Title || s.Path}
                    </li>
                  `)}
                </ul>
              </div>
            `;
          })}
          ${!this.summaries.length ? html`<p class="muted">No documents yet.</p>` : null}
        `}
      </div>
    `;
  }

  renderHits() {
    if (!this.hits.length) return html`<p class="muted">No matches.</p>`;
    return html`
      <ul class="tree">
        ${this.hits.map(h => html`
          <li @click=${() => this.open(h.Path)}>
            ${h.Title || h.Path}
            <span class="kind">${h.Kind}</span>
          </li>
        `)}
      </ul>
      <button style="margin-top:8px" @click=${() => { this.hits = null; this.search = ''; }}>Clear search</button>
    `;
  }

  renderReader() {
    if (this.creating) return this.renderCreateForm();
    if (!this.selected) return html`<div class="reader empty">Pick a document on the left, or create a new one.</div>`;
    if (this.editing) return this.renderEditor();
    return this.renderReadView();
  }

  renderCreateForm() {
    return html`
      <div class="reader">
        <header>
          <h2>New document</h2>
          <div class="spacer"></div>
          <button @click=${() => this.cancelCreate()}>Cancel</button>
          <button class="primary" @click=${() => this.saveNew()}>Create</button>
        </header>
        <div class="field">
          <label>Path (e.g. <code>projects/${this.projectId}/context/glossary.md</code>)</label>
          <input .value=${this.newPath} @input=${(e) => { this.newPath = e.target.value; }}
            placeholder="projects/${this.projectId}/context/your-doc.md">
        </div>
        <div class="field">
          <label>Markdown (with optional frontmatter)</label>
          <textarea .value=${this.draft} @input=${(e) => { this.draft = e.target.value; }}></textarea>
        </div>
      </div>
    `;
  }

  renderReadView() {
    const s = this.selected;
    return html`
      <div class="reader">
        <header>
          <h2>${s.Title || s.Path}</h2>
          <span class=${`badge ${s.Kind}`}>${s.Kind}</span>
          <div class="spacer"></div>
          <span class="meta">v${s.CurrentVersion}</span>
          <button @click=${() => this.startEdit()}>Edit</button>
          <button class="danger" @click=${() => this.del()}>Delete</button>
        </header>
        ${s.Description ? html`<p class="muted">${s.Description}</p>` : null}
        <div style="font-size:11px;color:#59636e;margin-bottom:8px">${s.Path}</div>
        <pre>${s.ContentMD || ''}</pre>
      </div>
    `;
  }

  renderEditor() {
    return html`
      <div class="reader">
        <header>
          <h2>${this.selected.Path}</h2>
          <div class="spacer"></div>
          <button @click=${() => { this.editing = false; }}>Cancel</button>
          <button class="primary" @click=${() => this.saveEdit()}>Save</button>
        </header>
        <textarea .value=${this.draft} @input=${(e) => { this.draft = e.target.value; }}></textarea>
      </div>
    `;
  }
}

customElements.define('nottario-docs-page', NottarioDocsPage);
