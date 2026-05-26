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
    priorities: { state: true },
    error: { state: true },
    now: { state: true },
    foldedFeatures: { state: true },
    _hover: { state: true },
    _selectedAnchor: { state: true },
    _selectedSet: { state: true },
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
    .band-bg.features {
      fill: #f0f2f5;
    }
    .band-label, .features-label {
      fill: #57606a;
      font-size: 10px;
      letter-spacing: 0.06em;
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-weight: 600;
    }
    .zone-label {
      fill: #59636e;
      font-size: 10px;
      text-transform: uppercase;
      font-family: ui-monospace, SFMono-Regular, monospace;
    }
    .priority-label {
      fill: #57606a;
      font-size: 10px;
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-weight: 600;
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
    /* Selection: when something is selected, non-connected bars dim
       and non-connected arrows fade so the focused subgraph reads
       loudly. Promoted arrows render in a second pass (after the
       task rects) so they sit on top of any box they cross. */
    .task-rect.dim { opacity: 0.35; }
    .arrow.dim { opacity: 0.18; }
    .arrow { cursor: pointer; }
    .arrow.promoted { stroke: #1f2328; }
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

    /* Hover popup. Replaces the native browser <title> tooltip on
       Gantt bars with something that matches the rest of the app
       (neutral palette, system sans, thin border). The .stage is
       position:relative so this card layers above the SVG; we
       compute the (left, top) when the hover fires and let CSS
       handle the rendering. */
    .hover-card {
      position: absolute;
      z-index: 5;
      pointer-events: none;
      background: #fff;
      color: #1f2328;
      border: 1px solid #d0d7de;
      border-radius: 8px;
      box-shadow: 0 4px 12px rgba(31, 35, 40, 0.12);
      padding: 8px 10px;
      min-width: 220px;
      max-width: 320px;
      font-size: 12px;
      line-height: 1.4;
    }
    .hover-card .title {
      font-size: 13px;
      font-weight: 600;
      color: #1f2328;
      margin-bottom: 4px;
    }
    .hover-card .row {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
      margin-top: 4px;
    }
    .hover-card .chip {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      padding: 1px 6px;
      border-radius: 4px;
      font-size: 11px;
      line-height: 1.3;
      font-weight: 500;
      background: #eaeef2;
      color: #1f2328;
    }
    .hover-card .chip.state-todo  { background: #eaeef2; color: #59636e; }
    .hover-card .chip.state-doing { background: #ddf4ff; color: #0969da; }
    .hover-card .chip.state-done  { background: #dafbe1; color: #1a7f37; }
    .hover-card .chip.type-bug    { background: #ffebe9; color: #cf222e; }
    .hover-card .chip.role        { color: #fff; }
    .hover-card .assignee {
      display: flex;
      align-items: center;
      gap: 6px;
      margin-top: 6px;
      color: #59636e;
    }
    .hover-card .assignee img {
      width: 16px; height: 16px;
      border-radius: 50%;
      object-fit: cover;
    }
    .hover-card .meta {
      color: #59636e;
      margin-top: 4px;
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
    this.foldedFeatures = new Set();      // feature IDs currently collapsed
    this._knownFeatureIDs = new Set();    // features we've ever seen
    // Hover-popup state. null when nothing is hovered/focused.
    // Carries the source task plus the anchor pixel coords relative
    // to the stage's content (so we can flip when overflowing).
    this._hover = null;
    this._reducedMotion = (typeof window !== 'undefined' && window.matchMedia)
      ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
      : false;
    // Selection: the anchor is the task the user actually clicked
    // first; the set is anchor + every task transitively reachable
    // via incoming/outgoing dependency edges (undirected BFS over
    // `this.deps`). Both null/empty means no selection.
    this._selectedAnchor = null;
    this._selectedSet = new Set();
  }

  // Compute the undirected connected component of taskID in the
  // dependency graph. Result is a Set of task IDs that always
  // contains taskID itself.
  _computeConnectedSet(taskID) {
    const adj = new Map();
    const add = (a, b) => {
      if (!adj.has(a)) adj.set(a, new Set());
      adj.get(a).add(b);
    };
    for (const d of (this.deps || [])) {
      add(d.TaskID, d.DependsOnID);
      add(d.DependsOnID, d.TaskID);
    }
    const out = new Set([taskID]);
    const stack = [taskID];
    while (stack.length) {
      const id = stack.pop();
      for (const n of (adj.get(id) || [])) {
        if (!out.has(n)) { out.add(n); stack.push(n); }
      }
    }
    return out;
  }

  _selectTask(taskID) {
    this._selectedAnchor = taskID;
    this._selectedSet = this._computeConnectedSet(taskID);
    this.requestUpdate();
  }

  _clearSelection() {
    if (!this._selectedAnchor && this._selectedSet.size === 0) return;
    this._selectedAnchor = null;
    this._selectedSet = new Set();
    this.requestUpdate();
  }

  // Click on a task bar. If the task is already in the selected set,
  // open the detail panel (the historical single-click behaviour).
  // Otherwise replace the selection with this task's connected set.
  _onTaskClick(t) {
    if (this._selectedSet.has(t.ID)) {
      this._emitSelect(t);
      return;
    }
    this._selectTask(t.ID);
  }

  // Click on a dependency arrow. Selects the union of both endpoints'
  // connected components (which is the same component since they're
  // already connected).
  _onArrowClick(fromID) {
    this._selectTask(fromID);
  }

  // Update the folded set when fresh tasks arrive: new feature IDs
  // start collapsed; previously-known IDs keep whatever the user set.
  _syncFoldedFeatures(tasks) {
    const next = new Set(this.foldedFeatures);
    const seen = new Set();
    for (const t of tasks) {
      if (t.Type !== 'feature') continue;
      seen.add(t.ID);
      if (!this._knownFeatureIDs.has(t.ID)) {
        next.add(t.ID);
        this._knownFeatureIDs.add(t.ID);
      }
    }
    // Forget features that no longer exist.
    for (const id of [...next]) if (!seen.has(id)) next.delete(id);
    for (const id of [...this._knownFeatureIDs]) if (!seen.has(id)) this._knownFeatureIDs.delete(id);
    this.foldedFeatures = next;
  }

  _toggleFold(featureID) {
    const next = new Set(this.foldedFeatures);
    if (next.has(featureID)) next.delete(featureID); else next.add(featureID);
    this.foldedFeatures = next;
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

  // Public: called from the board page's "↻ Now" button. Same target
  // as the initial centring pass but with rAF easing (ease-out-quart,
  // ~220ms), since the user pressed something so the motion is
  // expected. Honours prefers-reduced-motion by snapping instead.
  scrollToNow() {
    const stage = this.shadowRoot?.querySelector('.stage');
    const nowLine = this.shadowRoot?.querySelector('.now-line');
    if (!stage || !nowLine) return;
    const nowX = parseFloat(nowLine.getAttribute('x1') || '0');
    if (!stage.clientWidth) return;
    const target = Math.max(0, nowX - stage.clientWidth / 2);
    const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
    if (reduce) { stage.scrollLeft = target; return; }
    const start = stage.scrollLeft;
    const delta = target - start;
    if (Math.abs(delta) < 1) return;
    const duration = 220;
    const t0 = performance.now();
    const ease = (t) => 1 - Math.pow(1 - t, 4); // ease-out-quart
    const step = (now) => {
      const t = Math.min(1, (now - t0) / duration);
      stage.scrollLeft = start + delta * ease(t);
      if (t < 1) requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  }

  _centerOnNow() {
    // Defer two frames: first to let Lit flush its SVG render, second
    // to let the browser actually lay out the (now wider) sub-columns
    // and compute scrollWidth / clientWidth. Reading those numbers in
    // the same tick as render() returns stale zeroes — the sub-column
    // priority columns added enough X to push the now-line off-screen
    // before the layout pass had finished.
    requestAnimationFrame(() => requestAnimationFrame(() => {
      const stage = this.shadowRoot?.querySelector('.stage');
      const nowLine = this.shadowRoot?.querySelector('.now-line');
      if (!stage || !nowLine) return;
      const nowX = parseFloat(nowLine.getAttribute('x1') || '0');
      if (!stage.clientWidth) return; // not visible yet — try again on next update
      const target = Math.max(0, nowX - stage.clientWidth / 2);
      stage.scrollLeft = target;
      this._initialCenterDone = true;
    }));
  }

  _subscribe() {
    this._unsub?.();
    if (!this.projectId) return;
    this._unsub = subscribe(this.projectId, (ev) => {
      // Tasks and dependencies are what the gantt draws; reload on
      // any related event. 'realtime.reconnected' catches events that
      // happened while EventSource was reconnecting.
      if (ev.type === 'realtime.reconnected' || ev.type?.startsWith('task.')) this.load();
    });
  }

  _roleLabel(id) {
    const r = (this.roles || []).find(x => x.ID === id);
    return r ? r.Label : '';
  }

  _featureRoles(featureID) {
    // List the distinct roles of the feature's non-feature descendants,
    // separated by commas, in the project's role order.
    const taskByID = new Map((this.tasks || []).map(t => [t.ID, t]));
    const childrenByParent = new Map();
    for (const t of this.tasks || []) {
      if (!t.ParentTaskID) continue;
      if (!childrenByParent.has(t.ParentTaskID)) childrenByParent.set(t.ParentTaskID, []);
      childrenByParent.get(t.ParentTaskID).push(t.ID);
    }
    const seen = new Set();
    const walk = (id) => {
      for (const c of childrenByParent.get(id) || []) {
        if (seen.has(c)) continue;
        seen.add(c);
        walk(c);
      }
    };
    walk(featureID);
    const roleIDs = new Set();
    for (const id of seen) {
      const t = taskByID.get(id);
      if (t && t.Type !== 'feature' && t.TargetRoleID) roleIDs.add(t.TargetRoleID);
    }
    const sortedRoles = [...this.roles || []]
      .filter(r => roleIDs.has(r.ID))
      .sort((a, b) => (a.Position ?? 0) - (b.Position ?? 0))
      .map(r => r.Label.toLowerCase());
    return sortedRoles.length ? sortedRoles.join(', ') : 'no roles';
  }

  _priorityLabel(value) {
    if (this.priorities && this.priorities.length) {
      const exact = this.priorities.find(p => p.Value === value);
      if (exact) return exact.Key;
    }
    return `p${value}`;
  }

  async load() {
    if (!this.projectId) return;
    try {
      const [tr, rr, dr, mr, qr] = await Promise.all([
        fetch(`/api/projects/${this.projectId}/tasks?include_children=true`),
        fetch(`/api/projects/${this.projectId}/roles`),
        fetch(`/api/projects/${this.projectId}/tasks/dependencies`),
        fetch(`/api/projects/${this.projectId}/members`),
        fetch(`/api/projects/${this.projectId}/priorities`),
      ]);
      this.tasks = (await tr.json()).tasks || [];
      this._syncFoldedFeatures(this.tasks);
      this.roles = (await rr.json()).roles || [];
      this.deps = (await dr.json()).dependencies || [];
      this.members = (await mr.json()).members || [];
      this.priorities = (await qr.json()).priorities || [];
    } catch (e) {
      this.error = e.message;
    }
  }

  // Group tasks by role; falls back to a "general" pseudo-role for
  // tasks without target_role_id.
  bands() {
    const order = [...this.roles].sort((a, b) => (a.Position ?? 0) - (b.Position ?? 0));
    const general = { ID: '__general__', Key: 'general', Label: 'General', Color: '#59636e' };
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

    // ---- Past zone: right-packed by successor depth ----
    // Naive chronological slots leave empty space to the right of any
    // band whose latest done task is not the overall most-recent task
    // in the project. Instead we right-pack: each done task gets a
    // slot equal to K-1-succ(t), where succ(t) counts the longest
    // chain of "successors" descending from t. A successor is:
    //   (a) the next done task in the SAME band by actual_end, or
    //   (b) any done task that DEPENDS on t.
    // This guarantees:
    //   • Within each band, chronological order is preserved.
    //   • For any done→done dep edge, the dependent's slot is strictly
    //     greater than the precondition's — no backward arrows.
    //   • Bands with no later tasks and no done dependents sit at the
    //     rightmost slot, flush against NOW.
    const doneByID = new Map();
    for (const t of this.tasks || []) {
      if (t.State === 'done' && t.ActualEnd) doneByID.set(t.ID, t);
    }
    // A past task is "anchored" when it touches at least one dependency
    // edge that will be drawn as an arrow on the chart. Anchored tasks
    // keep their own column. "Free" past tasks (no arrows in or out)
    // tuck vertically into the previous column's empty lanes, which
    // shrinks the band's horizontal footprint without altering any
    // arrow geometry.
    const depTouched = new Set();
    for (const d of this.deps) {
      if (doneByID.has(d.TaskID))      depTouched.add(d.TaskID);
      if (doneByID.has(d.DependsOnID)) depTouched.add(d.DependsOnID);
    }
    // Future topological depths drive both the future X axis and the
    // lane count each band needs for non-done tasks (which the past
    // packing then reuses).
    const globalDepths = this.computeTopoDepths(this.tasks || []);
    // Compute the lane count each band needs from its non-done tasks.
    // Future zone: tasks sharing a (depth, priority) cell pile vertically
    // into lanes. Present zone: every `doing` task collides at the same X.
    // Past packing then reuses THIS lane count — it doesn't define a new
    // one. Bands with no future/present pressure cap at 1 lane (which
    // means past tasks keep their natural one-per-column layout).
    const futureLanesPerBand = bands.map(b => {
      const cellCounts = new Map();
      let doingCount = 0;
      for (const t of b.tasks) {
        if (t.State === 'doing') {
          doingCount++;
        } else if (t.State === 'todo') {
          const d = globalDepths.get(t.ID) || 0;
          const key = `${d}:${t.Priority}`;
          cellCounts.set(key, (cellCounts.get(key) || 0) + 1);
        }
      }
      let maxCellLanes = 0;
      for (const c of cellCounts.values()) maxCellLanes = Math.max(maxCellLanes, c);
      return Math.max(1, Math.max(maxCellLanes, doingCount));
    });
    // Anchored slots come from a global successor-depth pass: every
    // band-chronological successor BETWEEN ANCHORED TASKS and every
    // done→done dep edge pushes its source one slot further left. Free
    // tasks are deliberately excluded from the band-chain — they don't
    // have arrows, so they shouldn't force horizontal spread on their
    // own behalf. Their succession ends up at 0 and they cluster at
    // the rightmost slot, then gravitate around the anchored skeleton
    // in the relocation phase below.
    const bandSuccessor = new Map();
    for (const b of bands) {
      const anchoredChronoSorted = b.tasks
        .filter(t => t.State === 'done' && t.ActualEnd && depTouched.has(t.ID))
        .sort((x, y) => new Date(x.ActualEnd).getTime() - new Date(y.ActualEnd).getTime());
      for (let i = 0; i < anchoredChronoSorted.length - 1; i++) {
        bandSuccessor.set(anchoredChronoSorted[i].ID, anchoredChronoSorted[i + 1].ID);
      }
    }
    const depSuccessors = new Map();
    for (const d of this.deps) {
      if (doneByID.has(d.TaskID) && doneByID.has(d.DependsOnID)) {
        if (!depSuccessors.has(d.DependsOnID)) depSuccessors.set(d.DependsOnID, []);
        depSuccessors.get(d.DependsOnID).push(d.TaskID);
      }
    }
    const succession = new Map();
    const visiting = new Set();
    const computeSucc = (id) => {
      if (succession.has(id)) return succession.get(id);
      if (visiting.has(id)) return 0;
      visiting.add(id);
      let s = 0;
      const next = bandSuccessor.get(id);
      if (next) s = Math.max(s, 1 + computeSucc(next));
      for (const dep of depSuccessors.get(id) || []) {
        s = Math.max(s, 1 + computeSucc(dep));
      }
      visiting.delete(id);
      succession.set(id, s);
      return s;
    };
    for (const id of doneByID.keys()) computeSucc(id);
    const maxPastSlots = doneByID.size
      ? Math.max(0, ...succession.values()) + 1
      : 0;
    // Initial slot = K-1-succession (right-pack newest tasks flush
    // against NOW). Anchored tasks stay here; free tasks may move.
    const globalPastSlot = new Map();
    for (const [id, s] of succession) {
      globalPastSlot.set(id, maxPastSlots - 1 - s);
    }
    // Place FREE past tasks. Each free task gravitates toward NOW: we
    // walk them newest-first and drop each into the RIGHTMOST slot
    // (among any globally-used slot) where the task's band still has
    // lane room. Free tasks have no arrows, so they can occupy any
    // column without breaking arrow geometry. Cap per band = the
    // band's future-zone lane count, so past height never exceeds
    // what the future zone already established.
    const usedGlobalSlots = new Set(globalPastSlot.values());
    for (let bi = 0; bi < bands.length; bi++) {
      const b = bands[bi];
      const maxLanesPast = futureLanesPerBand[bi];
      // Build the band's per-slot occupancy from ANCHORED tasks only
      // (their slot is fixed by succession). Free tasks haven't been
      // placed yet — we re-place them here.
      const slotOccupants = new Map();
      for (const t of b.tasks) {
        if (t.State !== 'done' || !t.ActualEnd) continue;
        if (!depTouched.has(t.ID)) continue;
        const s = globalPastSlot.get(t.ID);
        slotOccupants.set(s, (slotOccupants.get(s) || 0) + 1);
      }
      // Newest free first: closer-to-NOW priority gets the rightmost
      // available slot.
      const freeSorted = b.tasks
        .filter(t => t.State === 'done' && t.ActualEnd && !depTouched.has(t.ID))
        .sort((x, y) => new Date(y.ActualEnd).getTime() - new Date(x.ActualEnd).getTime());
      for (const t of freeSorted) {
        let bestSlot = null;
        for (const s of usedGlobalSlots) {
          const count = slotOccupants.get(s) || 0;
          if (count >= maxLanesPast) continue;
          if (bestSlot == null || s > bestSlot) bestSlot = s;
        }
        if (bestSlot == null) {
          // No room anywhere — fall back to the task's initial slot.
          continue;
        }
        globalPastSlot.set(t.ID, bestSlot);
        slotOccupants.set(bestSlot, (slotOccupants.get(bestSlot) || 0) + 1);
        usedGlobalSlots.add(bestSlot);
      }
    }
    // Compact: drop slot indices that ended up empty after relocation
    // so the past zone width matches what's actually drawn. Preserves
    // the slot ORDER so cross-band dep arrows stay forward (succession
    // already encoded the right ordering, we're just renumbering).
    {
      const usedSlots = [...new Set(globalPastSlot.values())].sort((a, b) => a - b);
      const remap = new Map();
      usedSlots.forEach((s, i) => remap.set(s, i));
      for (const [id, s] of globalPastSlot) globalPastSlot.set(id, remap.get(s));
    }
    const compactedPastSlots = (() => {
      let m = -1;
      for (const s of globalPastSlot.values()) if (s > m) m = s;
      return m + 1;
    })();
    const pastSlotPerBand = bands.map(b => {
      const m = new Map();
      for (const t of b.tasks) {
        if (globalPastSlot.has(t.ID)) m.set(t.ID, globalPastSlot.get(t.ID));
      }
      return m;
    });
    const pastWidth = Math.max(360, compactedPastSlots * pastSlotW + 12);

    // The present zone gets enough room for one min-width bar plus
    // padding. Multiple concurrent `doing` tasks stack into lanes
    // (handled by the lane assignment below).
    const presentWidth = minBarWidth + 24;

    const presentX = labelWidth + pastWidth;
    const futureStartX = presentX + presentWidth;

    // Future zone topological columns: depth is computed GLOBALLY,
    // not per band, so a task that depends on a task in a different
    // band still ends up further to the right. (globalDepths is
    // computed earlier so the past-zone packing can also see it.)
    const futureDepthsPerBand = bands.map(() => globalDepths);

    // ---- Future sub-columns by priority within each depth ----
    // For each topological depth, collect the distinct priorities of
    // the todo tasks at that depth, sorted DESC. Each priority gets a
    // sub-column inside the depth's slot in the future zone; higher
    // priority means leftmost sub-column. The "next" eligible task is
    // therefore in the leftmost sub-column at depth 0.
    const futureSubColumnX = new Map(); // taskID -> x offset
    const futurePriorityBuckets = [];   // [{ depth, priority, x }] for labels
    {
      // tasksByDepth: depth -> [task]
      //
      // Feature parents WITH descendants render either as an
      // aggregate (folded) or as their hidden self with kids showing
      // through (unfolded). Their own priority is irrelevant to the
      // future-zone column grid, so we skip them. Otherwise (a
      // childless feature, common when an idea is parked as a top-
      // level `type=feature` row before role children are filed) the
      // feature renders as a standalone card and DOES need a slot —
      // include it so it lands in its own priority sub-column instead
      // of falling through to the leftmost (which reads as "critical"
      // and is the bug the user spotted).
      const hasDescendants = new Set();
      for (const t of this.tasks || []) {
        if (t.ParentTaskID) hasDescendants.add(t.ParentTaskID);
      }
      const tasksByDepth = new Map();
      for (const t of this.tasks || []) {
        if (t.State !== 'todo') continue;
        if (t.Type === 'feature' && hasDescendants.has(t.ID)) continue;
        const d = globalDepths.get(t.ID) || 0;
        if (!tasksByDepth.has(d)) tasksByDepth.set(d, []);
        tasksByDepth.get(d).push(t);
      }
      // For each depth, build a sorted list of distinct priorities and
      // record the X for each priority bucket.
      const depthsSorted = Array.from(tasksByDepth.keys()).sort((a, b) => a - b);
      let cursor = futureStartX;
      for (const d of depthsSorted) {
        const ts = tasksByDepth.get(d);
        const distinctPriorities = Array.from(new Set(ts.map(t => t.Priority)))
          .sort((a, b) => b - a); // DESC
        for (let i = 0; i < distinctPriorities.length; i++) {
          const p = distinctPriorities[i];
          const x = cursor + i * futureColumnWidth + 12;
          futurePriorityBuckets.push({ depth: d, priority: p, x });
          for (const t of ts) {
            if (t.Priority === p) futureSubColumnX.set(t.ID, x);
          }
        }
        cursor += Math.max(1, distinctPriorities.length) * futureColumnWidth;
      }
      // Stash the rightmost X so we can size the SVG accordingly.
      this._futureEndX = cursor;
    }

    // ---- Lane assignment per band ----
    // For each band, position every task on the X axis, then greedily
    // place it into the lowest-index lane whose previous occupant has
    // already ended (with a small horizontal gap). The number of lanes
    // determines the band's vertical height.
    const overlapGap = 6;

    // ---- Feature folding ----
    // Build parent→children index, then derive:
    //   • descendants of every feature (recursive)
    //   • the set of tasks hidden because a feature ancestor is folded
    //   • a per-feature aggregate position when folded (envelope of its
    //     non-feature descendants).
    const taskByID = new Map((this.tasks || []).map(t => [t.ID, t]));
    const childrenByParent = new Map();
    for (const t of this.tasks || []) {
      if (!t.ParentTaskID) continue;
      if (!childrenByParent.has(t.ParentTaskID)) childrenByParent.set(t.ParentTaskID, []);
      childrenByParent.get(t.ParentTaskID).push(t.ID);
    }
    const collectDescendants = (id, out) => {
      for (const c of childrenByParent.get(id) || []) {
        if (out.has(c)) continue;
        out.add(c);
        collectDescendants(c, out);
      }
      return out;
    };
    const hiddenByFold = new Set();
    for (const fid of this.foldedFeatures) {
      const f = taskByID.get(fid);
      if (!f || f.Type !== 'feature') continue;
      collectDescendants(fid, hiddenByFold);
    }

    // ---- Raw positions for every task (independent of fold state) ----
    // Computed in the task's "natural" band so we can build aggregates
    // even when descendants live in a band other than the feature's.
    const bandIndexByTaskID = new Map();
    bands.forEach((b, bi) => {
      for (const t of b.tasks) bandIndexByTaskID.set(t.ID, bi);
    });
    const rawPositions = new Map(); // taskID -> {from, to, bi}
    bands.forEach((b, bi) => {
      const depths = futureDepthsPerBand[bi];
      const slots = pastSlotPerBand[bi];
      for (const t of b.tasks) {
        const pastSlot = (t.State === 'done') ? (slots.get(t.ID) ?? null) : null;
        const futureX = (t.State === 'todo') ? futureSubColumnX.get(t.ID) : undefined;
        const x = this.taskX({
          t, labelWidth, pastSlotW, pastSlot,
          presentX, presentWidth, futureStartX, futureColumnWidth,
          futureX, depths, minBarWidth,
        });
        if (!x) continue;
        rawPositions.set(t.ID, { from: x.from, to: x.to, bi });
      }
    });

    // ---- Aggregate positions for folded features ----
    // First pass collects from/to plus the distinct set of role bands
    // each feature's non-feature descendants live in. If a feature has
    // descendants in 2+ role bands it's "cross-role" — we'll hoist it
    // into a dedicated Features lane (decided below). Single-role
    // features stay inside their natural role band.
    const featureAggregates = new Map(); // featureID -> {from, to, bi, crossRole, roleColors}
    let anyCrossRole = false;
    for (const fid of this.foldedFeatures) {
      const feat = taskByID.get(fid);
      if (!feat || feat.Type !== 'feature') continue;
      const desc = collectDescendants(fid, new Set());
      if (!desc.size) continue;
      let lo = Infinity, hi = -Infinity;
      const bandsSeen = new Set();
      const bandVotes = new Map();
      for (const did of desc) {
        const d = taskByID.get(did);
        if (!d || d.Type === 'feature') continue;
        const p = rawPositions.get(did);
        if (!p) continue;
        lo = Math.min(lo, p.from);
        hi = Math.max(hi, p.to);
        bandsSeen.add(p.bi);
        bandVotes.set(p.bi, (bandVotes.get(p.bi) || 0) + 1);
      }
      if (lo === Infinity) continue;
      const crossRole = bandsSeen.size > 1;
      if (crossRole) anyCrossRole = true;
      // Natural band (used when not cross-role): feature's own
      // target_role if set, else the band with the most descendants.
      let naturalBi = bandIndexByTaskID.get(fid);
      if (naturalBi == null) {
        let best = -1, max = -1;
        for (const [k, v] of bandVotes) if (v > max) { max = v; best = k; }
        naturalBi = best >= 0 ? best : 0;
      }
      // Capture the role colours of the involved bands, ordered by the
      // band's display position so the dots read top→bottom by role.
      const roleColors = [...bandsSeen]
        .sort((a, b) => a - b)
        .map(bi => bands[bi].role.Color || '#8c959f');
      featureAggregates.set(fid, { from: lo, to: hi, bi: naturalBi, crossRole, roleColors });
    }

    // ---- Optional Features lane (only when there's something to put in it) ----
    // When a folded feature spans 2+ role bands, we hoist its aggregate
    // into a synthetic lane at the top of the chart. The display order
    // of bands becomes [Features?, ...roleBands]; everything keyed by
    // band index is shifted by the offset.
    const featuresBand = anyCrossRole
      ? { role: { ID: '__features__', Key: 'features', Label: 'Features', Color: '#6e7781' }, tasks: [] }
      : null;
    const displayBands = featuresBand ? [featuresBand, ...bands] : bands;
    const bandOffset = featuresBand ? 1 : 0;

    // ---- Visible entries per (display) band ----
    const visiblePerBand = displayBands.map(() => []);
    for (const t of this.tasks || []) {
      if (hiddenByFold.has(t.ID)) continue;
      if (t.Type === 'feature') {
        if (this.foldedFeatures.has(t.ID) && featureAggregates.has(t.ID)) {
          const agg = featureAggregates.get(t.ID);
          const targetBi = agg.crossRole ? 0 : agg.bi + bandOffset;
          visiblePerBand[targetBi].push({ task: t, from: agg.from, to: agg.to, kind: 'feature-agg', crossRole: agg.crossRole, roleColors: agg.roleColors });
        } else if (!childrenByParent.has(t.ID)) {
          const p = rawPositions.get(t.ID);
          if (p) visiblePerBand[p.bi + bandOffset].push({ task: t, from: p.from, to: p.to, kind: 'normal' });
        }
        // unfolded feature with children: feature itself hidden, kids show through
        continue;
      }
      const p = rawPositions.get(t.ID);
      if (p) visiblePerBand[p.bi + bandOffset].push({ task: t, from: p.from, to: p.to, kind: 'normal' });
    }

    // ---- Lane assignment per band (greedy) ----
    const positions = []; // { task, bi, lane, from, to, kind }
    const lanesPerBand = displayBands.map(() => 1);
    visiblePerBand.forEach((entries, bi) => {
      entries.sort((a, b) => a.from - b.from);
      const laneEnds = [];
      for (const e of entries) {
        let placed = -1;
        for (let li = 0; li < laneEnds.length; li++) {
          if (laneEnds[li] + overlapGap <= e.from) { placed = li; break; }
        }
        if (placed < 0) { placed = laneEnds.length; laneEnds.push(e.to); }
        else { laneEnds[placed] = e.to; }
        positions.push({ task: e.task, bi, lane: placed, from: e.from, to: e.to, kind: e.kind, roleColors: e.roleColors });
      }
      lanesPerBand[bi] = Math.max(1, laneEnds.length);
    });

    // ---- Compute each band's Y top from the lane counts ----
    // Empty bands collapse to zero height so the synthetic "General"
    // band (and any other band without visible entries today) doesn't
    // leave a dead row in the chart.
    const bandTops = [];
    const bandHeights = [];
    {
      let cursor = headerH;
      for (let bi = 0; bi < displayBands.length; bi++) {
        bandTops.push(cursor);
        const isEmpty = visiblePerBand[bi].length === 0;
        const h = isEmpty ? 0 : (lanesPerBand[bi] * laneHeight + bandPad * 2 - laneGap);
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

    // Calculate total svg width — the future zone now sizes itself
    // from the sub-column buckets (depth × priority), so we use the
    // running cursor stashed in _futureEndX.
    const width = Math.max(futureStartX + futureColumnWidth, this._futureEndX || futureStartX) + 16;

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

    // Pre-compute band fill per band index so todo bars can read
    // solid against the lane background instead of letting arrows
    // show through a transparent fill.
    let visIdx = 0;
    const bandFill = displayBands.map(b => {
      if (b.role.ID === '__features__') return '#f0f2f5';
      const c = visIdx % 2 ? '#fff' : '#f6f8fa';
      visIdx++;
      return c;
    });

    // Selection-aware predicates: when nothing is selected every bar
    // and arrow renders at full strength. Otherwise the connected
    // set is bright and its arrows promote on top of the task rects.
    const hasSelection = this._selectedSet.size > 0;
    const isPromotedEdge = (d) =>
      hasSelection && (this._selectedSet.has(d.TaskID) || this._selectedSet.has(d.DependsOnID));

    return html`
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      <div class="stage" @keydown=${(e) => this._onStageKey(e)}
           @click=${() => this._clearSelection()}>
        ${this._renderHoverCard()}
        <svg width=${width} height=${totalHeight}
             role="img"
             aria-label=${`Gantt chart with ${(this.tasks||[]).length} tasks`}>
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
          <text class="zone-label" x=${futureStartX + 4} y="14">FUTURE · topological + priority</text>

          <!-- Priority labels at the top of each future sub-column -->
          ${futurePriorityBuckets.map(b => svg`
            <text class="priority-label" x=${b.x + 4} y=${headerH - 4}>${this._priorityLabel(b.priority)}</text>
          `)}

          <!-- Band rows (empty bands collapse to height 0 and skip render).
               The alternation counter is independent of the displayBands
               index so that hidden bands (height 0) don't break the
               stripe pattern of the bands that DO render. Features has
               its own class and is excluded from the count. -->
          ${(() => { this._visibleBandIdx = 0; return null; })()}
          ${displayBands.map((b, bi) => {
            if (bandHeights[bi] === 0) return null;
            const isFeatures = b.role.ID === '__features__';
            let cls;
            if (isFeatures) {
              cls = 'band-bg features';
            } else {
              cls = `band-bg ${this._visibleBandIdx % 2 ? 'alt' : ''}`;
              this._visibleBandIdx++;
            }
            return svg`
              <rect class=${cls}
                    x="0" y=${bandTops[bi]}
                    width=${width} height=${bandHeights[bi]}></rect>
              <text class="band-label"
                    x="8" y=${bandTops[bi] + bandHeights[bi] / 2 + 4}>
                ${isFeatures ? 'Features' : b.role.Label}
              </text>
            `;
          })}

          <!-- Zone dividers -->
          <line class="zone-divider" x1=${labelWidth} y1="0" x2=${labelWidth} y2=${totalHeight}></line>
          <line class="zone-divider" x1=${presentX} y1="0" x2=${presentX} y2=${totalHeight}></line>
          <line class="zone-divider" x1=${futureStartX} y1="0" x2=${futureStartX} y2=${totalHeight}></line>

          <!-- "now" marker -->
          <line class="now-line"
                x1=${presentX + presentWidth / 2} y1=${headerH - 8}
                x2=${presentX + presentWidth / 2} y2=${totalHeight}>
            <title>${this.now ? this.now.toLocaleString() : ''}</title>
          </line>
          <text class="now-label"
                x=${presentX + presentWidth / 2}
                y=${headerH - 12}
                text-anchor="middle">
            <title>${this.now ? this.now.toLocaleString() : ''}</title>
            now
          </text>

          <!-- Dependency arrows, FIRST PASS: rendered before the task
               rects so the bars paint on top of them. Selection-incident
               edges are deliberately SKIPPED here and re-rendered after
               the bars (promoted pass) so they read above any rect they
               cross. -->
          ${this.deps.map(d => {
            if (isPromotedEdge(d)) return null;
            return this._renderArrowPath(d, posByTaskID, taskCenterY, taskHeight, hasSelection);
          })}

          <!-- Tasks placed in their lane -->
          ${positions.map(p => {
            const t = p.task;
            const color = displayBands[p.bi].role.Color || '#59636e';
            const y = taskY(p.bi, p.lane);
            const w = Math.max(8, p.to - p.from);
            const user = t.AssigneeUserID ? usersById.get(t.AssigneeUserID) : null;
            const avatarX = p.from + avatarPad;
            const avatarY = y + (taskHeight - avatarSize) / 2;
            const labelX = user ? avatarX + avatarSize + 6 : p.from + 8;
            const labelMaxChars = Math.max(6, Math.floor((p.to - labelX - 6) / 7));
            if (p.kind === 'feature-agg') {
              const childCount = (childrenByParent.get(t.ID) || []).length;
              const dots = p.roleColors || [];
              const dotR = 4;
              const dotGap = 4;
              const dotPadRight = 10;
              // Reserve room for the dots so the label doesn't run under them.
              const dotsBlockW = dots.length * (dotR * 2) + Math.max(0, dots.length - 1) * dotGap;
              const dotsStartX = p.to - dotPadRight - dotsBlockW + dotR;
              const labelRoom = Math.max(6, Math.floor((dotsStartX - dotR - labelX - 6) / 7));
              const aggDimmed = hasSelection && !this._selectedSet.has(t.ID);
              return svg`
                <rect class=${`task-rect feature-agg${aggDimmed ? ' dim' : ''}`}
                      x=${p.from} y=${y}
                      width=${w} height=${taskHeight}
                      rx="6" ry="6"
                      fill="#eef0f3"
                      stroke=${color}
                      stroke-dasharray="6 3"
                      style="cursor:pointer"
                      @click=${(e) => { e.stopPropagation(); this._toggleFold(t.ID); }}
                      @mouseenter=${(e) => this._onBarHover(e, t, p.from, y)}
                      @mouseleave=${() => this._onBarLeave()}
                      @focus=${(e) => this._onBarHover(e, t, p.from, y)}
                      @blur=${() => this._onBarLeave()}
                      tabindex="0">
                </rect>
                <text class="task-label" x=${labelX} y=${y + taskHeight / 2 + 4}
                      style="pointer-events:none">
                  ▸ ${this._truncate(t.Title, labelRoom)}
                </text>
                ${dots.map((c, i) => svg`
                  <circle cx=${dotsStartX + i * (dotR * 2 + dotGap)}
                          cy=${y + taskHeight / 2}
                          r=${dotR}
                          fill=${c}
                          stroke="rgba(0,0,0,0.18)" stroke-width="0.5"
                          style="pointer-events:none"></circle>
                `)}
              `;
            }
            const isFeatureUnfolded = t.Type === 'feature' && !this.foldedFeatures.has(t.ID);
            // Childless features and (defensive) any feature reaching here render as normal.
            const taskDimmed = hasSelection && !this._selectedSet.has(t.ID);
            const todoFill = bandFill[p.bi] || '#fff';
            return svg`
              <rect class=${`task-rect ${t.State}${t.Type === 'bug' ? ' bug' : ''}${inconsistentIDs.has(t.ID) ? ' inconsistent' : ''}${taskDimmed ? ' dim' : ''}`}
                    x=${p.from} y=${y}
                    width=${w} height=${taskHeight}
                    rx="6" ry="6"
                    fill=${t.State === 'done' ? '#d1d9e0' : (t.State === 'doing' ? color : todoFill)}
                    stroke=${color}
                    @click=${(e) => { e.stopPropagation(); this._onTaskClick(t); }}
                    @dblclick=${(e) => { e.stopPropagation(); this._emitSelect(t); }}
                    @mouseenter=${(e) => this._onBarHover(e, t, p.from, y)}
                    @mouseleave=${() => this._onBarLeave()}
                    @focus=${(e) => this._onBarHover(e, t, p.from, y)}
                    @blur=${() => this._onBarLeave()}
                    tabindex="0">
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

          <!-- Dependency arrows, SECOND PASS: only the arrows incident
               to the currently-selected subgraph, rendered after the
               task rects so they sit on top of every box they cross. -->
          ${this.deps.map(d => {
            if (!isPromotedEdge(d)) return null;
            return this._renderArrowPath(d, posByTaskID, taskCenterY, taskHeight, hasSelection);
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
  taskX({ t, labelWidth, pastSlotW, pastSlot, presentX, presentWidth, futureStartX, futureColumnWidth, futureX, depths, minBarWidth }) {
    if (t.State === 'done') {
      if (pastSlot == null) return null; // no actual_end ⇒ not on past axis
      const from = labelWidth + pastSlot * pastSlotW + 6;
      return { from, to: from + minBarWidth };
    }
    if (t.State === 'doing') {
      const from = presentX + 12;
      return { from, to: from + minBarWidth };
    }
    // todo — futureX (sub-column X) computed by the priority grouping
    // pass. Fall back to depth-only if we somehow didn't precompute.
    let from = futureX;
    if (from == null) {
      const d = depths.get(t.ID) || 0;
      from = futureStartX + d * futureColumnWidth + 12;
    }
    return { from, to: from + minBarWidth };
  }

  // Render one dependency arrow as an SVG <path>. Used in two passes
  // by the main render: the default (non-promoted) pass before the
  // task rects, and the promoted pass after, so selection-incident
  // arrows always land on top of every box they cross.
  _renderArrowPath(d, posByTaskID, taskCenterY, taskHeight, hasSelection) {
    const from = posByTaskID.get(d.DependsOnID);
    const to = posByTaskID.get(d.TaskID);
    if (!from || !to) return null;
    const markerW = 8;
    const promoted = hasSelection &&
      (this._selectedSet.has(d.TaskID) || this._selectedSet.has(d.DependsOnID));
    const dimmed = hasSelection && !promoted;
    const cls = `arrow${promoted ? ' promoted' : ''}${dimmed ? ' dim' : ''}`;
    const onClick = (e) => { e.stopPropagation(); this._onArrowClick(d.DependsOnID); };
    const sourceRight = from.to;
    const targetLeft = to.from;
    const x1 = sourceRight;
    const y1 = taskCenterY(from.bi, from.lane);
    const y2 = taskCenterY(to.bi, to.lane);
    if (targetLeft >= sourceRight) {
      const x2 = Math.max(targetLeft - markerW, x1 + 2);
      const cx = (x1 + x2) / 2;
      return svg`<path class=${cls}
                       d=${`M ${x1} ${y1} C ${cx} ${y1} ${cx} ${y2} ${x2} ${y2}`}
                       @click=${onClick}></path>`;
    }
    const x2 = targetLeft - markerW;
    const bow = 60;
    const arch = Math.max(40, taskHeight + 24);
    const yMid = Math.max(y1, y2) + arch;
    const xMid = (x1 + x2) / 2;
    return svg`<path class=${cls}
                     d=${
      `M ${x1} ${y1} ` +
      `C ${x1 + bow} ${y1} ${x1 + bow} ${yMid} ${xMid} ${yMid} ` +
      `S ${x2 - bow} ${y2} ${x2} ${y2}`
    }
                     @click=${onClick}></path>`;
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

  // _onBarHover stashes the task + its on-canvas anchor coords so
  // the popup template can render. The anchor is the bar's
  // top-left in SVG coords; we convert to DOM-relative-to-stage
  // pixels at render time (the stage scrolls horizontally, so we
  // also account for current scrollLeft / scrollTop).
  _onBarHover(_e, task, barX, barY) {
    this._hover = { task, barX, barY };
  }
  _onBarLeave() {
    this._hover = null;
  }
  _onStageKey(e) {
    if (e.key !== 'Escape') return;
    if (this._hover) {
      this._hover = null;
      this.shadowRoot?.querySelector('.task-rect:focus')?.blur?.();
    }
    if (this._selectedAnchor || this._selectedSet.size > 0) {
      this._clearSelection();
    }
  }

  // Render the popup for the currently-hovered task. Position is
  // computed against the bar's SVG coords; the stage's scrollLeft
  // and a small offset keep the card visually anchored.
  _renderHoverCard() {
    if (!this._hover) return null;
    const { task: t, barX, barY } = this._hover;
    const stage = this.shadowRoot?.querySelector('.stage');
    const stageW = stage?.clientWidth || 800;
    const scrollLeft = stage?.scrollLeft || 0;
    // The card is `position: absolute` inside `.stage` (which is
    // `position: relative` + `overflow: auto`), so its coordinates
    // are in the scrolled SVG content space, not the viewport. We
    // do NOT subtract scrollLeft. Default anchor: 8px to the right
    // of the bar's start, just above it.
    const cardW = 320;
    let left = barX + 8;
    const top = Math.max(8, barY - 8);
    // Flip to the left side of the bar if the card would extend
    // past the visible viewport's right edge.
    const viewportRight = scrollLeft + stageW;
    if (left + cardW > viewportRight) {
      left = Math.max(scrollLeft + 8, barX - cardW - 8);
    }

    const role = t.TargetRoleID ? this.roles.find(r => r.ID === t.TargetRoleID) : null;
    const user = t.AssigneeUserID
      ? this.members.find(m => m.UserID === t.AssigneeUserID)
      : null;

    // Collapsed feature → short summary card.
    if (t.Type === 'feature' && this.foldedFeatures.has(t.ID)) {
      const childCount = (this.tasks || []).filter(x => x.ParentTaskID === t.ID).length;
      const rolesText = this._featureRoles(t.ID);
      return html`
        <div class="hover-card" role="tooltip"
             style=${`left:${left}px; top:${top}px`}>
          <div class="title">${t.Title}</div>
          <div class="meta">${childCount} task${childCount === 1 ? '' : 's'} · ${rolesText}</div>
          <div class="meta">Click to expand</div>
        </div>
      `;
    }

    // Regular task card.
    const dateStr = (iso) => iso ? new Date(iso).toLocaleDateString() : '';
    const start = t.ActualStart || t.PlannedStart;
    const end   = t.ActualEnd   || t.PlannedEnd;
    return html`
      <div class="hover-card" role="tooltip"
           style=${`left:${left}px; top:${top}px`}>
        <div class="title">${t.Title}</div>
        <div class="row">
          <span class=${`chip state-${t.State}`}>${t.State}</span>
          ${t.Type === 'bug'
            ? html`<span class="chip type-bug">bug</span>`
            : t.Type !== 'task'
              ? html`<span class="chip">${t.Type}</span>`
              : null}
          <span class="chip">${this._priorityLabel(t.Priority)}</span>
          ${role ? html`
            <span class="chip role" style=${`background:${role.Color || '#59636e'}`}>${role.Label}</span>
          ` : null}
        </div>
        ${start || end ? html`
          <div class="meta">${dateStr(start)}${start && end ? ' → ' : ''}${dateStr(end)}</div>
        ` : null}
        ${user ? html`
          <div class="assignee">
            ${user.AvatarURL ? html`<img src=${user.AvatarURL} alt="">` : null}
            <span>${user.DisplayName || user.GithubLogin}</span>
          </div>
        ` : null}
      </div>
    `;
  }
}

customElements.define('nottario-gantt', NottarioGantt);
