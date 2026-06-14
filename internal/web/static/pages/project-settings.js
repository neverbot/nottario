import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { PROJECT_VIEWS, viewByKey } from '/static/views.js';
import { buttonStyles } from '/static/components/buttons.js';
import { tableStyles, dialogStyles } from '/static/components/surfaces.js';
import { formStyles } from '/static/components/forms.js';
import { badgeStyles } from '/static/components/badges.js';
import { EscController } from '/static/components/esc.js';
import { toast } from '/static/components/toast.js';
import { formButton } from '/static/components/form-button.js';
import { confirm } from '/static/components/confirm-dialog.js';
import { tableActionStyles } from '/static/components/table-actions.js';
import { addRowStyles } from '/static/components/add-row.js';
import '/static/components/color-swatches.js';
import { BRAND_ROLE_PALETTE } from '/static/components/color-swatches.js';
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
    // Inline-confirm UX for the destructive Revoke action: id of the
    // token currently armed (Yes/Cancel shown in the row), or null.
    _revokeArmedId: { state: true },
    // Which "Copy …" button most recently fired its post-click ack
    // flash. Cleared on a timeout from the click handler.
    _copyAck: { state: true },
    // Currently-picked colour in the add-role swatch grid; null means
    // "use the suggested default" (first palette entry not yet in use).
    _addRoleColor: { state: true },
    // In-place row editor for an existing role: id of the row in
    // edit mode (only one at a time), plus draft label / colour. All
    // null when no row is being edited.
    _editingRoleId: { state: true },
    _editRoleLabel: { state: true },
    _editRoleColor: { state: true },
  };

  static styles = [
    buttonStyles,
    tableStyles,
    dialogStyles,
    formStyles,
    badgeStyles,
    tableActionStyles,
    addRowStyles,
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
    /* Role row in edit mode: inline form (label input + Save/Cancel)
       in the Label cell, the swatch picker in the Color cell. The
       form is a real <form> so Enter submits and formButton can ack
       on Save. */
    .role-edit-row .role-edit-form {
      display: flex;
      gap: 8px;
      align-items: center;
    }
    .role-edit-row .role-edit-form input[type="text"] {
      flex: 1;
      min-width: 0;
      padding: 4px 8px;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: var(--bg);
      font: inherit;
    }
    .role-edit-row .role-edit-form input[type="text"]:focus-visible {
      outline: 2px solid var(--accent);
      border-color: var(--accent);
    }
    .role-edit-row .role-edit-form .btn {
      padding: 4px 10px;
      font-size: 13px;
    }

    /* Inline-edit number input inside table cells. Matches the
       .field input chrome so the priorities table doesn't look like
       a different design language. */
    .inline-num {
      width: 84px;
      padding: 4px 8px;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: #fff;
      font: inherit;
      font-variant-numeric: tabular-nums;
      box-sizing: border-box;
    }
    .inline-num:focus { outline: 2px solid var(--accent); border-color: var(--accent); }

    .mono { font-family: ui-monospace, SFMono-Regular, monospace; font-size: 12px; }

    /* Member cells */
    .user-cell { display: flex; align-items: center; gap: 10px; }
    .user-cell .user-text { line-height: 1.3; }
    .user-cell .login { color: var(--fg-muted); font-family: ui-monospace, SFMono-Regular, monospace; font-size: 11px; }
    /* project-settings-only override: separation from inline name. */
    .badge.admin { margin-left: 6px; }
    .error { color: var(--danger); margin-bottom: 8px; font-size: 13px; }
    .owner-row {
      display: flex;
      align-items: center;
      gap: 12px;
      margin: 0 0 14px;
      padding: 10px 12px;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: var(--bg-subtle);
    }
    .owner-row .lbl {
      font-weight: 600;
      color: var(--fg);
      min-width: 60px;
    }
    .owner-row select {
      padding: 4px 8px;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: #fff;
      font: inherit;
    }
    tr[draggable] { cursor: grab; }
    tr.dragging { opacity: 0.45; }
    tr.drag-over td:first-child { box-shadow: inset 2px 0 0 0 var(--brand-blue); }
    .drag-handle {
      color: var(--gray-5);
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
      background: var(--tint-yellow);
      border: 1px solid var(--badge-warning-border);
      border-radius: 6px;
      padding: 12px;
      margin-bottom: 12px;
      color: var(--warning-text);
      box-sizing: border-box;
    }
    .secret {
      font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
      padding: 8px 12px;
      background: var(--bg-subtle);
      border: 1px solid var(--border);
      border-radius: 4px;
      word-break: break-all;
      user-select: all;
      box-sizing: border-box;
      font-size: 12px;
    }
    .snippet {
      font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
      padding: 10px 12px;
      background: var(--bg-subtle);
      border: 1px solid var(--border);
      border-radius: 6px;
      white-space: pre;
      overflow-x: auto;
      box-sizing: border-box;
      font-size: 12px;
      margin-top: 12px;
    }
    /* Default panel sits at the form's tighter 420; the issued-secret
       state widens to 560 (qualified by the .panel.wide class) so the
       install snippet doesn't wrap mid-flag. */
    .dialog .panel { width: 420px; }
    .dialog .panel.wide { width: 560px; }
    .dialog .panel h3 { margin: 0 0 16px 0; }
    .muted { color: var(--fg-muted); }
    /* Status pill: a small dot + label so the column scans at a
       glance in a list of many tokens. The dot carries the
       semantics; the text is the AA fallback. */
    .status {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      font-weight: 500;
    }
    .status::before {
      content: '';
      display: inline-block;
      width: 7px;
      height: 7px;
      border-radius: 50%;
      background: currentColor;
    }
    .status.active { color: var(--success); }
    .status.revoked { color: var(--fg-muted); font-weight: 400; }
    /* Inline revoke confirm: replaces the native window.confirm()
       so the confirm flow stays inside the table row, on-brand and
       scannable. */
    .revoke-confirm {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      font-size: 12px;
      color: var(--fg-muted);
    }
    .revoke-confirm strong { color: var(--fg); font-weight: 500; }
    .revoke-confirm button {
      padding: 2px 8px;
      font-size: 12px;
      line-height: 1.4;
      border-radius: 4px;
    }
    .revoke-confirm .yes {
      background: var(--danger);
      color: var(--fg-on-accent);
      border-color: var(--danger);
    }
    .revoke-confirm .yes:hover { background: var(--danger-hover); }
    /* Non-member empty state — tokenised. */
    .tokens-locked {
      padding: 24px;
      text-align: center;
      color: var(--fg-muted);
      background: var(--bg);
      border: 1px dashed var(--border);
      border-radius: 8px;
    }
    /* Copy-success ack: the same button briefly swaps to a
       check-mark + "Copied" so the user knows the click landed. */
    .copy-ack { color: var(--success); font-weight: 600; }
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
    this._revokeArmedId = null;
    this._copyAck = null;
    this._addRoleColor = null;
    this._editingRoleId = null;
    this._editRoleLabel = '';
    this._editRoleColor = '';
    new EscController(this, (e) => {
      if (this.showIssueToken) {
        this._closeTokenDialog();
        e.stopPropagation();
        return;
      }
      if (this._editingRoleId) {
        this._cancelRoleEdit();
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

  // Default for the add-role form: first palette colour no existing
  // role is using, falling back to the first if every colour is
  // taken. The palette itself lives in components/color-swatches.js.
  _nextRoleColor() {
    const used = new Set((this.roles || []).map((r) => (r.color || '').toLowerCase()));
    return BRAND_ROLE_PALETTE.find((c) => !used.has(c)) || BRAND_ROLE_PALETTE[0];
  }

  async addRole(e) {
    const form = e.target;
    const payload = {
      key: form.key.value.trim(),
      label: form.label.value.trim(),
      color: form.color.value.trim(),
    };
    try {
      await formButton(e, async () => {
        const res = await fetch(`/api/projects/${this.projectId}/roles`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        if (!res.ok) throw new Error((await res.json()).error || 'failed');
        form.reset();
        this._addRoleColor = null; // re-suggest the next free palette colour
        await this.load();
      });
    } catch (err) {
      this.error = err.message;
      toast.error(`Couldn't add role: ${err.message}`);
    }
  }

  _renderRoleRowRest(r, canDrag) {
    return html`
      <tr draggable=${canDrag ? 'true' : 'false'}
          data-id=${r.id}
          @dragstart=${(e) => this._dragStart(e, r.id)}
          @dragend=${(e) => this._dragEnd(e)}
          @dragover=${(e) => this._dragOver(e)}
          @dragleave=${(e) => this._dragLeave(e)}
          @drop=${(e) => this._drop(e, r.id)}>
        ${canDrag ? html`<td class="drag-handle" title="Drag to reorder">⋮⋮</td>` : null}
        <td class="mono">${r.key}</td>
        <td>${r.label}</td>
        <td>
          ${
            r.color
              ? html`<span class="color-dot" style=${`background:${r.color}`}></span><span class="mono" style="font-size:11px">${r.color}</span>`
              : html`<span class="muted">—</span>`
          }
        </td>
        <td class="row-actions">
          ${
            canDrag
              ? html`
              <button class="edit" title="Edit role" aria-label="Edit role"
                      @click=${() => this._startRoleEdit(r)}>
                <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                  <path d="M11.5 1.5L14.5 4.5L5 14H2V11L11.5 1.5Z"
                        stroke="currentColor" stroke-width="1.4"
                        stroke-linejoin="round" stroke-linecap="round"/>
                </svg>
              </button>
              <button class="delete" title="Delete role" aria-label="Delete role"
                      @click=${() => this.deleteRole(r.id)}>✕</button>`
              : null
          }
        </td>
      </tr>
    `;
  }

  _renderRoleRowEditing(r) {
    // The form spans the label cell only — Save and Cancel live in
    // the row-actions cell where Edit / Delete used to sit, so the
    // editing affordance doesn't fragment the row's left-to-right
    // alignment. The form id wires submit-from-outside the form.
    const formId = `edit-role-${r.id}`;
    return html`
      <tr class="role-edit-row" data-id=${r.id}>
        <td class="drag-handle" aria-hidden="true">⋮⋮</td>
        <td class="mono">${r.key}</td>
        <td>
          <form id=${formId}
                @submit=${(e) => this.updateRole(e, r.id)}
                class="role-edit-form">
            <input type="text"
                   .value=${this._editRoleLabel}
                   @input=${(e) => (this._editRoleLabel = e.target.value)}
                   aria-label="Role label"
                   required
                   autofocus>
          </form>
        </td>
        <td>
          <nottario-color-swatches
            .value=${this._editRoleColor}
            aria-label="Role colour"
            @change=${(e) => (this._editRoleColor = e.detail.value)}>
          </nottario-color-swatches>
        </td>
        <td class="row-actions row-actions-edit">
          <button type="submit" form=${formId}
                  class="btn primary edit-save">Save</button>
          <button type="button" class="btn"
                  @click=${() => this._cancelRoleEdit()}>Cancel</button>
        </td>
      </tr>
    `;
  }

  _startRoleEdit(role) {
    this._editingRoleId = role.id;
    this._editRoleLabel = role.label || '';
    this._editRoleColor = role.color || BRAND_ROLE_PALETTE[0];
  }

  _cancelRoleEdit() {
    this._editingRoleId = null;
    this._editRoleLabel = '';
    this._editRoleColor = '';
  }

  async updateRole(e, id) {
    const payload = {
      label: this._editRoleLabel.trim(),
      color: this._editRoleColor.trim(),
    };
    if (!payload.label) {
      toast.error('Role label is required.');
      e.preventDefault();
      return;
    }
    try {
      await formButton(e, async () => {
        const res = await fetch(`/api/projects/${this.projectId}/roles/${id}`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        if (!res.ok) {
          throw new Error((await res.json().catch(() => ({}))).error || 'failed');
        }
        this._cancelRoleEdit();
        await this.load();
      });
      toast.success('Role updated.');
    } catch (err) {
      this.error = err.message;
      toast.error(`Couldn't update role: ${err.message}`);
    }
  }

  async deleteRole(id) {
    const ok = await confirm({
      title: 'Delete this role?',
      body: 'Tasks targeting this role will lose their role assignment.',
      confirmLabel: 'Delete',
      danger: true,
    });
    if (!ok) return;
    try {
      const res = await fetch(`/api/projects/${this.projectId}/roles/${id}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error('delete failed');
      await this.load();
      toast.success('Role removed.');
    } catch (err) {
      this.error = err.message;
      toast.error(`Couldn't remove role: ${err.message}`);
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
      throw err;
    }
  }

  async addPriority(e) {
    const f = e.target;
    const pos = this.priorities.length;
    try {
      await formButton(e, async () => {
        await this.upsertPriority(f.key.value.trim(), f.value.value, pos);
        f.reset();
      });
    } catch (err) {
      toast.error(`Couldn't add priority: ${err.message}`);
    }
  }

  async deletePriority(key) {
    const ok = await confirm({
      title: `Delete priority "${key}"?`,
      body: 'Tasks using this bucket will keep their numeric priority but the bucket name disappears.',
      confirmLabel: 'Delete',
      danger: true,
    });
    if (!ok) return;
    try {
      const res = await fetch(
        `/api/projects/${this.projectId}/priorities/${encodeURIComponent(key)}`,
        {
          method: 'DELETE',
        },
      );
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || 'delete failed');
      await this.load();
      toast.success(`Priority "${key}" removed.`);
    } catch (err) {
      this.error = err.message;
      toast.error(`Couldn't remove priority: ${err.message}`);
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
        .subtitle=${this.project.slug}>
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
    const currentView = viewByKey(p.default_view || 'board/kanban');
    if (!admin) {
      return html`
        <dl>
          <dt><strong>Name</strong></dt><dd>${p.name}</dd>
          <dt><strong>Description</strong></dt><dd>${p.description || html`<span class="muted">none</span>`}</dd>
          <dt><strong>Primary language</strong></dt><dd>${p.primary_language || html`<span class="muted">none</span>`}</dd>
          <dt><strong>Project type</strong></dt><dd>${p.project_type || html`<span class="muted">none</span>`}</dd>
          <dt><strong>Default view</strong></dt><dd>${currentView.label}</dd>
          <dt><strong>Cycle label</strong></dt><dd>${p.cycle_label || 'sprint'}</dd>
          <dt><strong>Repositories</strong></dt>
          <dd>${
            p.repos && p.repos.length
              ? html`<ul style="margin:0;padding-left:18px;font-family:ui-monospace,monospace">${p.repos.map((r) => html`<li>${r}</li>`)}</ul>`
              : html`<span class="muted">none</span>`
          }</dd>
        </dl>
      `;
    }
    const reposText = (p.repos || []).join('\n');
    return html`
      <form class="general-form" @submit=${(e) => this.saveGeneral(e)}>
        <nottario-field label="Name">
          <input name="name" required .value=${p.name}>
        </nottario-field>
        <nottario-field label="Description">
          <input name="description" .value=${p.description || ''}>
        </nottario-field>
        <div style="display:flex;gap:12px">
          <nottario-field label="Primary language" style="flex:1">
            <input name="primary_language" placeholder="go, typescript, python…"
                   .value=${p.primary_language || ''}>
          </nottario-field>
          <nottario-field label="Project type" style="flex:1">
            <input name="project_type" placeholder="web-app, cli-tool, library…"
                   .value=${p.project_type || ''}>
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
          <input name="cycle_label" .value=${p.cycle_label || 'sprint'} style="max-width:260px">
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
            <input type="number" min="1" max="500" .value=${String(p.mcp_page_size || 50)}
                   @change=${(e) => this.saveMCPPageSize(e.target.value)}
                   style="width:96px;font-variant-numeric:tabular-nums">
          </nottario-field>`
          : html`
          <p style="margin:0 0 12px">
            <strong>Default page size for tasks.list:</strong>
            ${p.mcp_page_size || 50} tasks per page
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
      await formButton(e, async () => {
        const res = await fetch(`/api/projects/${this.projectId}`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        if (!res.ok) throw new Error((await res.json()).error || 'failed');
        await this.load();
      });
    } catch (err) {
      this.error = err.message;
      toast.error(`Couldn't save: ${err.message}`);
    }
  }

  renderRoles() {
    const sorted = [...this.roles].sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
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
          ${sorted.map((r) =>
            this._editingRoleId === r.id
              ? this._renderRoleRowEditing(r)
              : this._renderRoleRowRest(r, canDrag),
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
          <nottario-field label="Color" class="auto">
            <input name="color" type="hidden" .value=${this._addRoleColor || this._nextRoleColor()}>
            <nottario-color-swatches
              .value=${this._addRoleColor || this._nextRoleColor()}
              aria-label="Role colour"
              @change=${(e) => (this._addRoleColor = e.detail.value)}>
            </nottario-color-swatches>
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
    const sorted = [...this.roles].sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
    const without = sorted.filter((r) => r.id !== sourceId);
    const targetIdx = without.findIndex((r) => r.id === targetId);
    const sourceRole = sorted.find((r) => r.id === sourceId);
    without.splice(targetIdx, 0, sourceRole);
    const ids = without.map((r) => r.id);
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
      (a, b) => a.position - b.position || b.value - a.value,
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
              <td class="mono">${p.key}</td>
              <td>
                ${
                  this.me?.is_admin
                    ? html`<input type="number" min="0" max="100" .value=${String(p.value)}
                          @change=${(e) => this.upsertPriority(p.key, e.target.value, p.position)}
                          class="inline-num">`
                    : p.value
                }
              </td>
              <td class="row-actions">
                ${
                  this.me?.is_admin
                    ? html`<button class="delete" title="Delete priority" aria-label="Delete priority"
                                  @click=${() => this.deletePriority(p.key)}>✕</button>`
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
      // Input @change handler — no button to ack on, so toast.
      toast.success('MCP page size saved.');
    } catch (err) {
      this.error = err.message;
      toast.error(`Couldn't save: ${err.message}`);
    }
  }

  _uniqueMembers() {
    return [...new Map((this.members || []).map((m) => [m.user_id, m])).values()];
  }

  _memberByID(uid) {
    if (!uid) return null;
    return this._uniqueMembers().find((m) => m.user_id === uid) || null;
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
        toast.error(`Couldn't set owner: ${this.error}`);
        return;
      }
      await this.load();
      const newOwner = this._memberByID(userID);
      toast.success(`Owner set to ${newOwner?.display_name || newOwner?.github_login || 'user'}.`);
    } catch (err) {
      this.error = err.message;
      toast.error(`Couldn't set owner: ${err.message}`);
    }
  }

  renderMembers() {
    const admin = this.me?.is_admin;
    const colCount = admin ? 3 : 2;
    const owner = this._memberByID(this.project.owner_user_id);
    return html`
      ${
        admin
          ? html`
        <div class="owner-row">
          <span class="lbl">Owner</span>
          <select @change=${(e) => this._setOwner(e.target.value)}>
            ${this._uniqueMembers().map(
              (m) => html`
              <option value=${m.user_id} ?selected=${m.user_id === this.project.owner_user_id}>
                ${m.display_name || m.github_login}
              </option>
            `,
            )}
          </select>
        </div>
      `
          : html`
        <div class="owner-row">
          <span class="lbl">Owner</span>
          <span>${owner ? owner.display_name || owner.github_login : html`<span class="muted">—</span>`}</span>
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
                      .src=${m.avatar_url || ''}
                      .name=${m.display_name || m.github_login || ''}
                      .size=${28}></nottario-avatar>
                    <div class="user-text">
                      <div>${m.display_name || m.github_login}
                        ${m.is_admin ? html`<span class="badge admin">admin</span>` : ''}
                      </div>
                      <div class="login">@${m.github_login}</div>
                    </div>
                  </div>
                </td>
                <td>
                  ${m.role_color ? html`<span class="color-dot" style=${`background:${m.role_color}`}></span>` : ''}
                  ${m.role_label}
                </td>
                ${
                  admin
                    ? html`
                  <td class="row-actions">
                    <button class="delete"
                            title="Remove this role from ${m.display_name || m.github_login}"
                            @click=${() => this.removeMember(m.user_id, m.role_id)}>×</button>
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
                <option value=${u.id}>${u.display_name || u.github_login} (@${u.github_login})</option>
              `,
              )}
            </select>
          </nottario-field>
          <nottario-field label="Role" class="narrow" style="flex:0 0 180px;min-width:180px">
            <select name="role_id" required>
              <option value="" disabled selected>Pick a role…</option>
              ${this.roles.map((r) => html`<option value=${r.id}>${r.label}</option>`)}
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
    const form = e.target;
    const user_id = form.user_id.value;
    const role_id = form.role_id.value;
    if (!user_id || !role_id) {
      e.preventDefault();
      return;
    }
    try {
      await formButton(e, async () => {
        const res = await fetch(`/api/projects/${this.projectId}/members`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ user_id, role_id }),
        });
        if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || 'failed');
        form.reset();
        await this.load();
      });
    } catch (err) {
      this.error = err.message;
      toast.error(`Couldn't add member: ${err.message}`);
    }
  }

  // ---- Tokens tab ----

  _isMember() {
    if (this.me?.is_admin) return true;
    const ms = this.me?.memberships || [];
    return ms.some((m) => m.project_id === this.projectId);
  }

  // Created column: a compact absolute date (no time, no seconds). The
  // full ISO timestamp sits in the title attribute for power users.
  _fmtDate(d) {
    if (!d) return '—';
    const dt = new Date(d);
    return dt.toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    });
  }

  // Last-used column: relative ("3 days ago", "just now") so the
  // freshness of an active token reads at a glance. Older than 30
  // days falls back to the absolute date so we don't pretend to
  // know "47 days ago".
  _fmtRelativeDate(d) {
    if (!d) return '—';
    const then = new Date(d).getTime();
    const now = Date.now();
    const secs = Math.max(0, Math.floor((now - then) / 1000));
    if (secs < 45) return 'just now';
    const mins = Math.floor(secs / 60);
    if (mins < 60) return `${mins} min ago`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours} hour${hours === 1 ? '' : 's'} ago`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days} day${days === 1 ? '' : 's'} ago`;
    return this._fmtDate(d);
  }

  _armRevoke(id) {
    this._revokeArmedId = id;
  }

  _cancelRevoke() {
    this._revokeArmedId = null;
  }

  async _copyAndAck(text, which) {
    try {
      await navigator.clipboard.writeText(text);
      this._copyAck = which;
      // Brief confirmation; clears so the button text returns to
      // its rest state and the user can click again if needed.
      setTimeout(() => {
        if (this._copyAck === which) this._copyAck = null;
      }, 1500);
    } catch (_) {
      // Clipboard failed — leave the ack off; the user can fall back
      // to selecting the visible text manually (user-select: all).
    }
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
    const form = e.target;
    const name = form.name.value.trim();
    const roleVal = form.default_role_id ? form.default_role_id.value : '';
    const body = { name };
    if (roleVal) body.default_role_id = roleVal;
    try {
      await formButton(e, async () => {
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
      });
      // The dialog stays open and swaps into the reveal panel — no
      // toast needed; the visual transition IS the feedback.
    } catch (err) {
      this.tokenError = err.message;
      toast.error(`Couldn't issue token: ${err.message}`);
    }
  }

  async _revokeToken(id) {
    this._revokeArmedId = null;
    try {
      const res = await fetch(`/api/projects/${this.projectId}/tokens/${id}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error('failed');
      this.tokens = null;
      await this._loadTokens();
      toast.success('Token revoked.');
    } catch (err) {
      this.tokenError = err.message;
      toast.error(`Couldn't revoke token: ${err.message}`);
    }
  }

  renderTokens() {
    if (!this._isMember()) {
      return html`
        <div class="tokens-locked">
          Only project members can manage API tokens.
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
          API tokens authenticate MCP agents. Each is scoped to this project.
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
              : this.tokens.map((t) => this._renderTokenRow(t))
          }
        </tbody>
      </table>
      ${this.showIssueToken ? this._renderTokenDialog() : null}
    `;
  }

  _renderTokenRow(t) {
    const armed = this._revokeArmedId === t.id;
    return html`
      <tr>
        <td>${t.name}</td>
        <td class="mono">${t.prefix}…</td>
        <td title=${t.created_at || ''}>${this._fmtDate(t.created_at)}</td>
        <td title=${t.last_used_at || ''}>${this._fmtRelativeDate(t.last_used_at)}</td>
        <td>
          ${
            t.revoked_at
              ? html`<span class="status revoked">revoked</span>`
              : html`<span class="status active">active</span>`
          }
        </td>
        <td class="row-actions">
          ${
            t.revoked_at
              ? null
              : armed
                ? html`
                  <span class="revoke-confirm">
                    <strong>Revoke?</strong>
                    <button class="yes"
                            @click=${() => this._revokeToken(t.id)}>Yes</button>
                    <button @click=${() => this._cancelRevoke()}>Cancel</button>
                  </span>`
                : html`
                  <button class="delete" title="Revoke token" aria-label="Revoke token"
                          @click=${() => this._armRevoke(t.id)}>✕</button>`
          }
        </td>
      </tr>
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
      const copyTokenLabel =
        this._copyAck === 'token' ? html`<span class="copy-ack">✓ Copied</span>` : 'Copy token';
      const copySnippetLabel =
        this._copyAck === 'snippet' ? html`<span class="copy-ack">✓ Copied</span>` : 'Copy snippet';
      return html`
        <div class="dialog">
          <div class="panel wide">
            <h3>Token issued</h3>
            <div class="secret-banner">
              <strong>Copy this token now.</strong> It will not be shown again.
            </div>
            <div class="secret">${secret}</div>
            <p class="helper" style="margin:14px 0 4px">Install in Claude Code:</p>
            <div class="snippet">${snippet}</div>
            <div class="actions-row" style="margin-top:16px;display:flex;gap:8px;justify-content:flex-end">
              <button class="btn secondary"
                      @click=${() => this._copyAndAck(secret, 'token')}>${copyTokenLabel}</button>
              <button class="btn secondary"
                      @click=${() => this._copyAndAck(snippet, 'snippet')}>${copySnippetLabel}</button>
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
                ${this.roles.map((r) => html`<option value=${r.id}>${r.label}</option>`)}
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
    const ok = await confirm({
      title: 'Remove this membership?',
      body: 'The user keeps any other role memberships in this project.',
      confirmLabel: 'Remove',
      danger: true,
    });
    if (!ok) return;
    try {
      const res = await fetch(`/api/projects/${this.projectId}/members/${userID}/${roleID}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || 'remove failed');
      await this.load();
      toast.success('Member removed.');
    } catch (err) {
      this.error = err.message;
      toast.error(`Couldn't remove member: ${err.message}`);
    }
  }
}

customElements.define('nottario-project-settings', NottarioProjectSettings);
