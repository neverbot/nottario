import { LitElement, html, css, nothing, svg } from '/static/vendor/lit/lit.js';
import { buttonStyles } from '/static/components/buttons.js';
import {
  errorPanel,
  pageStateStyles,
  pausedBanner,
  skeleton,
} from '/static/components/page-states.js';
import { toast } from '/static/components/toast.js';
import '/static/components/avatar.js';
import '/static/components/chip-filter.js';
import '/static/components/page-header.js';
import '/static/components/question-card.js';
import {
  activityState,
  resumeCommand,
  sessionCopyText,
  sessionName,
  sortWaiting,
  stepLine,
} from '/static/live-model.js';
import { formatDateTime, formatDuration, formatRelativeTime } from '/static/time.js';
import { copyToClipboard } from '/static/utils/clipboard.js';
import { loadScope, subscribeScope } from '/static/utils/project-scope.js';

// <nottario-live-page> shows work in progress for one project
// (projectId set) or for every project the viewer belongs to
// (projectId empty), as three headed lists of rows:
//
//   Waiting for you    open questions addressed to the viewer, oldest
//                      first; Answer expands the answer form under the
//                      row (no modal). Only shown when non-empty.
//   Working now        tasks in doing, newest activity first, with the
//                      in-progress step, the agent session, the
//                      assignee and open questions.
//   Finished recently  tasks closed in the last 24 hours, dimmed.
//
// Any task or question event reloads the board (debounced). A Working
// row whose last activity moved tints its background for 1.5 s, off
// under prefers-reduced-motion; reordering is not animated. Relative
// times re-render every 15 s.

const RELOAD_MS = 300;
const TICK_MS = 15_000;
const TINT_MS = 1_500;
const DEFAULT_IDLE_MINUTES = 30;

const EMPTY_TEXT =
  'No one is working right now. Agents appear here when they claim a task or start a step.';

const reducedMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;

// Copy glyph (two stacked sheets), drawn at the 16-unit grid of the
// shared icon set.
function copyIcon(size = 14) {
  return html`<svg width=${size} height=${size} viewBox="0 0 16 16" fill="none" aria-hidden="true">
    ${svg`<rect x="5.5" y="5.5" width="8" height="8" rx="1.5" stroke="currentColor" stroke-width="1.3"/>
      <path d="M10.5 3.5v-.5a1.5 1.5 0 0 0-1.5-1.5H4A1.5 1.5 0 0 0 2.5 3v5A1.5 1.5 0 0 0 4 9.5h.5"
            stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/>`}
  </svg>`;
}

const plural = (n, word) => `${n} ${word}${n === 1 ? '' : 's'}`;

const taskHref = (task) => `/projects/${task.project_id}/board/kanban#task=${task.id}`;

class NottarioLivePage extends LitElement {
  static properties = {
    me: { type: Object },
    projectId: { type: String },
    _data: { state: true },
    _loading: { state: true },
    _error: { state: true },
    _paused: { state: true },
    _filter: { state: true },
    _expanded: { state: true },
    _changed: { state: true },
  };

  static styles = [
    buttonStyles,
    pageStateStyles,
    css`
      :host { display: block; box-sizing: border-box; }
      *, *::before, *::after { box-sizing: border-box; }

      .filters {
        display: flex;
        align-items: center;
        gap: 8px;
        margin-bottom: 12px;
      }

      section { margin-bottom: 24px; }
      section:last-child { margin-bottom: 0; }
      h2 {
        display: flex;
        align-items: center;
        gap: 8px;
        margin: 0 0 8px;
        font-size: 14px;
        font-weight: 600;
        color: var(--fg);
      }
      .count {
        display: inline-block;
        min-width: 20px;
        padding: 0 6px;
        border-radius: 2em;
        background: var(--gray-2);
        color: var(--fg);
        font-size: 12px;
        font-weight: 500;
        line-height: 18px;
        text-align: center;
      }
      .waiting h2 .count {
        background: var(--attention-subtle);
        color: var(--attention-fg);
      }

      .rows {
        list-style: none;
        margin: 0;
        padding: 0;
        border: 1px solid var(--border);
        border-radius: var(--radius);
        background: var(--bg);
      }
      .rows > li {
        padding: 8px 16px;
        border-top: 1px solid var(--border-muted);
        font-size: 14px;
        scroll-margin: 80px;
      }
      .rows > li:first-child { border-top: 0; border-radius: var(--radius) var(--radius) 0 0; }
      .rows > li:last-child { border-radius: 0 0 var(--radius) var(--radius); }
      .rows > li:only-child { border-radius: var(--radius); }
      .rows > li:focus { outline: none; }
      .rows > li:focus-visible { box-shadow: inset 0 0 0 2px var(--accent); }
      .waiting.has-open .rows { border-color: var(--attention-muted); }

      .line {
        display: flex;
        align-items: center;
        gap: 8px;
        min-width: 0;
        line-height: 20px;
      }
      .line.sub {
        flex-wrap: wrap;
        gap: 2px 8px;
        margin-top: 2px;
        padding-left: 16px;
        font-size: 12px;
        color: var(--fg-muted);
      }
      .sep { color: var(--fg-subtle); }
      .right {
        margin-left: auto;
        flex: 0 0 auto;
        font-size: 12px;
        color: var(--fg-muted);
        white-space: nowrap;
      }

      a.task {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        color: var(--fg);
        font-weight: 600;
        text-decoration: none;
      }
      a.task:hover { color: var(--accent); text-decoration: underline; }
      a.task:focus-visible,
      a.pill:focus-visible {
        outline: 2px solid var(--accent);
        outline-offset: 1px;
        border-radius: 2px;
      }

      .project {
        flex: 0 0 auto;
        display: inline-block;
        max-width: 160px;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        padding: 0 8px;
        border: 1px solid var(--border);
        border-radius: 999px;
        background: var(--bg-subtle);
        color: var(--fg);
        font-size: 11px;
        font-weight: 500;
        line-height: 18px;
      }
      .role {
        flex: 0 0 auto;
        font-size: 12px;
        color: var(--fg-muted);
      }

      /* Activity dot: green for fresh activity, a hollow grey ring when
         idle, a mid grey otherwise. */
      .dot {
        flex: 0 0 auto;
        width: 8px;
        height: 8px;
        border-radius: 50%;
        background: var(--gray-5);
      }
      .dot.recent { background: var(--success); }
      .dot.idle { background: transparent; border: 1.5px solid var(--gray-4); }
      .idle-text { color: var(--fg-subtle); }

      .step {
        min-width: 0;
        overflow-wrap: anywhere;
        color: var(--fg);
      }
      .session {
        display: inline-flex;
        align-items: center;
        gap: 2px;
        padding-left: 6px;
        border: 1px solid var(--border);
        border-radius: 999px;
        background: var(--bg-subtle);
        font-family: var(--mono);
        font-size: 11px;
        line-height: 18px;
        color: var(--fg);
      }
      .copy {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 20px;
        height: 18px;
        padding: 0;
        border: 0;
        border-radius: 999px;
        background: transparent;
        color: var(--fg-muted);
        cursor: pointer;
      }
      .copy:hover { color: var(--accent); background: var(--bg-hover); }
      .copy:focus-visible { outline: 2px solid var(--accent); outline-offset: 0; }
      .who {
        display: inline-flex;
        align-items: center;
        gap: 4px;
      }

      /* Amber is kept for what waits on a person. */
      a.pill {
        display: inline-block;
        padding: 0 7px;
        border: 1px solid var(--attention-muted);
        border-radius: 2em;
        background: var(--attention-subtle);
        color: var(--attention-fg);
        font-size: 11px;
        font-weight: 500;
        line-height: 18px;
        text-decoration: none;
      }
      a.pill:hover { border-color: var(--attention-emphasis); }

      .excerpt {
        min-width: 0;
        flex: 1 1 auto;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
        color: var(--fg);
      }
      .wait-meta {
        flex: 0 1 auto;
        min-width: 0;
        display: inline-flex;
        gap: 6px;
        overflow: hidden;
        white-space: nowrap;
        font-size: 12px;
        color: var(--fg-muted);
      }
      .wait-meta a {
        overflow: hidden;
        text-overflow: ellipsis;
        color: inherit;
        text-decoration: none;
      }
      .wait-meta a:hover { color: var(--accent); text-decoration: underline; }
      .wait-meta .sname { font-family: var(--mono); font-size: 11px; }
      .wait .right { margin-left: 0; }
      .btn.small { height: 26px; padding: 0 10px; font-size: 12px; flex: 0 0 auto; }
      .answer {
        margin: 8px 0 4px;
        padding: 12px;
        border: 1px solid var(--border-muted);
        border-radius: var(--radius);
        background: var(--bg-subtle);
      }

      .finished .rows > li { padding-top: 6px; padding-bottom: 6px; }
      .finished .line { color: var(--fg-muted); }
      .finished a.task { font-weight: 400; color: var(--fg-muted); }
      .finished .line > :not(.right) { opacity: 0.85; }
      .state {
        flex: 0 0 auto;
        font-size: 12px;
        color: var(--fg-subtle);
      }
      .state.done { color: var(--tint-green-fg); }

      .inline-empty {
        padding: 16px;
        text-align: center;
        font-size: 13px;
        color: var(--fg-muted);
      }
      .more {
        margin: 6px 0 0;
        font-size: 12px;
        color: var(--fg-muted);
      }

      /* A row whose activity just moved: a brief tint that fades out,
         as long as TINT_MS. */
      .rows > li.changed { animation: changed 1500ms ease-out; }
      @keyframes changed {
        0%, 60% { background: var(--tint-blue); }
        100% { background: var(--bg); }
      }
      @media (prefers-reduced-motion: reduce) {
        .rows > li.changed { animation: none; }
      }

      @media (max-width: 640px) {
        .rows > li { padding: 8px 12px; }
        .wait .line { flex-wrap: wrap; }
        .wait .excerpt { flex-basis: 100%; }
        .line.sub { padding-left: 0; }
      }
    `,
  ];

  constructor() {
    super();
    this.me = null;
    this.projectId = '';
    this._data = null;
    this._loading = true;
    this._error = '';
    this._paused = false;
    this._filter = [];
    this._expanded = new Set(); // question ids with the answer form open
    this._changed = new Set(); // task ids tinted right now
    this._project = null;
    this._projects = [];
    this._kept = new Map(); // question id -> question answered on this page
    this._seen = null; // task id -> last_activity_at at the previous load
    this._tintTimers = new Map();
    this._unsubs = [];
    this._seq = 0;
    this._startGen = 0;
    this._reloadTimer = null;
    this._ticker = null;
  }

  connectedCallback() {
    super.connectedCallback();
    this._start();
    this._ticker = setInterval(() => this.requestUpdate(), TICK_MS);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._startGen++;
    this._unsubscribe();
    clearTimeout(this._reloadTimer);
    clearInterval(this._ticker);
    this._ticker = null;
    for (const t of this._tintTimers.values()) clearTimeout(t);
    this._tintTimers.clear();
  }

  updated(changed) {
    // projectId "changes" from undefined on the first update, which
    // connectedCallback already handled.
    if (changed.has('projectId') && changed.get('projectId') !== undefined) {
      this._data = null;
      this._seen = null;
      this._kept.clear();
      this._expanded = new Set();
      this._filter = [];
      this._start();
    }
  }

  get _global() {
    return !this.projectId;
  }

  // ---- data ----

  // _start resolves the scope, subscribes and loads. A disconnect or a
  // new projectId while it awaits bumps _startGen, and the stale run
  // stops before subscribing anything.
  async _start() {
    const gen = ++this._startGen;
    const stale = () => gen !== this._startGen || !this.isConnected;
    this._loading = true;
    this._error = '';
    this._paused = false;
    this._unsubscribe();
    let scope;
    try {
      scope = await loadScope(this.projectId);
    } catch (e) {
      if (stale()) return;
      this._loading = false;
      this._error = e.message;
      return;
    }
    if (stale()) return;
    this._project = scope.project;
    this._projects = scope.projects;
    this._unsubs = subscribeScope(scope, (ev) => this._onEvent(ev));
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
    if (t.startsWith('task.') || t.startsWith('question.')) this._scheduleReload();
  }

  // An agent's step, comment and state change arrive as a burst of
  // events: reload once.
  _scheduleReload() {
    clearTimeout(this._reloadTimer);
    this._reloadTimer = setTimeout(() => this._load(), RELOAD_MS);
  }

  async _load() {
    const seq = ++this._seq;
    try {
      const qs = this._project ? `?project_id=${encodeURIComponent(this._project.id)}` : '';
      const r = await fetch(`/api/live${qs}`);
      const j = await r.json().catch(() => ({}));
      if (r.status === 403) throw new Error('You are not a member of this project.');
      if (!r.ok) throw new Error(j.error || `HTTP ${r.status}`);
      if (seq !== this._seq) return;
      const data = {
        projects: j.projects || [],
        waiting: j.waiting || [],
        working: j.working || [],
        finished: j.finished || [],
        truncated: j.truncated || {},
      };
      this._markChanged(data.working);
      this._data = data;
      this._error = '';
    } catch (e) {
      if (seq !== this._seq) return;
      // A background reload that fails keeps what is on screen; the
      // full error state is for a page with nothing to show yet.
      if (this._data) toast.error(`Could not refresh the live view: ${e.message}`);
      else this._error = `Could not load the live view: ${e.message}`;
    }
    this._loading = false;
  }

  // _markChanged tints the Working rows whose last activity moved since
  // the previous load, and rows that just appeared. The first load
  // tints nothing.
  _markChanged(working) {
    const prev = this._seen;
    this._seen = new Map(working.map((w) => [w.task.id, w.last_activity_at]));
    if (!prev || reducedMotion()) return;
    const fresh = working
      .filter((w) => prev.get(w.task.id) !== w.last_activity_at)
      .map((w) => w.task.id);
    if (!fresh.length) return;
    const next = new Set(this._changed);
    for (const id of fresh) {
      next.add(id);
      clearTimeout(this._tintTimers.get(id));
      this._tintTimers.set(
        id,
        setTimeout(() => {
          this._tintTimers.delete(id);
          const left = new Set(this._changed);
          left.delete(id);
          this._changed = left;
        }, TINT_MS),
      );
    }
    this._changed = next;
  }

  _inFilter(projectId) {
    return !this._global || this._filter.length === 0 || this._filter.includes(projectId);
  }

  // _waiting is the open questions plus the ones answered here, which
  // stay folded to their answer (with the resume command) until the
  // page is left.
  _waiting() {
    const open = this._data?.waiting || [];
    const ids = new Set(open.map((q) => q.id));
    const kept = [...this._kept.values()].filter((q) => !ids.has(q.id));
    return sortWaiting([...open, ...kept]).filter((q) => this._inFilter(q.project?.id));
  }

  // ---- interaction ----

  // _go keeps in-app links inside the single-page router; a modified
  // click (new tab, new window) keeps the browser's behaviour.
  _go(e) {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey)
      return;
    e.preventDefault();
    window.nottarioNavigate?.(e.currentTarget.getAttribute('href'));
  }

  async _toggle(q) {
    const next = new Set(this._expanded);
    const opening = !next.has(q.id);
    if (opening) next.add(q.id);
    else next.delete(q.id);
    this._expanded = next;
    await this.updateComplete;
    if (opening) {
      this.renderRoot.querySelector(`#qc-${q.id} nottario-question-card`)?.focus();
    } else {
      this.renderRoot.querySelector(`#w-${q.id} .toggle`)?.focus();
    }
  }

  _onAnswered(e) {
    const q = e.detail?.question;
    if (!q) return;
    this._kept.set(q.id, q);
    if (this._data) {
      this._data = {
        ...this._data,
        waiting: this._data.waiting.map((x) => (x.id === q.id ? q : x)),
      };
    }
  }

  // _jumpToQuestion moves to the first waiting row of a task, when the
  // viewer has one; otherwise the link opens the project's questions.
  async _jumpToQuestion(e, qid) {
    if (!qid) {
      this._go(e);
      return;
    }
    e.preventDefault();
    const li = this.renderRoot.querySelector(`#w-${qid}`);
    if (!li) return;
    li.focus({ preventScroll: true });
    li.scrollIntoView({ block: 'center', behavior: reducedMotion() ? 'auto' : 'smooth' });
  }

  async _copySession(session) {
    const text = sessionCopyText(session);
    if (!text) return;
    if (await copyToClipboard(text)) toast.success('Copied');
    else toast.error('Could not copy to the clipboard.');
  }

  // ---- render ----

  render() {
    return html`
      <nottario-page-header
        title="Live"
        .subtitle=${this._global ? 'Across your projects' : ''}></nottario-page-header>
      ${this._renderFilter()}
      ${this._paused ? pausedBanner() : nothing}
      ${this._renderBody()}
    `;
  }

  _renderFilter() {
    const projects = this._data?.projects || this._projects;
    if (!this._global || projects.length < 2) return nothing;
    return html`<div class="filters">
      <nottario-chip-filter
        label="Project"
        mode="multi"
        .options=${projects.map((p) => ({ value: p.id, label: p.name }))}
        .values=${this._filter}
        @change=${(e) => {
          this._filter = e.detail.values || [];
        }}></nottario-chip-filter>
    </div>`;
  }

  _renderBody() {
    if (this._loading && !this._data) {
      return skeleton('Loading live work…', ['short', 'long']);
    }
    if (this._error) return errorPanel(this._error, () => this._start());
    if (!this._data) return nothing;

    const now = Date.now();
    const waiting = this._waiting();
    const working = this._data.working.filter((w) => this._inFilter(w.task.project_id));
    const finished = this._data.finished.filter((f) => this._inFilter(f.task.project_id));

    if (!waiting.length && !working.length && !finished.length) {
      if (this._global && this._filter.length > 0) {
        return html`<div class="empty">
          <p>Nothing live in the selected projects.</p>
          <button class="btn secondary" type="button" @click=${() => {
            this._filter = [];
          }}>Clear filter</button>
        </div>`;
      }
      return html`<div class="empty"><p>${EMPTY_TEXT}</p></div>`;
    }

    const projects = new Map(this._data.projects.map((p) => [p.id, p]));
    // First waiting question of each task, for the question pill.
    const firstWaiting = new Map();
    for (const q of waiting) {
      if (q.state === 'open' && q.task && !firstWaiting.has(q.task.id))
        firstWaiting.set(q.task.id, q.id);
    }
    const truncated = this._data.truncated;

    return html`
      ${waiting.length ? this._renderWaiting(waiting, projects, truncated.waiting) : nothing}
      <section class="working" aria-labelledby="h-working">
        <h2 id="h-working">Working now <span class="count">${working.length}</span></h2>
        ${
          working.length
            ? html`<ul class="rows">
                ${working.map((w) => this._renderWorking(w, projects, firstWaiting, now))}
              </ul>`
            : html`<div class="rows"><p class="inline-empty">${EMPTY_TEXT}</p></div>`
        }
        ${truncated.working ? this._renderMore(working.length) : nothing}
      </section>
      ${
        finished.length
          ? html`<section class="finished" aria-labelledby="h-finished">
              <h2 id="h-finished">Finished recently</h2>
              <ul class="rows">${finished.map((f) => this._renderFinished(f, projects))}</ul>
              ${truncated.finished ? this._renderMore(finished.length) : nothing}
            </section>`
          : nothing
      }
    `;
  }

  _renderMore(n) {
    return html`<p class="more">Showing the most recent ${n}</p>`;
  }

  _projectChip(projectId, projects, fallback) {
    if (!this._global) return nothing;
    const name = projects.get(projectId)?.name || fallback || '';
    if (!name) return nothing;
    return html`<span class="project" title=${name}>${name}</span>`;
  }

  _renderWaiting(list, projects, truncated) {
    const open = list.filter((q) => q.state === 'open').length;
    return html`<section class="waiting ${open ? 'has-open' : ''}" aria-labelledby="h-waiting">
      <h2 id="h-waiting">
        Waiting for you ${open ? html`<span class="count">${open}</span>` : nothing}
      </h2>
      <ul class="rows"
          @question-answered=${this._onAnswered}
          @question-stale=${() => this._load()}>
        ${list.map((q) => this._renderWaitingRow(q, projects))}
      </ul>
      ${truncated ? this._renderMore(list.length) : nothing}
    </section>`;
  }

  _renderWaitingRow(q, projects) {
    const expanded = this._expanded.has(q.id);
    const label = q.state === 'open' ? 'Answer' : 'Show answer';
    return html`<li class="wait" id=${`w-${q.id}`} tabindex="-1">
      <div class="line">
        ${this._projectChip(q.project?.id, projects, q.project?.name)}
        <span class="excerpt" title=${q.excerpt}>${q.excerpt}</span>
        <span class="wait-meta">
          ${
            q.task
              ? html`<a href=${taskHref({ id: q.task.id, project_id: q.project?.id || this._project?.id })}
                   @click=${this._go}>${q.task.title}</a>`
              : nothing
          }
          ${
            q.session
              ? html`<span class="sep" aria-hidden="true">·</span>
                  <span class="sname" title=${q.session.key || ''}>${sessionName(q.session)}</span>`
              : nothing
          }
        </span>
        <time class="right" datetime=${q.created_at} title=${formatDateTime(q.created_at)}>
          ${formatRelativeTime(q.created_at)}
        </time>
        <button class="btn small toggle ${q.state === 'open' && !expanded ? 'primary' : ''}"
                type="button"
                aria-expanded=${expanded ? 'true' : 'false'}
                aria-controls=${`qc-${q.id}`}
                @click=${() => this._toggle(q)}>${expanded ? 'Close' : label}</button>
      </div>
      ${
        expanded
          ? html`<div class="answer" id=${`qc-${q.id}`}>
              <nottario-question-card
                compact
                .question=${q}
                .projectId=${this._project?.id || ''}></nottario-question-card>
            </div>`
          : nothing
      }
    </li>`;
  }

  _renderWorking(w, projects, firstWaiting, now) {
    const idleMinutes = projects.get(w.task.project_id)?.idle_after_minutes || DEFAULT_IDLE_MINUTES;
    const state = activityState(w, idleMinutes, now);
    const lastAt = Date.parse(w.last_activity_at);
    const dotLabel = {
      recent: 'Active in the last 2 minutes',
      idle: 'Idle',
      quiet: 'Active recently',
    }[state];
    const step = stepLine(w);
    const parts = [];
    if (step) parts.push(html`<span class="step">${step}</span>`);
    if (w.session) {
      const copyText = sessionCopyText(w.session);
      const what = resumeCommand(w.session) ? 'resume command' : 'session id';
      parts.push(html`<span class="session" title=${w.session.key || ''}>
        ${sessionName(w.session)}
        <button class="copy" type="button"
                aria-label=${`Copy ${what}`}
                title=${`Copy ${what}: ${copyText}`}
                @click=${() => this._copySession(w.session)}>${copyIcon(12)}</button>
      </span>`);
    }
    if (w.assignee) {
      const name = w.assignee.display_name || w.assignee.login;
      parts.push(html`<span class="who">
        <nottario-avatar size="16" .src=${w.assignee.avatar_url || ''} .name=${name}></nottario-avatar>
        ${name}
      </span>`);
    }
    if (w.started_at) {
      parts.push(
        html`<span title=${`In progress since ${formatDateTime(w.started_at)}`}>for ${formatDuration(now - Date.parse(w.started_at))}</span>`,
      );
    }
    if (w.questions_open > 0) {
      const qid = firstWaiting.get(w.task.id);
      const href = qid ? `#w-${qid}` : `/projects/${w.task.project_id}/questions`;
      parts.push(html`<a class="pill" href=${href}
          @click=${(e) => this._jumpToQuestion(e, qid)}>${plural(w.questions_open, 'question')}</a>`);
    }
    const sub = parts.flatMap((p, i) =>
      i ? [html`<span class="sep" aria-hidden="true">·</span>`, p] : [p],
    );
    return html`<li class=${this._changed.has(w.task.id) ? 'changed' : ''}>
      <div class="line">
        <span class="dot ${state}" role="img" aria-label=${dotLabel} title=${dotLabel}></span>
        ${this._projectChip(w.task.project_id, projects)}
        <a class="task" href=${taskHref(w.task)} @click=${this._go}>${w.task.title}</a>
        ${w.role ? html`<span class="role">${w.role.name}</span>` : nothing}
        <time class="right ${state === 'idle' ? 'idle-text' : ''}"
              datetime=${w.last_activity_at}
              title=${`Last activity ${formatDateTime(w.last_activity_at)}`}>
          ${
            state === 'idle'
              ? `No activity for ${formatDuration(now - lastAt)}`
              : formatRelativeTime(w.last_activity_at)
          }
        </time>
      </div>
      ${sub.length ? html`<div class="line sub">${sub}</div>` : nothing}
    </li>`;
  }

  _renderFinished(f, projects) {
    const name = f.assignee ? f.assignee.display_name || f.assignee.login : '';
    return html`<li>
      <div class="line">
        ${this._projectChip(f.task.project_id, projects)}
        <a class="task" href=${taskHref(f.task)} @click=${this._go}>${f.task.title}</a>
        <span class="state ${f.state === 'done' ? 'done' : ''}">
          ${f.state === 'done' ? 'done' : "won't do"}
        </span>
        ${
          f.assignee
            ? html`<span class="who role">
                <nottario-avatar size="16" .src=${f.assignee.avatar_url || ''} .name=${name}></nottario-avatar>
                ${name}
              </span>`
            : nothing
        }
        <time class="right" datetime=${f.ended_at} title=${formatDateTime(f.ended_at)}>
          ${formatRelativeTime(f.ended_at)}
        </time>
      </div>
    </li>`;
  }
}

customElements.define('nottario-live-page', NottarioLivePage);
