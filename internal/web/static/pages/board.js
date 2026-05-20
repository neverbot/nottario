import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';
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
    error: { state: true },
  };

  static styles = css`
    :host { display: block; }
    .header { display: flex; align-items: baseline; gap: 16px; margin-bottom: 16px; }
    .header h2 { margin: 0; }
    .spacer { flex: 1; }
    .columns {
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      gap: 16px;
    }
    .col {
      background: #f6f8fa;
      border-radius: 8px;
      padding: 8px;
      align-self: start; /* don't stretch to match the tallest column */
    }
    .col.empty { padding: 6px 8px; }
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
    .badge {
      display: inline-block;
      padding: 1px 7px;
      border-radius: 2em;
      font-size: 11px;
      border: 1px solid #d1d9e0;
      background: #fff;
    }
    .badge.bug { background: #ffebe9; border-color: #ffabab; color: #cf222e; }
    .badge.feature { background: #ddf4ff; border-color: #8ec0ff; color: #0969da; }
    .badge.chore { background: #fff8c5; border-color: #d4a72c; color: #7d4e00; }
    .badge.spike { background: #ddf4d1; border-color: #95d57e; color: #1a7f37; }
    .prio { font-family: ui-monospace, SFMono-Regular, monospace; }
    .dialog {
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.4);
      display: flex;
      align-items: center;
      justify-content: center;
      z-index: 10;
    }
    .panel {
      background: #fff;
      border-radius: 8px;
      padding: 24px;
      width: 560px;
      max-width: 92vw;
      max-height: 88vh;
      overflow: auto;
    }
    .field { margin-bottom: 12px; }
    .field label { display: block; margin-bottom: 4px; font-weight: 500; font-size: 13px; }
    .actions-row { margin-top: 16px; display: flex; gap: 8px; justify-content: flex-end; }
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
  `;

  constructor() {
    super();
    this.view = 'kanban';
    this.project = null;
    this.tasks = [];
    this.roles = [];
    this.members = [];
    this.showCreate = false;
    this.selected = null;
    this.error = '';
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
      const [pr, tr, rr, mr, qr] = await Promise.all([
        fetch(`/api/projects/${this.projectId}`),
        fetch(`/api/projects/${this.projectId}/tasks?include_children=true`),
        fetch(`/api/projects/${this.projectId}/roles`),
        fetch(`/api/projects/${this.projectId}/members`),
        fetch(`/api/projects/${this.projectId}/priorities`),
      ]);
      if (!pr.ok) throw new Error('project not found');
      this.project = await pr.json();
      this.tasks = (await tr.json()).tasks || [];
      this.roles = (await rr.json()).roles || [];
      this.members = (await mr.json()).members || [];
      this.priorities = (await qr.json()).priorities || [];
    } catch (e) {
      this.error = e.message;
    }
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

  byState(s) {
    return this.tasks.filter(t => t.State === s);
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
    return html`
      <div class="header">
        <button @click=${() => this.back()}>← Back</button>
        <h2>${this.project.Name}</h2>
        <span class="muted">${this.view === 'gantt' ? 'gantt' : 'board'}</span>
        <div class="spacer"></div>
        <div role="tablist" style="display:flex;gap:4px">
          <button class=${this.view === 'kanban' ? 'primary' : ''}
                  @click=${() => window.nottarioNavigate(`/projects/${this.projectId}/board`)}>Kanban</button>
          <button class=${this.view === 'gantt' ? 'primary' : ''}
                  @click=${() => window.nottarioNavigate(`/projects/${this.projectId}/board/gantt`)}>Gantt</button>
        </div>
        ${this.view === 'gantt' ? html`
          <button title="Scroll the Gantt back to the now line"
                  @click=${() => this.renderRoot.querySelector('nottario-gantt')?.scrollToNow()}>↻ Now</button>
        ` : null}
        <button class="primary" @click=${() => this.showCreate = true}>New task</button>
      </div>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      ${this.view === 'gantt'
        ? html`<nottario-gantt
                  .projectId=${this.projectId}
                  @task-selected=${(e) => this.open(e.detail.task)}></nottario-gantt>`
        : html`
          <div class="columns">
            ${['todo', 'doing', 'done'].map(s => {
              const items = this.byState(s);
              const isEmpty = items.length === 0;
              return html`
                <div class=${isEmpty ? 'col empty' : 'col'}>
                  <h3>${s} <span class="count">${items.length}</span></h3>
                  ${isEmpty
                    ? html`<div class="empty-note">${this._emptyCopy(s)}</div>`
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
            <div class="field">
              <label>Title</label>
              <input name="title" required autofocus>
            </div>
            <div class="field">
              <label>Description (markdown)</label>
              <textarea name="description" rows="4"></textarea>
            </div>
            <div class="field" style="display:flex;gap:12px">
              <div style="flex:1">
                <label>Type</label>
                <select name="type">
                  <option value="task">task</option>
                  <option value="bug">bug</option>
                  <option value="chore">chore</option>
                  <option value="spike">spike</option>
                  <option value="feature">feature</option>
                </select>
              </div>
              <div style="flex:1">
                <label>Priority</label>
                <select name="priority_key">
                  ${[...this.priorities].sort((a, b) => b.Value - a.Value).map(p =>
                    html`<option value=${p.Key} ?selected=${p.Key === 'medium'}>${p.Key} (${p.Value})</option>`)}
                </select>
              </div>
              <div style="flex:1">
                <label>Target role</label>
                <select name="target_role_id">
                  <option value="">— none —</option>
                  ${this.roles.map(r => html`<option value=${r.ID}>${r.Label}</option>`)}
                </select>
              </div>
            </div>
            <div class="actions-row">
              <button type="button" @click=${() => this.showCreate = false}>Cancel</button>
              <button type="submit" class="primary">Create</button>
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
                <button class=${task.State === s ? 'active' : ''} @click=${() => this.setState(task.ID, s)}>${s}</button>
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
                <button type="submit">Add comment</button>
              </div>
            </form>
          </div>

          <div class="actions-row">
            <button class="danger" @click=${() => this.deleteTask(task.ID)}>Delete</button>
            <div class="spacer" style="flex:1"></div>
            <button @click=${() => this.closeDetail()}>Close</button>
          </div>
        </div>
      </div>
    `;
  }
}

customElements.define('nottario-board-page', NottarioBoardPage);
