import { LitElement, html, css, svg } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';

// <nottario-gantt> renders the project's tasks as a horizontal
// timeline with three zones on the X axis:
//
//   PAST       PRESENT       FUTURE
//   ──────────►│ now │◄──────────
//   real time             topological order
//
// • Past zone: tasks in state 'done' are positioned between their
//   actual_start and actual_end on a real-time axis.
// • Present zone: tasks in 'doing' run from actual_start to the
//   "now" line.
// • Future zone: 'todo' tasks are positioned by topological depth
//   in the dependency graph (no calendar time).
//
// The Y axis groups by target_role (one band each) with a "general"
// band for tasks without a role.

class NottarioGantt extends LitElement {
  static properties = {
    projectId: { type: String, attribute: 'project-id' },
    tasks: { state: true },
    roles: { state: true },
    deps: { state: true },
    members: { state: true },
    error: { state: true },
    now: { state: true },
  };

  static styles = css`
    :host { display: block; }
    .stage {
      position: relative;
      background: #fff;
      border: 1px solid #d1d9e0;
      border-radius: 8px;
      overflow: auto;
      max-height: 70vh;
    }
    svg { display: block; }
    .band-bg {
      fill: #f6f8fa;
    }
    .band-bg.alt {
      fill: #fff;
    }
    .band-label {
      fill: #1f2328;
      font-size: 12px;
      font-weight: 600;
    }
    .zone-label {
      fill: #59636e;
      font-size: 10px;
      text-transform: uppercase;
      font-family: ui-monospace, SFMono-Regular, monospace;
    }
    .zone-divider {
      stroke: #afb8c1;
      stroke-width: 1;
      stroke-dasharray: 3 3;
    }
    .now-line {
      stroke: #cf222e;
      stroke-width: 1.5;
    }
    .now-label {
      fill: #cf222e;
      font-size: 11px;
      font-weight: 600;
    }
    .task-rect {
      stroke-width: 1.5;
      cursor: pointer;
      rx: 4;
      ry: 4;
    }
    .task-rect:hover { stroke: #1f2328; }
    .task-rect.done {
      fill: #d1d9e0;
      stroke: #afb8c1;
    }
    .task-rect.doing {
      stroke: #1f6feb;
    }
    .task-rect.todo {
      stroke-dasharray: 4 3;
    }
    /* Bug-type tasks get a tight dotted red stroke regardless of state. */
    .task-rect.bug {
      stroke: #cf222e !important;
      stroke-dasharray: 2 3 !important;
    }
    /* Inconsistent: the task is not yet done but one of its dependents
       is already done. Surface as a solid red 2.5px border. */
    .task-rect.inconsistent {
      stroke: #cf222e !important;
      stroke-dasharray: 0 !important;
      stroke-width: 2.5 !important;
    }
    .task-label {
      fill: #1f2328;
      font-size: 11px;
      pointer-events: none;
    }
    .task-label.on-dark { fill: #fff; }
    .arrow {
      stroke: #afb8c1;
      stroke-width: 1;
      fill: none;
      marker-end: url(#dep-arrowhead);
    }
    .legend {
      padding: 8px 16px;
      font-size: 11px;
      color: #59636e;
      display: flex;
      gap: 16px;
      flex-wrap: wrap;
    }
    .legend .swatch {
      display: inline-block;
      width: 10px;
      height: 10px;
      border-radius: 2px;
      vertical-align: middle;
      margin-right: 4px;
    }
    .error {
      color: #cf222e;
      background: #ffebe9;
      padding: 8px 12px;
      border-radius: 6px;
      margin-bottom: 8px;
    }
    .empty {
      padding: 40px;
      text-align: center;
      color: #59636e;
    }
  `;

  constructor() {
    super();
    this.tasks = null;
    this.roles = [];
    this.deps = [];
    this.members = [];
    this.error = '';
    this.now = new Date();
  }

  connectedCallback() {
    super.connectedCallback();
    this.load();
    this._subscribe();
    // Keep the "now" line live; once per minute is enough for the UI.
    this._tick = setInterval(() => { this.now = new Date(); }, 60 * 1000);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    clearInterval(this._tick);
    this._unsub?.();
  }

  updated(c) {
    if (c.has('projectId')) {
      this.load();
      this._subscribe();
      this._initialCenterDone = false; // re-centre when project changes
    }
    // Once the SVG is in the DOM and has its first computed layout,
    // scroll the stage so the "now" line sits in the middle. We only
    // do this once per project so we don't fight the user's scroll.
    if (!this._initialCenterDone && this.tasks && this.tasks.length) {
      this._centerOnNow();
    }
  }

  _centerOnNow() {
    const stage = this.shadowRoot?.querySelector('.stage');
    const nowLine = this.shadowRoot?.querySelector('.now-line');
    if (!stage || !nowLine) return;
    const nowX = parseFloat(nowLine.getAttribute('x1') || '0');
    const target = Math.max(0, nowX - stage.clientWidth / 2);
    stage.scrollLeft = target;
    this._initialCenterDone = true;
  }

  _subscribe() {
    this._unsub?.();
    if (!this.projectId) return;
    this._unsub = subscribe(this.projectId, (ev) => {
      // Tasks and dependencies are what the gantt draws; reload on
      // any related event.
      if (ev.type?.startsWith('task.')) this.load();
    });
  }

  async load() {
    if (!this.projectId) return;
    try {
      const [tr, rr, dr, mr] = await Promise.all([
        fetch(`/api/projects/${this.projectId}/tasks?include_children=true`),
        fetch(`/api/projects/${this.projectId}/roles`),
        fetch(`/api/projects/${this.projectId}/tasks/dependencies`),
        fetch(`/api/projects/${this.projectId}/members`),
      ]);
      this.tasks = (await tr.json()).tasks || [];
      this.roles = (await rr.json()).roles || [];
      this.deps = (await dr.json()).dependencies || [];
      this.members = (await mr.json()).members || [];
    } catch (e) {
      this.error = e.message;
    }
  }

  // Group tasks by role; falls back to a "general" pseudo-role for
  // tasks without target_role_id.
  bands() {
    const order = [...this.roles];
    const general = { ID: '__general__', Key: 'general', Label: 'general', Color: '#59636e' };
    const result = order.map(r => ({ role: r, tasks: [] }));
    result.push({ role: general, tasks: [] });
    const byID = new Map(order.map(r => [r.ID, result.find(b => b.role.ID === r.ID)]));
    for (const t of this.tasks || []) {
      const band = t.TargetRoleID ? byID.get(t.TargetRoleID) : result[result.length - 1];
      (band || result[result.length - 1]).tasks.push(t);
    }
    // Drop bands with no tasks except the general one if it has any.
    return result.filter(b => b.tasks.length > 0);
  }

  // Compute topological depth across ALL todo tasks in the project,
  // not per band. A task depending on a task in another band still
  // ends up further to the right by columns, so dependency arrows
  // never collapse to perfectly vertical lines.
  //
  // Depth = 0 when the task has no `todo`/`doing` predecessors;
  // otherwise = max(predecessor.depth) + 1.
  computeTopoDepths(tasks) {
    const taskByID = new Map(tasks.map(t => [t.ID, t]));
    const incoming = new Map();
    for (const t of tasks) incoming.set(t.ID, []);
    for (const d of this.deps) {
      if (incoming.has(d.TaskID) && taskByID.has(d.DependsOnID)) {
        incoming.get(d.TaskID).push(d.DependsOnID);
      }
    }
    const depth = new Map();
    const visit = (id) => {
      if (depth.has(id)) return depth.get(id);
      let d = 0;
      for (const pre of incoming.get(id) || []) {
        const t = taskByID.get(pre);
        if (!t) continue;
        // Done predecessors don't push a `todo` deeper into the future.
        if (t.State === 'done') continue;
        d = Math.max(d, visit(pre) + 1);
      }
      depth.set(id, d);
      return d;
    };
    for (const t of tasks) visit(t.ID);
    return depth;
  }

  render() {
    if (this.tasks === null) return html`<p>Loading…</p>`;
    if (!this.tasks.length) {
      return html`<div class="stage empty">No tasks yet. Create one in the Kanban board.</div>`;
    }

    const bands = this.bands();
    if (!bands.length) return html`<div class="stage empty">No tasks yet.</div>`;

    // Geometry ----------------------------------------------------------
    const taskHeight = 32;
    const minBarWidth = 140;
    const avatarSize = 22;
    const avatarPad = 5;
    const laneGap = 6;
    const laneHeight = taskHeight + laneGap;
    const bandPad = 8;            // vertical padding inside a band
    const labelWidth = 120;
    const pastSlotW = minBarWidth + 6;     // 146 — one done task per ordinal slot
    const futureColumnWidth = minBarWidth + 16;
    const headerH = 28;

    // ---- Past zone: ordinal slots ----
    // Each band orders its done tasks chronologically by actual_end and
    // assigns each one a fixed-width slot. The past axis becomes
    // "chronological order" instead of "real time", which keeps tasks
    // legible regardless of how close together they happened. The
    // tooltip on each bar still shows the actual timestamp.
    const pastSlotPerBand = bands.map(b => {
      const done = b.tasks
        .filter(t => t.State === 'done' && t.ActualEnd)
        .sort((a, b) => new Date(a.ActualEnd).getTime() - new Date(b.ActualEnd).getTime());
      const m = new Map();
      done.forEach((t, i) => m.set(t.ID, i));
      return m;
    });
    const maxPastSlots = pastSlotPerBand.reduce((m, x) => Math.max(m, x.size), 0);
    const pastWidth = Math.max(360, maxPastSlots * pastSlotW + 12);

    // The present zone gets enough room for one min-width bar plus
    // padding. Multiple concurrent `doing` tasks stack into lanes
    // (handled by the lane assignment below).
    const presentWidth = minBarWidth + 24;

    const presentX = labelWidth + pastWidth;
    const futureStartX = presentX + presentWidth;

    // Future zone topological columns: depth is computed GLOBALLY,
    // not per band, so a task that depends on a task in a different
    // band still ends up further to the right.
    const globalDepths = this.computeTopoDepths(this.tasks || []);
    const futureDepthsPerBand = bands.map(() => globalDepths);

    // ---- Lane assignment per band ----
    // For each band, position every task on the X axis, then greedily
    // place it into the lowest-index lane whose previous occupant has
    // already ended (with a small horizontal gap). The number of lanes
    // determines the band's vertical height.
    const overlapGap = 6;
    const positions = []; // { task, bi, lane, from, to }
    const lanesPerBand = bands.map(() => 1);
    bands.forEach((b, bi) => {
      const depths = futureDepthsPerBand[bi];
      const slots = pastSlotPerBand[bi];
      const ranged = [];
      for (const t of b.tasks) {
        const pastSlot = (t.State === 'done') ? (slots.get(t.ID) ?? null) : null;
        const x = this.taskX({
          t, labelWidth, pastSlotW, pastSlot,
          presentX, presentWidth, futureStartX, futureColumnWidth,
          depths, minBarWidth,
        });
        if (!x) continue;
        ranged.push({ task: t, from: x.from, to: x.to });
      }
      ranged.sort((a, b) => a.from - b.from);
      const laneEnds = []; // laneEnds[i] = X where lane i is occupied until
      for (const p of ranged) {
        let placed = -1;
        for (let li = 0; li < laneEnds.length; li++) {
          if (laneEnds[li] + overlapGap <= p.from) {
            placed = li;
            break;
          }
        }
        if (placed < 0) {
          placed = laneEnds.length;
          laneEnds.push(p.to);
        } else {
          laneEnds[placed] = p.to;
        }
        positions.push({
          task: p.task, bi, lane: placed,
          from: p.from,
          to: p.to,
        });
      }
      lanesPerBand[bi] = Math.max(1, laneEnds.length);
    });

    // ---- Compute each band's Y top from the lane counts ----
    const bandTops = [];
    const bandHeights = [];
    {
      let cursor = headerH;
      for (let bi = 0; bi < bands.length; bi++) {
        bandTops.push(cursor);
        const h = lanesPerBand[bi] * laneHeight + bandPad * 2 - laneGap;
        bandHeights.push(h);
        cursor += h;
      }
    }
    const totalHeight = bandTops.length
      ? bandTops[bandTops.length - 1] + bandHeights[bandHeights.length - 1] + 16
      : headerH + 80;

    // Y of a task's top edge given band index + lane.
    const taskY = (bi, lane) => bandTops[bi] + bandPad + lane * laneHeight;
    // Y of a task's vertical centre (used by dependency arrows).
    const taskCenterY = (bi, lane) => taskY(bi, lane) + taskHeight / 2;

    // Calculate total svg width.
    const maxDepth = futureDepthsPerBand.reduce((m, dmap) => {
      for (const t of dmap.keys()) {
        const tk = bands.flatMap(b => b.tasks).find(x => x.ID === t);
        if (tk && tk.State === 'todo') m = Math.max(m, dmap.get(t));
      }
      return m;
    }, 0);
    const futureWidth = (maxDepth + 1) * futureColumnWidth;
    const width = futureStartX + futureWidth + 16;

    // Build an index for dependency arrows.
    const posByTaskID = new Map(positions.map(p => [p.task.ID, p]));

    // Find tasks whose state is not 'done' but at least one task that
    // depends on them is already 'done'. That's a logical
    // inconsistency: the dependent completed before its precondition.
    // We surface it on the bar with a solid red border.
    const stateByID = new Map((this.tasks || []).map(t => [t.ID, t.State]));
    const dependentsByID = new Map();
    for (const d of this.deps) {
      // d.TaskID depends on d.DependsOnID — so d.TaskID is a dependent
      // (later task) of d.DependsOnID.
      if (!dependentsByID.has(d.DependsOnID)) dependentsByID.set(d.DependsOnID, []);
      dependentsByID.get(d.DependsOnID).push(d.TaskID);
    }
    const inconsistentIDs = new Set();
    for (const t of this.tasks || []) {
      if (t.State === 'done') continue;
      for (const dependentID of dependentsByID.get(t.ID) || []) {
        if (stateByID.get(dependentID) === 'done') {
          inconsistentIDs.add(t.ID);
          break;
        }
      }
    }

    // Index members by user id so we can render the assignee avatar
    // inside the task box. A user appears multiple times in the
    // memberships list (once per role), so dedupe by UserID.
    const usersById = new Map();
    for (const m of this.members || []) {
      if (!usersById.has(m.UserID)) {
        usersById.set(m.UserID, {
          AvatarURL: m.AvatarURL,
          DisplayName: m.DisplayName,
          GithubLogin: m.GithubLogin,
        });
      }
    }

    return html`
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      <div class="stage">
        <svg width=${width} height=${totalHeight}>
          <defs>
            <marker id="dep-arrowhead" viewBox="0 0 10 10" refX="0" refY="5"
                    markerWidth="8" markerHeight="8" orient="auto-start-reverse">
              <path d="M 0 0 L 10 5 L 0 10 z" fill="#afb8c1"></path>
            </marker>
            <clipPath id="gantt-avatar-clip">
              <circle cx=${avatarSize / 2} cy=${avatarSize / 2} r=${avatarSize / 2}></circle>
            </clipPath>
          </defs>

          <!-- Zone labels -->
          <text class="zone-label" x=${labelWidth + 4} y="14">PAST · chronological</text>
          <text class="zone-label" x=${presentX + 4} y="14">NOW</text>
          <text class="zone-label" x=${futureStartX + 4} y="14">FUTURE · topological</text>

          <!-- Band rows -->
          ${bands.map((b, bi) => svg`
            <rect class=${`band-bg ${bi % 2 ? 'alt' : ''}`}
                  x="0" y=${bandTops[bi]}
                  width=${width} height=${bandHeights[bi]}></rect>
            <text class="band-label"
                  x="8" y=${bandTops[bi] + bandHeights[bi] / 2 + 4}>
              ${b.role.Label}
            </text>
          `)}

          <!-- Zone dividers -->
          <line class="zone-divider" x1=${labelWidth} y1="0" x2=${labelWidth} y2=${totalHeight}></line>
          <line class="zone-divider" x1=${presentX} y1="0" x2=${presentX} y2=${totalHeight}></line>
          <line class="zone-divider" x1=${futureStartX} y1="0" x2=${futureStartX} y2=${totalHeight}></line>

          <!-- "now" marker -->
          <line class="now-line"
                x1=${presentX + presentWidth / 2} y1=${headerH - 8}
                x2=${presentX + presentWidth / 2} y2=${totalHeight}></line>
          <text class="now-label"
                x=${presentX + presentWidth / 2}
                y=${headerH - 12}
                text-anchor="middle">now</text>

          <!-- Tasks placed in their lane -->
          ${positions.map(p => {
            const t = p.task;
            const color = bands[p.bi].role.Color || '#59636e';
            const y = taskY(p.bi, p.lane);
            const w = Math.max(8, p.to - p.from);
            const user = t.AssigneeUserID ? usersById.get(t.AssigneeUserID) : null;
            const avatarX = p.from + avatarPad;
            const avatarY = y + (taskHeight - avatarSize) / 2;
            const labelX = user ? avatarX + avatarSize + 6 : p.from + 8;
            const labelMaxChars = Math.max(6, Math.floor((p.to - labelX - 6) / 7));
            return svg`
              <rect class=${`task-rect ${t.State}${t.Type === 'bug' ? ' bug' : ''}${inconsistentIDs.has(t.ID) ? ' inconsistent' : ''}`}
                    x=${p.from} y=${y}
                    width=${w} height=${taskHeight}
                    rx="6" ry="6"
                    fill=${t.State === 'done' ? '#d1d9e0' : (t.State === 'doing' ? color : 'transparent')}
                    stroke=${color}
                    @click=${(e) => { e.stopPropagation(); this._emitSelect(t); }}>
                <title>${t.Title}${user ? ` — assigned to ${user.DisplayName}` : ''}</title>
              </rect>
              ${user && user.AvatarURL ? svg`
                <g transform=${`translate(${avatarX}, ${avatarY})`} style="pointer-events:none">
                  <image href=${user.AvatarURL}
                         width=${avatarSize} height=${avatarSize}
                         clip-path="url(#gantt-avatar-clip)"></image>
                  <circle cx=${avatarSize / 2} cy=${avatarSize / 2} r=${avatarSize / 2}
                          fill="none" stroke="rgba(0,0,0,0.15)" stroke-width="1"></circle>
                </g>
              ` : null}
              <text class=${`task-label ${t.State === 'doing' ? 'on-dark' : ''}`}
                    x=${labelX} y=${y + taskHeight / 2 + 4}
                    style="pointer-events:none">
                ${this._truncate(t.Title, labelMaxChars)}
              </text>
            `;
          })}

          <!-- Dependency arrows: now anchored to each task's actual lane Y. -->
          ${this.deps.map(d => {
            const from = posByTaskID.get(d.DependsOnID);
            const to = posByTaskID.get(d.TaskID);
            if (!from || !to) return null;
            const x1 = from.to;
            const y1 = taskCenterY(from.bi, from.lane);
            const x2 = to.from;
            const y2 = taskCenterY(to.bi, to.lane);
            const cx = (x1 + x2) / 2;
            return svg`<path class="arrow" d=${`M ${x1} ${y1} C ${cx} ${y1} ${cx} ${y2} ${x2} ${y2}`}></path>`;
          })}
        </svg>
      </div>
      <div class="legend">
        <span><span class="swatch" style="background:#d1d9e0;border:1px solid #afb8c1"></span> done</span>
        <span><span class="swatch" style="background:#1f6feb"></span> doing</span>
        <span><span class="swatch" style="border:1px dashed #59636e"></span> todo</span>
        <span><span class="swatch" style="border:1px dotted #cf222e"></span> bug</span>
        <span><span class="swatch" style="border:2px solid #cf222e"></span> inconsistent (dependent already done)</span>
        <span><span class="swatch" style="background:#cf222e;width:2px"></span> now</span>
      </div>
    `;
  }

  // Compute the (from, to) X coordinates of a task on the timeline.
  //
  // Past tasks use their ordinal slot index (passed in as pastSlot).
  // Doing tasks are anchored to a fixed slot in the present zone.
  // Todo tasks are placed by their topological depth in the future
  // zone. Returns null when the task cannot be placed.
  taskX({ t, labelWidth, pastSlotW, pastSlot, presentX, presentWidth, futureStartX, futureColumnWidth, depths, minBarWidth }) {
    if (t.State === 'done') {
      if (pastSlot == null) return null; // no actual_end ⇒ not on past axis
      const from = labelWidth + pastSlot * pastSlotW + 6;
      return { from, to: from + minBarWidth };
    }
    if (t.State === 'doing') {
      const from = presentX + 12;
      return { from, to: from + minBarWidth };
    }
    // todo
    const d = depths.get(t.ID) || 0;
    const from = futureStartX + d * futureColumnWidth + 12;
    return { from, to: from + minBarWidth };
  }

  _emitSelect(t) {
    this.dispatchEvent(new CustomEvent('task-selected', {
      detail: { task: t },
      bubbles: true,
      composed: true,
    }));
  }

  _truncate(s, n) {
    if (!s) return '';
    return s.length > n ? s.slice(0, Math.max(0, n - 1)) + '…' : s;
  }
}

customElements.define('nottario-gantt', NottarioGantt);
