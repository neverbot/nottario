import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { defaultPathFor } from './views.js';
import './components/topbar.js';
import './components/update-banner.js';
import './pages/login.js';
import './pages/projects.js';
import './pages/project-settings.js';
import './pages/users.js';
import './pages/profile.js';
import './pages/board.js';
import './pages/docs.js';
import './pages/arch.js';
import './pages/search.js';
import './pages/questions.js';
import './pages/live.js';
import './components/task-dialog.js';
import {
  hashWithoutTask,
  openTask,
  parseTaskHash,
  parseTaskURL,
  taskHash,
} from './utils/task-link.js';

// deepActiveElement follows focus into shadow roots, so the dialog can
// hand focus back to the exact control that opened it.
function deepActiveElement() {
  let el = document.activeElement;
  while (el?.shadowRoot?.activeElement) el = el.shadowRoot.activeElement;
  return el;
}

class NottarioShell extends LitElement {
  static properties = {
    me: { state: true },
    route: { state: true },
    loading: { state: true },
    // The task shown in the task dialog, read from the URL hash:
    // { taskId, projectId } or null.
    _task: { state: true },
  };

  static styles = css`
    :host {
      display: block;
      min-height: 100vh;
      background: var(--bg-subtle, var(--bg-subtle));
    }
    main {
      max-width: 1080px;
      margin: 0 auto;
      padding: 24px;
    }
    main:focus { outline: none; }
    /* Skip-link styles also live in the global stylesheet so they
       work even when the shadow root isn't yet upgraded. Duplicating
       the rule here keeps shadow-DOM users covered. */
    .skip-link {
      position: absolute;
      top: 0;
      left: 0;
      padding: 8px 12px;
      background: #fff;
      color: var(--accent);
      border: 1px solid var(--border);
      border-radius: 6px;
      font-weight: 600;
      text-decoration: none;
      transform: translateY(-200%);
      z-index: 1000;
    }
    .skip-link:focus { transform: translateY(8px); }
    .loading {
      padding: 48px;
      text-align: center;
      color: var(--fg-muted);
    }
  `;

  constructor() {
    super();
    this.me = null;
    this.route = window.location.pathname;
    this.loading = true;
    this._task = null;
    window.addEventListener('popstate', () => {
      this.route = window.location.pathname;
      this._syncTask();
      this._afterBack();
    });
    window.addEventListener('hashchange', () => this._syncTask());
    window.addEventListener('nottario-navigate', (e) => {
      this.navigate(e.detail);
    });
    window.addEventListener('nottario-open-task', (e) => {
      this.openTask(e.detail.taskId, e.detail.projectId);
    });
    // Any link to a task (the task-link helper's, a markdown chip's,
    // the older /board/kanban#task= form) opens the dialog in place.
    // Capture phase, so it runs before a component's own handler (which
    // then sees defaultPrevented and leaves the click alone). Modified
    // clicks keep the browser's new-tab behaviour.
    document.addEventListener('click', (e) => this._onTaskLinkClick(e), true);
    this._syncTask();
  }

  // ---- Task dialog ---------------------------------------------------
  //
  // The URL hash is the source of truth: #task=<id> on a project route,
  // #task=<id>&project=<pid> elsewhere (utils/task-link.js). Opening
  // pushes a history entry marked { taskDialog: true }, so the browser's
  // Back closes the dialog and leaves the page where it was. Opening
  // another task while one is showing replaces that entry instead, so
  // Back still closes rather than walking through every task visited.

  _syncTask() {
    const t = parseTaskHash(window.location.pathname, window.location.hash);
    const cur = this._task;
    if (!t) {
      if (cur) {
        this._task = null;
        this._restoreFocus();
      }
      return;
    }
    if (cur && cur.taskId === t.taskId && cur.projectId === t.projectId) return;
    if (!cur) this._returnFocus = deepActiveElement();
    this._task = t;
  }

  openTask(taskId, projectId) {
    if (!taskId || !projectId) return;
    // A close is still walking back through history: pushing now would
    // land before the back and be undone by it. Open once it settled.
    if (this._pendingBack) {
      this._pendingOpen = { taskId, projectId };
      return;
    }
    // Built from the live location so the page's own hash state (board
    // cycle and filters, docs path, ...) is kept.
    const { pathname, search } = window.location;
    const target = pathname + search + taskHash(taskId, projectId, window.location);
    if (this._task) {
      window.history.replaceState(window.history.state, '', target);
    } else {
      window.history.pushState({ taskDialog: true }, '', target);
    }
    this._syncTask();
  }

  _closeTask() {
    if (!this._task) return;
    this._task = null;
    if (window.history.state?.taskDialog) {
      // The entry is ours: going back lands on the page as it was.
      // history.back() is asynchronous; an open requested before its
      // popstate waits for it (_afterBack). The timer covers a back
      // that never reports (a cross-document entry).
      this._pendingBack = true;
      clearTimeout(this._backTimer);
      this._backTimer = setTimeout(() => this._afterBack(), 1000);
      window.history.back();
    } else {
      // A deep link (reload, pasted URL): drop the task from the hash
      // without leaving the page.
      const { pathname, search, hash } = window.location;
      window.history.replaceState(
        window.history.state,
        '',
        pathname + search + hashWithoutTask(hash),
      );
    }
    this._restoreFocus();
  }

  _afterBack() {
    if (!this._pendingBack) return;
    this._pendingBack = false;
    clearTimeout(this._backTimer);
    const next = this._pendingOpen;
    this._pendingOpen = null;
    if (next) this.openTask(next.taskId, next.projectId);
  }

  // The page behind is inert while the dialog shows; focus can only go
  // back once the render that lifts it has landed.
  async _restoreFocus() {
    const el = this._returnFocus;
    this._returnFocus = null;
    await this.updateComplete;
    if (el?.isConnected && !this._task) el.focus?.({ preventScroll: true });
  }

  _onTaskLinkClick(e) {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) {
      return;
    }
    const a = e.composedPath().find((n) => n.tagName === 'A' && n.hasAttribute?.('href'));
    if (!a || (a.target && a.target !== '_self') || a.hasAttribute('download')) return;
    const t = parseTaskURL(a.getAttribute('href'), window.location.href);
    if (!t) return;
    e.preventDefault();
    openTask(t.taskId, t.projectId);
  }

  async connectedCallback() {
    super.connectedCallback();
    await this.refreshMe();
  }

  async refreshMe() {
    this.loading = true;
    try {
      const res = await fetch('/api/me');
      if (res.ok) {
        this.me = await res.json();
      } else {
        this.me = null;
      }
    } catch (_) {
      this.me = null;
    } finally {
      this.loading = false;
    }
  }

  // Resolves the canonical /projects/{id} URL: fetches the project,
  // navigates to its default_view. Guarded against repeat triggers
  // during the brief render-in-flight window.
  async _redirectToDefaultView(pid) {
    if (this._resolvingPid === pid) return;
    this._resolvingPid = pid;
    try {
      const r = await fetch(`/api/projects/${pid}`);
      if (!r.ok) {
        this.navigate('/');
        return;
      }
      const p = await r.json();
      this.navigate(defaultPathFor(p));
    } catch (_) {
      this.navigate('/');
    } finally {
      this._resolvingPid = null;
    }
  }

  navigate(path) {
    window.history.pushState({}, '', path);
    // Route matching uses the pathname only; the hash is consumed by
    // the destination page for deep-linking (search results, etc.).
    this.route = path.split('#')[0];
    // pushState does not fire `hashchange` even when the hash differs;
    // dispatch one manually so pages already on the same path react.
    window.dispatchEvent(new HashChangeEvent('hashchange'));
  }

  async logout() {
    await fetch('/auth/logout', { method: 'POST' });
    this.me = null;
    this.navigate('/');
  }

  renderTopbar() {
    if (!this.me) return null;
    return html`
      <nottario-topbar
        ?inert=${!!this._task}
        .me=${this.me}
        .route=${this.route}
        @nottario-logout=${() => this.logout()}>
      </nottario-topbar>
    `;
  }

  linkNav(path) {
    return (e) => {
      e.preventDefault();
      this.navigate(path);
    };
  }

  renderBody() {
    if (this.loading) {
      return html`<div class="loading">Loading…</div>`;
    }
    if (!this.me) {
      return html`<nottario-login></nottario-login>`;
    }
    const path = this.route;
    if (path === '/' || path === '/projects') {
      return html`<nottario-projects-page .me=${this.me}></nottario-projects-page>`;
    }
    if (path === '/users') {
      return html`<nottario-users-page .me=${this.me}></nottario-users-page>`;
    }
    if (path === '/live') {
      return html`<nottario-live-page .me=${this.me}></nottario-live-page>`;
    }
    if (path === '/questions') {
      return html`<nottario-questions-page .me=${this.me}></nottario-questions-page>`;
    }
    if (path === '/me' || path === '/profile') {
      return html`<nottario-profile-page .me=${this.me}></nottario-profile-page>`;
    }
    // /projects/{id} (no suffix) is the canonical project URL: resolve
    // it to the project's default_view server-side, then redirect. We
    // hit this when external links / agents reference the bare path,
    // or when the user types it manually.
    const bareProjectMatch = path.match(/^\/projects\/([^/]+)\/?$/);
    if (bareProjectMatch) {
      this._redirectToDefaultView(bareProjectMatch[1]);
      return html`<div class="loading">Opening project…</div>`;
    }
    const settingsMatch = path.match(/^\/projects\/([^/]+)\/settings$/);
    if (settingsMatch) {
      return html`<nottario-project-settings
        .me=${this.me} .projectId=${settingsMatch[1]}></nottario-project-settings>`;
    }
    // Board: /board (default kanban) | /board/kanban | /board/gantt
    const boardMatch = path.match(/^\/projects\/([^/]+)\/board(?:\/(kanban|gantt))?$/);
    if (boardMatch) {
      const view = boardMatch[2] || 'kanban';
      return html`<nottario-board-page
        .me=${this.me} .projectId=${boardMatch[1]} .view=${view}></nottario-board-page>`;
    }
    const docsMatch = path.match(/^\/projects\/([^/]+)\/docs$/);
    if (docsMatch) {
      return html`<nottario-docs-page
        .me=${this.me} .projectId=${docsMatch[1]}></nottario-docs-page>`;
    }
    const liveMatch = path.match(/^\/projects\/([^/]+)\/live$/);
    if (liveMatch) {
      return html`<nottario-live-page
        .me=${this.me} .projectId=${liveMatch[1]}></nottario-live-page>`;
    }
    const questionsMatch = path.match(/^\/projects\/([^/]+)\/questions$/);
    if (questionsMatch) {
      return html`<nottario-questions-page
        .me=${this.me} .projectId=${questionsMatch[1]}></nottario-questions-page>`;
    }
    // Architecture: /arch (default diagram) | /arch/diagram | /arch/tree
    const archMatch = path.match(/^\/projects\/([^/]+)\/arch(?:\/(diagram|tree))?$/);
    if (archMatch) {
      const view = archMatch[2] || 'diagram';
      return html`<nottario-arch-page
        .me=${this.me} .projectId=${archMatch[1]} .view=${view}></nottario-arch-page>`;
    }
    return html`
      <div class="card" style="padding: 24px; text-align: center;">
        <h2>Not found</h2>
        <p class="muted">The path ${path} does not match any view.</p>
        <button @click=${this.linkNav('/')}>Back to projects</button>
      </div>
    `;
  }

  render() {
    return html`
      <a class="skip-link" href="#main-content" ?inert=${!!this._task}
         @click=${(e) => {
           e.preventDefault();
           const m = this.shadowRoot?.getElementById('main-content');
           m?.focus();
         }}>Skip to main content</a>
      ${this.renderTopbar()}
      ${this.me ? html`<nottario-update-banner ?inert=${!!this._task} .me=${this.me}></nottario-update-banner>` : null}
      <main id="main-content" tabindex="-1" ?inert=${!!this._task}>${this.renderBody()}</main>
      ${
        this.me
          ? html`<nottario-task-dialog
              .me=${this.me}
              .projectId=${this._task?.projectId || ''}
              .taskId=${this._task?.taskId || ''}
              @task-dialog-close=${() => this._closeTask()}></nottario-task-dialog>`
          : null
      }
    `;
  }
}

customElements.define('nottario-shell', NottarioShell);

// Helper for child components to trigger navigation without prop drilling.
window.nottarioNavigate = (path) => {
  window.dispatchEvent(new CustomEvent('nottario-navigate', { detail: path }));
};
