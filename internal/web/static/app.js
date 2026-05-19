import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import './pages/login.js';
import './pages/projects.js';
import './pages/project-settings.js';
import './pages/tokens.js';
import './pages/board.js';
import './pages/docs.js';

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
    header.topbar {
      display: flex;
      align-items: center;
      gap: 16px;
      padding: 12px 24px;
      background: #24292f;
      color: #fff;
      border-bottom: 1px solid #1b1f23;
    }
    header.topbar strong { font-size: 16px; }
    header.topbar .spacer { flex: 1; }
    header.topbar a, header.topbar button.link {
      color: #fff;
      opacity: 0.85;
      padding: 4px 8px;
      border-radius: 4px;
      background: transparent;
      border: none;
      cursor: pointer;
      font-size: 14px;
    }
    header.topbar a:hover, header.topbar button.link:hover {
      opacity: 1;
      background: rgba(255, 255, 255, 0.1);
      text-decoration: none;
    }
    .user {
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .user img {
      width: 24px;
      height: 24px;
      border-radius: 50%;
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
    this.route = path;
  }

  async logout() {
    await fetch('/auth/logout', { method: 'POST' });
    this.me = null;
    this.navigate('/');
  }

  renderTopbar() {
    if (!this.me) return null;
    return html`
      <header class="topbar">
        <strong>Nottario</strong>
        <a href="/" @click=${this.linkNav('/')}>Projects</a>
        <a href="/tokens" @click=${this.linkNav('/tokens')}>Tokens</a>
        <div class="spacer"></div>
        <div class="user">
          ${this.me.avatar_url ? html`<img src=${this.me.avatar_url} alt="">` : ''}
          <span>${this.me.display_name}</span>
          ${this.me.is_admin ? html`<span class="badge admin">admin</span>` : ''}
        </div>
        <button class="link" @click=${() => this.logout()}>Sign out</button>
      </header>
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
    const settingsMatch = path.match(/^\/projects\/([^/]+)\/settings$/);
    if (settingsMatch) {
      return html`<nottario-project-settings
        .me=${this.me} .projectId=${settingsMatch[1]}></nottario-project-settings>`;
    }
    const boardMatch = path.match(/^\/projects\/([^/]+)\/board$/);
    if (boardMatch) {
      return html`<nottario-board-page
        .me=${this.me} .projectId=${boardMatch[1]}></nottario-board-page>`;
    }
    const docsMatch = path.match(/^\/projects\/([^/]+)\/docs$/);
    if (docsMatch) {
      return html`<nottario-docs-page
        .me=${this.me} .projectId=${docsMatch[1]}></nottario-docs-page>`;
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
