import { LitElement, html, css, nothing } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';
import { buttonStyles } from '/static/components/buttons.js';
import '/static/components/page-header.js';
import '/static/components/segmented-control.js';
import '/static/components/chip-filter.js';
import '/static/components/question-card.js';

// <nottario-questions-page> lists the questions agents filed for the
// humans of a project (projectId set) or of every project the viewer
// belongs to (projectId empty), with the answer form of each one in
// place. Tabs: Open (oldest first, the default), Answered, All.
//
// Keyboard: j/k move focus between questions; inside a focused choice
// question 1-9 pick an option and Cmd/Ctrl+Enter answers (both handled
// by the card). #q-<id> in the URL scrolls to and focuses a question.
//
// The cards are kept in a Map by question id and placed in the list by
// hand (see _syncList): a reload after a live event then reorders the
// existing elements instead of handing a half-typed answer to another
// question's card, which is what positional re-rendering would do.

const TABS = ['open', 'answered', 'all'];

const EMPTY = {
  open: 'No questions waiting for you.',
  answered: 'Nothing answered yet.',
  all: 'No questions yet.',
};

// typingTarget reports whether a keydown comes from a text field,
// where j and k are letters, not shortcuts.
function typingTarget(el) {
  if (!el) return false;
  if (el.isContentEditable) return true;
  if (el.tagName === 'TEXTAREA' || el.tagName === 'SELECT') return true;
  if (el.tagName === 'INPUT') return !['radio', 'checkbox', 'button', 'submit'].includes(el.type);
  return false;
}

class NottarioQuestionsPage extends LitElement {
  static properties = {
    me: { type: Object },
    projectId: { type: String },
    _tab: { state: true },
    _items: { state: true },
    _loading: { state: true },
    _error: { state: true },
    _paused: { state: true },
    _project: { state: true },
    _projects: { state: true },
    _filter: { state: true },
  };

  static styles = [
    buttonStyles,
    css`
      :host { display: block; box-sizing: border-box; }
      *, *::before, *::after { box-sizing: border-box; }

      .filters {
        display: flex;
        align-items: center;
        gap: 8px;
        margin-bottom: 12px;
      }

      .paused {
        margin: 0 0 12px;
        padding: 8px 12px;
        border: 1px solid var(--border);
        border-radius: var(--radius);
        background: var(--bg-subtle);
        color: var(--fg-muted);
        font-size: 13px;
      }

      .list {
        list-style: none;
        margin: 0;
        padding: 0;
        border: 1px solid var(--border);
        border-radius: var(--radius);
        background: var(--bg);
      }
      .list[hidden] { display: none; }
      .list > li {
        padding: 16px;
        border-top: 1px solid var(--border-muted);
      }
      .list > li:first-child { border-top: 0; }
      .list nottario-question-card::part(card) {
        border: 0;
        border-radius: 0;
        padding: 0;
        background: transparent;
      }
      .list nottario-question-card { outline: none; scroll-margin: 80px; }
      .list nottario-question-card:focus-visible {
        outline: 2px solid var(--accent);
        outline-offset: 6px;
        border-radius: var(--radius-sm);
      }

      .empty,
      .error {
        padding: 32px 16px;
        border: 1px solid var(--border);
        border-radius: var(--radius);
        background: var(--bg);
        text-align: center;
        color: var(--fg-muted);
        font-size: 14px;
      }
      .error { color: var(--danger-text); }
      .error p { margin: 0 0 12px; }

      /* Loading placeholders: static grey bars, no shimmer. */
      .skeleton { list-style: none; margin: 0; padding: 0; }
      .skeleton li {
        padding: 16px;
        border: 1px solid var(--border);
        border-top-width: 0;
        background: var(--bg);
      }
      .skeleton li:first-child {
        border-top-width: 1px;
        border-radius: var(--radius) var(--radius) 0 0;
      }
      .skeleton li:last-child { border-radius: 0 0 var(--radius) var(--radius); }
      .bar {
        height: 10px;
        border-radius: 3px;
        background: var(--gray-2);
        margin-bottom: 10px;
      }
      .bar.short { width: 30%; }
      .bar.long { width: 70%; }
      .bar.mid { width: 50%; margin-bottom: 0; }

      .sr-only {
        position: absolute;
        width: 1px;
        height: 1px;
        overflow: hidden;
        clip: rect(0 0 0 0);
        white-space: nowrap;
      }
    `,
  ];

  constructor() {
    super();
    this.me = null;
    this.projectId = '';
    this._tab = 'open';
    this._items = [];
    this._openAll = null; // open questions, for the tab count
    this._loading = true;
    this._error = '';
    this._paused = false;
    this._project = null;
    this._projects = [];
    this._filter = [];
    this._cards = new Map(); // question id -> <li> holding its card
    this._kept = new Map(); // id -> question answered here while on Open
    this._unsubs = [];
    this._seq = 0;
    this._reloadTimer = null;
    this._pendingHash = '';
    this._lastCard = null; // last question that held focus, for j/k
    this._onKey = this._onKey.bind(this);
    this._onHash = () => this._applyHash();
  }

  connectedCallback() {
    super.connectedCallback();
    this._pendingHash = window.location.hash;
    this._start();
    document.addEventListener('keydown', this._onKey);
    window.addEventListener('hashchange', this._onHash);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unsubscribe();
    clearTimeout(this._reloadTimer);
    document.removeEventListener('keydown', this._onKey);
    window.removeEventListener('hashchange', this._onHash);
  }

  updated(changed) {
    // projectId "changes" from undefined on the first update, which
    // connectedCallback already handled.
    if (changed.has('projectId') && changed.get('projectId') !== undefined) {
      this._cards.clear();
      this._kept.clear();
      this._items = [];
      this._start();
    }
    this._syncList();
  }

  get _global() {
    return !this.projectId;
  }

  // ---- data ----

  async _start() {
    this._loading = true;
    this._error = '';
    this._unsubscribe();
    if (this._global) {
      this._project = null;
      try {
        const r = await fetch('/api/projects');
        const j = r.ok ? await r.json() : {};
        this._projects = j.projects || [];
      } catch (_) {
        this._projects = [];
      }
      for (const p of this._projects) this._unsubs.push(subscribe(p.id, (ev) => this._onEvent(ev)));
    } else {
      try {
        const r = await fetch(`/api/projects/${encodeURIComponent(this.projectId)}`);
        if (!r.ok) throw new Error(r.status === 404 ? 'Project not found.' : `HTTP ${r.status}`);
        this._project = await r.json();
      } catch (e) {
        this._loading = false;
        this._error = `Could not load the project: ${e.message}`;
        return;
      }
      this._unsubs.push(subscribe(this._project.id, (ev) => this._onEvent(ev)));
    }
    await this._load();
  }

  _unsubscribe() {
    for (const u of this._unsubs) u();
    this._unsubs = [];
  }

  _onEvent(ev) {
    const t = ev?.type || '';
    if (t === 'realtime.disconnected') {
      this._paused = true;
      return;
    }
    if (t === 'realtime.reconnected') {
      this._paused = false;
      this._scheduleReload();
      return;
    }
    if (t.startsWith('question.')) this._scheduleReload();
  }

  // Several events usually arrive together (an answer is one update,
  // a closed task withdraws several questions): reload once.
  _scheduleReload() {
    clearTimeout(this._reloadTimer);
    this._reloadTimer = setTimeout(() => this._load(), 200);
  }

  async _fetchState(state) {
    const qs = new URLSearchParams({ state });
    if (this._project) qs.set('project_id', this._project.id);
    const r = await fetch(`/api/questions?${qs}`);
    const j = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error(j.error || `HTTP ${r.status}`);
    return j.questions || [];
  }

  async _load() {
    const seq = ++this._seq;
    const tab = this._tab;
    try {
      const open = await this._fetchState('open');
      let items = open;
      if (tab === 'open' && this._kept.size) {
        // Questions answered on this tab stay in place, folded to their
        // answer, until the tab changes; refresh them from Answered.
        const answered = await this._fetchState('answered');
        const kept = answered.filter((q) => this._kept.has(q.id));
        items = [...open, ...kept].sort(
          (a, b) => Date.parse(a.created_at) - Date.parse(b.created_at),
        );
      } else if (tab !== 'open') {
        items = await this._fetchState(tab);
      }
      if (seq !== this._seq) return;
      this._openAll = open;
      this._items = items;
      this._error = '';
    } catch (e) {
      if (seq !== this._seq) return;
      this._error = `Could not load questions: ${e.message}`;
    }
    this._loading = false;
    await this.updateComplete;
    this._applyHash(true);
  }

  _visible(list) {
    if (!this._global || this._filter.length === 0) return list;
    return list.filter((q) => this._filter.includes(q.project?.id));
  }

  // ---- interaction ----

  _setTab(tab) {
    if (!TABS.includes(tab) || tab === this._tab) return;
    this._tab = tab;
    this._kept.clear();
    this._loading = true;
    this._load();
  }

  _onAnswered(e) {
    const q = e.detail?.question;
    if (!q) return;
    if (this._tab === 'open') this._kept.set(q.id, q);
    this._items = this._items.map((x) => (x.id === q.id ? q : x));
  }

  _onKey(e) {
    if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key !== 'j' && e.key !== 'k') return;
    if (typingTarget(e.composedPath()[0])) return;
    const cards = [...this.renderRoot.querySelectorAll('.list nottario-question-card')];
    if (!cards.length) return;
    // Answering removes the focused control, which drops focus to the
    // page; carry on from the question that had it.
    let current = cards.indexOf(this.renderRoot.activeElement);
    if (current < 0 && this._lastCard) current = cards.indexOf(this._lastCard);
    let next;
    if (current < 0) next = e.key === 'j' ? 0 : cards.length - 1;
    else next = Math.min(cards.length - 1, Math.max(0, current + (e.key === 'j' ? 1 : -1)));
    e.preventDefault();
    cards[next].focus();
    cards[next].scrollIntoView({ block: 'nearest' });
  }

  // _applyHash scrolls to and focuses the question named by #q-<id>.
  // A question that is no longer open is not on the Open tab: switch
  // to All once so the link still lands.
  _applyHash(afterLoad = false) {
    const hash = afterLoad ? this._pendingHash : window.location.hash;
    if (!afterLoad) this._pendingHash = hash;
    const m = /^#q-([0-9a-f-]{36})$/i.exec(hash || '');
    if (!m || this._loading) return;
    const li = this._cards.get(m[1]);
    const card = li?.isConnected ? li.querySelector('nottario-question-card') : null;
    if (!card) {
      if (this._tab === 'open') this._setTab('all');
      else this._pendingHash = '';
      return;
    }
    this._pendingHash = '';
    const smooth = !window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    card.focus({ preventScroll: true });
    card.scrollIntoView({ block: 'center', behavior: smooth ? 'smooth' : 'auto' });
  }

  // _syncList puts one <li> per visible question into the list, in
  // order, reusing the element a question already had.
  _syncList() {
    const ol = this.renderRoot.querySelector('ol.list');
    if (!ol) return;
    const items = this._visible(this._items);
    const want = new Set(items.map((q) => q.id));
    for (const [id, li] of this._cards) {
      if (!want.has(id)) {
        li.remove();
        this._cards.delete(id);
      }
    }
    let prev = null;
    for (const q of items) {
      let li = this._cards.get(q.id);
      if (!li) {
        li = document.createElement('li');
        const card = document.createElement('nottario-question-card');
        card.tabIndex = -1;
        card.showTask = true;
        card.dataset.id = q.id;
        li.appendChild(card);
        this._cards.set(q.id, li);
      }
      const card = li.firstElementChild;
      card.projectId = this._project?.id || '';
      if (card.question !== q) card.question = q;
      const at = prev ? prev.nextSibling : ol.firstChild;
      if (at !== li) ol.insertBefore(li, at);
      prev = li;
    }
  }

  // ---- render ----

  render() {
    const open = this._visible(this._openAll || []);
    const items = this._visible(this._items);
    const tabs = [
      { value: 'open', label: this._loading && !this._openAll ? 'Open' : `Open (${open.length})` },
      { value: 'answered', label: 'Answered' },
      { value: 'all', label: 'All' },
    ];
    const showList = !this._loading && !this._error && items.length > 0;
    return html`
      <nottario-page-header
        title="Questions"
        .subtitle=${this._global ? 'Across your projects' : ''}>
        <span slot="switcher">
          <nottario-segmented-control
            .options=${tabs}
            .value=${this._tab}
            @change=${(e) => this._setTab(e.detail.value)}></nottario-segmented-control>
        </span>
      </nottario-page-header>
      ${
        this._global && this._projects.length > 1
          ? html`<div class="filters">
              <nottario-chip-filter
                label="Project"
                mode="multi"
                .options=${this._projects.map((p) => ({ value: p.id, label: p.name }))}
                .values=${this._filter}
                @change=${(e) => {
                  this._filter = e.detail.values || [];
                }}></nottario-chip-filter>
            </div>`
          : nothing
      }
      ${
        this._paused
          ? html`<p class="paused" role="status">Live updates paused, reconnecting…</p>`
          : nothing
      }
      ${this._renderState(items)}
      <ol class="list" aria-label="Questions" ?hidden=${!showList}
          @focusin=${(e) => {
            const card = e.target.closest?.('nottario-question-card');
            if (card) this._lastCard = card;
          }}
          @question-answered=${this._onAnswered}
          @question-stale=${() => this._load()}></ol>
    `;
  }

  _renderState(items) {
    if (this._loading) {
      return html`<ul class="skeleton" aria-hidden="true">
          ${[0, 1, 2].map(
            () =>
              html`<li><div class="bar short"></div><div class="bar long"></div><div class="bar mid"></div></li>`,
          )}
        </ul>
        <span class="sr-only" role="status">Loading questions…</span>`;
    }
    if (this._error) {
      return html`<div class="error" role="alert">
        <p>${this._error}</p>
        <button class="btn secondary" type="button" @click=${() => this._start()}>Try again</button>
      </div>`;
    }
    if (items.length === 0) return html`<div class="empty">${EMPTY[this._tab]}</div>`;
    return nothing;
  }
}

customElements.define('nottario-questions-page', NottarioQuestionsPage);
