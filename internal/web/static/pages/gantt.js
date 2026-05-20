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
    this.foldedFeatures = new Set();      // feature IDs currently collapsed
    this._knownFeatureIDs = new Set();    // features we've ever seen
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
      // any related event.
      if (ev.type?.startsWith('task.')) this.load();
    });
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
    const bandSuccessor = new Map(); // taskID -> next-in-band taskID
    for (const b of bands) {
      const sorted = b.tasks
        .filter(t => t.State === 'done' && t.ActualEnd)
        .sort((x, y) => new Date(x.ActualEnd).getTime() - new Date(y.ActualEnd).getTime());
      for (let i = 0; i < sorted.length - 1; i++) {
        bandSuccessor.set(sorted[i].ID, sorted[i + 1].ID);
      }
    }
    const depSuccessors = new Map(); // taskID -> [dependent taskIDs] (done only)
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
      if (visiting.has(id)) return 0; // cycle guard (shouldn't happen with cycle-free deps)
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
    const globalPastSlot = new Map();
    for (const [id, s] of succession) {
      globalPastSlot.set(id, maxPastSlots - 1 - s);
    }
    const pastSlotPerBand = bands.map(b => {
      const m = new Map();
      for (const t of b.tasks) {
        if (globalPastSlot.has(t.ID)) m.set(t.ID, globalPastSlot.get(t.ID));
      }
      return m;
    });
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

    // ---- Future sub-columns by priority within each depth ----
    // For each topological depth, collect the distinct priorities of
    // the todo tasks at that depth, sorted DESC. Each priority gets a
    // sub-column inside the depth's slot in the future zone; higher
    // priority means leftmost sub-column. The "next" eligible task is
    // therefore in the leftmost sub-column at depth 0.
    const futureSubColumnX = new Map(); // taskID -> x offset
    const futurePriorityBuckets = [];   // [{ depth, priority, x }] for labels
    {
      // tasksByDepth: depth -> [{ priority, count }]
      const tasksByDepth = new Map();
      for (const t of this.tasks || []) {
        if (t.State !== 'todo') continue;
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
    const featureAggregates = new Map(); // featureID -> {from, to, bi}
    for (const fid of this.foldedFeatures) {
      const feat = taskByID.get(fid);
      if (!feat || feat.Type !== 'feature') continue;
      const desc = collectDescendants(fid, new Set());
      if (!desc.size) continue;
      let lo = Infinity, hi = -Infinity;
      const bandVotes = new Map();
      for (const did of desc) {
        const d = taskByID.get(did);
        if (!d || d.Type === 'feature') continue; // ignore sub-features
        const p = rawPositions.get(did);
        if (!p) continue;
        lo = Math.min(lo, p.from);
        hi = Math.max(hi, p.to);
        bandVotes.set(p.bi, (bandVotes.get(p.bi) || 0) + 1);
      }
      if (lo === Infinity) continue;
      // Feature's own band if it has a target_role; else the band that
      // contains the most descendants; else the general fallback.
      let bi = bandIndexByTaskID.get(fid);
      if (bi == null) {
        let best = -1, max = -1;
        for (const [k, v] of bandVotes) if (v > max) { max = v; best = k; }
        bi = best >= 0 ? best : 0;
      }
      featureAggregates.set(fid, { from: lo, to: hi, bi });
    }

    // ---- Visible entries per band ----
    const visiblePerBand = bands.map(() => []);
    for (const t of this.tasks || []) {
      if (hiddenByFold.has(t.ID)) continue;
      if (t.Type === 'feature') {
        if (this.foldedFeatures.has(t.ID) && featureAggregates.has(t.ID)) {
          const agg = featureAggregates.get(t.ID);
          visiblePerBand[agg.bi].push({ task: t, from: agg.from, to: agg.to, kind: 'feature-agg' });
        } else if (!childrenByParent.has(t.ID)) {
          const p = rawPositions.get(t.ID);
          if (p) visiblePerBand[p.bi].push({ task: t, from: p.from, to: p.to, kind: 'normal' });
        }
        // unfolded feature with children: feature itself hidden, kids show through
        continue;
      }
      const p = rawPositions.get(t.ID);
      if (p) visiblePerBand[p.bi].push({ task: t, from: p.from, to: p.to, kind: 'normal' });
    }

    // ---- Lane assignment per band (greedy) ----
    const positions = []; // { task, bi, lane, from, to, kind }
    const lanesPerBand = bands.map(() => 1);
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
        positions.push({ task: e.task, bi, lane: placed, from: e.from, to: e.to, kind: e.kind });
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
          <text class="zone-label" x=${futureStartX + 4} y="14">FUTURE · topological + priority</text>

          <!-- Priority labels at the top of each future sub-column -->
          ${futurePriorityBuckets.map(b => svg`
            <text class="priority-label" x=${b.x + 4} y=${headerH - 4}>${this._priorityLabel(b.priority)}</text>
          `)}

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
            if (p.kind === 'feature-agg') {
              const childCount = (childrenByParent.get(t.ID) || []).length;
              return svg`
                <rect class="task-rect feature-agg"
                      x=${p.from} y=${y}
                      width=${w} height=${taskHeight}
                      rx="6" ry="6"
                      fill="#eef0f3"
                      stroke=${color}
                      stroke-dasharray="6 3"
                      style="cursor:pointer"
                      @click=${(e) => { e.stopPropagation(); this._toggleFold(t.ID); }}>
                  <title>${t.Title} — feature with ${childCount} task${childCount === 1 ? '' : 's'} (click to expand)</title>
                </rect>
                <text class="task-label" x=${labelX} y=${y + taskHeight / 2 + 4}
                      style="pointer-events:none">
                  ▸ ${this._truncate(t.Title, labelMaxChars - 2)}
                </text>
              `;
            }
            const isFeatureUnfolded = t.Type === 'feature' && !this.foldedFeatures.has(t.ID);
            // Childless features and (defensive) any feature reaching here render as normal.
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

          <!-- Dependency arrows: anchored to each task's actual lane Y.
               The path stops one markerWidth before the target rect's
               left edge so the arrowhead tip lands exactly on the
               border, not inside the box. -->
          ${this.deps.map(d => {
            const from = posByTaskID.get(d.DependsOnID);
            const to = posByTaskID.get(d.TaskID);
            if (!from || !to) return null;
            const markerW = 8;
            // Forward / backward decision uses the BOX edges, not the
            // marker-adjusted endpoints. Otherwise a target sitting in
            // the slot immediately to the right of the source (its left
            // edge ≈ source.right + 6, then -8 for the marker) would
            // appear as if it were behind the source by 2 px and
            // trigger an unnecessary loop.
            const sourceRight = from.to;
            const targetLeft = to.from;
            const x1 = sourceRight;
            const y1 = taskCenterY(from.bi, from.lane);
            const y2 = taskCenterY(to.bi, to.lane);

            // Forward edge (target's left edge is at or to the right of
            // source's right edge): one S-curve. We compute the path
            // endpoint so the marker tip lands on the target's left
            // border, but clamp it forward of the source so the cubic's
            // end tangent is always +x. Otherwise adjacent boxes (gap
            // smaller than markerW) would produce a tiny backward
            // tangent and `auto-start-reverse` would flip the marker.
            if (targetLeft >= sourceRight) {
              const x2 = Math.max(targetLeft - markerW, x1 + 2);
              const cx = (x1 + x2) / 2;
              return svg`<path class="arrow" d=${`M ${x1} ${y1} C ${cx} ${y1} ${cx} ${y2} ${x2} ${y2}`}></path>`;
            }
            const x2 = targetLeft - markerW;

            // Backward edge (target is at or left of source — typically an
            // inconsistency where a done task depends on a still-todo one).
            // Both endpoints must keep horizontal tangents pointing right
            // so the marker ends up rotated toward the target's left edge.
            // We detour below the lower endpoint with two cubic segments
            // meeting at a midpoint waypoint.
            const bow = 60;                              // horizontal stick-out
            const arch = Math.max(40, taskHeight + 24);  // vertical detour
            const yMid = Math.max(y1, y2) + arch;
            const xMid = (x1 + x2) / 2;
            return svg`<path class="arrow" d=${
              `M ${x1} ${y1} ` +
              `C ${x1 + bow} ${y1} ${x1 + bow} ${yMid} ${xMid} ${yMid} ` +
              `S ${x2 - bow} ${y2} ${x2} ${y2}`
            }></path>`;
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
