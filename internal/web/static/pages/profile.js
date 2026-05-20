import { LitElement, html, css } from '/static/vendor/lit/lit.js';

class NottarioProfilePage extends LitElement {
  static properties = {
    me: { type: Object },
    error: { state: true },
  };

  static styles = css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }

    .wrap {
      max-width: 720px;
      margin: 0 auto;
      padding: 0 4px 48px;
    }

    h2 {
      font-size: 14px;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: #59636e;
      margin: 28px 0 10px;
      padding-bottom: 6px;
      border-bottom: 1px solid #eaeef2;
      font-weight: 600;
    }
    h2:first-of-type { margin-top: 0; }

    .identity {
      display: flex;
      align-items: center;
      gap: 16px;
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 10px;
      padding: 18px 20px;
      box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04);
    }
    .identity .avatar {
      width: 56px;
      height: 56px;
      border-radius: 50%;
      object-fit: cover;
      background: #d0d7de;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      font-size: 20px;
      font-weight: 600;
      color: #fff;
    }
    .identity .name { font-size: 20px; font-weight: 600; margin: 0; }
    .identity .login {
      color: #59636e;
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 13px;
    }
    .identity .meta-line {
      color: #59636e;
      font-size: 12px;
      margin-top: 6px;
    }
    .badge.admin {
      display: inline-block;
      font-size: 11px;
      font-weight: 600;
      padding: 1px 7px;
      border-radius: 999px;
      background: #fff8c5;
      color: #9a6700;
      border: 1px solid #eac54f;
      margin-left: 8px;
      vertical-align: 2px;
    }

    table.memberships {
      width: 100%;
      border-collapse: separate;
      border-spacing: 0;
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      overflow: hidden;
      box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04);
    }
    table.memberships th,
    table.memberships td {
      text-align: left;
      padding: 9px 14px;
      border-bottom: 1px solid #eaeef2;
      font-size: 13px;
      vertical-align: middle;
    }
    table.memberships tbody tr:last-child td { border-bottom: none; }
    table.memberships th {
      background: #f6f8fa;
      font-weight: 600;
      color: #59636e;
      text-transform: uppercase;
      font-size: 11px;
      letter-spacing: 0.04em;
    }
    table.memberships .roles {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
    }
    table.memberships .role {
      display: inline-block;
      padding: 1px 8px;
      border-radius: 999px;
      border: 1px solid #d0d7de;
      background: #fff;
      font-size: 12px;
    }
    table.memberships a.project-link {
      color: #1f2328;
      font-weight: 500;
      text-decoration: none;
    }
    table.memberships a.project-link:hover { text-decoration: underline; }

    .settings-list,
    .account-list {
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      overflow: hidden;
    }
    .row {
      display: flex;
      align-items: center;
      gap: 16px;
      padding: 12px 16px;
      border-bottom: 1px solid #eaeef2;
    }
    .row:last-child { border-bottom: none; }
    .row .label {
      flex: 0 0 180px;
      font-weight: 500;
      color: #1f2328;
      font-size: 14px;
    }
    .row .value { flex: 1; font-size: 13px; color: #1f2328; }
    .row .value.muted { color: #59636e; font-style: italic; }
    .row a.tokens-link {
      color: #0969da;
      text-decoration: none;
    }
    .row a.tokens-link:hover { text-decoration: underline; }
    .row button.signout {
      padding: 6px 14px;
      border: 1px solid #cf222e;
      color: #cf222e;
      background: #fff;
      border-radius: 6px;
      cursor: pointer;
      font: inherit;
      font-weight: 500;
      font-size: 13px;
    }
    .row button.signout:hover { background: #ffebe9; }

    .empty {
      padding: 18px 16px;
      color: #59636e;
      font-size: 13px;
      background: #fff;
      border: 1px dashed #d0d7de;
      border-radius: 8px;
    }
    .error { color: #cf222e; margin-bottom: 12px; font-size: 13px; }
  `;

  constructor() {
    super();
    this.error = '';
  }

  _initials(name) {
    if (!name) return '?';
    const parts = name.trim().split(/\s+/).slice(0, 2);
    return parts.map(p => p.charAt(0)).join('');
  }

  _groupedMemberships() {
    const memberships = this.me?.memberships || [];
    const byProject = new Map();
    for (const m of memberships) {
      if (!byProject.has(m.ProjectID)) {
        byProject.set(m.ProjectID, {
          ProjectID: m.ProjectID,
          ProjectSlug: m.ProjectSlug,
          ProjectName: m.ProjectName,
          roles: [],
        });
      }
      byProject.get(m.ProjectID).roles.push({
        Label: m.RoleLabel,
        Color: m.RoleColor,
        Position: m.RolePosition,
      });
    }
    return Array.from(byProject.values()).map(p => {
      p.roles.sort((a, b) => (a.Position ?? 0) - (b.Position ?? 0));
      return p;
    }).sort((a, b) => a.ProjectName.localeCompare(b.ProjectName));
  }

  async _logout() {
    await fetch('/auth/logout', { method: 'POST' });
    window.nottarioNavigate('/');
  }

  render() {
    if (!this.me) {
      return html`<div class="wrap"><div class="empty">Loading profile…</div></div>`;
    }
    const me = this.me;
    const memberships = this._groupedMemberships();
    const memberSince = me.created_at
      ? new Date(me.created_at).toLocaleDateString(undefined,
          { year: 'numeric', month: 'long', day: 'numeric' })
      : null;
    return html`
      <div class="wrap">
        ${this.error ? html`<div class="error">${this.error}</div>` : null}

        <h2>Profile</h2>
        <div class="identity">
          ${me.avatar_url
            ? html`<img class="avatar" src=${me.avatar_url} alt="">`
            : html`<span class="avatar">${this._initials(me.display_name)}</span>`}
          <div>
            <div class="name">
              ${me.display_name || me.github_login}
              ${me.is_admin ? html`<span class="badge admin">admin</span>` : ''}
            </div>
            <div class="login">@${me.github_login}</div>
            ${memberSince
              ? html`<div class="meta-line">Member since ${memberSince}</div>`
              : null}
          </div>
        </div>

        <h2>Project memberships</h2>
        ${memberships.length === 0
          ? html`<div class="empty">You don't belong to any project yet.</div>`
          : html`
            <table class="memberships">
              <thead>
                <tr><th>Project</th><th>Roles</th></tr>
              </thead>
              <tbody>
                ${memberships.map(p => html`
                  <tr>
                    <td>
                      <a class="project-link" href=${`/projects/${p.ProjectID}`}
                         @click=${(e) => { e.preventDefault(); window.nottarioNavigate(`/projects/${p.ProjectID}`); }}>${p.ProjectName}</a>
                    </td>
                    <td>
                      <div class="roles">
                        ${p.roles.map(r => html`
                          <span class="role"
                                style=${r.Color ? `border-color:${r.Color}; color:${r.Color}` : ''}>${r.Label}</span>
                        `)}
                      </div>
                    </td>
                  </tr>
                `)}
              </tbody>
            </table>
          `}

        <h2>Settings</h2>
        <div class="settings-list">
          <div class="row">
            <div class="label">Theme</div>
            <div class="value muted">Coming soon.</div>
          </div>
          <div class="row">
            <div class="label">Notifications</div>
            <div class="value muted">Coming with the per-user notifications feature.</div>
          </div>
          <div class="row">
            <div class="label">Default landing</div>
            <div class="value muted">Coming soon.</div>
          </div>
        </div>

        <h2>Account</h2>
        <div class="account-list">
          <div class="row">
            <div class="label">API tokens</div>
            <div class="value">
              <a class="tokens-link" href="/tokens"
                 @click=${(e) => { e.preventDefault(); window.nottarioNavigate('/tokens'); }}>Manage your API tokens →</a>
            </div>
          </div>
          <div class="row">
            <div class="label">Session</div>
            <div class="value">
              <button class="signout" @click=${() => this._logout()}>Sign out</button>
            </div>
          </div>
        </div>
      </div>
    `;
  }
}

customElements.define('nottario-profile-page', NottarioProfilePage);
