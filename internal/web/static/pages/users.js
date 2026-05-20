import { LitElement, html, css } from '/static/vendor/lit/lit.js';

class NottarioUsersPage extends LitElement {
  static properties = {
    me: { type: Object },
    users: { state: true },
    filter: { state: true },
    error: { state: true },
  };

  static styles = css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }
    .header { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; }
    h2 { margin: 0; }
    .spacer { flex: 1; }
    input.filter {
      width: 240px;
      padding: 6px 10px;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      font: inherit;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      overflow: hidden;
    }
    th, td {
      text-align: left;
      padding: 10px 14px;
      border-bottom: 1px solid #eaeef2;
      font-size: 13px;
      vertical-align: middle;
    }
    tbody tr:last-child td { border-bottom: none; }
    th {
      background: #f6f8fa;
      font-weight: 600;
      color: #59636e;
      text-transform: uppercase;
      font-size: 11px;
      letter-spacing: 0.04em;
    }
    .user-cell {
      display: flex;
      align-items: center;
      gap: 10px;
    }
    .avatar {
      width: 28px;
      height: 28px;
      border-radius: 50%;
      object-fit: cover;
      background: #d0d7de;
      flex: 0 0 auto;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      font-size: 11px;
      font-weight: 600;
      color: #fff;
      text-transform: uppercase;
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
    .empty {
      padding: 32px;
      text-align: center;
      color: #59636e;
      background: #fff;
      border: 1px dashed #d1d9e0;
      border-radius: 8px;
    }
    .error { color: #cf222e; font-size: 13px; margin-bottom: 8px; }
  `;

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

  _initials(name) {
    if (!name) return '?';
    const parts = name.trim().split(/\s+/).slice(0, 2);
    return parts.map(p => p.charAt(0)).join('');
  }

  render() {
    if (this.users === null) {
      return html`<div class="empty">Loading users…</div>`;
    }
    const rows = this._filtered();
    return html`
      <div class="header">
        <h2>Users</h2>
        <span class="muted">${this.users.length} ${this.users.length === 1 ? 'user' : 'users'}</span>
        <div class="spacer"></div>
        <input class="filter" type="search" placeholder="Filter by name or login…"
               .value=${this.filter}
               @input=${(e) => this.filter = e.target.value}>
      </div>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      ${rows.length === 0
        ? html`<div class="empty">No users match the filter.</div>`
        : html`
          <table>
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
                      ${u.AvatarURL
                        ? html`<img class="avatar" src=${u.AvatarURL} alt="">`
                        : html`<span class="avatar">${this._initials(u.DisplayName)}</span>`}
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
