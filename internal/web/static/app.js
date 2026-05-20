import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import './components/topbar.js';
import './pages/login.js';
import './pages/projects.js';
import './pages/project-settings.js';
import './pages/tokens.js';
import './pages/users.js';
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
      background: var(--bg-subtle, #f6f8fa);
    }
    main {
      max-width: 1080px;
      margin: 0 auto;
      padding: 24px;
    }
    .loading {
      padding: 48px;
      text-align: center;
      color: #59636e;
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
    if (path === '/tokens') {
      return html`<nottario-tokens-page .me=${this.me}></nottario-tokens-page>`;
    }
    if (path === '/users') {
      return html`<nottario-users-page .me=${this.me}></nottario-users-page>`;
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
      ${this.renderTopbar()}
      <main>${this.renderBody()}</main>
    `;
  }
}

customElements.define('nottario-shell', NottarioShell);

// Helper for child components to trigger navigation without prop drilling.
window.nottarioNavigate = (path) => {
  window.dispatchEvent(new CustomEvent('nottario-navigate', { detail: path }));
};
