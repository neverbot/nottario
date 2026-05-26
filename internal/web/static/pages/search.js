import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-search-box> is a topbar input + dropdown of results.
// It needs a projectId to scope the search; when null it shows a
// disabled state telling the user to open a project first.

class NottarioSearchBox extends LitElement {
  static properties = {
    projectId: { type: String, attribute: 'project-id' },
    query: { state: true },
    hits: { state: true },
    loading: { state: true },
    open: { state: true },
  };

  static styles = css`
    :host {
      display: block;
      position: relative;
      /* The host belongs in the right cluster of the topbar. It does
         NOT grow horizontally — the spacer to the left does. It is
         the first element that shrinks when the topbar runs out of
         room (the user cluster and sign-out are flex:0 0 auto). */
      flex: 0 1 320px;
      min-width: 140px;
      max-width: 320px;
    }
    input {
      box-sizing: border-box;
      width: 100%;
      padding: 5px 10px;
      border: 1px solid #afb8c1;
      border-radius: 6px;
      background: rgba(255, 255, 255, 0.08);
      color: #fff;
      font-size: 13px;
      font-family: inherit;
    }
    input::placeholder { color: rgba(255,255,255,0.6); }
    input:focus-visible {
      outline: 2px solid #0969da;
      outline-offset: 1px;
      background: #fff;
      color: #1f2328;
      border-color: #0969da;
    }
    input:focus::placeholder { color: #59636e; }
    .panel {
      position: absolute;
      top: calc(100% + 4px);
      left: 0;
      right: 0;
      background: #fff;
      color: #1f2328;
      border: 1px solid #d1d9e0;
      border-radius: 6px;
      box-shadow: 0 6px 16px rgba(0,0,0,0.18);
      max-height: 60vh;
      overflow: auto;
      z-index: 20;
    }
    .hit {
      padding: 8px 12px;
      border-bottom: 1px solid #eaeef2;
      cursor: pointer;
      font-size: 13px;
    }
    .hit:last-child { border-bottom: none; }
    .hit:hover { background: #f6f8fa; }
    .hit .title { font-weight: 500; }
    .hit .desc {
      color: #59636e;
      font-size: 12px;
      margin-top: 2px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    /* ts_headline yields snippets with <mark> around matches. The
       server escapes everything else so the only HTML here is the
       <mark> tag itself. */
    .hit mark {
      background: #fff8c5;
      color: inherit;
      padding: 0 1px;
      border-radius: 2px;
      font-weight: 600;
    }
    .hit .meta {
      font-size: 11px;
      color: #59636e;
      margin-top: 2px;
    }
    .kind-pill {
      display: inline-block;
      padding: 0 6px;
      border-radius: 2em;
      font-size: 10px;
      text-transform: uppercase;
      background: #eaeef2;
      color: #1f2328;
      margin-right: 6px;
      font-weight: 500;
    }
    .kind-pill.task { background: #ddf4ff; color: #0969da; }
    .kind-pill.document { background: #fff8c5; color: #7d4e00; }
    .kind-pill.arch_node { background: #f3eaff; color: #6f42c1; }
    .empty, .hint {
      padding: 12px;
      color: #59636e;
      font-size: 12px;
      text-align: center;
    }
  `;

  constructor() {
    super();
    this.query = '';
    this.hits = null;
    this.loading = false;
    this.open = false;
    this._timer = null;
  }

  connectedCallback() {
    super.connectedCallback();
    // Global `/` shortcut focuses this search input. Skips while
    // the user is already typing into a text field.
    this._slashHandler = (e) => {
      if (e.key !== '/') return;
      const t = e.target;
      const tag = t?.tagName?.toLowerCase();
      if (tag === 'input' || tag === 'textarea' || t?.isContentEditable) return;
      e.preventDefault();
      this.shadowRoot?.querySelector('input')?.focus();
    };
    window.addEventListener('keydown', this._slashHandler);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    if (this._slashHandler) window.removeEventListener('keydown', this._slashHandler);
  }

  onInput(e) {
    this.query = e.target.value;
    this.open = true;
    clearTimeout(this._timer);
    this._timer = setTimeout(() => this.runSearch(), 200);
  }

  async runSearch() {
    if (!this.projectId || !this.query.trim()) {
      this.hits = null;
      return;
    }
    this.loading = true;
    try {
      const r = await fetch(
        `/api/search?project_id=${encodeURIComponent(this.projectId)}&q=${encodeURIComponent(this.query)}`
      );
      if (!r.ok) throw new Error('search failed');
      this.hits = (await r.json()).hits || [];
    } catch (_) {
      this.hits = [];
    } finally {
      this.loading = false;
    }
  }

  onFocus() { if (this.query) this.open = true; }
  onBlur() { setTimeout(() => { this.open = false; }, 150); }

  // After every render, paint the highlighted snippets into the
  // matching elements. The server already escaped everything except
  // the `<mark>` tags it added, so writing innerHTML here is safe.
  // We do this in `updated` instead of using Lit's `unsafeHTML`
  // directive because that directive isn't part of the vendored Lit
  // bundle (see internal/web/static/components/markdown.js).
  updated() {
    if (!this.hits || !this.hits.length) return;
    const root = this.shadowRoot;
    if (!root) return;
    this.hits.forEach((h, i) => {
      if (h.title_html) {
        const el = root.querySelector(`[data-html-idx="t${i}"]`);
        if (el) el.innerHTML = h.title_html;
      }
      if (h.description_html) {
        const el = root.querySelector(`[data-html-idx="d${i}"]`);
        if (el) el.innerHTML = h.description_html;
      }
    });
  }

  goto(hit) {
    let path = null;
    switch (hit.kind) {
      case 'task':
        path = `/projects/${this.projectId}/board#task=${hit.task_id}`;
        break;
      case 'document':
        path = `/projects/${this.projectId}/docs#path=${encodeURIComponent(hit.doc_path)}`;
        break;
      case 'arch_node':
        path = `/projects/${this.projectId}/arch#node=${encodeURIComponent(hit.node_slug)}`;
        break;
    }
    if (path) window.nottarioNavigate(path);
    this.query = '';
    this.hits = null;
    this.open = false;
  }

  render() {
    const placeholder = this.projectId
      ? 'Search this project…'
      : 'Open a project to search';
    return html`
      <input type="search"
             role="searchbox"
             aria-label=${placeholder}
             placeholder=${placeholder}
             .value=${this.query}
             ?disabled=${!this.projectId}
             @input=${this.onInput}
             @focus=${this.onFocus}
             @blur=${this.onBlur}>
      ${this.open && this.query ? html`
        <div class="panel">
          ${this.loading && this.hits === null ? html`<div class="hint">Searching…</div>` : null}
          ${this.hits && this.hits.length === 0 ? html`<div class="empty">No matches.</div>` : null}
          ${this.hits ? this.hits.map((h, i) => html`
            <div class="hit" @mousedown=${(e) => { e.preventDefault(); this.goto(h); }}>
              <div>
                <span class=${`kind-pill ${h.kind}`}>${h.kind.replace('_', ' ')}</span>
                <span class="title" data-html-idx=${`t${i}`}>${h.title || '(untitled)'}</span>
              </div>
              ${h.description || h.description_html
                ? html`<div class="desc" data-html-idx=${`d${i}`}>${h.description || ''}</div>`
                : null}
              <div class="meta">
                ${h.kind === 'task' ? html`state: ${h.task_state} · type: ${h.task_type}` : null}
                ${h.kind === 'document' ? html`path: ${h.doc_path}` : null}
                ${h.kind === 'arch_node' ? html`slug: ${h.node_slug} · kind: ${h.node_kind}` : null}
              </div>
            </div>
          `) : null}
        </div>
      ` : null}
    `;
  }
}

customElements.define('nottario-search-box', NottarioSearchBox);
