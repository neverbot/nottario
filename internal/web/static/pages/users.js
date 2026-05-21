import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { buttonStyles } from '/static/components/buttons.js';
import { surfaceStyles, tableStyles } from '/static/components/surfaces.js';
import '/static/components/avatar.js';
import '/static/components/page-header.js';

class NottarioUsersPage extends LitElement {
  static properties = {
    me: { type: Object },
    users: { state: true },
    filter: { state: true },
    error: { state: true },
  };

  static styles = [buttonStyles, surfaceStyles, tableStyles, css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }
    .spacer { flex: 1; }
    input.filter {
      width: 240px;
      padding: 6px 10px;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      font: inherit;
    }
    .user-cell {
      display: flex;
      align-items: center;
      gap: 10px;
    }
    .login { color: #59636e; font-family: ui-monospace, SFMono-Regular, monospace; font-size: 12px; }
    .badge.admin {
      display: inline-block;
      font-size: 11px;
      font-weight: 600;
      padding: 1px 6px;
      border-radius: 999px;
      background: #fff8c5;
      color: #9a6700;
      border: 1px solid #eac54f;
    }
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
        <input slot="actions" class="filter" type="search" placeholder="Filter by name or login…"
               .value=${this.filter}
               @input=${(e) => this.filter = e.target.value}>
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
