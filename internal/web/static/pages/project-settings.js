import { LitElement, html, css } from '/static/vendor/lit/lit.js';

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

  static styles = css`
    :host { display: block; }
    .header {
      display: flex;
      align-items: baseline;
      gap: 16px;
      margin-bottom: 16px;
    }
    .header h2 { margin: 0; }
    .tabs {
      border-bottom: 1px solid #d1d9e0;
      margin-bottom: 16px;
      display: flex;
      gap: 4px;
    }
    .tab {
      padding: 8px 16px;
      background: transparent;
      border: none;
      border-bottom: 2px solid transparent;
      cursor: pointer;
      color: #59636e;
    }
    .tab.active {
      color: #1f2328;
      border-bottom-color: #fd8c73;
      font-weight: 500;
    }
    .panel {
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      padding: 16px;
    }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 8px 12px; text-align: left; border-bottom: 1px solid #eaeef2; }
    th { font-size: 12px; text-transform: uppercase; color: #59636e; }
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
    .add-row { display: flex; gap: 8px; margin-top: 12px; align-items: center; }
    .add-row input { flex: 1; }
    .error { color: #cf222e; margin-bottom: 8px; font-size: 13px; }
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
  `;

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
      return html`<div class="panel">Loading…${this.error ? html`<div class="error">${this.error}</div>` : ''}</div>`;
    }
    return html`
      <div class="header">
        <button @click=${() => this.back()}>← Back</button>
        <h2>${this.project.Name}</h2>
        <span class="muted">${this.project.Slug}</span>
      </div>
      <div class="tabs">
        ${['general', 'roles', 'priorities', 'members'].map(t => html`
          <button class="tab ${this.activeTab === t ? 'active' : ''}"
                  @click=${() => this.activeTab = t}>
            ${t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        `)}
      </div>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      <div class="panel">
        ${this.activeTab === 'general' ? this.renderGeneral() : null}
        ${this.activeTab === 'roles' ? this.renderRoles() : null}
        ${this.activeTab === 'priorities' ? this.renderPriorities() : null}
        ${this.activeTab === 'members' ? this.renderMembers() : null}
      </div>
    `;
  }

  renderGeneral() {
    const p = this.project;
    return html`
      <dl>
        <dt><strong>Name</strong></dt><dd>${p.Name}</dd>
        <dt><strong>Description</strong></dt><dd>${p.Description || html`<span class="muted">none</span>`}</dd>
        <dt><strong>Primary language</strong></dt><dd>${p.PrimaryLanguage || html`<span class="muted">none</span>`}</dd>
        <dt><strong>Project type</strong></dt><dd>${p.ProjectType || html`<span class="muted">none</span>`}</dd>
        <dt><strong>Repositories</strong></dt>
        <dd>${p.Repos && p.Repos.length
              ? html`<ul style="margin:0;padding-left:18px;font-family:ui-monospace,monospace">${p.Repos.map(r => html`<li>${r}</li>`)}</ul>`
              : html`<span class="muted">none</span>`}</dd>
      </dl>
    `;
  }

  renderRoles() {
    const sorted = [...this.roles].sort((a, b) => (a.Position ?? 0) - (b.Position ?? 0));
    const canDrag = this.me?.is_admin;
    return html`
      ${canDrag ? html`<p class="muted" style="margin:0 0 8px">Drag rows to reorder. Order is shared with the Gantt view.</p>` : null}
      <table>
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
              <td>${r.Color ? html`<span class="color-dot" style=${`background:${r.Color}`}></span>${r.Color}` : ''}</td>
              <td class="row-actions">
                ${canDrag ? html`<button class="danger" @click=${() => this.deleteRole(r.ID)}>Delete</button>` : null}
              </td>
            </tr>
          `)}
        </tbody>
      </table>
      ${canDrag ? html`
        <form class="add-row" @submit=${(e) => this.addRole(e)}>
          <input name="key" placeholder="key (snake-case)" required>
          <input name="label" placeholder="Label" required>
          <input name="color" placeholder="#hex" style="max-width:90px">
          <button type="submit" class="primary">Add role</button>
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
      <p class="muted" style="margin:0 0 12px">
        Named priority buckets. Tasks store the numeric value; agents pick by key
        (e.g. <code>high</code>) via the MCP. Higher value = pulled first.
      </p>
      <table>
        <thead>
          <tr><th>Key</th><th>Value</th><th></th></tr>
        </thead>
        <tbody>
          ${sorted.map(p => html`
            <tr>
              <td class="mono">${p.Key}</td>
              <td>
                ${this.me?.is_admin
                  ? html`<input type="number" min="0" max="100" .value=${String(p.Value)}
                          @change=${(e) => this.upsertPriority(p.Key, e.target.value, p.Position)}
                          style="width:80px">`
                  : p.Value}
              </td>
              <td class="row-actions">
                ${this.me?.is_admin
                  ? html`<button class="danger" @click=${() => this.deletePriority(p.Key)}>Delete</button>`
                  : null}
              </td>
            </tr>
          `)}
        </tbody>
      </table>
      ${this.me?.is_admin ? html`
        <form class="add-row" @submit=${(e) => this.addPriority(e)}>
          <input name="key" placeholder="key (e.g. urgent)" required>
          <input name="value" type="number" min="0" max="100" placeholder="value (0-100)" required style="max-width:140px">
          <button type="submit" class="primary">Add bucket</button>
        </form>
      ` : null}
    `;
  }

  renderMembers() {
    return html`
      <table>
        <thead>
          <tr><th>User</th><th>Role</th></tr>
        </thead>
        <tbody>
          ${this.members.length === 0
            ? html`<tr><td colspan="2" class="muted" style="text-align:center;padding:16px">No members yet.</td></tr>`
            : this.members.map(m => html`
              <tr>
                <td>
                  ${m.AvatarURL ? html`<img src=${m.AvatarURL} alt="" style="width:20px;height:20px;border-radius:50%;vertical-align:middle;margin-right:6px">` : ''}
                  ${m.DisplayName} <span class="muted">@${m.GithubLogin}</span>
                  ${m.IsAdmin ? html`<span class="badge admin" style="margin-left:6px">admin</span>` : ''}
                </td>
                <td>
                  ${m.RoleColor ? html`<span class="color-dot" style=${`background:${m.RoleColor}`}></span>` : ''}
                  ${m.RoleLabel}
                </td>
              </tr>
            `)}
        </tbody>
      </table>
      <p class="muted" style="margin-top:12px">Adding members from the UI is coming in a later milestone. For now, the first user who logs in is admin; other users self-register via GitHub OAuth on first login and an admin can grant them roles via the API.</p>
    `;
  }
}

customElements.define('nottario-project-settings', NottarioProjectSettings);
