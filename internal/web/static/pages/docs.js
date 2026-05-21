import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';
import { buttonStyles } from '/static/components/buttons.js';
import { fieldStyles } from '/static/components/fields.js';
import { badgeStyles } from '/static/components/badges.js';
import '/static/components/page-header.js';
import '/static/components/search-input.js';
import '/static/components/markdown.js';

class NottarioDocsPage extends LitElement {
  static properties = {
    me: { type: Object },
    projectId: { type: String },
    project: { state: true },
    summaries: { state: true },
    selected: { state: true },
    editing: { state: true },
    draft: { state: true },
    creating: { state: true },
    newPath: { state: true },
    error: { state: true },
    info: { state: true },
    search: { state: true },
    hits: { state: true },
    historyOpen: { state: true },
    historyVersions: { state: true },
    viewingVersion: { state: true },
  };

  static styles = [buttonStyles, fieldStyles, badgeStyles, css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }

    /* No outer cards. The page is a single split: a navigation rail
       on the left, a reader on the right, separated by one hairline.
       This is the GitHub Files pattern the rest of the app already
       leans on. */
    .layout {
      display: grid;
      grid-template-columns: 260px 1fr;
      gap: 0;
      align-items: start;
      border-top: 1px solid #d1d9e0;
      min-height: calc(100vh - 200px);
    }
    .rail {
      border-right: 1px solid #d1d9e0;
      /* Left padding gives focus outlines and active-row chrome room
         to render without clipping against the page's left edge. */
      padding: 12px 12px 24px 4px;
      max-height: calc(100vh - 200px);
      overflow: auto;
    }
    .reader-col {
      padding: 12px 0 24px 24px;
      min-width: 0;
    }

    .rail-search { margin-bottom: 12px; }
    kbd {
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 10px;
      background: #f6f8fa;
      border: 1px solid #d0d7de;
      border-radius: 3px;
      padding: 1px 4px;
      color: #59636e;
    }

    /* Group eyebrow matches the existing UPPERCASE muted style used
       in profile / settings. No background, no border. */
    .group { margin-bottom: 16px; }
    .group-eyebrow {
      font-size: 10px;
      letter-spacing: 0.06em;
      text-transform: uppercase;
      color: #8b949e;
      font-weight: 600;
      padding: 0 6px 4px;
    }
    .tree {
      list-style: none;
      padding: 0;
      margin: 0;
    }
    /* GitHub-Files row pattern: weight + dark text + no background.
       The active row goes bold and gets a darker text colour so the
       eye finds it; no pill, no fill. Search-dimmed rows fade to
       30% opacity but stay in place. */
    .tree li {
      padding: 4px 6px;
      cursor: pointer;
      border-radius: 4px;
      font-size: 13px;
      color: #1f2328;
      line-height: 1.4;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      transition: opacity 120ms ease-out;
    }
    .tree li:hover { background: #f6f8fa; }
    .tree li.active {
      font-weight: 600;
      color: #0969da;
      background: #f6f8fa;
    }
    .tree li.dim { opacity: 0.3; }
    .tree li.keyboard-cursor {
      box-shadow: inset 2px 0 0 #0969da;
      padding-left: 4px;
    }
    .tree mark {
      background: #fff8c5;
      color: inherit;
      padding: 0 1px;
      border-radius: 2px;
    }

    .rail-footer {
      margin-top: 12px;
      padding: 8px 6px 0;
      border-top: 1px solid #eaeef2;
    }
    .rail-footer .btn {
      width: 100%;
      justify-content: center;
    }

    /* Reader chrome — title strip, then prose. The path lives as a
       mono breadcrumb on its own line below the title; segments are
       muted except the final filename. Actions cluster on the right
       of the title strip. */
    .reader-title {
      display: flex;
      align-items: baseline;
      gap: 10px;
      flex-wrap: wrap;
    }
    .reader-title h2 {
      margin: 0;
      font-size: 22px;
      font-weight: 600;
      letter-spacing: -0.01em;
      color: #1f2328;
    }
    .reader-title .spacer { flex: 1; }
    .reader-title .actions { display: flex; align-items: center; gap: 4px; }
    .reader-title .actions .btn { font-size: 12px; padding: 4px 10px; }

    /* Delete is a quiet trash-icon button at rest, picking up the
       danger colour on hover. Trash icon (not an X) because the
       header already has a "close-style" X in many places elsewhere
       and "X next to Edit/History" reads as "close the view", not
       "delete the document". */
    .actions .delete {
      width: 28px;
      height: 28px;
      padding: 0;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      color: #8b949e;
      background: transparent;
      border: 1px solid transparent;
      border-radius: 6px;
      cursor: pointer;
      font: inherit;
    }
    .actions .delete svg { display: block; }
    .actions .delete:hover,
    .actions .delete:focus-visible {
      color: #cf222e;
      border-color: rgba(207, 34, 46, 0.4);
      background: #ffebe9;
      outline: none;
    }

    .reader-meta {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-top: 6px;
      font-size: 12px;
      color: #59636e;
      font-family: ui-monospace, SFMono-Regular, monospace;
      flex-wrap: wrap;
    }
    .reader-meta .crumb-seg { color: #8b949e; }
    .reader-meta .crumb-seg.last { color: #1f2328; font-weight: 600; }
    .reader-meta .sep { color: #d0d7de; }
    .reader-meta .version-btn {
      background: transparent;
      border: 1px solid transparent;
      color: inherit;
      font: inherit;
      padding: 1px 6px;
      border-radius: 4px;
      cursor: pointer;
    }
    .reader-meta .version-btn:hover {
      color: #1f2328;
      background: #f6f8fa;
      border-color: #d0d7de;
    }
    .reader-meta .version-btn.open {
      color: #1f2328;
      background: #ddf4ff;
      border-color: #0969da;
    }

    /* The prose container now lives inside <nottario-markdown>; the
       reader only owns the description line above it. */
    nottario-markdown { display: block; margin: 24px 0 0; }
    .description {
      max-width: 76ch;
      margin: 24px 0 0;
      color: #59636e;
      font-style: italic;
      font-size: 14px;
    }

    /* Reading a historical version: thin amber strip at the top of
       the reader signals read-only context, with a return-to-current
       link. */
    .version-banner {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-top: 12px;
      padding: 6px 10px;
      background: #fff8c5;
      border: 1px solid rgba(212, 167, 44, 0.5);
      border-radius: 6px;
      font-size: 12px;
      color: #7d4e00;
    }
    .version-banner .spacer { flex: 1; }
    .version-banner button {
      background: transparent;
      border: 1px solid rgba(125, 78, 0, 0.4);
      color: inherit;
      font: inherit;
      font-size: 12px;
      padding: 2px 8px;
      border-radius: 4px;
      cursor: pointer;
    }
    .version-banner button:hover { background: #fff3a8; }

    /* History popover anchored to the version button. Slim list:
       each row shows version, relative time, author hint, and the
       commit-style message. Newest first. Click a row → load that
       version into the reader read-only. */
    .history-pop {
      position: absolute;
      top: calc(100% + 6px);
      right: 0;
      width: 360px;
      max-height: 420px;
      overflow: auto;
      background: #ffffff;
      border: 1px solid #d0d7de;
      border-radius: 8px;
      box-shadow: 0 8px 24px rgba(31, 35, 40, 0.12);
      z-index: 50;
      padding: 6px;
    }
    .history-pop .empty { padding: 12px; color: #59636e; font-size: 13px; }
    .history-pop ul { list-style: none; margin: 0; padding: 0; }
    .history-pop li {
      padding: 8px 10px;
      border-radius: 6px;
      cursor: pointer;
      display: grid;
      grid-template-columns: 36px 1fr auto;
      gap: 8px;
      align-items: baseline;
    }
    .history-pop li:hover { background: #f6f8fa; }
    .history-pop li.current { background: #ddf4ff; }
    .history-pop .vn {
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 12px;
      color: #0969da;
      font-weight: 600;
    }
    .history-pop .msg {
      font-size: 13px;
      color: #1f2328;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .history-pop .msg.empty-msg { color: #8b949e; font-style: italic; }
    .history-pop .when {
      font-size: 11px;
      color: #8b949e;
      white-space: nowrap;
    }

    .pop-anchor { position: relative; display: inline-block; }

    /* Empty state teaches the interface instead of just naming the
       gap. Three short examples (skill / context / note) tell the
       reader what kinds of docs live here and what they're for. */
    .empty-pane {
      max-width: 52ch;
      margin: 64px 0 0;
      color: #1f2328;
    }
    .empty-pane h3 {
      margin: 0 0 8px;
      font-size: 18px;
      font-weight: 600;
    }
    .empty-pane p {
      margin: 0 0 16px;
      color: #59636e;
      font-size: 14px;
      line-height: 1.55;
    }
    .empty-pane dl {
      display: grid;
      grid-template-columns: 80px 1fr;
      gap: 6px 16px;
      margin: 0 0 20px;
      font-size: 13px;
    }
    .empty-pane dt { color: #1f2328; font-weight: 600; }
    .empty-pane dd { margin: 0; color: #59636e; }
    .empty-pane .cta { display: flex; gap: 8px; }

    /* Create + edit panes share the prose width. The editor
       textarea is mono, generously sized, transparent so it doesn't
       read as "inside a code block". */
    .editor-form {
      max-width: 76ch;
      margin: 20px 0 0;
    }
    .editor-form .field { margin-top: 12px; }
    .editor-form .field label { font-size: 12px; }
    .editor-form textarea {
      width: 100%;
      min-height: 50vh;
      font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
      font-size: 13px;
      line-height: 1.6;
      background: #ffffff;
    }
    .editor-form .actions-row {
      margin-top: 16px;
      justify-content: flex-end;
      gap: 8px;
      display: flex;
    }

    /* Status strip is tighter than the previous full-width alert
       boxes — they used to push the layout down on every save. */
    .status {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 4px 10px;
      border-radius: 999px;
      font-size: 12px;
      margin-top: 8px;
    }
    .status.error { color: #cf222e; background: #ffebe9; border: 1px solid rgba(207, 34, 46, 0.4); }
    .status.info  { color: #1a7f37; background: #dafbe1; border: 1px solid rgba(31, 136, 61, 0.4); }
  `];

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
    this.historyOpen = false;
    this.historyVersions = null;
    this.viewingVersion = null;
    this._cursorIdx = -1;
    this._onKey = this._onKey.bind(this);
    this._onDocClick = this._onDocClick.bind(this);
  }

  connectedCallback() {
    super.connectedCallback();
    this.load().then(() => this._applyHash());
    this._subscribe();
    this._hashHandler = () => this._applyHash();
    window.addEventListener('hashchange', this._hashHandler);
    window.addEventListener('keydown', this._onKey);
    document.addEventListener('click', this._onDocClick);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unsub?.();
    window.removeEventListener('hashchange', this._hashHandler);
    window.removeEventListener('keydown', this._onKey);
    document.removeEventListener('click', this._onDocClick);
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
      if (ev.type === 'realtime.reconnected') {
        this.load();
        if (this.selected) this.open(this.selected.Path);
        return;
      }
      if (!ev.type?.startsWith('doc.')) return;
      this.load();
      if (this.selected && ev.path === this.selected.Path) {
        this.open(this.selected.Path);
      }
    });
  }

  _onKey(e) {
    // Don't hijack typing inside form controls. Slash-to-search and
    // arrow nav only apply when focus is loose.
    const tag = e.target?.tagName;
    const isFormFocus = tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT';

    if (e.key === 'Escape') {
      if (this.historyOpen) { this.historyOpen = false; e.preventDefault(); return; }
      if (this.viewingVersion) { this._returnToCurrent(); e.preventDefault(); return; }
      if (this.editing) { this.editing = false; e.preventDefault(); return; }
      if (this.creating) { this.cancelCreate(); e.preventDefault(); return; }
      if (this.search) { this._clearSearch(); e.preventDefault(); return; }
      return;
    }

    if (isFormFocus) return;

    if (e.key === '/') {
      const search = this.shadowRoot?.querySelector('nottario-search-input');
      search?.focus();
      search?.select();
      e.preventDefault();
      return;
    }

    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      const visible = this._visibleSummaries();
      if (!visible.length) return;
      if (this._cursorIdx < 0) {
        this._cursorIdx = e.key === 'ArrowDown' ? 0 : visible.length - 1;
      } else {
        this._cursorIdx += (e.key === 'ArrowDown' ? 1 : -1);
        if (this._cursorIdx < 0) this._cursorIdx = visible.length - 1;
        if (this._cursorIdx >= visible.length) this._cursorIdx = 0;
      }
      this.requestUpdate();
      // bring the cursor row into view if needed
      requestAnimationFrame(() => {
        const row = this.shadowRoot?.querySelector('.tree li.keyboard-cursor');
        row?.scrollIntoView({ block: 'nearest' });
      });
      e.preventDefault();
      return;
    }

    if (e.key === 'Enter') {
      const visible = this._visibleSummaries();
      if (this._cursorIdx >= 0 && visible[this._cursorIdx]) {
        this.open(visible[this._cursorIdx].Path);
        e.preventDefault();
      }
      return;
    }
  }

  _onDocClick(e) {
    if (!this.historyOpen) return;
    // Close history when clicking anywhere outside it.
    const path = e.composedPath?.() || [];
    if (!path.some(n => n?.classList?.contains?.('history-pop') ||
                        n?.classList?.contains?.('version-btn'))) {
      this.historyOpen = false;
    }
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
    this.viewingVersion = null;
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
      this.info = `Saved (v${doc.CurrentVersion})`;
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

  _clearSearch() {
    this.search = '';
    this.hits = null;
  }

  // History popover: load once per open, keep result cached on the
  // selected doc until the doc changes.
  async toggleHistory() {
    if (this.historyOpen) { this.historyOpen = false; return; }
    if (!this.selected) return;
    this.historyOpen = true;
    if (this.historyVersions === null ||
        this._historyPath !== this.selected.Path) {
      try {
        const r = await fetch(
          `/api/docs/history?scope=project&project_id=${this.projectId}&path=${encodeURIComponent(this.selected.Path)}`
        );
        if (!r.ok) throw new Error('history failed');
        const j = await r.json();
        this.historyVersions = j.versions || [];
        this._historyPath = this.selected.Path;
      } catch (e) {
        this.error = e.message;
        this.historyVersions = [];
      }
    }
  }

  async openVersion(v) {
    this.historyOpen = false;
    if (v === this.selected?.CurrentVersion) {
      this.viewingVersion = null;
      return;
    }
    try {
      const r = await fetch(
        `/api/docs/read-version?scope=project&project_id=${this.projectId}` +
        `&path=${encodeURIComponent(this.selected.Path)}&version=${v}`
      );
      if (!r.ok) throw new Error('version read failed');
      this.viewingVersion = await r.json();
    } catch (e) { this.error = e.message; }
  }

  _returnToCurrent() {
    this.viewingVersion = null;
  }

  groupByKind() {
    const out = { skill: [], context: [], note: [] };
    for (const s of this.summaries || []) {
      (out[s.Kind] ?? out.context).push(s);
    }
    return out;
  }

  // For soft-filter search: the tree stays visible; non-matching
  // rows fade out. _visibleSummaries returns the same items in the
  // order they appear in the rail (skill → context → note) so the
  // keyboard cursor follows the visual order.
  _visibleSummaries() {
    const groups = this.groupByKind();
    return [...groups.skill, ...groups.context, ...groups.note];
  }

  _matchesSearch(s) {
    if (!this.search.trim()) return true;
    const q = this.search.trim().toLowerCase();
    return (s.Title || '').toLowerCase().includes(q) ||
           (s.Path  || '').toLowerCase().includes(q);
  }

  _renderTitleWithMark(title) {
    const q = this.search.trim();
    if (!q) return html`${title || ''}`;
    const t = title || '';
    const idx = t.toLowerCase().indexOf(q.toLowerCase());
    if (idx < 0) return html`${t}`;
    return html`${t.slice(0, idx)}<mark>${t.slice(idx, idx + q.length)}</mark>${t.slice(idx + q.length)}`;
  }

  render() {
    if (!this.project) return html`<p class="status info" style="margin:24px">Loading...</p>`;
    return html`
      <nottario-page-header title="Docs"></nottario-page-header>
      ${this.error ? html`<div class="status error">${this.error}</div>` : null}
      ${this.info ? html`<div class="status info">${this.info}</div>` : null}
      <div class="layout">
        ${this.renderRail()}
        ${this.renderReaderCol()}
      </div>
    `;
  }

  renderRail() {
    if (this.summaries === null) return html`<div class="rail">Loading...</div>`;
    const groups = this.groupByKind();
    const order = ['skill', 'context', 'note'];
    const visible = this._visibleSummaries();
    return html`
      <div class="rail">
        <nottario-search-input class="rail-search"
            placeholder="Filter or search..."
            .value=${this.search}
            @input=${(e) => { this.search = e.detail.value; this._cursorIdx = -1; }}
            @clear=${() => this._clearSearch()}
            @enter=${() => this.runSearch()}>
          <span slot="hint">
            <kbd>/</kbd> focus  <kbd>↑↓</kbd> move  <kbd>Enter</kbd> open
          </span>
        </nottario-search-input>

        ${this.hits !== null ? this.renderHits() : html`
          ${order.map(kind => {
            const items = groups[kind];
            if (!items || !items.length) return null;
            return html`
              <div class="group">
                <div class="group-eyebrow">${kind}</div>
                <ul class="tree">
                  ${items.map(s => {
                    const match = this._matchesSearch(s);
                    const cursorIdx = visible.indexOf(s);
                    const isCursor = cursorIdx === this._cursorIdx;
                    const cls = [
                      this.selected?.Path === s.Path ? 'active' : '',
                      !match ? 'dim' : '',
                      isCursor ? 'keyboard-cursor' : '',
                    ].filter(Boolean).join(' ');
                    return html`
                      <li class=${cls} @click=${() => this.open(s.Path)} title=${s.Path}>
                        ${this._renderTitleWithMark(s.Title || s.Path)}
                      </li>
                    `;
                  })}
                </ul>
              </div>
            `;
          })}
          ${!this.summaries.length ? html`
            <p style="color:#59636e;font-size:13px;padding:0 6px">
              No documents yet. Use <strong>New document</strong> below to create one.
            </p>
          ` : null}
        `}

        <div class="rail-footer">
          <button class="btn secondary" @click=${() => this.startCreate()}>+ New document</button>
        </div>
      </div>
    `;
  }

  renderHits() {
    if (!this.hits.length) return html`
      <p style="color:#59636e;font-size:13px;padding:0 6px">
        No matches in the project documents.
      </p>
    `;
    return html`
      <div class="group">
        <div class="group-eyebrow">Search results</div>
        <ul class="tree">
          ${this.hits.map(h => html`
            <li class=${this.selected?.Path === h.Path ? 'active' : ''}
                @click=${() => this.open(h.Path)}
                title=${h.Path}>
              ${this._renderTitleWithMark(h.Title || h.Path)}
            </li>
          `)}
        </ul>
      </div>
    `;
  }

  renderReaderCol() {
    if (this.creating) return this.renderCreateForm();
    if (!this.selected) return this.renderEmpty();
    if (this.editing) return this.renderEditor();
    return this.renderReadView();
  }

  renderEmpty() {
    return html`
      <div class="reader-col">
        <div class="empty-pane">
          <h3>No document selected.</h3>
          <p>
            Pick a document from the rail to read it, or start a new one.
            This project's documents live in three kinds:
          </p>
          <dl>
            <dt>Skill</dt>
            <dd>Operating instructions for agents using the MCP server.</dd>
            <dt>Context</dt>
            <dd>Shared notes that survive across conversations (design, glossary, decisions).</dd>
            <dt>Note</dt>
            <dd>Free-form scratch pads.</dd>
          </dl>
          <div class="cta">
            <button class="btn primary" @click=${() => this.startCreate()}>+ New document</button>
          </div>
        </div>
      </div>
    `;
  }

  renderReadView() {
    const s = this.selected;
    const viewing = this.viewingVersion;
    // breadcrumb segments from the path. The last segment (the
    // filename) is rendered bold + dark; everything before is muted.
    const segs = (s.Path || '').split('/');
    const last = segs.pop();
    return html`
      <div class="reader-col">
        <div class="reader-title">
          <h2>${s.Title || last}</h2>
          <span class=${`badge ${s.Kind}`}>${s.Kind}</span>
          <div class="spacer"></div>
          <div class="actions">
            ${viewing ? null : html`
              <button class="btn ghost" @click=${() => this.startEdit()}>Edit</button>
            `}
            <div class="pop-anchor">
              <button class=${`btn ghost version-btn ${this.historyOpen ? 'open' : ''}`}
                      @click=${() => this.toggleHistory()}>
                History
              </button>
              ${this.historyOpen ? this.renderHistoryPop() : null}
            </div>
            ${viewing ? null : html`
              <button class="delete" title="Delete document" aria-label="Delete document" @click=${() => this.del()}>
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                  <path d="M6 2.5h4M3 4.5h10M4.5 4.5l.6 8.2a1 1 0 0 0 1 .9h3.8a1 1 0 0 0 1-.9l.6-8.2M6.8 7v4M9.2 7v4"
                        stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              </button>
            `}
          </div>
        </div>
        <div class="reader-meta">
          ${segs.map((p, i) => html`<span class="crumb-seg">${p}</span><span class="sep">/</span>`)}
          <span class="crumb-seg last">${last}</span>
          <span class="sep">·</span>
          <button class=${`version-btn ${this.historyOpen ? 'open' : ''}`}
                  @click=${() => this.toggleHistory()}>v${s.CurrentVersion}</button>
        </div>

        ${viewing ? html`
          <div class="version-banner">
            <span>Viewing version <strong>v${viewing.Version}</strong> read-only.
              ${viewing.Message ? html`Message: "${viewing.Message}"` : null}</span>
            <span class="spacer"></span>
            <button @click=${() => this._returnToCurrent()}>Back to current (v${s.CurrentVersion})</button>
          </div>
        ` : null}

        ${(viewing ? viewing.Description : s.Description)
          ? html`<p class="description">${viewing ? viewing.Description : s.Description}</p>`
          : null}
        <nottario-markdown
          project-id=${this.projectId}
          .html=${(viewing ? viewing.ContentHTML : s.ContentHTML) || ''}>
        </nottario-markdown>
      </div>
    `;
  }

  renderHistoryPop() {
    if (this.historyVersions === null) {
      return html`<div class="history-pop"><div class="empty">Loading...</div></div>`;
    }
    if (!this.historyVersions.length) {
      return html`<div class="history-pop"><div class="empty">No history yet.</div></div>`;
    }
    const current = this.selected.CurrentVersion;
    return html`
      <div class="history-pop">
        <ul>
          ${this.historyVersions.map(v => html`
            <li class=${v.Version === current ? 'current' : ''}
                @click=${() => this.openVersion(v.Version)}>
              <span class="vn">v${v.Version}</span>
              <span class=${v.Message ? 'msg' : 'msg empty-msg'}>${v.Message || 'no message'}</span>
              <span class="when">${this._relTime(v.CreatedAt)}</span>
            </li>
          `)}
        </ul>
      </div>
    `;
  }

  _relTime(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    const diff = (Date.now() - d.getTime()) / 1000;
    if (diff < 60) return 'just now';
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    if (diff < 86400 * 30) return `${Math.floor(diff / 86400)}d ago`;
    return d.toLocaleDateString();
  }

  renderCreateForm() {
    return html`
      <div class="reader-col">
        <div class="reader-title">
          <h2>New document</h2>
          <div class="spacer"></div>
          <div class="actions">
            <button class="btn ghost" @click=${() => this.cancelCreate()}>Cancel</button>
            <button class="btn primary" @click=${() => this.saveNew()}>Create</button>
          </div>
        </div>
        <div class="editor-form">
          <div class="field">
            <label>Path
              <span style="font-weight:400;color:#59636e">
                e.g. <code>projects/${this.projectId}/context/glossary.md</code>
              </span>
            </label>
            <input .value=${this.newPath} @input=${(e) => { this.newPath = e.target.value; }}
                   placeholder="projects/${this.projectId}/context/your-doc.md">
          </div>
          <div class="field">
            <label>Markdown (with optional frontmatter)</label>
            <textarea .value=${this.draft} @input=${(e) => { this.draft = e.target.value; }}></textarea>
          </div>
        </div>
      </div>
    `;
  }

  renderEditor() {
    const s = this.selected;
    const segs = (s.Path || '').split('/');
    const last = segs.pop();
    return html`
      <div class="reader-col">
        <div class="reader-title">
          <h2>${s.Title || last}</h2>
          <span class="badge ${s.Kind}">${s.Kind}</span>
          <div class="spacer"></div>
          <div class="actions">
            <button class="btn ghost" @click=${() => { this.editing = false; }}>Cancel</button>
            <button class="btn primary" @click=${() => this.saveEdit()}>Save changes</button>
          </div>
        </div>
        <div class="reader-meta">
          ${segs.map((p) => html`<span class="crumb-seg">${p}</span><span class="sep">/</span>`)}
          <span class="crumb-seg last">${last}</span>
          <span class="sep">·</span>
          <span>editing v${s.CurrentVersion}</span>
        </div>
        <div class="editor-form">
          <textarea .value=${this.draft} @input=${(e) => { this.draft = e.target.value; }}></textarea>
        </div>
      </div>
    `;
  }
}

customElements.define('nottario-docs-page', NottarioDocsPage);
