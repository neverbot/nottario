import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { defaultPathFor } from './views.js';
import './components/topbar.js';
import './pages/login.js';
import './pages/projects.js';
import './pages/project-settings.js';
import './pages/users.js';
import './pages/profile.js';
import './pages/board.js';
import './pages/docs.js';
import './pages/arch.js';
import './pages/search.js';

class NottarioShell extends LitElement {
  static properties = {
    me: { state: true },
    route: { state: true },
    loading: { state: true },
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
    window.addEventListener('popstate', () => {
      this.route = window.location.pathname;
    });
    window.addEventListener('nottario-navigate', (e) => {
      this.navigate(e.detail);
    });
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
      <a class="skip-link" href="#main-content"
         @click=${(e) => {
           e.preventDefault();
           const m = this.shadowRoot?.getElementById('main-content');
           m?.focus();
         }}>Skip to main content</a>
      ${this.renderTopbar()}
      <main id="main-content" tabindex="-1">${this.renderBody()}</main>
    `;
  }
}

customElements.define('nottario-shell', NottarioShell);

// Helper for child components to trigger navigation without prop drilling.
window.nottarioNavigate = (path) => {
  window.dispatchEvent(new CustomEvent('nottario-navigate', { detail: path }));
};
