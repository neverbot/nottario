import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { badgeStyles } from './badges.js';
import './avatar.js';
import '../pages/search.js';

// <nottario-topbar> renders the persistent app-shell topbar that wraps
// every authenticated page. It owns: brand, primary nav, search and
// the user dropdown. Logout is fired upward as a 'nottario-logout'
// event so the shell can refresh its session state without coupling
// to fetch URLs.
class NottarioTopbar extends LitElement {
  static properties = {
    me: { type: Object },
    route: { type: String },
    open: { state: true }, // user dropdown open flag
    _projectName: { state: true }, // cached name of the active project
  };

  static styles = [
    badgeStyles,
    css`
    :host {
      box-sizing: border-box;
      display: block;
      color: #fff;
      background: var(--fg); /* slightly cooler than the previous #24292f */
      border-bottom: 1px solid #14171a;
      font-size: 14px;
    }
    * { box-sizing: border-box; }

    .bar {
      display: flex;
      align-items: center;
      gap: 14px;
      max-width: 1280px;
      margin: 0 auto;
      padding: 10px 20px;
    }
    .brand {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      font-weight: 600;
      letter-spacing: 0.01em;
      color: #fff;
      text-decoration: none;
      padding: 4px 6px 4px 4px;
      border-radius: 6px;
    }
    .brand:hover { background: rgba(255,255,255,0.06); }
    .brand-mark {
      width: 22px;
      height: 22px;
      border-radius: 5px;
      background: linear-gradient(135deg, var(--brand-green) 0%, var(--brand-blue) 100%);
      display: inline-block;
    }
    .brand-name { font-size: 15px; }

    nav.primary {
      display: flex;
      align-items: center;
      gap: 2px;
      margin-left: 6px;
    }
    nav.primary a {
      position: relative;
      display: inline-flex;
      align-items: center;
      height: 32px;
      padding: 0 10px;
      color: rgba(255,255,255,0.78);
      text-decoration: none;
      border-radius: 6px;
      font-weight: 500;
    }
    nav.primary a:hover {
      color: #fff;
      background: rgba(255,255,255,0.06);
    }
    nav.primary a.active {
      color: #fff;
    }
    .project-row {
      background: var(--fg);
      border-top: 1px solid rgba(255,255,255,0.06);
    }
    .project-row .inner {
      max-width: 1280px;
      margin: 0 auto;
      padding: 0 20px;
      display: flex;
      align-items: stretch;
      gap: 4px;
    }
    .project-row .pname {
      display: inline-flex;
      align-items: center;
      padding: 0 10px 0 0;
      margin-right: 6px;
      color: #fff;
      font-weight: 600;
      font-size: 13px;
      letter-spacing: 0.01em;
      border-right: 1px solid rgba(255,255,255,0.1);
    }
    nav.project {
      display: flex;
      align-items: stretch;
      gap: 4px;
    }
    nav.project a {
      position: relative;
      display: inline-flex;
      align-items: center;
      height: 36px;
      padding: 0 12px;
      color: rgba(255,255,255,0.72);
      text-decoration: none;
      font-size: 13px;
      font-weight: 500;
      border-bottom: 2px solid transparent;
    }
    nav.project a:hover { color: #fff; }
    nav.project a.active {
      color: #fff;
      border-bottom-color: #ff8c42;
    }
    nav.primary a.active::after {
      content: "";
      position: absolute;
      left: 10px;
      right: 10px;
      bottom: -10px;
      height: 2px;
      background: #ff8c42;
      border-radius: 2px;
    }

    .spacer { flex: 1 1 0; min-width: 0; }

    .right {
      display: flex;
      align-items: center;
      gap: 10px;
      flex: 0 0 auto;
    }

    .user-trigger {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      height: 32px;
      padding: 0 10px 0 4px;
      border: 1px solid transparent;
      background: transparent;
      color: #fff;
      border-radius: 999px;
      cursor: pointer;
      font: inherit;
    }
    .user-trigger:hover { background: rgba(255,255,255,0.06); }
    .user-trigger[aria-expanded="true"] {
      background: rgba(255,255,255,0.10);
      border-color: rgba(255,255,255,0.16);
    }
    .user-trigger .name {
      max-width: 140px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .user-trigger .chevron {
      width: 0;
      height: 0;
      border-left: 4px solid transparent;
      border-right: 4px solid transparent;
      border-top: 4px solid rgba(255,255,255,0.7);
      margin-left: 2px;
    }

    .menu-wrap { position: relative; }
    .menu {
      position: absolute;
      top: calc(100% + 6px);
      right: 0;
      min-width: 240px;
      background: #ffffff;
      color: var(--fg);
      border: 1px solid var(--border);
      border-radius: 8px;
      box-shadow: 0 8px 24px rgba(31, 35, 40, 0.12);
      padding: 6px;
      z-index: 50;
    }
    .menu .who {
      padding: 8px 10px 10px;
      border-bottom: 1px solid var(--gray-2);
      margin-bottom: 4px;
    }
    .menu .who .display { font-weight: 600; }
    .menu .who .login { color: var(--fg-muted); font-size: 12px; }
    .menu .who .badges { margin-top: 6px; }
    .menu .item {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 7px 10px;
      border-radius: 5px;
      color: var(--fg);
      text-decoration: none;
      background: transparent;
      border: none;
      width: 100%;
      text-align: left;
      cursor: pointer;
      font: inherit;
    }
    .menu .item:hover { background: #f3f4f6; }
    .menu .item.danger { color: var(--danger); }
    .menu .item.danger:hover,
    .menu .item.danger:focus-visible { background: var(--tint-red); }
    .menu .sep { height: 1px; background: var(--gray-2); margin: 4px 2px; }
  `,
  ];

  constructor() {
    super();
    this.me = null;
    this.route = '/';
    this.open = false;
    this._projectName = '';
    this._projectNameFor = '';
    this._onDocClick = (e) => {
      if (!this.open) return;
      if (e.composedPath().includes(this)) return;
      this.open = false;
    };
    this._onKey = (e) => {
      if (!this.open) return;
      if (e.key === 'Escape') {
        this.open = false;
        const t = this.renderRoot.querySelector('.user-trigger');
        if (t) t.focus();
      }
    };
  }

  connectedCallback() {
    super.connectedCallback();
    document.addEventListener('click', this._onDocClick, true);
    document.addEventListener('keydown', this._onKey);
  }
  disconnectedCallback() {
    document.removeEventListener('click', this._onDocClick, true);
    document.removeEventListener('keydown', this._onKey);
    super.disconnectedCallback();
  }

  activeProjectId() {
    const m = (this.route || '').match(/^\/projects\/([^/]+)/);
    return m ? m[1] : null;
  }

  updated(changed) {
    if (changed.has('route')) {
      const pid = this.activeProjectId();
      if (pid && pid !== this._projectNameFor) {
        this._projectNameFor = pid;
        this._projectName = '';
        fetch(`/api/projects/${pid}`)
          .then((r) => (r.ok ? r.json() : null))
          .then((p) => {
            if (p && this._projectNameFor === pid) this._projectName = p.name;
          })
          .catch(() => {
            /* ignore */
          });
      } else if (!pid) {
        this._projectNameFor = '';
        this._projectName = '';
      }
    }
  }

  _go(path) {
    return (e) => {
      e.preventDefault();
      this.open = false;
      if (window.nottarioNavigate) {
        window.nottarioNavigate(path);
      } else {
        window.location.href = path;
      }
    };
  }

  _toggleMenu(e) {
    e.stopPropagation();
    this.open = !this.open;
    if (this.open) {
      // Move focus to the first menu item on next tick so keyboard
      // users land inside the dropdown immediately.
      requestAnimationFrame(() => {
        const first = this.renderRoot.querySelector('.menu .item');
        if (first) first.focus();
      });
    }
  }

  _onMenuKey(e) {
    const items = Array.from(this.renderRoot.querySelectorAll('.menu .item'));
    if (!items.length) return;
    const i = items.indexOf(document.activeElement === this ? null : this.shadowRoot.activeElement);
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      items[(i + 1 + items.length) % items.length].focus();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      items[(i - 1 + items.length) % items.length].focus();
    } else if (e.key === 'Home') {
      e.preventDefault();
      items[0].focus();
    } else if (e.key === 'End') {
      e.preventDefault();
      items[items.length - 1].focus();
    }
  }

  _logout() {
    this.open = false;
    this.dispatchEvent(new CustomEvent('nottario-logout', { bubbles: true, composed: true }));
  }

  _isActive(path) {
    if (path === '/') return this.route === '/' || this.route === '/projects';
    return this.route === path || this.route.startsWith(path + '/');
  }

  // Per-project sub-navigation. Active when the route is inside a
  // project (`/projects/{id}/...`). Mirrors GitHub's repo sub-nav.
  //
  // Board and Gantt are siblings, not parent+sub-view. They share
  // the URL prefix `/board/` for historical reasons; the nav matches
  // their full path so each entry lights up independently.
  _projectNavItems(projectId) {
    const base = `/projects/${projectId}`;
    const r = this.route || '';
    const match = (prefix) => r === prefix || r.startsWith(prefix + '/');
    const items = [
      { label: 'Board', href: `${base}/board/kanban`, active: match(`${base}/board/kanban`) },
      { label: 'Gantt', href: `${base}/board/gantt`, active: match(`${base}/board/gantt`) },
      { label: 'Docs', href: `${base}/docs`, active: match(`${base}/docs`) },
      { label: 'Architecture', href: `${base}/arch/diagram`, active: match(`${base}/arch`) },
    ];
    if (this.me?.is_admin) {
      items.push({
        label: 'Settings',
        href: `${base}/settings`,
        active: match(`${base}/settings`),
      });
    }
    return items;
  }

  render() {
    if (!this.me) return null;
    const proj = this.activeProjectId();
    const projectItems = proj ? this._projectNavItems(proj) : null;
    return html`
      <header role="banner">
      <div class="bar">
        <a class="brand" href="/" @click=${this._go('/')}
           aria-label="Nottario home">
          <span class="brand-mark"></span>
          <span class="brand-name">Nottario</span>
        </a>
        <nav class="primary" aria-label="Primary navigation">
          <a href="/"
             class=${this._isActive('/') ? 'active' : ''}
             aria-current=${this._isActive('/') ? 'page' : 'false'}
             @click=${this._go('/')}>Projects</a>
          <a href="/users"
             class=${this._isActive('/users') ? 'active' : ''}
             aria-current=${this._isActive('/users') ? 'page' : 'false'}
             @click=${this._go('/users')}>Users</a>
        </nav>
        <div class="spacer"></div>
        <div class="right">
          <nottario-search-box project-id=${proj || ''}></nottario-search-box>
          <div class="menu-wrap">
            <button class="user-trigger"
                    aria-haspopup="menu"
                    aria-expanded=${this.open ? 'true' : 'false'}
                    @click=${this._toggleMenu}>
              <nottario-avatar
                .src=${this.me.avatar_url || ''}
                .name=${this.me.display_name || this.me.github_login || ''}
                .size=${24}></nottario-avatar>
              <span class="name">${this.me.display_name || this.me.github_login}</span>
              <span class="chevron"></span>
            </button>
            ${
              this.open
                ? html`
              <div class="menu" role="menu" @keydown=${this._onMenuKey}>
                <div class="who">
                  <div class="display">${this.me.display_name || this.me.github_login}</div>
                  ${this.me.github_login ? html`<div class="login">@${this.me.github_login}</div>` : ''}
                  ${
                    this.me.is_admin
                      ? html`<div class="badges"><span class="badge admin">admin</span></div>`
                      : ''
                  }
                </div>
                <a class="item" role="menuitem" tabindex="0" href="/me" @click=${this._go('/me')}>Profile</a>
                <div class="sep"></div>
                <button class="item danger" role="menuitem" tabindex="0"
                        @click=${() => this._logout()}>Sign out</button>
              </div>
            `
                : null
            }
          </div>
        </div>
      </div>
        ${
          projectItems
            ? html`
          <div class="project-row">
            <div class="inner">
              ${this._projectName ? html`<span class="pname">${this._projectName}</span>` : null}
              <nav class="project" aria-label="Project navigation">
                ${projectItems.map(
                  (it) => html`
                  <a href=${it.href}
                     class=${it.active ? 'active' : ''}
                     aria-current=${it.active ? 'page' : 'false'}
                     @click=${this._go(it.href)}>${it.label}</a>
                `,
                )}
              </nav>
            </div>
          </div>`
            : null
        }
      </header>
    `;
  }
}

customElements.define('nottario-topbar', NottarioTopbar);
