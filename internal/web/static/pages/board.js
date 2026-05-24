import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';
import { EscController } from '/static/components/esc.js';
import { buttonStyles } from '/static/components/buttons.js';
import { dialogStyles } from '/static/components/surfaces.js';
import { fieldStyles } from '/static/components/fields.js';
import { badgeStyles } from '/static/components/badges.js';
import '/static/components/field.js';
import '/static/components/page-header.js';
import '/static/components/markdown.js';
import '/static/components/avatar.js';
import '/static/components/task-chip.js';
import './gantt.js';

class NottarioBoardPage extends LitElement {
  static properties = {
    me: { type: Object },
    projectId: { type: String },
    // 'kanban' (default) or 'gantt'. Driven by the URL via the shell.
    view: { type: String },
    project: { state: true },
    tasks: { state: true },
    roles: { state: true },
    members: { state: true },
    priorities: { state: true },
    showCreate: { state: true },
    selected: { state: true },
    expandDoing: { state: true },
    error: { state: true },
  };

  static styles = [buttonStyles, dialogStyles, fieldStyles, badgeStyles, css`
    :host { display: block; }
    .spacer { flex: 1; }
    .columns {
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      gap: 16px;
    }
    .columns.two { grid-template-columns: repeat(2, 1fr); }
    .doing-pill {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      height: 26px;
      padding: 0 10px;
      border-radius: 999px;
      border: 1px solid #d0d7de;
      background: #fff;
      color: #59636e;
      font-size: 12px;
      font-weight: 500;
      cursor: pointer;
      font: inherit;
    }
    .doing-pill:hover { border-color: #afb8c1; color: #1f2328; }
    .doing-pill .dot {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: #afb8c1;
    }
    .col {
      background: #f6f8fa;
      border-radius: 8px;
      padding: 8px;
      align-self: start; /* don't stretch to match the tallest column */
    }
    .col.empty { padding: 6px 8px; }
    .col.empty.doing { padding: 8px; }
    .upnext {
      background: #fff;
      border: 1px dashed #afb8c1;
      border-radius: 8px;
      padding: 12px 12px 10px;
      margin: 4px 0;
      display: flex;
      flex-direction: column;
      gap: 8px;
    }
    .upnext .eyebrow {
      font-size: 11px;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      color: #59636e;
      font-weight: 600;
    }
    .upnext .title {
      font-weight: 600;
      font-size: 14px;
      color: #1f2328;
      line-height: 1.3;
    }
    .upnext .meta {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
      font-size: 12px;
      color: #59636e;
    }
    .upnext .row {
      display: flex;
      gap: 8px;
      align-items: center;
      margin-top: 4px;
    }
    .upnext .row .spacer { flex: 1; }
    .upnext button.start {
      background: #1f883d;
      color: #fff;
      border: 1px solid rgba(31, 35, 40, 0.15);
      padding: 5px 12px;
      border-radius: 6px;
      font-weight: 500;
      font-size: 13px;
      cursor: pointer;
    }
    .upnext button.start:hover { background: #1a7f37; }
    .upnext button.peek {
      background: transparent;
      color: #0969da;
      border: none;
      cursor: pointer;
      font-size: 12px;
      padding: 4px 6px;
    }
    .upnext button.peek:hover { text-decoration: underline; }
    .col h3 {
      margin: 4px 4px 8px 4px;
      font-size: 13px;
      text-transform: uppercase;
      color: #59636e;
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .col.empty h3 { margin-bottom: 4px; }
    .col .empty-note {
      font-size: 12px;
      color: #8b949e;
      padding: 0 4px 2px;
      font-style: italic;
    }
    .count {
      background: #eaeef2;
      color: #59636e;
      border-radius: 2em;
      padding: 0 8px;
      font-size: 12px;
      font-weight: 500;
    }
    .card {
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 6px;
      padding: 10px 12px;
      margin-bottom: 8px;
      cursor: pointer;
      box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04);
    }
    .card:hover { border-color: #afb8c1; }
    .card .title { font-weight: 500; margin-bottom: 4px; }
    .card .meta {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
      font-size: 12px;
      color: #59636e;
    }
    .prio { font-family: ui-monospace, SFMono-Regular, monospace; }
    .error { color: #cf222e; margin-bottom: 8px; font-size: 13px; }

    /* ---- Task-detail dialog ---- */

    /* Wider than dialogStyles default so the description, table-laden
       markdown and threaded comments breathe. */
    .dialog .panel.detail { width: 720px; padding: 0; }
    /* box-sizing isn't inherited across shadow boundaries, and the
       panel has its own padding contract. Force border-box on every
       descendant so width: 100% on the comment textarea (and any
       future form control) doesn't push past the panel edge. */
    .panel.detail, .panel.detail * { box-sizing: border-box; }

    /* Header strip: title row first (title leads, no clutter to its
       left), a smaller meta line under it, then the meta strip with
       state / priority / role / assignee.

       The leading short-id and type badge moved to that second line
       so the eye lands on the title without competing chrome. */
    .detail .head {
      padding: 20px 22px 14px;
      border-bottom: 1px solid #eaeef2;
    }
    .detail .head .title-row {
      display: flex;
      align-items: flex-start;
      gap: 10px;
    }
    .detail .head h3 {
      margin: 0;
      font-size: 22px;
      font-weight: 600;
      line-height: 1.25;
      letter-spacing: -0.01em;
      color: #1f2328;
      flex: 1;
      min-width: 0;
    }
    .detail .head .sub-line {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-top: 6px;
      font-size: 12px;
      color: #59636e;
    }
    .detail .head .short-id {
      font-family: ui-monospace, SFMono-Regular, monospace;
      color: #8b949e;
      font-size: 12px;
    }
    .detail .head .sub-line .dot { color: #d0d7de; }
    .detail .head .title-actions {
      display: flex;
      align-items: center;
      gap: 4px;
      flex: 0 0 auto;
    }
    /* Hover-revealed icon button — same chrome as docs reader trash
       and project-settings row delete. */
    .detail .head .icon-btn {
      width: 28px;
      height: 28px;
      padding: 0;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      color: #8b949e;
      background: transparent;
      border: 1px solid transparent;
      border-radius: 6px;
      cursor: pointer;
      font: inherit;
    }
    .detail .head .icon-btn svg { display: block; }
    .detail .head .icon-btn:hover {
      color: #1f2328;
      background: #f6f8fa;
      border-color: #d0d7de;
    }
    .detail .head .icon-btn:focus-visible {
      outline: 2px solid #0969da;
      outline-offset: 1px;
    }
    .detail .head .icon-btn.danger:hover,
    .detail .head .icon-btn.danger:focus-visible {
      color: #cf222e;
      background: #ffebe9;
      border-color: rgba(207, 34, 46, 0.4);
    }

    /* Meta strip: one row of inline label+value pairs separated by
       a thin dot. Wraps on narrow viewports but stays compact at
       720px. */
    .detail .meta {
      display: flex;
      flex-wrap: wrap;
      gap: 12px 18px;
      margin-top: 12px;
      font-size: 12px;
      color: #59636e;
      align-items: center;
    }
    .detail .meta .field-line {
      display: inline-flex;
      align-items: center;
      gap: 6px;
    }
    .detail .meta .lbl { color: #8b949e; font-size: 11px; text-transform: uppercase; letter-spacing: 0.04em; font-weight: 600; }
    .detail .meta .val { color: #1f2328; }
    .detail .meta .val .muted { color: #8b949e; font-style: italic; font-weight: 400; }
    .detail .meta .author-cell { display: inline-flex; align-items: center; gap: 6px; }

    /* State control as compact segmented pill — three buttons share a
       single rounded shell; the active one is the GitHub-green primary
       (matches the kanban "done" reading), the others stay neutral. */
    .detail .state-control {
      display: inline-flex;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      overflow: hidden;
      background: #ffffff;
    }
    .detail .state-control button {
      padding: 4px 12px;
      font: inherit;
      font-size: 12px;
      background: transparent;
      border: none;
      border-right: 1px solid #d0d7de;
      cursor: pointer;
      color: #59636e;
    }
    .detail .state-control button:last-child { border-right: none; }
    .detail .state-control button:hover { background: #f6f8fa; color: #1f2328; }
    .detail .state-control button.active {
      background: #1f883d;
      color: #ffffff;
      font-weight: 600;
    }
    .detail .state-control button.active:hover { background: #1a7f37; }

    /* Compact priority number input. Width tuned to fit 3 digits + the
       hidden-spinner chrome. */
    .detail .meta input[type="number"] {
      width: 56px;
      padding: 2px 6px;
      border: 1px solid #d0d7de;
      border-radius: 4px;
      font: inherit;
      font-size: 12px;
      font-variant-numeric: tabular-nums;
      text-align: center;
    }
    .detail .meta input[type="number"]::-webkit-inner-spin-button,
    .detail .meta input[type="number"]::-webkit-outer-spin-button {
      -webkit-appearance: none; appearance: none; margin: 0;
    }

    /* Body sections — description, deps, commits, comments. Eyebrow
       headings echo the docs rail / profile pattern. */
    .detail .body { padding: 16px 20px 20px; }
    .detail .body > section { margin-top: 18px; }
    .detail .body > section:first-child { margin-top: 0; }
    .detail .eyebrow {
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 0.06em;
      color: #8b949e;
      font-weight: 600;
      margin: 0 0 8px;
    }

    .detail .deps-list {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
    }

    .detail .commits-list {
      display: flex;
      flex-direction: column;
      gap: 4px;
      font-size: 12px;
      font-family: ui-monospace, SFMono-Regular, monospace;
    }
    .detail .commits-list .commit {
      padding: 4px 8px;
      background: #f6f8fa;
      border: 1px solid #d1d9e0;
      border-radius: 4px;
      color: #1f2328;
    }
    .detail .commits-list .commit .sha { color: #0969da; }

    /* Comments thread — each row has a small leading avatar column. */
    .detail .comment {
      display: grid;
      grid-template-columns: 28px 1fr;
      gap: 10px;
      padding: 10px 0;
      border-top: 1px solid #eaeef2;
    }
    .detail .comment:first-of-type { border-top: none; padding-top: 0; }
    .detail .comment .ava { padding-top: 1px; }
    .detail .comment .meta-line {
      display: flex;
      align-items: baseline;
      gap: 8px;
      font-size: 12px;
      color: #59636e;
      margin-bottom: 2px;
    }
    .detail .comment .meta-line .name { color: #1f2328; font-weight: 600; }
    .detail .comment .meta-line .when { color: #8b949e; }

    .detail .add-comment {
      margin-top: 14px;
      padding-top: 14px;
      border-top: 1px solid #eaeef2;
    }
    .detail .add-comment textarea {
      width: 100%;
      min-height: 64px;
      padding: 8px 10px;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      font: inherit;
      font-size: 13px;
      line-height: 1.5;
      resize: vertical;
      background: #ffffff;
    }
    .detail .add-comment textarea:focus {
      outline: 2px solid #0969da;
      outline-offset: 0;
      border-color: #0969da;
    }
    .detail .add-comment .row {
      display: flex;
      justify-content: flex-end;
      gap: 8px;
      margin-top: 8px;
    }

    .detail .empty {
      font-size: 13px;
      color: #8b949e;
      font-style: italic;
    }
  `];

  constructor() {
    super();
    this.view = 'kanban';
    this.project = null;
    this.tasks = [];
    this.roles = [];
    this.members = [];
    this.showCreate = false;
    this.selected = null;
    this.expandDoing = false;
    this.error = '';
    new EscController(this, (e) => this._onEsc(e));
  }

  _onEsc(e) {
    // Topmost first: the task detail panel sits over the create form
    // when both happen to be open. Stop propagation after closing so
    // an outer listener (topbar dropdown, etc.) doesn't also react.
    if (this.selected)   { this.closeDetail();       e.stopPropagation(); return; }
    if (this.showCreate) { this.showCreate = false;  e.stopPropagation(); return; }
  }

  connectedCallback() {
    super.connectedCallback();
    this.load().then(() => this._applyHash());
    this._subscribe();
    this._hashHandler = () => this._applyHash();
    window.addEventListener('hashchange', this._hashHandler);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unsub?.();
    window.removeEventListener('hashchange', this._hashHandler);
  }

  updated(c) {
    if (c.has('projectId')) {
      this.load().then(() => this._applyHash());
      this._subscribe();
    }
  }

  _applyHash() {
    const h = new URLSearchParams(window.location.hash.slice(1));
    const taskId = h.get('task');
    if (!taskId) return;
    const t = this.tasks.find(x => x.ID === taskId);
    if (t) this.open(t);
  }

  _subscribe() {
    this._unsub?.();
    if (!this.projectId) return;
    this._unsub = subscribe(this.projectId, (ev) => {
      if (!ev.type) return;
      // 'realtime.reconnected' fires after EventSource recovers from a
      // disconnect — any events during the gap were lost, so reload.
      if (ev.type === 'realtime.reconnected' || ev.type.startsWith('task.')) {
        this.load();
        if (this.selected) this.loadDetail(this.selected.task.ID);
      }
    });
  }

  async load() {
    if (!this.projectId) return;
    try {
      const [pr, tr, rr, mr, qr, dr] = await Promise.all([
        fetch(`/api/projects/${this.projectId}`),
        fetch(`/api/projects/${this.projectId}/tasks?include_children=true`),
        fetch(`/api/projects/${this.projectId}/roles`),
        fetch(`/api/projects/${this.projectId}/members`),
        fetch(`/api/projects/${this.projectId}/priorities`),
        fetch(`/api/projects/${this.projectId}/tasks/dependencies`),
      ]);
      if (!pr.ok) throw new Error('project not found');
      this.project = await pr.json();
      this.tasks = (await tr.json()).tasks || [];
      this.roles = (await rr.json()).roles || [];
      this.members = (await mr.json()).members || [];
      this.priorities = (await qr.json()).priorities || [];
      this.deps = (await dr.json()).dependencies || [];
      // Auto-reset the manual expand toggle on every load: if no tasks
      // are doing, the column hides again with its pill. Once the user
      // clicks the pill the Up-next card is exposed for as long as the
      // user stays on this snapshot.
      if (this.byState('doing').length > 0) this.expandDoing = false;
    } catch (e) {
      this.error = e.message;
    }
  }

  // Next eligible todo task: highest priority among todo tasks whose
  // preconditions (if any) are already done. Mirrors the same logic
  // tasks.next exposes over MCP — so the empty `doing` column shows
  // exactly what an agent would pick up next.
  _nextEligible() {
    if (!this.tasks || !this.tasks.length) return null;
    const taskByID = new Map(this.tasks.map(t => [t.ID, t]));
    const blocked = new Set();
    for (const d of (this.deps || [])) {
      const preID = d.DependsOnID || d.depends_on_id || d.depends_on_task_id;
      const tid = d.TaskID || d.task_id;
      const pre = taskByID.get(preID);
      if (pre && pre.State !== 'done') blocked.add(tid);
    }
    const eligible = this.tasks
      .filter(t => t.State === 'todo' && t.Type !== 'feature' && !blocked.has(t.ID))
      .sort((a, b) => b.Priority - a.Priority
        || (new Date(a.CreatedAt) - new Date(b.CreatedAt)));
    return eligible[0] || null;
  }

  roleByID(id) { return this.roles.find(r => r.ID === id); }

  _priorityLabel(value) {
    if (!this.priorities || !this.priorities.length) return `p${value}`;
    const exact = this.priorities.find(p => p.Value === value);
    if (exact) return exact.Key;
    return `p${value}`;
  }

  back() { window.nottarioNavigate('/'); }

  _emptyCopy(state) {
    switch (state) {
      case 'todo':  return 'Backlog clear.';
      case 'doing': return 'Nothing in progress.';
      case 'done':  return 'No completed tasks yet.';
      default:      return 'Empty.';
    }
  }

  // Empty-column bodies. `doing` is special: instead of a passive
  // note, surface the next eligible task with a Start affordance, so
  // the column's empty state IS a workflow handoff. Other columns
  // (todo/done) keep the muted note — they don't carry the same
  // "what should happen next" semantic.
  _renderEmptyBody(state) {
    if (state === 'doing') {
      const next = this._nextEligible();
      if (!next) {
        return html`<div class="empty-note">${
          this.byState('todo').length === 0 ? 'All caught up.' : 'Nothing eligible to start.'
        }</div>`;
      }
      const role = next.TargetRoleID ? this.roleByID(next.TargetRoleID) : null;
      return html`
        <div class="upnext">
          <div class="eyebrow">Up next</div>
          <div class="title">${next.Title}</div>
          <div class="meta">
            <span class="badge ${next.Type}">${next.Type}</span>
            <span class="prio">${this._priorityLabel(next.Priority)}</span>
            ${role ? html`<span class="badge"
              style=${`background:${role.Color || '#eee'}1a; border-color:${role.Color || '#ddd'}`}>${role.Label}</span>` : ''}
          </div>
          <div class="row">
            <button class="btn primary" @click=${() => this.setState(next.ID, 'doing')}>Start</button>
            <button class="btn ghost" @click=${() => this.open(next)}>Open</button>
            <div class="spacer"></div>
          </div>
        </div>
      `;
    }
    return html`<div class="empty-note">${this._emptyCopy(state)}</div>`;
  }

  byState(s) {
    const items = this.tasks.filter(t => t.State === s);
    if (s === 'done') {
      // Most recently finished at the top — fall back to UpdatedAt
      // when ActualEnd is null (legacy rows / manual edits).
      items.sort((a, b) => {
        const at = new Date(a.ActualEnd || a.UpdatedAt).getTime();
        const bt = new Date(b.ActualEnd || b.UpdatedAt).getTime();
        return bt - at;
      });
    }
    return items;
  }

  open(t) {
    this.selected = { task: t, deps: [], commits: [], comments: [] };
    this.loadDetail(t.ID);
  }

  async loadDetail(id) {
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${id}`);
    if (r.ok) {
      const j = await r.json();
      this.selected = {
        task: j.task,
        deps: j.depends_on || [],
        commits: j.commits || [],
        comments: j.comments || [],
      };
    }
  }

  closeDetail() { this.selected = null; }

  async setState(taskID, state) {
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${taskID}/state`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ state }),
    });
    if (r.ok) {
      await this.load();
      if (this.selected) await this.loadDetail(taskID);
    } else {
      this.error = (await r.json()).error || 'failed';
    }
  }

  async deleteTask(id) {
    if (!confirm('Delete this task?')) return;
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${id}`, { method: 'DELETE' });
    if (r.ok) {
      this.selected = null;
      await this.load();
    } else {
      this.error = (await r.json()).error || 'failed';
    }
  }

  async createTask(e) {
    e.preventDefault();
    const f = e.target;
    const body = {
      title: f.title.value.trim(),
      description: f.description.value.trim(),
      type: f.type.value,
      priority_key: f.priority_key.value,
    };
    if (f.target_role_id.value) body.target_role_id = f.target_role_id.value;
    try {
      const r = await fetch(`/api/projects/${this.projectId}/tasks`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!r.ok) throw new Error((await r.json()).error || 'failed');
      this.showCreate = false;
      await this.load();
    } catch (err) { this.error = err.message; }
  }

  async addComment(taskID, body) {
    if (!body.trim()) return;
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${taskID}/comments`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ body }),
    });
    if (r.ok) await this.loadDetail(taskID);
  }

  async setPriority(taskID, value) {
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${taskID}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ priority: parseInt(value, 10) }),
    });
    if (r.ok) {
      await this.load();
      if (this.selected) await this.loadDetail(taskID);
    }
  }

  renderCard(t) {
    const role = t.TargetRoleID ? this.roleByID(t.TargetRoleID) : null;
    return html`
      <div class="card" @click=${() => this.open(t)}>
        <div class="title">${t.Title}</div>
        <div class="meta">
          <span class="badge ${t.Type}">${t.Type}</span>
          <span class="prio">${this._priorityLabel(t.Priority)}</span>
          ${role ? html`<span class="badge" style=${`background:${role.Color || '#eee'}1a; border-color:${role.Color || '#ddd'}`}>${role.Label}</span>` : ''}
        </div>
      </div>
    `;
  }

  render() {
    if (!this.project) return html`<p>Loading…</p>`;
    const doingCount = this.byState('doing').length;
    const hideDoing = this.view === 'kanban' && doingCount === 0 && !this.expandDoing;
    return html`
      <nottario-page-header
        .title=${this.view === 'gantt' ? 'Gantt' : 'Board'}>
        ${hideDoing
          ? html`<button slot="actions" class="btn ghost" title="Show the doing column"
                         @click=${() => this.expandDoing = true}>· 0 doing</button>`
          : null}
        ${this.view === 'gantt'
          ? html`<button slot="actions" class="btn ghost"
                         title="Scroll the Gantt back to the now line"
                         @click=${() => this.renderRoot.querySelector('nottario-gantt')?.scrollToNow()}>↻ Now</button>`
          : null}
        <button slot="actions" class="btn primary"
                @click=${() => this.showCreate = true}>New task</button>
      </nottario-page-header>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      ${this.view === 'gantt'
        ? html`<nottario-gantt
                  .projectId=${this.projectId}
                  @task-selected=${(e) => this.open(e.detail.task)}></nottario-gantt>`
        : html`
          <div class=${hideDoing ? 'columns two' : 'columns'}>
            ${(hideDoing ? ['todo', 'done'] : ['todo', 'doing', 'done']).map(s => {
              const items = this.byState(s);
              const isEmpty = items.length === 0;
              const cls = isEmpty
                ? (s === 'doing' ? 'col empty doing' : 'col empty')
                : 'col';
              return html`
                <div class=${cls}>
                  <h3>${s} <span class="count">${items.length}</span></h3>
                  ${isEmpty
                    ? this._renderEmptyBody(s)
                    : items.map(t => this.renderCard(t))}
                </div>
              `;
            })}
          </div>
        `}
      ${this.showCreate ? this.renderCreate() : null}
      ${this.selected ? this.renderDetail() : null}
    `;
  }

  renderCreate() {
    return html`
      <div class="dialog" @click=${(e) => e.target.classList.contains('dialog') && (this.showCreate = false)}>
        <div class="panel">
          <h3>New task</h3>
          <form @submit=${(e) => this.createTask(e)}>
            <nottario-field label="Title">
              <input name="title" required autofocus>
            </nottario-field>
            <nottario-field label="Description" hint="markdown">
              <textarea name="description" rows="4"></textarea>
            </nottario-field>
            <div style="display:flex;gap:12px">
              <nottario-field label="Type" style="flex:1">
                <select name="type">
                  <option value="task">task</option>
                  <option value="bug">bug</option>
                  <option value="chore">chore</option>
                  <option value="spike">spike</option>
                  <option value="feature">feature</option>
                </select>
              </nottario-field>
              <nottario-field label="Priority" style="flex:1">
                <select name="priority_key">
                  ${[...this.priorities].sort((a, b) => b.Value - a.Value).map(p =>
                    html`<option value=${p.Key} ?selected=${p.Key === 'medium'}>${p.Key} (${p.Value})</option>`)}
                </select>
              </nottario-field>
              <nottario-field label="Target role" style="flex:1">
                <select name="target_role_id">
                  <option value="">— none —</option>
                  ${this.roles.map(r => html`<option value=${r.ID}>${r.Label}</option>`)}
                </select>
              </nottario-field>
            </div>
            <div class="actions-row">
              <button type="button" class="btn secondary" @click=${() => this.showCreate = false}>Cancel</button>
              <button type="submit" class="btn primary">Create</button>
            </div>
          </form>
        </div>
      </div>
    `;
  }

  // Look up a member by UserID. Members carry display name + avatar
  // URL; comments and the task assignee link to one of them.
  _memberByID(uid) {
    if (!uid) return null;
    return (this.members || []).find(m => m.UserID === uid) || null;
  }

  _taskByID(id) {
    return (this.tasks || []).find(t => t.ID === id) || null;
  }

  _relTime(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    const diff = (Date.now() - d.getTime()) / 1000;
    if (diff < 60)    return 'just now';
    if (diff < 3600)  return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    if (diff < 86400 * 30) return `${Math.floor(diff / 86400)}d ago`;
    return d.toLocaleDateString();
  }

  renderDetail() {
    const { task, deps, commits, comments } = this.selected;
    const role = task.TargetRoleID ? this.roleByID(task.TargetRoleID) : null;
    const assignee = this._memberByID(task.AssigneeUserID);
    const shortID = (task.ID || '').slice(0, 7);
    return html`
      <div class="dialog" @click=${(e) => e.target.classList.contains('dialog') && this.closeDetail()}>
        <div class="panel detail">
          <header class="head">
            <div class="title-row">
              <h3>${task.Title}</h3>
              <div class="title-actions">
                <button class="icon-btn danger" title="Delete task" aria-label="Delete task"
                        @click=${() => this.deleteTask(task.ID)}>
                  <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                    <path d="M6 2.5h4M3 4.5h10M4.5 4.5l.6 8.2a1 1 0 0 0 1 .9h3.8a1 1 0 0 0 1-.9l.6-8.2M6.8 7v4M9.2 7v4"
                          stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>
                  </svg>
                </button>
                <button class="icon-btn" title="Close (Esc)" aria-label="Close"
                        @click=${() => this.closeDetail()}>
                  <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
                    <path d="M3 3 L9 9 M9 3 L3 9" stroke="currentColor"
                          stroke-width="1.6" stroke-linecap="round"/>
                  </svg>
                </button>
              </div>
            </div>

            <div class="sub-line">
              <span class="badge ${task.Type}">${task.Type}</span>
              <span class="dot">·</span>
              <span class="short-id">#${shortID}</span>
            </div>

            <div class="meta">
              <div class="field-line">
                <span class="lbl">State</span>
                <div class="state-control">
                  ${['todo', 'doing', 'done'].map(s => html`
                    <button class=${task.State === s ? 'active' : ''}
                            @click=${() => this.setState(task.ID, s)}>${s}</button>
                  `)}
                </div>
              </div>

              <div class="field-line">
                <span class="lbl">Priority</span>
                <input type="number" .value=${String(task.Priority)} min="0" max="100"
                       @change=${(e) => this.setPriority(task.ID, e.target.value)}>
              </div>

              <div class="field-line">
                <span class="lbl">Role</span>
                <span class="val">${role ? role.Label : html`<span class="muted">none</span>`}</span>
              </div>

              <div class="field-line">
                <span class="lbl">Assignee</span>
                <span class="val">
                  ${assignee ? html`
                    <span class="author-cell">
                      <nottario-avatar size="20"
                        src=${assignee.AvatarURL || ''}
                        name=${assignee.DisplayName || assignee.GithubLogin || ''}></nottario-avatar>
                      ${assignee.DisplayName || assignee.GithubLogin}
                    </span>
                  ` : html`<span class="muted">unassigned</span>`}
                </span>
              </div>
            </div>
          </header>

          <div class="body">
            ${task.DescriptionMD ? html`
              <section>
                <nottario-markdown
                  project-id=${this.projectId}
                  .source=${task.DescriptionMD}></nottario-markdown>
              </section>
            ` : null}

            ${deps.length ? html`
              <section>
                <h4 class="eyebrow">Depends on</h4>
                <div class="deps-list">
                  ${deps.map(id => html`
                    <nottario-task-chip
                      project-id=${this.projectId}
                      .task=${this._taskByID(id) || { ID: id, Title: id.slice(0, 8) + ' (not loaded)' }}>
                    </nottario-task-chip>
                  `)}
                </div>
              </section>
            ` : null}

            <section>
              <h4 class="eyebrow">Commits</h4>
              ${commits.length === 0
                ? html`<p class="empty">No commits linked.</p>`
                : html`
                  <div class="commits-list">
                    ${commits.map(c => html`
                      <div class="commit">
                        ${c.Repo}<span class="sha">@${(c.SHA || '').slice(0, 8)}</span>
                        ${c.Message ? html` ${c.Message}` : null}
                      </div>
                    `)}
                  </div>
                `}
            </section>

            <section>
              <h4 class="eyebrow">Comments</h4>
              ${comments.length === 0
                ? html`<p class="empty">No comments yet.</p>`
                : comments.map(c => {
                  const author = this._memberByID(c.AuthorUserID);
                  return html`
                    <div class="comment">
                      <div class="ava">
                        <nottario-avatar size="24"
                          src=${author?.AvatarURL || ''}
                          name=${author?.DisplayName || author?.GithubLogin || 'agent'}></nottario-avatar>
                      </div>
                      <div>
                        <div class="meta-line">
                          <span class="name">${author?.DisplayName || author?.GithubLogin || 'agent'}</span>
                          <span class="when">${this._relTime(c.CreatedAt)}</span>
                        </div>
                        <nottario-markdown
                          project-id=${this.projectId}
                          .source=${c.BodyMD || ''}></nottario-markdown>
                      </div>
                    </div>
                  `;
                })}

              <form class="add-comment"
                    @submit=${(e) => { e.preventDefault(); const t = e.target.body; this.addComment(task.ID, t.value); t.value = ''; }}>
                <textarea name="body" placeholder="Write a comment in markdown..."></textarea>
                <div class="row">
                  <button type="submit" class="btn primary">Comment</button>
                </div>
              </form>
            </section>
          </div>
        </div>
      </div>
    `;
  }
}

customElements.define('nottario-board-page', NottarioBoardPage);
