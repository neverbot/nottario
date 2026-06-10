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
    selectedIndex: { state: true },
    error: { state: true },
  };

  static styles = css`
    /* Project rule: every Lit shadow root explicitly sets
       box-sizing on host AND descendants because global resets do
       not penetrate. */
    *, *::before, *::after { box-sizing: border-box; }
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
    /* Discoverable shortcut: a tiny kbd hint floats inside the input
       when it is unfocused and empty, telling the user how to reach
       this search from anywhere via the global "/" handler. Hidden
       on focus (the input's own caret takes over) and when there is
       any query content (the hint would overlap typed text). */
    .kbd-hint {
      position: absolute;
      right: 9px;
      top: 50%;
      transform: translateY(-50%);
      padding: 1px 5px;
      background: rgba(255, 255, 255, 0.15);
      border: 1px solid rgba(255, 255, 255, 0.3);
      border-radius: 4px;
      font-size: 10px;
      color: rgba(255, 255, 255, 0.7);
      font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, monospace;
      line-height: 1;
      pointer-events: none;
    }
    :host(:focus-within) .kbd-hint { display: none; }
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
    /* Result group header — replaces the per-row pill as the
       primary signal of "what kind of thing am I looking at". The
       pill is still rendered inside each hit row as a secondary
       label, useful when a search returns one kind only and the
       header could otherwise be missed. */
    .group-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 6px 12px;
      background: #f6f8fa;
      border-bottom: 1px solid #eaeef2;
      font-size: 11px;
      font-weight: 600;
      color: #59636e;
      text-transform: uppercase;
      letter-spacing: 0.04em;
    }
    .group-header .count {
      font-weight: 500;
      color: #8b949e;
      text-transform: none;
      letter-spacing: 0;
    }
    .group + .group .group-header { border-top: 1px solid #eaeef2; }
    .hit {
      padding: 8px 12px;
      border-bottom: 1px solid #eaeef2;
      cursor: pointer;
      font-size: 13px;
    }
    .group .hit:last-child { border-bottom: none; }
    .hit:hover { background: #f6f8fa; }
    .hit.selected,
    .hit.selected:hover { background: #ddf4ff; }
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
    /* document pill: GitHub's attention-subtle (orange) pair —
       earlier yellow #fff8c5 collided with the <mark> highlight that
       uses the same hue, so document rows stuttered visually. */
    .kind-pill.document { background: #fff1e5; color: #9a6700; }
    .kind-pill.arch_node { background: #f3eaff; color: #6f42c1; }
    .empty, .hint, .error {
      padding: 12px;
      color: #59636e;
      font-size: 12px;
      text-align: center;
    }
    .empty strong { color: #1f2328; }
    .empty .hint-sub {
      display: block;
      margin-top: 4px;
      font-size: 11px;
      color: #8b949e;
    }
    .error {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12px;
      padding: 8px 12px;
      background: #ffebe9;
      color: #82071e;
      border-bottom: 1px solid #ffc1ba;
      text-align: left;
    }
    .retry-btn {
      flex: 0 0 auto;
      background: #fff;
      border: 1px solid #ffc1ba;
      color: #82071e;
      font-size: 11px;
      font-weight: 500;
      padding: 2px 8px;
      border-radius: 4px;
      cursor: pointer;
      font-family: inherit;
    }
    .retry-btn:hover { background: #ffebe9; }
  `;

  constructor() {
    super();
    this.query = '';
    this.hits = null;
    this.loading = false;
    this.open = false;
    this.selectedIndex = -1;
    this.error = null;
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

  // Arrow / Enter / Escape navigation inside the dropdown. Kept on
  // the input itself rather than the panel so focus never has to
  // leave the textbox to drive a selection.
  onKeyDown(e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      this.open = false;
      e.target.blur();
      return;
    }
    if (!this.hits || this.hits.length === 0) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      this.selectedIndex = Math.min(this.hits.length - 1, this.selectedIndex + 1);
      this._scrollSelectedIntoView();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      this.selectedIndex = Math.max(0, this.selectedIndex - 1);
      this._scrollSelectedIntoView();
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (this.selectedIndex >= 0) this.goto(this.hits[this.selectedIndex]);
    }
  }

  _scrollSelectedIntoView() {
    this.updateComplete.then(() => {
      const sel = this.shadowRoot?.querySelector('.hit.selected');
      sel?.scrollIntoView({ block: 'nearest' });
    });
  }

  async runSearch() {
    if (!this.projectId || !this.query.trim()) {
      this.hits = null;
      this.error = null;
      this.selectedIndex = -1;
      return;
    }
    this.loading = true;
    this.error = null;
    try {
      const r = await fetch(
        `/api/search?project_id=${encodeURIComponent(this.projectId)}&q=${encodeURIComponent(this.query)}`,
      );
      if (!r.ok) throw new Error('search failed');
      this.hits = (await r.json()).hits || [];
      // Auto-select first hit so Enter immediately opens the top
      // match (Raycast / Linear pattern). Reset to -1 when there's
      // nothing to point at.
      this.selectedIndex = this.hits.length > 0 ? 0 : -1;
    } catch (_) {
      // Surface the failure: leave previous hits alone (or null if
      // first attempt) and set error. Without this branch a 500 / a
      // revoked token / a network drop all render as "No matches.",
      // a UX-vs-trust collision the user has no way to disambiguate.
      this.error = 'Search failed.';
    } finally {
      this.loading = false;
    }
  }

  onFocus() {
    if (this.query) this.open = true;
  }
  onBlur() {
    setTimeout(() => {
      this.open = false;
    }, 150);
  }

  // After the hits change, paint the highlighted snippets into the
  // matching elements. The server already escaped everything except
  // the `<mark>` tags it added, so writing innerHTML here is safe.
  // We do this in `updated` instead of using Lit's `unsafeHTML`
  // directive because that directive isn't part of the vendored Lit
  // bundle (see internal/web/static/components/markdown.js).
  // The guard on `changedProperties` skips the work when only state
  // unrelated to hits flipped (loading, selectedIndex, etc.).
  updated(changedProperties) {
    if (!changedProperties.has('hits')) return;
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
    this.selectedIndex = -1;
    this.error = null;
  }

  // Partitions the flat hit list into ordered groups while preserving
  // each hit's original index so arrow-key selection and the
  // `updated()` innerHTML painter both keep operating on the flat
  // `this.hits` array.
  _groupedHits() {
    if (!this.hits || !this.hits.length) return [];
    const buckets = { task: [], document: [], arch_node: [] };
    this.hits.forEach((hit, flatIndex) => {
      if (buckets[hit.kind]) buckets[hit.kind].push({ hit, flatIndex });
    });
    const labels = { task: 'Tasks', document: 'Documents', arch_node: 'Architecture' };
    return ['task', 'document', 'arch_node']
      .filter((k) => buckets[k].length)
      .map((k) => ({ kind: k, label: labels[k], items: buckets[k] }));
  }

  render() {
    const placeholder = this.projectId ? 'Search this project…' : 'Open a project to search';
    return html`
      <input type="search"
             role="combobox"
             aria-label=${placeholder}
             aria-autocomplete="list"
             aria-expanded=${this.open && !!this.query}
             aria-controls="search-listbox"
             aria-activedescendant=${this.selectedIndex >= 0 ? `hit-${this.selectedIndex}` : ''}
             placeholder=${placeholder}
             .value=${this.query}
             ?disabled=${!this.projectId}
             @input=${this.onInput}
             @keydown=${this.onKeyDown}
             @focus=${this.onFocus}
             @blur=${this.onBlur}>
      ${this.projectId && !this.query ? html`<kbd class="kbd-hint">/</kbd>` : null}
      ${
        this.open && this.query
          ? html`
        <div class="panel" id="search-listbox" role="listbox">
          ${
            this.error
              ? html`
                <div class="error" role="alert">
                  <span>${this.error}</span>
                  <button type="button" class="retry-btn"
                          @mousedown=${(e) => e.preventDefault()}
                          @click=${() => this.runSearch()}>Retry</button>
                </div>`
              : null
          }
          ${
            this.loading && this.hits === null && !this.error
              ? html`<div class="hint">Searching…</div>`
              : null
          }
          ${
            this.hits && this.hits.length === 0 && !this.error
              ? html`<div class="empty">
                No matches for <strong>${this.query}</strong> in this project.
                <span class="hint-sub">
                  Try a shorter term, or check spelling. Search is scoped to the current project.
                </span>
              </div>`
              : null
          }
          ${this._groupedHits().map(
            (group) => html`
            <section class="group">
              <div class="group-header">
                <span>${group.label}</span>
                <span class="count">${group.items.length}</span>
              </div>
              ${group.items.map(({ hit, flatIndex }) => {
                const sel = flatIndex === this.selectedIndex;
                return html`
                  <div class=${`hit${sel ? ' selected' : ''}`}
                       id=${`hit-${flatIndex}`}
                       role="option"
                       aria-selected=${sel ? 'true' : 'false'}
                       @mouseenter=${() => {
                         this.selectedIndex = flatIndex;
                       }}
                       @mousedown=${(e) => {
                         e.preventDefault();
                         this.goto(hit);
                       }}>
                    <div>
                      <span class=${`kind-pill ${hit.kind}`}>${hit.kind.replace('_', ' ')}</span>
                      <span class="title" data-html-idx=${`t${flatIndex}`}>${hit.title || '(untitled)'}</span>
                    </div>
                    ${
                      hit.description || hit.description_html
                        ? html`<div class="desc" data-html-idx=${`d${flatIndex}`}>${hit.description || ''}</div>`
                        : null
                    }
                    <div class="meta">
                      ${hit.kind === 'task' ? html`${hit.task_state} · ${hit.task_type}` : null}
                      ${hit.kind === 'document' ? html`${hit.doc_path}` : null}
                      ${hit.kind === 'arch_node' ? html`${hit.node_kind} · ${hit.node_slug}` : null}
                    </div>
                  </div>
                `;
              })}
            </section>
          `,
          )}
        </div>
      `
          : null
      }
    `;
  }
}

customElements.define('nottario-search-box', NottarioSearchBox);
