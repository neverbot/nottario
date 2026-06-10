import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { PROJECT_VIEWS, viewByKey } from '/static/views.js';
import { buttonStyles } from '/static/components/buttons.js';
import { tableStyles, dialogStyles } from '/static/components/surfaces.js';
import { formStyles } from '/static/components/forms.js';
import { badgeStyles } from '/static/components/badges.js';
import { EscController } from '/static/components/esc.js';
import '/static/components/field.js';
import '/static/components/avatar.js';
import '/static/components/tabs.js';
import '/static/components/page-header.js';

class NottarioProjectSettings extends LitElement {
  static properties = {
    me: { type: Object },
    projectId: { type: String },
    project: { state: true },
    roles: { state: true },
    members: { state: true },
    priorities: { state: true },
    users: { state: true },
    activeTab: { state: true },
    error: { state: true },
    tokens: { state: true },
    showIssueToken: { state: true },
    issuedToken: { state: true },
    tokenError: { state: true },
  };

  static styles = [
    buttonStyles,
    tableStyles,
    dialogStyles,
    formStyles,
    badgeStyles,
    css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }

    .color-dot {
      display: inline-block;
      width: 10px;
      height: 10px;
      border-radius: 50%;
      vertical-align: middle;
      margin-right: 6px;
    }
    .row-actions { text-align: right; }
    .row-actions button { margin-left: 4px; }

    /* Quieter destructive buttons in table rows: at rest a small
       ghost X; armed/hover swaps to the loud red .btn.danger.
       Keeps tables visually calm while still putting the destructive
       affordance one click away. */
    .row-actions .delete {
      width: 26px;
      height: 26px;
      padding: 0;
      font-size: 12px;
      line-height: 1;
      color: #8b949e;
      background: transparent;
      border: 1px solid transparent;
      border-radius: 6px;
      cursor: pointer;
      font: inherit;
    }
    .row-actions .delete:hover {
      color: #cf222e;
      border-color: rgba(207, 34, 46, 0.4);
      background: #ffebe9;
    }
    .row-actions .delete:focus-visible {
      outline: 2px solid #cf222e;
      outline-offset: 1px;
    }
    /* Add-row forms below tables. End-to-end fields aligned with the
       table widths above, labels visible. The 'inline' modifier
       collapses labels to a single line and is used by the role/
       priority add forms; the default stacked form is used for
       multi-line creates. */
    .add-row {
      display: flex;
      gap: 12px;
      margin-top: 16px;
      align-items: flex-end;
      flex-wrap: wrap;
    }
    .add-row .field { margin-bottom: 0; flex: 1; min-width: 120px; }
    .add-row .field.narrow { flex: 0 0 110px; }
    .add-row nottario-field { margin-bottom: 0; flex: 1; min-width: 120px; }
    .add-row nottario-field.narrow { flex: 0 0 110px; }
    .add-row .add-action { display: flex; align-items: center; height: 32px; }

    /* Inline-edit number input inside table cells. Matches the
       .field input chrome so the priorities table doesn't look like
       a different design language. */
    .inline-num {
      width: 84px;
      padding: 4px 8px;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      background: #fff;
      font: inherit;
      font-variant-numeric: tabular-nums;
      box-sizing: border-box;
    }
    .inline-num:focus { outline: 2px solid #0969da; border-color: #0969da; }

    .mono { font-family: ui-monospace, SFMono-Regular, monospace; font-size: 12px; }

    /* Member cells */
    .user-cell { display: flex; align-items: center; gap: 10px; }
    .user-cell .user-text { line-height: 1.3; }
    .user-cell .login { color: #59636e; font-family: ui-monospace, SFMono-Regular, monospace; font-size: 11px; }
    /* project-settings-only override: separation from inline name. */
    .badge.admin { margin-left: 6px; }
    .error { color: #cf222e; margin-bottom: 8px; font-size: 13px; }
    .owner-row {
      display: flex;
      align-items: center;
      gap: 12px;
      margin: 0 0 14px;
      padding: 10px 12px;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      background: #f6f8fa;
    }
    .owner-row .lbl {
      font-weight: 600;
      color: #1f2328;
      min-width: 60px;
    }
    .owner-row select {
      padding: 4px 8px;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      background: #fff;
      font: inherit;
    }
    tr[draggable] { cursor: grab; }
    tr.dragging { opacity: 0.45; }
    tr.drag-over td:first-child { box-shadow: inset 2px 0 0 0 #1f6feb; }
    .drag-handle {
      color: #8c959f;
      cursor: grab;
      user-select: none;
      padding: 0 6px;
      font-family: ui-monospace, monospace;
    }

    /* Tokens tab */
    .tokens-header {
      display: flex;
      align-items: baseline;
      gap: 12px;
      margin-bottom: 14px;
    }
    .tokens-header .spacer { flex: 1; }
    .tokens-header p.helper { margin: 0; }
    .secret-banner {
      background: #fff8c5;
      border: 1px solid #d4a72c;
      border-radius: 6px;
      padding: 12px;
      margin-bottom: 12px;
      color: #7d4e00;
      box-sizing: border-box;
    }
    .secret {
      font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
      padding: 8px 12px;
      background: #f6f8fa;
      border: 1px solid #d1d9e0;
      border-radius: 4px;
      word-break: break-all;
      user-select: all;
      box-sizing: border-box;
      font-size: 12px;
    }
    .snippet {
      font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
      padding: 10px 12px;
      background: #f6f8fa;
      border: 1px solid #d1d9e0;
      border-radius: 6px;
      white-space: pre;
      overflow-x: auto;
      box-sizing: border-box;
      font-size: 12px;
      margin-top: 12px;
    }
    .dialog .panel { width: 560px; }
    .dialog .panel h3 { margin: 0 0 16px 0; }
    .muted { color: #59636e; }
    .status-active { color: #1f883d; font-weight: 500; }
    .status-revoked { color: #59636e; }
  `,
  ];

  constructor() {
    super();
    this.project = null;
    this.roles = [];
    this.members = [];
    this.priorities = [];
    this.users = [];
    this.activeTab = 'general';
    this.error = '';
    this.tokens = null;
    this.showIssueToken = false;
    this.issuedToken = null;
    this.tokenError = '';
    new EscController(this, (e) => {
      if (this.showIssueToken) {
        this._closeTokenDialog();
        e.stopPropagation();
      }
    });
  }

  connectedCallback() {
    super.connectedCallback();
    this.load();
  }

  updated(changed) {
    if (changed.has('projectId')) this.load();
  }

  async load() {
    if (!this.projectId) return;
    try {
      const [pr, rr, mr, qr, ur] = await Promise.all([
        fetch(`/api/projects/${this.projectId}`),
        fetch(`/api/projects/${this.projectId}/roles`),
        fetch(`/api/projects/${this.projectId}/members`),
        fetch(`/api/projects/${this.projectId}/priorities`),
        fetch(`/api/users`),
      ]);
      if (!pr.ok) throw new Error('project not found');
      this.project = await pr.json();
      this.roles = (await rr.json()).roles || [];
      this.members = (await mr.json()).members || [];
      this.priorities = (await qr.json()).priorities || [];
      this.users = ur.ok ? (await ur.json()).users || [] : [];
    } catch (e) {
      this.error = e.message;
    }
  }

  async addRole(e) {
    e.preventDefault();
    const form = e.target;
    const payload = {
      key: form.key.value.trim(),
      label: form.label.value.trim(),
      color: form.color.value.trim(),
    };
    try {
      const res = await fetch(`/api/projects/${this.projectId}/roles`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!res.ok) throw new Error((await res.json()).error || 'failed');
      form.reset();
      await this.load();
    } catch (err) {
      this.error = err.message;
    }
  }

  async deleteRole(id) {
    if (!confirm('Delete this role?')) return;
    try {
      const res = await fetch(`/api/projects/${this.projectId}/roles/${id}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error('delete failed');
      await this.load();
    } catch (err) {
      this.error = err.message;
    }
  }

  async upsertPriority(key, value, position) {
    try {
      const res = await fetch(`/api/projects/${this.projectId}/priorities`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key, value: Number(value), position: Number(position) }),
      });
      if (!res.ok) throw new Error((await res.json()).error || 'failed');
      await this.load();
    } catch (err) {
      this.error = err.message;
    }
  }

  async addPriority(e) {
    e.preventDefault();
    const f = e.target;
    const pos = this.priorities.length;
    await this.upsertPriority(f.key.value.trim(), f.value.value, pos);
    f.reset();
  }

  async deletePriority(key) {
    if (!confirm(`Delete priority bucket "${key}"?`)) return;
    try {
      const res = await fetch(
        `/api/projects/${this.projectId}/priorities/${encodeURIComponent(key)}`,
        {
          method: 'DELETE',
        },
      );
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || 'delete failed');
      await this.load();
    } catch (err) {
      this.error = err.message;
    }
  }

  back() {
    window.nottarioNavigate('/');
  }

  render() {
    if (!this.project) {
      return html`<div>Loading…${this.error ? html`<div class="error">${this.error}</div>` : ''}</div>`;
    }
    const tabs = [
      { id: 'general', label: 'General', body: () => this.renderGeneral() },
      { id: 'roles', label: 'Roles', body: () => this.renderRoles() },
      { id: 'priorities', label: 'Priorities', body: () => this.renderPriorities() },
      { id: 'members', label: 'Members', body: () => this.renderMembers() },
      { id: 'tokens', label: 'Tokens', body: () => this.renderTokens() },
      { id: 'mcp', label: 'MCP', body: () => this.renderMCP() },
    ];
    const active = tabs.find((t) => t.id === this.activeTab) || tabs[0];
    return html`
      <nottario-page-header
        title="Settings"
        .subtitle=${this.project.Slug}>
      </nottario-page-header>
      <nottario-tabs
        .options=${tabs.map((t) => ({ id: t.id, label: t.label }))}
        .value=${active.id}
        @change=${(e) => (this.activeTab = e.detail.value)}>
      </nottario-tabs>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      ${active.body()}
    `;
  }

  renderGeneral() {
    const p = this.project;
    const admin = this.me?.is_admin;
    const currentView = viewByKey(p.DefaultView || 'board/kanban');
    if (!admin) {
      return html`
        <dl>
          <dt><strong>Name</strong></dt><dd>${p.Name}</dd>
          <dt><strong>Description</strong></dt><dd>${p.Description || html`<span class="muted">none</span>`}</dd>
          <dt><strong>Primary language</strong></dt><dd>${p.PrimaryLanguage || html`<span class="muted">none</span>`}</dd>
          <dt><strong>Project type</strong></dt><dd>${p.ProjectType || html`<span class="muted">none</span>`}</dd>
          <dt><strong>Default view</strong></dt><dd>${currentView.label}</dd>
          <dt><strong>Cycle label</strong></dt><dd>${p.CycleLabel || 'sprint'}</dd>
          <dt><strong>Repositories</strong></dt>
          <dd>${
            p.Repos && p.Repos.length
              ? html`<ul style="margin:0;padding-left:18px;font-family:ui-monospace,monospace">${p.Repos.map((r) => html`<li>${r}</li>`)}</ul>`
              : html`<span class="muted">none</span>`
          }</dd>
        </dl>
      `;
    }
    const reposText = (p.Repos || []).join('\n');
    return html`
      <form class="general-form" @submit=${(e) => this.saveGeneral(e)}>
        <nottario-field label="Name">
          <input name="name" required .value=${p.Name}>
        </nottario-field>
        <nottario-field label="Description">
          <input name="description" .value=${p.Description || ''}>
        </nottario-field>
        <div style="display:flex;gap:12px">
          <nottario-field label="Primary language" style="flex:1">
            <input name="primary_language" placeholder="go, typescript, python…"
                   .value=${p.PrimaryLanguage || ''}>
          </nottario-field>
          <nottario-field label="Project type" style="flex:1">
            <input name="project_type" placeholder="web-app, cli-tool, library…"
                   .value=${p.ProjectType || ''}>
          </nottario-field>
        </div>
        <nottario-field label="Default view" hint="where a project card on the home page navigates">
          <select name="default_view" style="max-width:260px">
            ${PROJECT_VIEWS.map(
              (v) => html`
              <option value=${v.key} ?selected=${v.key === currentView.key}>${v.label}</option>
            `,
            )}
          </select>
        </nottario-field>
        <nottario-field label="Cycle label" hint="e.g. sprint, iteration, milestone — used when auto-naming new cycles">
          <input name="cycle_label" .value=${p.CycleLabel || 'sprint'} style="max-width:260px">
        </nottario-field>
        <nottario-field label="Repositories" hint="one per line or comma-separated, format owner/repo">
          <textarea name="repos" rows="3" .value=${reposText}></textarea>
        </nottario-field>
        <div class="actions-row" style="justify-content:flex-end;margin-top:8px">
          <button type="submit" class="btn primary">Save changes</button>
        </div>
      </form>
    `;
  }

  renderMCP() {
    const p = this.project;
    const admin = this.me?.is_admin;
    return html`
      <p class="helper" style="margin:0 0 12px">
        Settings that affect how this project is exposed over the MCP server.
      </p>
      ${
        admin
          ? html`
          <nottario-field label="Default page size for tasks.list" hint="tasks per page" style="max-width:320px">
            <input type="number" min="1" max="500" .value=${String(p.MCPPageSize || 50)}
                   @change=${(e) => this.saveMCPPageSize(e.target.value)}
                   style="width:96px;font-variant-numeric:tabular-nums">
          </nottario-field>`
          : html`
          <p style="margin:0 0 12px">
            <strong>Default page size for tasks.list:</strong>
            ${p.MCPPageSize || 50} tasks per page
            <span class="muted">(admin only)</span>
          </p>`
      }
      <p class="helper" style="margin-top:8px">
        Agents that call <code>nottario.tasks.list</code> without an explicit
        <code>limit</code> get this many tasks per page. Hard range: 1–500.
      </p>
    `;
  }

  async saveGeneral(e) {
    e.preventDefault();
    const f = e.target;
    const payload = {
      name: f.name.value.trim(),
      description: f.description.value.trim(),
      primary_language: f.primary_language.value.trim(),
      project_type: f.project_type.value.trim(),
      default_view: f.default_view.value,
      cycle_label: f.cycle_label.value.trim(),
      repos: f.repos.value
        .split(/\s*,\s*|\n+/)
        .map((s) => s.trim())
        .filter(Boolean),
    };
    try {
      const res = await fetch(`/api/projects/${this.projectId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!res.ok) throw new Error((await res.json()).error || 'failed');
      await this.load();
    } catch (err) {
      this.error = err.message;
    }
  }

  renderRoles() {
    const sorted = [...this.roles].sort((a, b) => (a.Position ?? 0) - (b.Position ?? 0));
    const canDrag = this.me?.is_admin;
    return html`
      ${canDrag ? html`<p class="helper" style="margin:0 0 10px">Drag rows to reorder. Order is shared with the Gantt view.</p>` : null}
      <table class="data-table">
        <thead>
          <tr>
            ${canDrag ? html`<th style="width:24px"></th>` : null}
            <th>Key</th><th>Label</th><th>Color</th><th></th>
          </tr>
        </thead>
        <tbody>
          ${sorted.map(
            (r) => html`
            <tr draggable=${canDrag ? 'true' : 'false'}
                data-id=${r.ID}
                @dragstart=${(e) => this._dragStart(e, r.ID)}
                @dragend=${(e) => this._dragEnd(e)}
                @dragover=${(e) => this._dragOver(e)}
                @dragleave=${(e) => this._dragLeave(e)}
                @drop=${(e) => this._drop(e, r.ID)}>
              ${canDrag ? html`<td class="drag-handle" title="Drag to reorder">⋮⋮</td>` : null}
              <td class="mono">${r.Key}</td>
              <td>${r.Label}</td>
              <td>${
                r.Color
                  ? html`<span class="color-dot" style=${`background:${r.Color}`}></span><span class="mono" style="font-size:11px">${r.Color}</span>`
                  : html`<span class="muted">—</span>`
              }</td>
              <td class="row-actions">
                ${
                  canDrag
                    ? html`<button class="delete" title="Delete role" aria-label="Delete role"
                                          @click=${() => this.deleteRole(r.ID)}>✕</button>`
                    : null
                }
              </td>
            </tr>
          `,
          )}
        </tbody>
      </table>
      ${
        canDrag
          ? html`
        <form class="add-row" @submit=${(e) => this.addRole(e)}>
          <nottario-field label="Key">
            <input name="key" placeholder="backend" required>
          </nottario-field>
          <nottario-field label="Label">
            <input name="label" placeholder="Backend" required>
          </nottario-field>
          <nottario-field label="Color" class="narrow">
            <input name="color" placeholder="#1f6feb">
          </nottario-field>
          <div class="add-action">
            <button type="submit" class="btn primary">Add role</button>
          </div>
        </form>
      `
          : null
      }
    `;
  }

  _dragStart(e, id) {
    this._dragId = id;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', id);
    e.currentTarget.classList.add('dragging');
  }
  _dragEnd(e) {
    e.currentTarget.classList.remove('dragging');
    this.shadowRoot.querySelectorAll('tr.drag-over').forEach((el) => {
      el.classList.remove('drag-over');
    });
  }
  _dragOver(e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    const row = e.currentTarget;
    this.shadowRoot.querySelectorAll('tr.drag-over').forEach((el) => {
      if (el !== row) el.classList.remove('drag-over');
    });
    row.classList.add('drag-over');
  }
  _dragLeave(e) {
    e.currentTarget.classList.remove('drag-over');
  }
  async _drop(e, targetId) {
    e.preventDefault();
    e.currentTarget.classList.remove('drag-over');
    const sourceId = this._dragId;
    if (!sourceId || sourceId === targetId) return;
    const sorted = [...this.roles].sort((a, b) => (a.Position ?? 0) - (b.Position ?? 0));
    const without = sorted.filter((r) => r.ID !== sourceId);
    const targetIdx = without.findIndex((r) => r.ID === targetId);
    const sourceRole = sorted.find((r) => r.ID === sourceId);
    without.splice(targetIdx, 0, sourceRole);
    const ids = without.map((r) => r.ID);
    // Optimistic local update.
    this.roles = without.map((r, i) => ({ ...r, Position: i }));
    try {
      const res = await fetch(`/api/projects/${this.projectId}/roles/reorder`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ role_ids: ids }),
      });
      if (!res.ok) throw new Error('reorder failed');
    } catch (err) {
      this.error = err.message;
      await this.load();
    }
  }

  renderPriorities() {
    const sorted = [...this.priorities].sort(
      (a, b) => a.Position - b.Position || b.Value - a.Value,
    );
    return html`
      <p class="helper" style="margin:0 0 12px">
        Named priority buckets. Tasks store the numeric value; agents pick by key
        (e.g. <code>high</code>) via the MCP. Higher value = pulled first.
      </p>
      <table class="data-table">
        <thead>
          <tr><th>Key</th><th style="width:140px">Value</th><th style="width:60px"></th></tr>
        </thead>
        <tbody>
          ${sorted.map(
            (p) => html`
            <tr>
              <td class="mono">${p.Key}</td>
              <td>
                ${
                  this.me?.is_admin
                    ? html`<input type="number" min="0" max="100" .value=${String(p.Value)}
                          @change=${(e) => this.upsertPriority(p.Key, e.target.value, p.Position)}
                          class="inline-num">`
                    : p.Value
                }
              </td>
              <td class="row-actions">
                ${
                  this.me?.is_admin
                    ? html`<button class="delete" title="Delete priority" aria-label="Delete priority"
                                  @click=${() => this.deletePriority(p.Key)}>✕</button>`
                    : null
                }
              </td>
            </tr>
          `,
          )}
        </tbody>
      </table>
      ${
        this.me?.is_admin
          ? html`
        <form class="add-row" @submit=${(e) => this.addPriority(e)}>
          <nottario-field label="Key">
            <input name="key" placeholder="urgent" required>
          </nottario-field>
          <nottario-field label="Value" class="narrow">
            <input name="value" type="number" min="0" max="100" placeholder="0-100" required>
          </nottario-field>
          <div class="add-action">
            <button type="submit" class="btn primary">Add bucket</button>
          </div>
        </form>
      `
          : null
      }
    `;
  }

  async saveMCPPageSize(value) {
    const n = parseInt(value, 10);
    if (!n || n < 1 || n > 500) {
      this.error = 'page size must be between 1 and 500';
      return;
    }
    try {
      const res = await fetch(`/api/projects/${this.projectId}/mcp`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mcp_page_size: n }),
      });
      if (!res.ok) throw new Error((await res.json()).error || 'failed');
      await this.load();
    } catch (err) {
      this.error = err.message;
    }
  }

  _uniqueMembers() {
    return [...new Map((this.members || []).map((m) => [m.UserID, m])).values()];
  }

  _memberByID(uid) {
    if (!uid) return null;
    return this._uniqueMembers().find((m) => m.UserID === uid) || null;
  }

  async _setOwner(userID) {
    if (!userID) return;
    try {
      const r = await fetch(`/api/projects/${this.projectId}/owner`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ owner_user_id: userID }),
      });
      if (!r.ok) {
        const body = await r.json().catch(() => ({}));
        this.error = body.error || 'failed to set owner';
        return;
      }
      await this.load();
    } catch (err) {
      this.error = err.message;
    }
  }

  renderMembers() {
    const admin = this.me?.is_admin;
    const colCount = admin ? 3 : 2;
    const owner = this._memberByID(this.project.OwnerUserID);
    return html`
      ${
        admin
          ? html`
        <div class="owner-row">
          <span class="lbl">Owner</span>
          <select @change=${(e) => this._setOwner(e.target.value)}>
            ${this._uniqueMembers().map(
              (m) => html`
              <option value=${m.UserID} ?selected=${m.UserID === this.project.OwnerUserID}>
                ${m.DisplayName || m.GithubLogin}
              </option>
            `,
            )}
          </select>
        </div>
      `
          : html`
        <div class="owner-row">
          <span class="lbl">Owner</span>
          <span>${owner ? owner.DisplayName || owner.GithubLogin : html`<span class="muted">—</span>`}</span>
        </div>
      `
      }
      <table class="data-table">
        <thead>
          <tr>
            <th>User</th>
            <th style="width:180px">Role</th>
            ${admin ? html`<th style="width:60px"></th>` : null}
          </tr>
        </thead>
        <tbody>
          ${
            this.members.length === 0
              ? html`<tr><td colspan=${colCount} class="muted" style="text-align:center;padding:16px">No members yet.</td></tr>`
              : this.members.map(
                  (m) => html`
              <tr>
                <td>
                  <div class="user-cell">
                    <nottario-avatar
                      .src=${m.AvatarURL || ''}
                      .name=${m.DisplayName || m.GithubLogin || ''}
                      .size=${28}></nottario-avatar>
                    <div class="user-text">
                      <div>${m.DisplayName || m.GithubLogin}
                        ${m.IsAdmin ? html`<span class="badge admin">admin</span>` : ''}
                      </div>
                      <div class="login">@${m.GithubLogin}</div>
                    </div>
                  </div>
                </td>
                <td>
                  ${m.RoleColor ? html`<span class="color-dot" style=${`background:${m.RoleColor}`}></span>` : ''}
                  ${m.RoleLabel}
                </td>
                ${
                  admin
                    ? html`
                  <td class="row-actions">
                    <button class="delete"
                            title="Remove this role from ${m.DisplayName || m.GithubLogin}"
                            @click=${() => this.removeMember(m.UserID, m.RoleID)}>×</button>
                  </td>
                `
                    : null
                }
              </tr>
            `,
                )
          }
        </tbody>
      </table>
      ${
        admin
          ? html`
        <form class="add-row" @submit=${(e) => this.addMember(e)}>
          <nottario-field label="User">
            <select name="user_id" required>
              <option value="" disabled selected>Pick a user…</option>
              ${this.users.map(
                (u) => html`
                <option value=${u.ID}>${u.DisplayName || u.GithubLogin} (@${u.GithubLogin})</option>
              `,
              )}
            </select>
          </nottario-field>
          <nottario-field label="Role" class="narrow" style="flex:0 0 180px;min-width:180px">
            <select name="role_id" required>
              <option value="" disabled selected>Pick a role…</option>
              ${this.roles.map((r) => html`<option value=${r.ID}>${r.Label}</option>`)}
            </select>
          </nottario-field>
          <div class="add-action">
            <button type="submit" class="btn primary">Add membership</button>
          </div>
        </form>
        <p class="helper" style="margin-top:12px">
          A user can hold multiple roles in a project — add the same user with a different role to grant both. Removing the last role of a user takes them out of the project entirely.
        </p>
      `
          : html`
        <p class="helper" style="margin-top:12px">Only admins can add or remove members.</p>
      `
      }
    `;
  }

  async addMember(e) {
    e.preventDefault();
    const form = e.target;
    const user_id = form.user_id.value;
    const role_id = form.role_id.value;
    if (!user_id || !role_id) return;
    try {
      const res = await fetch(`/api/projects/${this.projectId}/members`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user_id, role_id }),
      });
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || 'failed');
      form.reset();
      await this.load();
    } catch (err) {
      this.error = err.message;
    }
  }

  // ---- Tokens tab ----

  _isMember() {
    if (this.me?.is_admin) return true;
    const ms = this.me?.memberships || [];
    return ms.some((m) => m.ProjectID === this.projectId);
  }

  _fmtDate(d) {
    return d ? new Date(d).toLocaleString() : '—';
  }

  async _loadTokens() {
    if (this.tokens !== null) return; // load once when tab opens
    try {
      const res = await fetch(`/api/projects/${this.projectId}/tokens`);
      if (!res.ok) throw new Error('failed to load tokens');
      this.tokens = (await res.json()).tokens || [];
    } catch (e) {
      this.tokenError = e.message;
      this.tokens = [];
    }
  }

  _openTokenDialog() {
    this.showIssueToken = true;
    this.issuedToken = null;
    this.tokenError = '';
  }

  _closeTokenDialog() {
    this.showIssueToken = false;
    this.issuedToken = null;
    this.tokenError = '';
  }

  async _issueToken(e) {
    e.preventDefault();
    const form = e.target;
    const name = form.name.value.trim();
    const roleVal = form.default_role_id ? form.default_role_id.value : '';
    const body = { name };
    if (roleVal) body.default_role_id = roleVal;
    try {
      const res = await fetch(`/api/projects/${this.projectId}/tokens`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || 'failed');
      this.issuedToken = await res.json();
      // refresh list
      this.tokens = null;
      await this._loadTokens();
    } catch (err) {
      this.tokenError = err.message;
    }
  }

  async _revokeToken(id) {
    if (!confirm('Revoke this token? Agents using it will be locked out immediately.')) return;
    try {
      const res = await fetch(`/api/projects/${this.projectId}/tokens/${id}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error('failed');
      this.tokens = null;
      await this._loadTokens();
    } catch (err) {
      this.tokenError = err.message;
    }
  }

  renderTokens() {
    if (!this._isMember()) {
      return html`
        <div class="empty" style="padding:24px;text-align:center;color:#59636e;background:#fff;border:1px dashed #d0d7de;border-radius:8px">
          Only project members can manage API tokens for this project.
        </div>
      `;
    }
    // Lazy-load on first paint of this tab.
    if (this.tokens === null) {
      this._loadTokens();
      return html`<div class="muted">Loading tokens…</div>`;
    }
    return html`
      <div class="tokens-header">
        <p class="helper">
          API tokens authenticate agents (MCP clients) against this project. Each token is scoped to this project only.
        </p>
        <div class="spacer"></div>
        <button class="btn primary" @click=${() => this._openTokenDialog()}>New token</button>
      </div>
      ${this.tokenError ? html`<div class="error">${this.tokenError}</div>` : null}
      <table class="data-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Prefix</th>
            <th>Created</th>
            <th>Last used</th>
            <th>Status</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          ${
            this.tokens.length === 0
              ? html`<tr><td colspan="6" class="muted" style="text-align:center;padding:24px">No tokens yet.</td></tr>`
              : this.tokens.map(
                  (t) => html`
              <tr>
                <td>${t.Name}</td>
                <td class="mono">${t.Prefix}…</td>
                <td>${this._fmtDate(t.CreatedAt)}</td>
                <td>${this._fmtDate(t.LastUsedAt)}</td>
                <td>${
                  t.RevokedAt
                    ? html`<span class="status-revoked">revoked</span>`
                    : html`<span class="status-active">active</span>`
                }</td>
                <td class="row-actions">
                  ${
                    t.RevokedAt
                      ? null
                      : html`
                    <button class="delete" title="Revoke token" aria-label="Revoke token"
                            @click=${() => this._revokeToken(t.ID)}>✕</button>`
                  }
                </td>
              </tr>
            `,
                )
          }
        </tbody>
      </table>
      ${this.showIssueToken ? this._renderTokenDialog() : null}
    `;
  }

  _renderTokenDialog() {
    if (this.issuedToken) {
      const secret = this.issuedToken.plaintext;
      // Build the snippet from the URL the user is currently viewing
      // this page on — that's the same origin they (and their agent)
      // will use to reach the MCP. Works for localhost dev, VPN-only
      // self-hosts and public Traefik deployments without per-instance
      // config.
      const snippet = `claude mcp add nottario ${window.location.origin}/mcp \\
  --transport http \\
  --header "Authorization: Bearer ${secret}" \\
  --scope local`;
      return html`
        <div class="dialog">
          <div class="panel">
            <h3>Token issued</h3>
            <div class="secret-banner">
              <strong>Copy this token now.</strong> It will not be shown again.
            </div>
            <div class="secret">${secret}</div>
            <p class="helper" style="margin:14px 0 4px">Install in Claude Code:</p>
            <div class="snippet">${snippet}</div>
            <div class="actions-row" style="margin-top:16px;display:flex;gap:8px;justify-content:flex-end">
              <button class="btn secondary"
                      @click=${() => navigator.clipboard.writeText(secret)}>Copy token</button>
              <button class="btn secondary"
                      @click=${() => navigator.clipboard.writeText(snippet)}>Copy snippet</button>
              <button class="btn primary" @click=${() => this._closeTokenDialog()}>Done</button>
            </div>
          </div>
        </div>
      `;
    }
    return html`
      <div class="dialog">
        <div class="panel">
          <h3>New API token</h3>
          ${this.tokenError ? html`<div class="error">${this.tokenError}</div>` : null}
          <form @submit=${(e) => this._issueToken(e)}>
            <nottario-field label="Name" hint="so you remember which agent uses it">
              <input name="name" required autofocus placeholder="laptop, ci-runner, …">
            </nottario-field>
            <nottario-field label="Default role" hint="optional — used when the agent doesn't specify one">
              <select name="default_role_id">
                <option value="">(none)</option>
                ${this.roles.map((r) => html`<option value=${r.ID}>${r.Label}</option>`)}
              </select>
            </nottario-field>
            <div class="actions-row" style="margin-top:16px;display:flex;gap:8px;justify-content:flex-end">
              <button type="button" class="btn secondary"
                      @click=${() => this._closeTokenDialog()}>Cancel</button>
              <button type="submit" class="btn primary">Issue</button>
            </div>
          </form>
        </div>
      </div>
    `;
  }

  async removeMember(userID, roleID) {
    if (!confirm('Remove this membership?')) return;
    try {
      const res = await fetch(`/api/projects/${this.projectId}/members/${userID}/${roleID}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || 'remove failed');
      await this.load();
    } catch (err) {
      this.error = err.message;
    }
  }
}

customElements.define('nottario-project-settings', NottarioProjectSettings);
