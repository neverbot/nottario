import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { buttonStyles } from '/static/components/buttons.js';
import { surfaceStyles, tableStyles } from '/static/components/surfaces.js';
import { badgeStyles } from '/static/components/badges.js';
import '/static/components/avatar.js';
import '/static/components/page-header.js';

class NottarioProfilePage extends LitElement {
  static properties = {
    me: { type: Object },
    error: { state: true },
    // Notification prefs live inline; loaded on mount, PATCH on toggle.
    _prefs: { state: true },
    _prefsSaving: { state: true },
    _prefsDisabled: { state: true },
  };

  static styles = [
    buttonStyles,
    surfaceStyles,
    tableStyles,
    badgeStyles,
    css`
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
      color: var(--fg-muted);
      margin: 28px 0 10px;
      padding-bottom: 6px;
      border-bottom: 1px solid var(--gray-2);
      font-weight: 600;
    }
    h2:first-of-type { margin-top: 0; }

    .identity {
      display: flex;
      align-items: center;
      gap: 16px;
      background: #fff;
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 18px 20px;
      box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04);
    }
    .identity .name { font-size: 20px; font-weight: 600; margin: 0; }
    .identity .login {
      color: var(--fg-muted);
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 13px;
    }
    .identity .meta-line {
      color: var(--fg-muted);
      font-size: 12px;
      margin-top: 6px;
    }
    /* profile-only override: extra left margin when stacked next to name. */
    .badge.admin { margin-left: 8px; }

    table.memberships .roles {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
    }
    table.memberships .role {
      display: inline-block;
      padding: 1px 8px;
      border-radius: 999px;
      border: 1px solid var(--border);
      background: #fff;
      font-size: 12px;
    }
    table.memberships a.project-link {
      color: var(--fg);
      font-weight: 500;
      text-decoration: none;
    }
    table.memberships a.project-link:hover { text-decoration: underline; }

    .settings-list,
    .account-list {
      background: #fff;
      border: 1px solid var(--border);
      border-radius: 8px;
      overflow: hidden;
    }
    .row {
      display: flex;
      align-items: center;
      gap: 16px;
      padding: 12px 16px;
      border-bottom: 1px solid var(--gray-2);
    }
    .row:last-child { border-bottom: none; }
    .row .label {
      flex: 0 0 180px;
      font-weight: 500;
      color: var(--fg);
      font-size: 14px;
    }
    .row .value { flex: 1; font-size: 13px; color: var(--fg); }
    .row .value.muted { color: var(--fg-muted); font-style: italic; }
    .row a.tokens-link {
      color: var(--accent);
      text-decoration: none;
    }
    .row a.tokens-link:hover { text-decoration: underline; }
    .prefs {
      display: flex;
      flex-direction: column;
      gap: 6px;
    }
    .prefs .pref {
      display: flex;
      align-items: center;
      gap: 8px;
      font-size: 13px;
      color: var(--fg);
    }
    .prefs .pref input[type="checkbox"] {
      accent-color: var(--accent);
    }
    /* sign-out reuses the shared .btn.danger from components/buttons.js */

    .empty {
      padding: 18px 16px;
      color: var(--fg-muted);
      font-size: 13px;
      background: #fff;
      border: 1px dashed var(--border);
      border-radius: 8px;
    }
    .error { color: var(--danger); margin-bottom: 12px; font-size: 13px; }
  `,
  ];

  constructor() {
    super();
    this.error = '';
    this._prefs = null;
    this._prefsSaving = false;
    this._prefsDisabled = false;
  }

  connectedCallback() {
    super.connectedCallback();
    this._loadPrefs();
  }

  async _loadPrefs() {
    try {
      // Piggyback on the unread_count endpoint to detect the kill
      // switch; we don't want to render toggles if the whole feature
      // is off. When the poller reports disabled: true we hide the
      // prefs block entirely so the surface matches the bell.
      const cr = await fetch('/api/notifications/unread_count');
      if (cr.ok) {
        const j = await cr.json();
        if (j.disabled) {
          this._prefsDisabled = true;
          return;
        }
      }
      const r = await fetch('/api/me/notification_preferences');
      if (!r.ok) return;
      this._prefs = await r.json();
    } catch (_) {
      // Silent: keep the placeholder legible until the request lands.
    }
  }

  async _togglePref(key) {
    if (this._prefsSaving || !this._prefs) return;
    const next = { ...this._prefs, [key]: !this._prefs[key] };
    this._prefsSaving = true;
    try {
      const r = await fetch('/api/me/notification_preferences', {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ [key]: next[key] }),
      });
      if (!r.ok) return;
      this._prefs = await r.json();
    } finally {
      this._prefsSaving = false;
    }
  }

  _groupedMemberships() {
    const memberships = this.me?.memberships || [];
    const byProject = new Map();
    for (const m of memberships) {
      if (!byProject.has(m.project_id)) {
        byProject.set(m.project_id, {
          ProjectID: m.project_id,
          ProjectSlug: m.project_slug,
          ProjectName: m.project_name,
          roles: [],
        });
      }
      byProject.get(m.project_id).roles.push({
        Label: m.role_label,
        Color: m.role_color,
        Position: m.role_position,
      });
    }
    return Array.from(byProject.values())
      .map((p) => {
        p.roles.sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
        return p;
      })
      .sort((a, b) => a.project_name.localeCompare(b.project_name));
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
      ? new Date(me.created_at).toLocaleDateString(undefined, {
          year: 'numeric',
          month: 'long',
          day: 'numeric',
        })
      : null;
    return html`
      <div class="wrap">
        <nottario-page-header
          title="Your account"
          .subtitle=${me.github_login ? `@${me.github_login}` : ''}>
        </nottario-page-header>
        ${this.error ? html`<div class="error">${this.error}</div>` : null}

        <h2>Profile</h2>
        <div class="identity">
          <nottario-avatar
            .src=${me.avatar_url || ''}
            .name=${me.display_name || me.github_login || ''}
            .size=${56}></nottario-avatar>
          <div>
            <div class="name">
              ${me.display_name || me.github_login}
              ${me.is_admin ? html`<span class="badge admin">admin</span>` : ''}
            </div>
            <div class="login">@${me.github_login}</div>
            ${memberSince ? html`<div class="meta-line">Member since ${memberSince}</div>` : null}
          </div>
        </div>

        <h2>Project memberships</h2>
        ${
          memberships.length === 0
            ? html`<div class="empty">You don't belong to any project yet.</div>`
            : html`
            <table class="data-table memberships">
              <thead>
                <tr><th>Project</th><th>Roles</th></tr>
              </thead>
              <tbody>
                ${memberships.map(
                  (p) => html`
                  <tr>
                    <td>
                      <a class="project-link" href=${`/projects/${p.project_id}`}
                         @click=${(e) => {
                           e.preventDefault();
                           window.nottarioNavigate(`/projects/${p.project_id}`);
                         }}>${p.project_name}</a>
                    </td>
                    <td>
                      <div class="roles">
                        ${p.roles.map(
                          (r) => html`
                          <span class="role"
                                style=${r.color ? `border-color:${r.color}; color:${r.color}` : ''}>${r.label}</span>
                        `,
                        )}
                      </div>
                    </td>
                  </tr>
                `,
                )}
              </tbody>
            </table>
          `
        }

        <h2>Settings</h2>
        <div class="settings-list">
          <div class="row">
            <div class="label">Theme</div>
            <div class="value muted">Coming soon.</div>
          </div>
          ${
            this._prefsDisabled
              ? null
              : html`
          <div class="row">
            <div class="label">Notifications</div>
            <div class="value">
              ${
                this._prefs
                  ? html`
                <div class="prefs">
                  <label class="pref">
                    <input type="checkbox"
                           .checked=${!!this._prefs.task_assigned}
                           ?disabled=${this._prefsSaving}
                           @change=${() => this._togglePref('task_assigned')}>
                    Someone assigns me to a task
                  </label>
                  <label class="pref">
                    <input type="checkbox"
                           .checked=${!!this._prefs.task_commented}
                           ?disabled=${this._prefsSaving}
                           @change=${() => this._togglePref('task_commented')}>
                    Someone comments on a task I'm assigned to or created
                  </label>
                  <label class="pref">
                    <input type="checkbox"
                           .checked=${!!this._prefs.task_closed}
                           ?disabled=${this._prefsSaving}
                           @change=${() => this._togglePref('task_closed')}>
                    A task I'm assigned to or created is closed
                  </label>
                </div>
              `
                  : html`<span class="muted">Loading…</span>`
              }
            </div>
          </div>
          `
          }
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
              <a class="tokens-link" href="/"
                 @click=${(e) => {
                   e.preventDefault();
                   window.nottarioNavigate('/');
                 }}>Manage tokens in a project's Settings →</a>
            </div>
          </div>
          <div class="row">
            <div class="label">Session</div>
            <div class="value">
              <button class="btn danger" @click=${() => this._logout()}>Sign out</button>
            </div>
          </div>
        </div>
      </div>
    `;
  }
}

customElements.define('nottario-profile-page', NottarioProfilePage);
