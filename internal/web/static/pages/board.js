import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';
import { EscController } from '/static/components/esc.js';
import { buttonStyles } from '/static/components/buttons.js';
import { dialogStyles } from '/static/components/surfaces.js';
import { fieldStyles } from '/static/components/fields.js';
import { badgeStyles } from '/static/components/badges.js';
import '/static/components/field.js';
import '/static/components/page-header.js';
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
    /* Detail panel wider than the shared default; everything else
       inherits from dialogStyles in components/surfaces.js. */
    .dialog .panel { width: 560px; }
    .error { color: #cf222e; margin-bottom: 8px; font-size: 13px; }
    .detail h3 { margin: 0 0 8px 0; }
    .detail .meta-grid {
      display: grid;
      grid-template-columns: max-content 1fr;
      gap: 6px 16px;
      margin-bottom: 12px;
    }
    .detail .meta-grid > div:nth-child(odd) {
      color: #59636e;
      font-size: 13px;
    }
    .state-buttons { display: flex; gap: 6px; }
    .state-buttons button.active {
      background: #0969da;
      color: #fff;
      border-color: #0969da;
    }
    .commits, .comments { margin-top: 12px; }
    .commits pre, .comments .item {
      background: #f6f8fa;
      border: 1px solid #d1d9e0;
      border-radius: 6px;
      padding: 8px 12px;
      margin: 4px 0;
      font-size: 12px;
      white-space: pre-wrap;
    }
    .comments .item .author {
      font-size: 11px;
      color: #59636e;
      margin-bottom: 4px;
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
      if (ev.type.startsWith('task.')) {
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

  renderDetail() {
    const { task, deps, commits, comments } = this.selected;
    const role = task.TargetRoleID ? this.roleByID(task.TargetRoleID) : null;
    return html`
      <div class="dialog" @click=${(e) => e.target.classList.contains('dialog') && this.closeDetail()}>
        <div class="panel detail">
          <h3>${task.Title}</h3>
          <div class="meta-grid">
            <div>Type</div><div><span class="badge ${task.Type}">${task.Type}</span></div>
            <div>State</div>
            <div class="state-buttons">
              ${['todo', 'doing', 'done'].map(s => html`
                <button class=${'btn ' + (task.State === s ? 'primary' : 'secondary')}
                        @click=${() => this.setState(task.ID, s)}>${s}</button>
              `)}
            </div>
            <div>Priority</div>
            <div><input type="number" value=${task.Priority} min="0" max="100"
              style="width:80px"
              @change=${(e) => this.setPriority(task.ID, e.target.value)}></div>
            <div>Target role</div><div>${role ? role.Label : html`<span class="muted">none</span>`}</div>
            ${deps.length ? html`
              <div>Dependencies</div>
              <div>${deps.map(id => html`<code style="font-size:11px">${id}</code> `)}</div>
            ` : null}
          </div>
          ${task.DescriptionMD ? html`<pre style="white-space:pre-wrap">${task.DescriptionMD}</pre>` : null}

          <div class="commits">
            <strong>Commits</strong>
            ${commits.length === 0 ? html`<p class="muted">No commits linked.</p>` :
              commits.map(c => html`<pre>${c.Repo}@${c.SHA}  ${c.Message}</pre>`)}
          </div>

          <div class="comments">
            <strong>Comments</strong>
            ${comments.length === 0 ? html`<p class="muted">No comments yet.</p>` :
              comments.map(c => html`<div class="item">
                <div class="author">${new Date(c.CreatedAt).toLocaleString()}</div>
                <div>${c.BodyMD}</div>
              </div>`)}
            <form @submit=${(e) => { e.preventDefault(); const t = e.target.body; this.addComment(task.ID, t.value); t.value = ''; }}>
              <textarea name="body" rows="2" placeholder="Add a comment…" style="margin-top:8px"></textarea>
              <div class="actions-row">
                <button type="submit" class="btn secondary">Add comment</button>
              </div>
            </form>
          </div>

          <div class="actions-row">
            <button class="btn danger" @click=${() => this.deleteTask(task.ID)}>Delete</button>
            <div class="spacer" style="flex:1"></div>
            <button class="btn secondary" @click=${() => this.closeDetail()}>Close</button>
          </div>
        </div>
      </div>
    `;
  }
}

customElements.define('nottario-board-page', NottarioBoardPage);
