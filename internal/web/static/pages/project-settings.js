import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { PROJECT_VIEWS, viewByKey } from '/static/views.js';
import { buttonStyles } from '/static/components/buttons.js';
import { tableStyles } from '/static/components/surfaces.js';
import '/static/components/page-header.js';

class NottarioProjectSettings extends LitElement {
  static properties = {
    me: { type: Object },
    projectId: { type: String },
    project: { state: true },
    roles: { state: true },
    members: { state: true },
    priorities: { state: true },
    activeTab: { state: true },
    error: { state: true },
  };

  static styles = [buttonStyles, tableStyles, css`
    :host { display: block; box-sizing: border-box; }
    * { box-sizing: border-box; }

    /* Tabs: four entries. Active tab gets a thin coloured underline.
       The underline colour stays muted (orange) so it never reads as
       a primary CTA. */
    .tabs {
      display: flex;
      gap: 4px;
      margin-bottom: 16px;
      border-bottom: 1px solid #d1d9e0;
    }
    .tab {
      padding: 8px 14px;
      background: transparent;
      border: none;
      border-bottom: 2px solid transparent;
      cursor: pointer;
      color: #59636e;
      font: inherit;
      font-size: 13px;
      font-weight: 500;
      margin-bottom: -1px;
    }
    .tab:hover { color: #1f2328; }
    .tab.active {
      color: #1f2328;
      border-bottom-color: #ff8c42;
    }
    .tab:focus-visible {
      outline: 2px solid #0969da;
      outline-offset: 2px;
      border-radius: 4px;
    }

    /* Hide the browser-native number spinner everywhere on this page.
       It's a system-styled control that clashes with our chrome, and
       GitHub-likes hide it by convention. The keyboard (↑/↓) and
       typing still work. */
    input[type="number"]::-webkit-inner-spin-button,
    input[type="number"]::-webkit-outer-spin-button {
      -webkit-appearance: none;
      appearance: none;
      margin: 0;
    }
    input[type="number"] { -moz-appearance: textfield; }
    .helper {
      color: #59636e;
      font-size: 12px;
      margin: 0;
    }
    .helper code {
      font-family: ui-monospace, SFMono-Regular, monospace;
      background: #f6f8fa;
      padding: 0 4px;
      border-radius: 3px;
      font-size: 11px;
    }

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
    .row-actions .delete:hover,
    .row-actions .delete:focus-visible {
      color: #cf222e;
      border-color: rgba(207, 34, 46, 0.4);
      background: #ffebe9;
      outline: none;
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
    .member-avatar {
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
    .badge.admin {
      display: inline-block;
      font-size: 11px;
      font-weight: 600;
      padding: 1px 6px;
      border-radius: 999px;
      background: #fff8c5;
      color: #9a6700;
      border: 1px solid #eac54f;
      margin-left: 6px;
      vertical-align: 2px;
    }
    .error { color: #cf222e; margin-bottom: 8px; font-size: 13px; }
    .field { margin-bottom: 12px; }
    .field label {
      display: block;
      margin-bottom: 4px;
      font-weight: 500;
      font-size: 13px;
      color: #1f2328;
    }
    .field input,
    .field textarea,
    .field select {
      width: 100%;
      padding: 6px 10px;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      font: inherit;
      background: #fff;
      box-sizing: border-box;
    }
    .field input:focus,
    .field textarea:focus,
    .field select:focus {
      outline: 2px solid #0969da;
      outline-offset: 0;
      border-color: #0969da;
    }
    .field textarea { resize: vertical; min-height: 60px; font-family: ui-monospace, SFMono-Regular, monospace; font-size: 12px; }
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
  `];

  constructor() {
    super();
    this.project = null;
    this.roles = [];
    this.members = [];
    this.priorities = [];
    this.activeTab = 'general';
    this.error = '';
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
      const [pr, rr, mr, qr] = await Promise.all([
        fetch(`/api/projects/${this.projectId}`),
        fetch(`/api/projects/${this.projectId}/roles`),
        fetch(`/api/projects/${this.projectId}/members`),
        fetch(`/api/projects/${this.projectId}/priorities`),
      ]);
      if (!pr.ok) throw new Error('project not found');
      this.project = await pr.json();
      this.roles = (await rr.json()).roles || [];
      this.members = (await mr.json()).members || [];
      this.priorities = (await qr.json()).priorities || [];
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
    } catch (err) { this.error = err.message; }
  }

  async deleteRole(id) {
    if (!confirm('Delete this role?')) return;
    try {
      const res = await fetch(`/api/projects/${this.projectId}/roles/${id}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error('delete failed');
      await this.load();
    } catch (err) { this.error = err.message; }
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
    } catch (err) { this.error = err.message; }
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
      const res = await fetch(`/api/projects/${this.projectId}/priorities/${encodeURIComponent(key)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || 'delete failed');
      await this.load();
    } catch (err) { this.error = err.message; }
  }

  back() { window.nottarioNavigate('/'); }

  render() {
    if (!this.project) {
      return html`<div>Loading…${this.error ? html`<div class="error">${this.error}</div>` : ''}</div>`;
    }
    const tabs = [
      { id: 'general',    label: 'General',    body: () => this.renderGeneral() },
      { id: 'roles',      label: 'Roles',      body: () => this.renderRoles() },
      { id: 'priorities', label: 'Priorities', body: () => this.renderPriorities() },
      { id: 'members',    label: 'Members',    body: () => this.renderMembers() },
      { id: 'mcp',        label: 'MCP',        body: () => this.renderMCP() },
    ];
    const active = tabs.find(t => t.id === this.activeTab) || tabs[0];
    return html`
      <nottario-page-header
        title="Settings"
        .subtitle=${this.project.Slug}>
      </nottario-page-header>
      <div class="tabs" role="tablist">
        ${tabs.map(t => html`
          <button class=${'tab' + (t.id === active.id ? ' active' : '')}
                  role="tab"
                  aria-selected=${t.id === active.id ? 'true' : 'false'}
                  @click=${() => this.activeTab = t.id}>${t.label}</button>
        `)}
      </div>
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
          <dt><strong>Repositories</strong></dt>
          <dd>${p.Repos && p.Repos.length
                ? html`<ul style="margin:0;padding-left:18px;font-family:ui-monospace,monospace">${p.Repos.map(r => html`<li>${r}</li>`)}</ul>`
                : html`<span class="muted">none</span>`}</dd>
        </dl>
      `;
    }
    const reposText = (p.Repos || []).join('\n');
    return html`
      <form class="general-form" @submit=${(e) => this.saveGeneral(e)}>
        <div class="field">
          <label>Name</label>
          <input name="name" required .value=${p.Name}>
        </div>
        <div class="field">
          <label>Description</label>
          <input name="description" .value=${p.Description || ''}>
        </div>
        <div class="field" style="display:flex;gap:12px">
          <div style="flex:1">
            <label>Primary language</label>
            <input name="primary_language" placeholder="go, typescript, python…"
                   .value=${p.PrimaryLanguage || ''}>
          </div>
          <div style="flex:1">
            <label>Project type</label>
            <input name="project_type" placeholder="web-app, cli-tool, library…"
                   .value=${p.ProjectType || ''}>
          </div>
        </div>
        <div class="field">
          <label>Default view <span class="muted" style="font-weight:400">where a project card on the home page navigates</span></label>
          <select name="default_view" style="max-width:260px">
            ${PROJECT_VIEWS.map(v => html`
              <option value=${v.key} ?selected=${v.key === currentView.key}>${v.label}</option>
            `)}
          </select>
        </div>
        <div class="field">
          <label>Repositories <span class="muted" style="font-weight:400">one per line or comma-separated, format owner/repo</span></label>
          <textarea name="repos" rows="3" .value=${reposText}></textarea>
        </div>
        <div class="actions-row" style="display:flex;justify-content:flex-end;gap:8px;margin-top:8px">
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
      <div class="field" style="max-width:320px">
        <label>Default page size for <code>tasks.list</code></label>
        ${admin
          ? html`
            <div style="display:flex;align-items:center;gap:8px">
              <input type="number" min="1" max="500" .value=${String(p.MCPPageSize || 50)}
                     @change=${(e) => this.saveMCPPageSize(e.target.value)}
                     style="width:96px;font-variant-numeric:tabular-nums">
              <span class="muted">tasks per page</span>
            </div>`
          : html`${p.MCPPageSize || 50} tasks per page <span class="muted">(admin only)</span>`}
      </div>
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
      repos: f.repos.value.split(/\s*,\s*|\n+/).map(s => s.trim()).filter(Boolean),
    };
    try {
      const res = await fetch(`/api/projects/${this.projectId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (!res.ok) throw new Error((await res.json()).error || 'failed');
      await this.load();
    } catch (err) { this.error = err.message; }
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
          ${sorted.map(r => html`
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
              <td>${r.Color
                    ? html`<span class="color-dot" style=${`background:${r.Color}`}></span><span class="mono" style="font-size:11px">${r.Color}</span>`
                    : html`<span class="muted">—</span>`}</td>
              <td class="row-actions">
                ${canDrag ? html`<button class="delete" title="Delete role" aria-label="Delete role"
                                          @click=${() => this.deleteRole(r.ID)}>✕</button>` : null}
              </td>
            </tr>
          `)}
        </tbody>
      </table>
      ${canDrag ? html`
        <form class="add-row" @submit=${(e) => this.addRole(e)}>
          <div class="field">
            <label>Key</label>
            <input name="key" placeholder="backend" required>
          </div>
          <div class="field">
            <label>Label</label>
            <input name="label" placeholder="Backend" required>
          </div>
          <div class="field narrow">
            <label>Color</label>
            <input name="color" placeholder="#1f6feb">
          </div>
          <div class="add-action">
            <button type="submit" class="btn primary">Add role</button>
          </div>
        </form>
      ` : null}
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
    this.shadowRoot.querySelectorAll('tr.drag-over').forEach(el => el.classList.remove('drag-over'));
  }
  _dragOver(e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    const row = e.currentTarget;
    this.shadowRoot.querySelectorAll('tr.drag-over').forEach(el => el !== row && el.classList.remove('drag-over'));
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
    const without = sorted.filter(r => r.ID !== sourceId);
    const targetIdx = without.findIndex(r => r.ID === targetId);
    const sourceRole = sorted.find(r => r.ID === sourceId);
    without.splice(targetIdx, 0, sourceRole);
    const ids = without.map(r => r.ID);
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
    const sorted = [...this.priorities].sort((a, b) => (a.Position - b.Position) || (b.Value - a.Value));
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
          ${sorted.map(p => html`
            <tr>
              <td class="mono">${p.Key}</td>
              <td>
                ${this.me?.is_admin
                  ? html`<input type="number" min="0" max="100" .value=${String(p.Value)}
                          @change=${(e) => this.upsertPriority(p.Key, e.target.value, p.Position)}
                          class="inline-num">`
                  : p.Value}
              </td>
              <td class="row-actions">
                ${this.me?.is_admin
                  ? html`<button class="delete" title="Delete priority" aria-label="Delete priority"
                                  @click=${() => this.deletePriority(p.Key)}>✕</button>`
                  : null}
              </td>
            </tr>
          `)}
        </tbody>
      </table>
      ${this.me?.is_admin ? html`
        <form class="add-row" @submit=${(e) => this.addPriority(e)}>
          <div class="field">
            <label>Key</label>
            <input name="key" placeholder="urgent" required>
          </div>
          <div class="field narrow">
            <label>Value</label>
            <input name="value" type="number" min="0" max="100" placeholder="0-100" required>
          </div>
          <div class="add-action">
            <button type="submit" class="btn primary">Add bucket</button>
          </div>
        </form>
      ` : null}
    `;
  }

  async saveMCPPageSize(value) {
    const n = parseInt(value, 10);
    if (!n || n < 1 || n > 500) { this.error = 'page size must be between 1 and 500'; return; }
    try {
      const res = await fetch(`/api/projects/${this.projectId}/mcp`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mcp_page_size: n }),
      });
      if (!res.ok) throw new Error((await res.json()).error || 'failed');
      await this.load();
    } catch (err) { this.error = err.message; }
  }

  renderMembers() {
    return html`
      <table class="data-table">
        <thead>
          <tr><th>User</th><th style="width:180px">Role</th></tr>
        </thead>
        <tbody>
          ${this.members.length === 0
            ? html`<tr><td colspan="2" class="muted" style="text-align:center;padding:16px">No members yet.</td></tr>`
            : this.members.map(m => html`
              <tr>
                <td>
                  <div class="user-cell">
                    ${m.AvatarURL
                      ? html`<img class="member-avatar" src=${m.AvatarURL} alt="">`
                      : html`<span class="member-avatar fallback">${this._initials(m.DisplayName || m.GithubLogin)}</span>`}
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
              </tr>
            `)}
        </tbody>
      </table>
      <p class="helper" style="margin-top:12px">Adding members from the UI is coming in a later milestone. For now, the first user who logs in is admin; other users self-register via GitHub OAuth on first login and an admin can grant them roles via the API.</p>
    `;
  }

  _initials(name) {
    if (!name) return '?';
    const parts = name.trim().split(/\s+/).slice(0, 2);
    return parts.map(p => p.charAt(0)).join('');
  }
}

customElements.define('nottario-project-settings', NottarioProjectSettings);
