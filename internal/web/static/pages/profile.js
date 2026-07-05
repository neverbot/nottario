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
    // Cross-project API tokens loaded from /api/me/tokens.
    _tokens: { state: true },
    _tokensLoading: { state: true },
    _revoking: { state: true },
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
    .row .value { flex: 1; font-size: 13px; color: var(--fg); }

    /* Notification prefs live inside the shared .surface primitive so
       the block carries the same visual weight as the identity card
       and the token/memberships tables below. Rows separate with a
       hairline; each row is [label + hint muted] on the left and the
       toggle right-aligned on its own column so the labels read as
       full sentences. */
    .prefs.surface {
      padding: 0;
      overflow: hidden;
    }
    .prefs .pref {
      display: grid;
      grid-template-columns: 1fr auto;
      align-items: center;
      gap: 16px;
      padding: 12px 16px;
      border-top: 1px solid var(--gray-2);
      cursor: pointer;
    }
    .prefs .pref:first-child { border-top: none; }
    .prefs .pref:hover { background: var(--bg-subtle); }
    .prefs .pref .label {
      font-size: 14px;
      color: var(--fg);
      font-weight: 500;
    }
    .prefs .pref .hint {
      display: block;
      font-size: 12px;
      color: var(--fg-muted);
      font-weight: 400;
      margin-top: 2px;
    }
    .prefs .pref input[type="checkbox"] {
      accent-color: var(--accent);
      width: 16px;
      height: 16px;
      margin: 0;
    }

    /* Token table: same data-table shape as memberships, tuned columns. */
    table.tokens code {
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 12px;
      color: var(--fg-muted);
      background: var(--gray-2);
      padding: 1px 6px;
      border-radius: 4px;
    }
    table.tokens td.row-actions,
    table.tokens th.row-actions {
      text-align: right;
    }
    table.tokens tr.revoked td { color: var(--fg-muted); }
    table.tokens tr.revoked td a.project-link { color: var(--fg-muted); }
    table.tokens a.project-link {
      color: var(--fg);
      font-weight: 500;
      text-decoration: none;
    }
    table.tokens a.project-link:hover { text-decoration: underline; }
    .badge.revoked {
      background: var(--gray-2);
      color: var(--fg-muted);
      border: 1px solid var(--border);
      padding: 1px 8px;
      border-radius: 999px;
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.04em;
    }
    /* Small ghost button used in the token row's Revoke action. */
    .btn.ghost.sm {
      padding: 4px 10px;
      font-size: 12px;
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
    .empty strong {
      display: block;
      color: var(--fg);
      font-size: 14px;
      margin-bottom: 4px;
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
    this._tokens = null;
    this._tokensLoading = false;
    this._revoking = null;
  }

  connectedCallback() {
    super.connectedCallback();
    this._loadPrefs();
    this._loadTokens();
  }

  async _loadTokens() {
    this._tokensLoading = true;
    try {
      const r = await fetch('/api/me/tokens');
      if (!r.ok) return;
      const j = await r.json();
      this._tokens = j.tokens || [];
    } finally {
      this._tokensLoading = false;
    }
  }

  async _revokeToken(token) {
    // Confirm inline: revoke is destructive across projects and the row
    // is small enough that a modal would be over-engineered. window.
    // confirm is intentionally low-drama here.
    if (!window.confirm(`Revoke "${token.name}" on ${token.project_name}? This cannot be undone.`))
      return;
    this._revoking = token.id;
    try {
      const r = await fetch(`/api/projects/${token.project_id}/tokens/${token.id}`, {
        method: 'DELETE',
      });
      if (r.ok) {
        await this._loadTokens();
      }
    } finally {
      this._revoking = null;
    }
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
          project_id: m.project_id,
          project_slug: m.project_slug,
          project_name: m.project_name,
          roles: [],
        });
      }
      byProject.get(m.project_id).roles.push({
        label: m.role_label,
        color: m.role_color,
        position: m.role_position,
      });
    }
    return Array.from(byProject.values())
      .map((p) => {
        p.roles.sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
        return p;
      })
      .sort((a, b) => (a.project_name || '').localeCompare(b.project_name || ''));
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

        ${
          this._prefsDisabled
            ? null
            : html`
        <h2>Notifications</h2>
        ${
          this._prefs
            ? html`
              <div class="prefs surface">
                ${this._renderPref(
                  'task_assigned',
                  'Someone assigns me to a task',
                  'Fires when another user changes the assignee to you.',
                )}
                ${this._renderPref(
                  'task_commented',
                  "Someone comments on a task I'm assigned to or created",
                  'Excludes your own comments.',
                )}
                ${this._renderPref(
                  'task_closed',
                  "A task I'm assigned to or created is closed",
                  "Fires when someone else transitions it to done or won't do.",
                )}
              </div>
            `
            : html`<div class="empty">Loading…</div>`
        }
        `
        }

        <h2>API tokens</h2>
        ${this._renderTokens()}

        <h2>Session</h2>
        <div class="account-list">
          <div class="row">
            <div class="value">
              <button class="btn danger" @click=${() => this._logout()}>Sign out</button>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  // Row renderer for the Notifications section. Two columns:
  // - Label + hint muted (label reads as a sentence; hint carries the
  //   caveat like "excludes your own comments").
  // - Toggle right-aligned so the eye lands on the label first.
  _renderPref(key, label, hint) {
    return html`
      <label class="pref">
        <span>
          <span class="label">${label}</span>
          <span class="hint">${hint}</span>
        </span>
        <input type="checkbox"
               .checked=${!!this._prefs?.[key]}
               ?disabled=${this._prefsSaving}
               @change=${() => this._togglePref(key)}>
      </label>
    `;
  }

  _renderTokens() {
    if (this._tokens === null) {
      return html`<div class="empty">Loading…</div>`;
    }
    if (this._tokens.length === 0) {
      return html`
        <div class="empty">
          <strong>No API tokens yet.</strong>
          Head to a project's Settings → Tokens to issue one for an agent.
        </div>
      `;
    }
    return html`
      <table class="data-table tokens">
        <thead>
          <tr>
            <th>Name</th>
            <th>Project</th>
            <th>Prefix</th>
            <th>Created</th>
            <th>Last used</th>
            <th class="row-actions"></th>
          </tr>
        </thead>
        <tbody>
          ${this._tokens.map((t) => this._renderTokenRow(t))}
        </tbody>
      </table>
    `;
  }

  _renderTokenRow(t) {
    const created = t.created_at ? new Date(t.created_at).toLocaleDateString() : '';
    const lastUsed = t.last_used_at ? this._relTime(t.last_used_at) : '—';
    const revoked = !!t.revoked_at;
    return html`
      <tr class=${revoked ? 'revoked' : ''}>
        <td>${t.name}</td>
        <td>
          <a class="project-link" href=${`/projects/${t.project_id}`}
             @click=${(e) => {
               e.preventDefault();
               window.nottarioNavigate(`/projects/${t.project_id}`);
             }}>${t.project_name}</a>
        </td>
        <td><code>${t.prefix}…</code></td>
        <td>${created}</td>
        <td>${lastUsed}</td>
        <td class="row-actions">
          ${
            revoked
              ? html`<span class="badge revoked">revoked</span>`
              : html`
              <button class="btn ghost danger sm"
                      ?disabled=${this._revoking === t.id}
                      @click=${() => this._revokeToken(t)}>Revoke</button>
            `
          }
        </td>
      </tr>
    `;
  }

  _relTime(iso) {
    const then = new Date(iso).getTime();
    if (!Number.isFinite(then)) return '';
    const diff = Date.now() - then;
    if (diff < 60_000) return 'just now';
    const m = Math.floor(diff / 60_000);
    if (m < 60) return `${m}m ago`;
    const h = Math.floor(m / 60);
    if (h < 24) return `${h}h ago`;
    const d = Math.floor(h / 24);
    if (d < 7) return `${d}d ago`;
    const w = Math.floor(d / 7);
    if (w < 12) return `${w}w ago`;
    return new Date(iso).toLocaleDateString();
  }
}

customElements.define('nottario-profile-page', NottarioProfilePage);
