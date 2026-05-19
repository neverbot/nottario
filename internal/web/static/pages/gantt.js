import { LitElement, html, css, svg } from '/static/vendor/lit/lit.js';

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
    this.error = '';
    this.now = new Date();
  }

  connectedCallback() {
    super.connectedCallback();
    this.load();
    this._tick = setInterval(() => { this.now = new Date(); }, 60 * 1000);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    clearInterval(this._tick);
  }

  updated(c) {
    if (c.has('projectId')) this.load();
  }

  async load() {
    if (!this.projectId) return;
    try {
      const [tr, rr, dr] = await Promise.all([
        fetch(`/api/projects/${this.projectId}/tasks?include_children=true`),
        fetch(`/api/projects/${this.projectId}/roles`),
        fetch(`/api/projects/${this.projectId}/tasks/dependencies`),
      ]);
      this.tasks = (await tr.json()).tasks || [];
      this.roles = (await rr.json()).roles || [];
      this.deps = (await dr.json()).dependencies || [];
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

  // Compute topological depth for `todo` tasks within their band.
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
    const bandHeight = 56;
    const taskHeight = 24;
    const labelWidth = 120;
    const pastWidth = 360;
    const presentWidth = 80;
    const futureColumnWidth = 140;

    // Past zone: real time span from oldest actual_start to now.
    const pastStartCandidates = (this.tasks || [])
      .filter(t => t.ActualStart)
      .map(t => new Date(t.ActualStart).getTime());
    const pastMin = pastStartCandidates.length
      ? Math.min(...pastStartCandidates)
      : this.now.getTime() - 7 * 24 * 3600 * 1000;
    const pastMax = this.now.getTime();
    const pastSpan = Math.max(60_000, pastMax - pastMin);

    const xFromTime = (ms) => {
      const t = Math.max(pastMin, Math.min(pastMax, ms));
      return labelWidth + ((t - pastMin) / pastSpan) * pastWidth;
    };
    const presentX = labelWidth + pastWidth;
    const futureStartX = presentX + presentWidth;

    // Future zone topological columns per band.
    const futureDepthsPerBand = bands.map(b => this.computeTopoDepths(b.tasks));

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
    const height = bands.length * bandHeight + 60;
    const headerH = 28;

    return html`
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      <div class="stage">
        <svg width=${width} height=${height}>
          <defs>
            <marker id="dep-arrowhead" viewBox="0 0 10 10" refX="9" refY="5"
                    markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 0 L 10 5 L 0 10 z" fill="#afb8c1"></path>
            </marker>
          </defs>

          <!-- Zone labels -->
          <text class="zone-label" x=${labelWidth + 4} y="14">PAST · real time</text>
          <text class="zone-label" x=${presentX + 4} y="14">NOW</text>
          <text class="zone-label" x=${futureStartX + 4} y="14">FUTURE · topological</text>

          <!-- Band rows -->
          ${bands.map((b, bi) => svg`
            <rect class=${`band-bg ${bi % 2 ? 'alt' : ''}`}
                  x="0" y=${headerH + bi * bandHeight}
                  width=${width} height=${bandHeight}></rect>
            <text class="band-label"
                  x="8" y=${headerH + bi * bandHeight + bandHeight / 2 + 4}>
              ${b.role.Label}
            </text>
          `)}

          <!-- Zone dividers -->
          <line class="zone-divider" x1=${labelWidth} y1="0" x2=${labelWidth} y2=${height}></line>
          <line class="zone-divider" x1=${presentX} y1="0" x2=${presentX} y2=${height}></line>
          <line class="zone-divider" x1=${futureStartX} y1="0" x2=${futureStartX} y2=${height}></line>

          <!-- "now" marker (vertical line over the present zone) -->
          <line class="now-line"
                x1=${presentX + presentWidth / 2} y1=${headerH - 8}
                x2=${presentX + presentWidth / 2} y2=${height}></line>
          <text class="now-label"
                x=${presentX + presentWidth / 2}
                y=${headerH - 12}
                text-anchor="middle">now</text>

          <!-- Tasks -->
          ${bands.map((b, bi) => {
            const bandY = headerH + bi * bandHeight + (bandHeight - taskHeight) / 2;
            const depths = futureDepthsPerBand[bi];
            const color = b.role.Color || '#59636e';
            return b.tasks.map((t, ti) => {
              const x = this.taskX({ t, bandY, xFromTime, presentX, presentWidth, futureStartX, futureColumnWidth, depths });
              if (!x) return null;
              // Stagger overlapping tasks slightly within their band so
              // labels do not collide vertically.
              const yOffset = (ti % 2 === 0) ? 0 : 0;
              return svg`
                <rect class=${`task-rect ${t.State}`}
                      x=${x.from} y=${bandY + yOffset}
                      width=${Math.max(8, x.to - x.from)} height=${taskHeight}
                      fill=${t.State === 'done' ? '#d1d9e0' : (t.State === 'doing' ? color : 'transparent')}
                      stroke=${color}
                      @click=${(e) => { e.stopPropagation(); this._emitSelect(t); }}>
                  <title>${t.Title}</title>
                </rect>
                <text class=${`task-label ${t.State === 'doing' ? 'on-dark' : ''}`}
                      x=${x.from + 6} y=${bandY + yOffset + 16}
                      style="pointer-events:none">
                  ${this._truncate(t.Title, Math.max(8, Math.floor((x.to - x.from) / 7)))}
                </text>
              `;
            });
          })}

          <!-- Dependency arrows (only show edges where at least one endpoint is visible at this view) -->
          ${this._renderArrows(bands, futureDepthsPerBand, {
            xFromTime, presentX, presentWidth, futureStartX, futureColumnWidth,
            headerH, bandHeight, taskHeight,
          })}
        </svg>
      </div>
      <div class="legend">
        <span><span class="swatch" style="background:#d1d9e0;border:1px solid #afb8c1"></span> done</span>
        <span><span class="swatch" style="background:#1f6feb"></span> doing</span>
        <span><span class="swatch" style="border:1px dashed #59636e"></span> todo (future, topological)</span>
        <span><span class="swatch" style="background:#cf222e;width:2px"></span> now</span>
      </div>
    `;
  }

  // Compute the (from, to) X coordinates of a task on the timeline.
  // Returns null when the task cannot be placed (e.g. doing without
  // actual_start, which shouldn't happen).
  taskX({ t, xFromTime, presentX, presentWidth, futureStartX, futureColumnWidth, depths }) {
    if (t.State === 'done') {
      if (!t.ActualStart || !t.ActualEnd) return null;
      const from = xFromTime(new Date(t.ActualStart).getTime());
      const to = Math.max(from + 4, xFromTime(new Date(t.ActualEnd).getTime()));
      return { from, to };
    }
    if (t.State === 'doing') {
      if (!t.ActualStart) {
        return { from: presentX + 4, to: presentX + presentWidth - 4 };
      }
      const from = xFromTime(new Date(t.ActualStart).getTime());
      const to = presentX + presentWidth / 2;
      return { from, to: Math.max(from + 4, to) };
    }
    // todo
    const d = depths.get(t.ID) || 0;
    const from = futureStartX + d * futureColumnWidth + 12;
    return { from, to: from + futureColumnWidth - 24 };
  }

  _renderArrows(bands, futureDepthsPerBand, geom) {
    const { xFromTime, presentX, presentWidth, futureStartX, futureColumnWidth,
            headerH, bandHeight, taskHeight } = geom;
    const allTasks = bands.flatMap(b => b.tasks);
    const taskByID = new Map(allTasks.map(t => [t.ID, t]));
    const bandIndexByTask = new Map();
    bands.forEach((b, bi) => b.tasks.forEach(t => bandIndexByTask.set(t.ID, bi)));
    const arrows = [];
    for (const d of this.deps) {
      const from = taskByID.get(d.DependsOnID);
      const to = taskByID.get(d.TaskID);
      if (!from || !to) continue;
      const fromBand = bandIndexByTask.get(from.ID);
      const toBand = bandIndexByTask.get(to.ID);
      const depthsFrom = futureDepthsPerBand[fromBand];
      const depthsTo = futureDepthsPerBand[toBand];
      const a = this.taskX({ t: from, xFromTime, presentX, presentWidth, futureStartX, futureColumnWidth, depths: depthsFrom });
      const b = this.taskX({ t: to, xFromTime, presentX, presentWidth, futureStartX, futureColumnWidth, depths: depthsTo });
      if (!a || !b) continue;
      const x1 = a.to;
      const y1 = headerH + fromBand * bandHeight + bandHeight / 2;
      const x2 = b.from;
      const y2 = headerH + toBand * bandHeight + bandHeight / 2;
      const cx = (x1 + x2) / 2;
      arrows.push(svg`
        <path class="arrow" d=${`M ${x1} ${y1} C ${cx} ${y1} ${cx} ${y2} ${x2} ${y2}`}></path>
      `);
    }
    return arrows;
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
