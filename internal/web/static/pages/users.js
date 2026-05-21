import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { buttonStyles } from '/static/components/buttons.js';
import { surfaceStyles, tableStyles } from '/static/components/surfaces.js';
import { badgeStyles } from '/static/components/badges.js';
import '/static/components/avatar.js';
import '/static/components/page-header.js';
import '/static/components/search-input.js';

class NottarioUsersPage extends LitElement {
  static properties = {
    me: { type: Object },
    users: { state: true },
    filter: { state: true },
    error: { state: true },
  };

  static styles = [buttonStyles, surfaceStyles, tableStyles, badgeStyles, css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }
    .spacer { flex: 1; }
    nottario-search-input { width: 240px; }
    .user-cell {
      display: flex;
      align-items: center;
      gap: 10px;
    }
    .login { color: #59636e; font-family: ui-monospace, SFMono-Regular, monospace; font-size: 12px; }
    .muted { color: #59636e; }
    .error { color: #cf222e; font-size: 13px; margin-bottom: 8px; }
  `];

  constructor() {
    super();
    this.users = null;
    this.filter = '';
    this.error = '';
  }

  connectedCallback() {
    super.connectedCallback();
    this.load();
  }

  async load() {
    try {
      const r = await fetch('/api/users');
      if (!r.ok) throw new Error('failed to load users');
      this.users = (await r.json()).users || [];
    } catch (e) {
      this.error = e.message;
      this.users = [];
    }
  }

  _filtered() {
    if (!this.filter) return this.users;
    const q = this.filter.toLowerCase();
    return this.users.filter(u =>
      (u.DisplayName || '').toLowerCase().includes(q) ||
      (u.GithubLogin || '').toLowerCase().includes(q));
  }


  render() {
    if (this.users === null) {
      return html`<div class="empty">Loading users…</div>`;
    }
    const rows = this._filtered();
    return html`
      <nottario-page-header
        title="Users"
        .subtitle=${`${this.users.length} ${this.users.length === 1 ? 'user' : 'users'}`}>
        <nottario-search-input slot="actions"
            placeholder="Filter by name or login…"
            .value=${this.filter}
            @input=${(e) => this.filter = e.detail.value}
            @clear=${() => this.filter = ''}></nottario-search-input>
      </nottario-page-header>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      ${rows.length === 0
        ? html`<div class="empty">No users match the filter.</div>`
        : html`
          <table class="data-table">
            <thead>
              <tr>
                <th>User</th>
                <th>Member since</th>
                <th>Projects</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              ${rows.map(u => html`
                <tr>
                  <td>
                    <div class="user-cell">
                      <nottario-avatar
                        .src=${u.AvatarURL || ''}
                        .name=${u.DisplayName || u.GithubLogin || ''}
                        .size=${28}></nottario-avatar>
                      <div>
                        <div>${u.DisplayName || u.GithubLogin}</div>
                        <div class="login">@${u.GithubLogin}</div>
                      </div>
                    </div>
                  </td>
                  <td>${new Date(u.CreatedAt).toLocaleDateString()}</td>
                  <td>${u.ProjectCount}</td>
                  <td>${u.IsAdmin ? html`<span class="badge admin">admin</span>` : ''}</td>
                </tr>
              `)}
            </tbody>
          </table>
        `}
    `;
  }
}

customElements.define('nottario-users-page', NottarioUsersPage);
