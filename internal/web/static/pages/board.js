import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';
import { EscController } from '/static/components/esc.js';
import { buttonStyles } from '/static/components/buttons.js';
import { dialogStyles } from '/static/components/surfaces.js';
import { formStyles } from '/static/components/forms.js';
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
    _draggingID: { state: true },
    _dragOverState: { state: true },
    // Cycles: the list of cycles for this project, the currently-
    // viewed cycle id (null = follow active), the dropdown open
    // state, and the end-sprint dialog open state.
    cycles: { state: true },
    cycleId: { state: true },
    _cycleDropdownOpen: { state: true },
    _endSprintOpen: { state: true },
  };

  static styles = [buttonStyles, dialogStyles, formStyles, badgeStyles, css`
    :host { display: block; }
    .spacer { flex: 1; }

    /* ---- Cycle switcher (header cluster) ---- */
    .cycle-switcher {
      position: relative;
      display: inline-flex;
      align-items: center;
      gap: 10px;
      font-size: 12px;
      color: #59636e;
    }
    .cycle-switcher .pill {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      height: 28px;
      padding: 0 10px;
      border-radius: 999px;
      border: 1px solid #d0d7de;
      background: #fff;
      color: #1f2328;
      font: inherit;
      font-size: 12px;
      font-weight: 500;
      cursor: pointer;
    }
    .cycle-switcher .pill:hover { border-color: #afb8c1; }
    .cycle-switcher .pill .caret { color: #59636e; font-size: 10px; }
    .cycle-switcher .pill .muted { color: #8b949e; font-weight: 400; }
    .cycle-dropdown {
      position: absolute;
      top: 32px;
      left: 0;
      z-index: 30;
      margin: 0;
      padding: 4px 0;
      list-style: none;
      background: #fff;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      box-shadow: 0 8px 24px rgba(31, 35, 40, 0.12);
      min-width: 220px;
      max-height: 320px;
      overflow-y: auto;
    }
    .cycle-dropdown li {
      display: flex;
      align-items: baseline;
      justify-content: space-between;
      gap: 12px;
      padding: 6px 12px;
      cursor: pointer;
      font-size: 13px;
      color: #1f2328;
    }
    .cycle-dropdown li:hover { background: #f3f4f6; }
    .cycle-dropdown li.current { font-weight: 600; background: #ddf4ff; }
    .cycle-dropdown li .muted { color: #8b949e; font-size: 11px; }
    .cycle-counts {
      color: #59636e;
      font-size: 12px;
      white-space: nowrap;
    }
    .cycle-counts .sep { color: #d0d7de; margin: 0 6px; }

    /* ---- End-sprint dialog ---- */
    .end-sprint-dialog .panel { width: 480px; }
    .end-sprint-dialog ul {
      margin: 8px 0 16px;
      padding-left: 20px;
      font-size: 13px;
      color: #1f2328;
      line-height: 1.6;
    }
    .end-sprint-dialog ul li { margin-bottom: 2px; }

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
      position: relative;
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 6px;
      padding: 10px 12px;
      margin-bottom: 8px;
      cursor: pointer;
      box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04);
    }
    /* Assignee avatar: small, bottom-right corner, doesn't compete
       with the meta chips. White ring for separation against the
       role badge when it happens to sit next to it. */
    .card .assignee {
      position: absolute;
      bottom: 8px;
      right: 8px;
      border-radius: 50%;
      box-shadow: 0 0 0 2px #fff;
    }
    .card:hover { border-color: #afb8c1; }
    /* DnD: the card the user is dragging fades; the column it's
       being dragged over picks up a subtle accent ring to confirm
       it will receive the drop. */
    .card[draggable="true"] { cursor: grab; }
    .card[draggable="true"]:active { cursor: grabbing; }
    .card.dragging { opacity: 0.4; }
    .col.drag-over {
      outline: 2px solid #0969da;
      outline-offset: -2px;
      background: #ddf4ff;
    }
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
    /* Inline assignee picker: keep the avatar + select on one row.
       The select gets the standard nottario-field chrome via the
       same chevron-normalisation pattern (see components/field.js). */
    .detail .meta .assignee-edit {
      display: inline-flex;
      align-items: center;
      gap: 6px;
    }
    .detail .meta .assignee-edit select {
      padding: 4px 28px 4px 8px;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      font: inherit;
      font-size: 12px;
      background: #fff;
      appearance: none;
      -webkit-appearance: none;
      background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 12 12'><path d='M3 4.5l3 3 3-3' fill='none' stroke='%2359636e' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/></svg>");
      background-repeat: no-repeat;
      background-position: right 8px center;
    }
    .detail .meta .assignee-edit select:focus {
      outline: 2px solid #0969da;
      border-color: #0969da;
    }

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

    /* Priority dropdown — same chrome as the new-task dialog's
       select, narrow enough not to dominate the meta row. */
    .detail .meta select.priority {
      padding: 2px 22px 2px 8px;
      border: 1px solid #d0d7de;
      border-radius: 4px;
      font: inherit;
      font-size: 12px;
      background: #ffffff;
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
    this._draggingID = null;
    this._dragOverState = null;
    this.cycles = [];
    this.cycleId = null;
    this._cycleDropdownOpen = false;
    this._endSprintOpen = false;
    new EscController(this, (e) => this._onEsc(e));
  }

  // ---- Drag and drop between columns ----------------------------
  _onCardDragStart(e, t) {
    this._draggingID = t.ID;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', t.ID);
  }
  _onCardDragEnd() {
    this._draggingID = null;
    this._dragOverState = null;
  }
  _onColDragOver(e, state) {
    if (!this._draggingID) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    if (this._dragOverState !== state) this._dragOverState = state;
  }
  _onColDragLeave(e, state) {
    // dragleave also fires when entering child elements; only clear
    // when the pointer truly left the section.
    if (!e.currentTarget.contains(e.relatedTarget)) {
      if (this._dragOverState === state) this._dragOverState = null;
    }
  }
  async _onColDrop(e, state) {
    e.preventDefault();
    const id = e.dataTransfer.getData('text/plain');
    this._draggingID = null;
    this._dragOverState = null;
    if (!id) return;
    const task = this.tasks.find(x => x.ID === id);
    if (!task || task.State === state) return;
    await this.setState(id, state);
  }

  _onEsc(e) {
    // Topmost first: the task detail panel sits over the create form
    // when both happen to be open. Stop propagation after closing so
    // an outer listener (topbar dropdown, etc.) doesn't also react.
    if (this._cycleDropdownOpen) { this._cycleDropdownOpen = false; e.stopPropagation(); return; }
    if (this._endSprintOpen) { this._endSprintOpen = false; e.stopPropagation(); return; }
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
    // Cycle deep-link (#cycle=<uuid>). When the hash changes (back/
    // forward, manual edit, switcher click) we reload the tasks for
    // the selected cycle.
    const cid = h.get('cycle') || null;
    if (cid !== this.cycleId) {
      this.cycleId = cid;
      // Don't await — keep the UI snappy; subsequent updates re-render.
      this.load();
    }
    const taskId = h.get('task');
    if (!taskId) return;
    const t = (this.tasks || []).find(x => x.ID === taskId);
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
      // Cycle lifecycle: a new cycle opened or the active one closed.
      // If we were explicitly viewing the cycle that just closed, snap
      // to the new active one — otherwise the view stays anchored on a
      // now-closed sprint and the user sees no narrowing after End
      // Sprint. replaceState avoids the hashchange recursion that a
      // direct `location.hash = ''` would trigger.
      if (ev.type === 'cycle.closed') {
        if (this.cycleId && ev.cycle_id === this.cycleId) {
          this.cycleId = null;
          const h = new URLSearchParams(window.location.hash.slice(1));
          h.delete('cycle');
          const s = h.toString();
          const url = s ? `#${s}` : window.location.pathname + window.location.search;
          history.replaceState(null, '', url);
        }
        this.load();
      } else if (ev.type === 'cycle.created') {
        this.load();
      }
    });
  }

  async load() {
    if (!this.projectId) return;
    try {
      const cycleParam = this.cycleId ? `&cycle_id=${encodeURIComponent(this.cycleId)}` : '';
      const [pr, tr, rr, mr, qr, dr, cr] = await Promise.all([
        fetch(`/api/projects/${this.projectId}`),
        fetch(`/api/projects/${this.projectId}/tasks?include_children=true${cycleParam}`),
        fetch(`/api/projects/${this.projectId}/roles`),
        fetch(`/api/projects/${this.projectId}/members`),
        fetch(`/api/projects/${this.projectId}/priorities`),
        fetch(`/api/projects/${this.projectId}/tasks/dependencies`),
        fetch(`/api/projects/${this.projectId}/cycles`),
      ]);
      if (!pr.ok) throw new Error('project not found');
      this.project = await pr.json();
      this.tasks = (await tr.json()).tasks || [];
      this.roles = (await rr.json()).roles || [];
      this.members = (await mr.json()).members || [];
      this.priorities = (await qr.json()).priorities || [];
      this.deps = (await dr.json()).dependencies || [];
      this.cycles = cr.ok ? ((await cr.json()).cycles || []) : [];
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

  // Find the priority bucket whose Value is closest to `value`. Used
  // to pre-select the dropdown when the stored priority happens to
  // land between buckets (e.g. someone set a raw integer via SQL or
  // the legacy number input).
  _nearestBucketKey(value) {
    if (!this.priorities || !this.priorities.length) return '';
    let best = this.priorities[0];
    let bestDiff = Math.abs(best.Value - value);
    for (let i = 1; i < this.priorities.length; i++) {
      const d = Math.abs(this.priorities[i].Value - value);
      if (d < bestDiff) { best = this.priorities[i]; bestDiff = d; }
    }
    return best.Key;
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
    if (f.assignee_user_id.value) body.assignee_user_id = f.assignee_user_id.value;
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

  // value is a priority bucket key (e.g. 'medium', 'high'). Resolve
  // it to the integer value the REST API expects via the cached
  // priorities catalogue.
  async setPriority(taskID, key) {
    const bucket = (this.priorities || []).find(p => p.Key === key);
    if (!bucket) return;
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${taskID}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ priority: bucket.Value }),
    });
    if (r.ok) {
      await this.load();
      if (this.selected) await this.loadDetail(taskID);
    }
  }

  async setAssignee(taskID, userID) {
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${taskID}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ assignee_user_id: userID }),
    });
    if (r.ok) {
      await this.load();
      if (this.selected) await this.loadDetail(taskID);
    } else {
      this.error = (await r.json().catch(() => ({}))).error || 'failed';
    }
  }

  renderCard(t) {
    const role = t.TargetRoleID ? this.roleByID(t.TargetRoleID) : null;
    const assignee = t.AssigneeUserID ? this._memberByID(t.AssigneeUserID) : null;
    const assigneeName = assignee
      ? (assignee.DisplayName || assignee.GithubLogin || '')
      : '';
    const a11yLabel = `${t.Title}, ${t.Type}, ${t.State}` +
      (role ? `, role ${role.Label}` : '') +
      `, priority ${this._priorityLabel(t.Priority)}` +
      (assigneeName ? `, assigned to ${assigneeName}` : '');
    const dragging = this._draggingID === t.ID;
    return html`
      <div class=${`card${dragging ? ' dragging' : ''}`}
           role="button"
           tabindex="0"
           draggable="true"
           aria-label=${a11yLabel}
           @click=${() => this.open(t)}
           @dragstart=${(e) => this._onCardDragStart(e, t)}
           @dragend=${() => this._onCardDragEnd()}
           @keydown=${(e) => {
             if (e.key === 'Enter' || e.key === ' ') {
               e.preventDefault();
               this.open(t);
             }
           }}>
        <div class="title">${t.Title}</div>
        <div class="meta">
          <span class="badge ${t.Type}">${t.Type}</span>
          <span class="prio">${this._priorityLabel(t.Priority)}</span>
          ${role ? html`<span class="badge" style=${`background:${role.Color || '#eee'}1a; border-color:${role.Color || '#ddd'}`}>${role.Label}</span>` : ''}
        </div>
        ${assignee ? html`
          <nottario-avatar class="assignee" size="20"
                          src=${assignee.AvatarURL || ''}
                          name=${assigneeName}
                          title=${assigneeName}></nottario-avatar>
        ` : null}
      </div>
    `;
  }

  // ---- Cycle helpers ------------------------------------------------

  // The cycle currently being viewed: explicit selection takes priority;
  // otherwise we follow whichever cycle is active (closed_at = null).
  _currentCycle() {
    const list = this.cycles || [];
    if (this.cycleId) return list.find(c => c.ID === this.cycleId) || null;
    return list.find(c => !c.ClosedAt) || null;
  }

  _toggleCycleDropdown() {
    this._cycleDropdownOpen = !this._cycleDropdownOpen;
  }

  _selectCycle(id) {
    this._cycleDropdownOpen = false;
    // Follow the active cycle when picking it (clean URL); otherwise
    // record the explicit selection in the hash so a refresh preserves
    // the view.
    const active = (this.cycles || []).find(c => !c.ClosedAt);
    if (active && active.ID === id) {
      // Use replaceState so we don't pollute history with hash flips.
      history.replaceState(null, '', window.location.pathname + window.location.search);
      this.cycleId = null;
    } else {
      window.location.hash = `cycle=${id}`;
      this.cycleId = id;
    }
    this.load();
  }

  // Compact status string for the current cycle: "3 doing · 5 todo · 2 done".
  // Counts run against this.tasks (already filtered to the viewed cycle
  // server-side).
  _cycleCountsString() {
    const tasks = this.tasks || [];
    const done = tasks.filter(t => t.State === 'done').length;
    const doing = tasks.filter(t => t.State === 'doing').length;
    const todo = tasks.filter(t => t.State === 'todo').length;
    const total = tasks.length;
    return `${done}/${total} done · ${doing} doing · ${todo} todo`;
  }

  // Can the current caller end the viewed sprint? Owner or instance
  // admin. The button is also hidden when the viewed cycle is closed.
  _canEndSprint() {
    if (!this.me || !this.project) return false;
    if (this.me.is_admin) return true;
    return this.project.OwnerUserID === this.me.id;
  }

  renderCycleSwitcher() {
    const current = this._currentCycle();
    if (!current) return null;
    const list = (this.cycles || []);
    return html`
      <div slot="actions" class="cycle-switcher">
        <button class="pill"
                aria-haspopup="listbox"
                aria-expanded=${this._cycleDropdownOpen ? 'true' : 'false'}
                @click=${() => this._toggleCycleDropdown()}>
          ${current.Name}
          ${!current.ClosedAt ? html`<span class="muted">(active)</span>` : html`<span class="muted">(closed)</span>`}
          <span class="caret">▾</span>
        </button>
        ${this._cycleDropdownOpen ? html`
          <ul class="cycle-dropdown" role="listbox">
            ${list.map(c => html`
              <li role="option"
                  aria-selected=${c.ID === current.ID ? 'true' : 'false'}
                  class=${c.ID === current.ID ? 'current' : ''}
                  @click=${() => this._selectCycle(c.ID)}>
                <span>${c.Name}</span>
                ${c.ClosedAt
                  ? html`<span class="muted">closed ${this._relTime(c.ClosedAt)}</span>`
                  : html`<span class="muted">active</span>`}
              </li>
            `)}
          </ul>` : null}
        <span class="cycle-counts">${this._cycleCountsString()}</span>
      </div>
    `;
  }

  // ---- End-sprint dialog -------------------------------------------

  // Snapshot used by the dialog copy. Mirrors the server-side rules in
  // internal/cycles/end_cycle.go so the user previews what the close
  // will actually do.
  _computeEndCounts() {
    const tasks = this.tasks || [];
    const features = tasks.filter(t => t.Type === 'feature' && t.State !== 'done');
    const partialFeatureIDs = new Set(features.map(f => f.ID));
    let partialFeatureDoneChildren = 0;
    for (const t of tasks) {
      if (t.ParentTaskID && partialFeatureIDs.has(t.ParentTaskID) && t.State === 'done') {
        partialFeatureDoneChildren++;
      }
    }
    return {
      doing: tasks.filter(t => t.State === 'doing').length,
      // Top-level todo only — children of a partial feature move with
      // their parent and shouldn't be double-counted.
      todo:  tasks.filter(t => t.State === 'todo' && !t.ParentTaskID).length,
      partialFeatures: features.length,
      partialFeatureDoneChildren,
      standaloneDone: tasks.filter(t => t.State === 'done' && !t.ParentTaskID).length,
    };
  }

  // "sprint-3" → "sprint-4"; falls back to "<name>-next" when there is
  // no trailing number to increment.
  _defaultNextName() {
    const current = this._currentCycle();
    if (!current) return '';
    const m = current.Name.match(/^(.*?)(\d+)$/);
    if (m) return m[1] + (parseInt(m[2], 10) + 1);
    return `${current.Name}-next`;
  }

  _openEndSprint() { this._endSprintOpen = true; }

  async _confirmEndSprint() {
    const dialog = this.shadowRoot.querySelector('.end-sprint-dialog');
    if (!dialog) return;
    const nextName = dialog.querySelector('[name=next_name]').value.trim();
    try {
      const r = await fetch(`/api/projects/${this.projectId}/cycles/end`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ next_name: nextName }),
      });
      if (!r.ok) {
        this.error = (await r.json().catch(() => ({}))).error || 'failed';
        return;
      }
      this._endSprintOpen = false;
      // Follow the new active cycle. Clear any cycle hash so the URL
      // doesn't lock us to the just-closed cycle.
      history.replaceState(null, '', window.location.pathname + window.location.search);
      this.cycleId = null;
      await this.load();
    } catch (e) {
      this.error = e.message;
    }
  }

  renderEndSprintDialog() {
    if (!this._endSprintOpen) return null;
    const current = this._currentCycle();
    if (!current) return null;
    const counts = this._computeEndCounts();
    const defaultName = this._defaultNextName();
    return html`
      <div class="dialog end-sprint-dialog"
           role="dialog"
           aria-modal="true"
           aria-labelledby="end-sprint-title"
           @click=${(e) => e.target.classList.contains('dialog') && (this._endSprintOpen = false)}
           @keydown=${(e) => { if (e.key === 'Escape') this._endSprintOpen = false; }}>
        <div class="panel">
          <h3 id="end-sprint-title">End ${current.Name}</h3>
          <nottario-field label="Next sprint name">
            <input name="next_name" .value=${defaultName}>
          </nottario-field>
          <p>This will:</p>
          <ul>
            <li>Close <strong>${current.Name}</strong> (irreversible).</li>
            <li>Move ${counts.doing} doing + ${counts.todo} todo tasks forward.</li>
            <li>Re-stamp ${counts.partialFeatures} partial features
              (incl. ${counts.partialFeatureDoneChildren} done children).</li>
            <li>Leave ${counts.standaloneDone} standalone done tasks in ${current.Name}.</li>
          </ul>
          <div class="actions-row">
            <button class="btn secondary"
                    @click=${() => this._endSprintOpen = false}>Cancel</button>
            <button class="btn danger"
                    @click=${() => this._confirmEndSprint()}>End ${current.Name}</button>
          </div>
        </div>
      </div>
    `;
  }

  render() {
    if (!this.project) return html`<p>Loading…</p>`;
    const doingCount = this.byState('doing').length;
    const hideDoing = this.view === 'kanban' && doingCount === 0 && !this.expandDoing;
    const current = this._currentCycle();
    const viewingActive = current && !current.ClosedAt;
    return html`
      <nottario-page-header
        .title=${this.view === 'gantt' ? 'Gantt' : 'Board'}>
        ${this.renderCycleSwitcher()}
        ${hideDoing
          ? html`<button slot="actions" class="btn ghost" title="Show the doing column"
                         @click=${() => this.expandDoing = true}>· 0 doing</button>`
          : null}
        ${this.view === 'gantt'
          ? html`<button slot="actions" class="btn ghost"
                         title="Scroll the Gantt back to the now line"
                         @click=${() => this.renderRoot.querySelector('nottario-gantt')?.scrollToNow()}>↻ Now</button>`
          : null}
        ${viewingActive && this._canEndSprint()
          ? html`<button slot="actions" class="btn danger"
                         title="Close this cycle and open the next"
                         @click=${() => this._openEndSprint()}>End ${current.Name}</button>`
          : null}
        <button slot="actions" class="btn primary"
                @click=${() => this.showCreate = true}>New task</button>
      </nottario-page-header>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      ${this.view === 'gantt'
        ? html`<nottario-gantt
                  .projectId=${this.projectId}
                  .cycleId=${this.cycleId || ''}
                  @task-selected=${(e) => this.open(e.detail.task)}></nottario-gantt>`
        : html`
          <div class=${hideDoing ? 'columns two' : 'columns'}>
            ${(hideDoing ? ['todo', 'done'] : ['todo', 'doing', 'done']).map(s => {
              const items = this.byState(s);
              const isEmpty = items.length === 0;
              const dragOver = this._dragOverState === s && this._draggingID;
              const draggingFromThis = this._draggingID && this.tasks.find(x => x.ID === this._draggingID)?.State === s;
              const baseCls = isEmpty
                ? (s === 'doing' ? 'col empty doing' : 'col empty')
                : 'col';
              const cls = `${baseCls}${dragOver && !draggingFromThis ? ' drag-over' : ''}`;
              return html`
                <section class=${cls}
                         role="region"
                         aria-label=${`${s} (${items.length})`}
                         @dragover=${(e) => this._onColDragOver(e, s)}
                         @dragleave=${(e) => this._onColDragLeave(e, s)}
                         @drop=${(e) => this._onColDrop(e, s)}>
                  <h3>${s} <span class="count">${items.length}</span></h3>
                  ${isEmpty
                    ? this._renderEmptyBody(s)
                    : items.map(t => this.renderCard(t))}
                </section>
              `;
            })}
          </div>
        `}
      ${this.showCreate ? this.renderCreate() : null}
      ${this.selected ? this.renderDetail() : null}
      ${this.renderEndSprintDialog()}
    `;
  }

  renderCreate() {
    return html`
      <div class="dialog"
           role="dialog"
           aria-modal="true"
           aria-labelledby="new-task-title"
           @click=${(e) => e.target.classList.contains('dialog') && (this.showCreate = false)}
           @keydown=${(e) => { if (e.key === 'Escape') this.showCreate = false; }}>
        <div class="panel">
          <h3 id="new-task-title">New task</h3>
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
            <nottario-field label="Assignee" hint="optional">
              <select name="assignee_user_id">
                <option value="">— none —</option>
                ${[...new Map((this.members || []).map(m => [m.UserID, m])).values()]
                  .map(m => html`<option value=${m.UserID}>${m.DisplayName || m.GithubLogin}</option>`)}
              </select>
            </nottario-field>
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
      <div class="dialog"
           role="dialog"
           aria-modal="true"
           aria-labelledby="task-dialog-title"
           @click=${(e) => e.target.classList.contains('dialog') && this.closeDetail()}
           @keydown=${(e) => { if (e.key === 'Escape') this.closeDetail(); }}>
        <div class="panel detail">
          <header class="head">
            <div class="title-row">
              <h3 id="task-dialog-title">${task.Title}</h3>
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
                <select class="priority"
                        @change=${(e) => this.setPriority(task.ID, e.target.value)}>
                  ${[...this.priorities].sort((a, b) => b.Value - a.Value).map(p => html`
                    <option value=${p.Key}
                            ?selected=${p.Key === this._nearestBucketKey(task.Priority)}>
                      ${p.Key} (${p.Value})
                    </option>
                  `)}
                </select>
              </div>

              <div class="field-line">
                <span class="lbl">Role</span>
                <span class="val">${role ? role.Label : html`<span class="muted">none</span>`}</span>
              </div>

              <div class="field-line">
                <span class="lbl">Assignee</span>
                <span class="val assignee-edit">
                  ${assignee && assignee.AvatarURL
                    ? html`<nottario-avatar size="20"
                              src=${assignee.AvatarURL}
                              name=${assignee.DisplayName || assignee.GithubLogin || ''}></nottario-avatar>`
                    : null}
                  <select @change=${(e) => this.setAssignee(task.ID, e.target.value)}>
                    <option value="" ?selected=${!task.AssigneeUserID}>— unassigned —</option>
                    ${[...new Map((this.members || []).map(m => [m.UserID, m])).values()]
                      .map(m => html`
                        <option value=${m.UserID} ?selected=${m.UserID === task.AssigneeUserID}>
                          ${m.DisplayName || m.GithubLogin}
                        </option>
                      `)}
                  </select>
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
