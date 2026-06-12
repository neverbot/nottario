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
    // Optional cycle filter. When null/empty, the API defaults to the
    // project's active cycle. Owned by the parent page (board.js); we
    // just forward it to the tasks endpoint.
    cycleId: { type: String, attribute: 'cycle-id' },
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
    _justAppeared: { state: true },
    _stageWidth: { state: true },
  };

  static styles = css`
    :host { display: block; }
    .stage {
      position: relative;
      background: var(--bg);
      border: 1px solid var(--border);
      border-radius: 8px;
      overflow: auto;
      max-height: 70vh;
    }
    .stage.empty {
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 240px;
      color: var(--fg-muted);
      font-size: 13px;
      text-align: center;
      padding: 32px;
    }
    svg { display: block; }
    /* Single-fill lanes; rhythm comes from a 1px hairline between
       rows (rendered as a separate line element below), not from
       zebra stripes. Features band gets a slightly darker fill so
       it reads as a parent row. */
    .band-bg {
      fill: var(--gantt-band-1);
    }
    .band-bg.features {
      fill: var(--gantt-band-features);
    }
    .band-separator {
      stroke: var(--gantt-band-separator);
      stroke-width: 1;
    }
    .zone-past-tint {
      fill: var(--gantt-zone-past-tint);
    }
    .band-label, .features-label {
      fill: var(--gantt-label-muted);
      font-size: 11px;
      letter-spacing: 0.04em;
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-weight: 600;
      font-variant-numeric: tabular-nums;
    }
    .zone-label {
      fill: var(--gantt-label-muted);
      font-size: 11px;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-variant-numeric: tabular-nums;
    }
    .priority-label {
      fill: var(--gantt-label-muted);
      font-size: 11px;
      letter-spacing: 0.04em;
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-weight: 600;
      font-variant-numeric: tabular-nums;
    }
    .zone-divider {
      stroke: var(--gantt-zone-divider);
      stroke-width: 1;
    }
    .now-line {
      stroke: var(--gantt-now-line);
      stroke-width: 1.5;
    }
    .now-pill-bg {
      fill: var(--gantt-now-line);
    }
    .now-pill-text {
      fill: var(--fg-on-accent);
      font-size: 9.5px;
      font-weight: 700;
      letter-spacing: 0.08em;
      font-family: ui-monospace, SFMono-Regular, monospace;
      dominant-baseline: central;
    }
    .task-rect {
      stroke-width: 1.5;
      cursor: pointer;
      rx: 4;
      ry: 4;
    }
    .task-rect:hover { stroke: var(--fg); }
    /* Selection: when something is selected, non-connected bars dim
       and non-connected arrows fade so the focused subgraph reads
       loudly. Promoted arrows render in a second pass (after the
       task rects) so they sit on top of any box they cross. */
    .task-rect.dim { opacity: 0.35; }
    .arrow.dim { opacity: 0.18; }
    .arrow { cursor: pointer; }
    .arrow.promoted { stroke: var(--fg); }
    /* A bar that's part of the active selection (single-task pipeline
       or just-unfolded feature subtree) gets a thicker, darker stroke
       so done bars — which sit in pale gray and don't visually
       distinguish themselves from dimmed neighbours otherwise — read
       as "this is what you're looking at right now". */
    .task-rect.promoted { stroke: var(--fg) !important; stroke-width: 2 !important; }
    /* Bars that became visible via a feature unfold get a brief
       opacity fade-in so the eye locates them after the layout
       reflows. Refold has no motion. */
    .task-rect.just-appeared {
      animation: gantt-bar-appear 240ms ease-out;
    }
    @keyframes gantt-bar-appear {
      from { opacity: 0; }
      to   { opacity: 1; }
    }
    @media (prefers-reduced-motion: reduce) {
      .task-rect.just-appeared { animation: none; }
    }
    /* Done bars are intentionally monochrome — past noise calmed so
       the eye gravitates to NOW and FUTURE. */
    .task-rect.done {
      fill: var(--gantt-bar-done-fill);
      stroke: var(--gantt-bar-done-stroke);
    }
    /* Todo bars share the role-color tint with doing bars and only
       differ by a dashed stroke. Fill/stroke are set inline (role
       hex → color-mix tint) so a per-project role palette works
       without re-skinning the CSS. */
    .task-rect.todo {
      stroke-dasharray: 4 3;
    }
    /* Inconsistent: the task is not yet done but one of its dependents
       is already done. Surface as a solid red 2.5px border. */
    .task-rect.inconsistent {
      stroke: var(--danger) !important;
      stroke-dasharray: 0 !important;
      stroke-width: 2.5 !important;
    }
    .task-label {
      fill: var(--gantt-label);
      font-size: 11px;
      pointer-events: none;
    }
    /* Labels painted on top of a role-color tint use a darkened
       version of the role colour (the "ink" — set inline). */
    .task-label.on-tint {
      font-weight: 600;
    }
    /* Bug indicator: typography, not colour. Italic title + a
       "bug:" prefix. Red is reserved for the inconsistency alarm and
       the NOW line, so it never reads as "this task needs attention
       NOW" when a bar is merely a bug. */
    .task-label.bug {
      font-style: italic;
    }
    .arrow {
      stroke: var(--border-strong);
      stroke-width: 1;
      fill: none;
      marker-end: url(#dep-arrowhead);
    }
    .legend {
      padding: 8px 16px;
      font-size: 11px;
      color: var(--fg-muted);
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
      box-sizing: border-box;
    }
    .legend .bug-mark {
      font-style: italic;
      font-weight: 600;
      color: var(--fg);
      margin-right: 4px;
    }
    .error {
      color: var(--danger);
      background: var(--tint-red);
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
      /* Shadow DOM ignores global box-sizing rules (see claude.md).
         Without this, max-width applies to the content area only,
         and the actual offsetWidth = max-width + padding + border —
         the card would render ~22px wider than the runtime cap. */
      box-sizing: border-box;
      background: #fff;
      color: var(--fg);
      border: 1px solid var(--border);
      border-radius: 8px;
      box-shadow: 0 4px 12px rgba(31, 35, 40, 0.12);
      padding: 8px 10px;
      min-width: 220px;
      max-width: 320px;
      font-size: 12px;
      line-height: 1.4;
      /* Safety: on extremely narrow stages the runtime max-width
         override may still hit a min-content floor (a chip wider
         than the box). Clip so the card never visibly overflows. */
      overflow: hidden;
      /* Title can be a long unbroken slug; break aggressively. */
      overflow-wrap: anywhere;
    }
    .hover-card .title {
      font-size: 13px;
      font-weight: 600;
      color: var(--fg);
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
      background: var(--gray-2);
      color: var(--fg);
    }
    .hover-card .chip.state-todo  { background: var(--gray-2); color: var(--fg-muted); }
    .hover-card .chip.state-doing { background: var(--tint-blue); color: var(--tint-blue-fg); }
    .hover-card .chip.state-done  { background: var(--tint-green); color: var(--tint-green-fg); }
    .hover-card .chip.type-bug    { background: var(--tint-red); color: var(--tint-red-fg); }
    /* Role chip: background and foreground are derived from the
       project's role colour via color-mix; the inline style sets
       them at render time so contrast is always AAA-safe even for
       neon role colours. */
    .hover-card .chip.role        { font-weight: 600; }
    .hover-card .assignee {
      display: flex;
      align-items: center;
      gap: 6px;
      margin-top: 6px;
      color: var(--fg-muted);
    }
    .hover-card .assignee img {
      width: 16px; height: 16px;
      border-radius: 50%;
      object-fit: cover;
    }
    .hover-card .meta {
      color: var(--fg-muted);
      margin-top: 4px;
    }
    /* Faint footer line ("Double-click to open / expand") so the
       interaction hint reads as guidance, not as data. */
    .hover-card .meta.hint {
      color: var(--gray-5);
      font-size: 11px;
      margin-top: 6px;
    }
    .empty {
      padding: 40px;
      text-align: center;
      color: var(--fg-muted);
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
    this.foldedFeatures = new Set(); // feature IDs currently collapsed
    this._knownFeatureIDs = new Set(); // features we've ever seen
    // Hover-popup state. null when nothing is hovered/focused.
    // Carries the source task plus the anchor pixel coords relative
    // to the stage's content (so we can flip when overflowing).
    this._hover = null;
    this._reducedMotion =
      typeof window !== 'undefined' && window.matchMedia
        ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
        : false;
    // Selection: the anchor is the task the user actually clicked
    // first; the set is anchor + every task transitively reachable
    // via incoming/outgoing dependency edges (undirected BFS over
    // `this.deps`). Both null/empty means no selection.
    this._selectedAnchor = null;
    this._selectedSet = new Set();
    // IDs of task bars that just became visible via a feature
    // unfold; rendered with the `.just-appeared` CSS class for a
    // brief opacity fade-in. Cleared after the animation finishes.
    this._justAppeared = new Set();
    // Set when the next render should scroll-to-leftmost of the
    // just-appeared bars. Cleared once the scroll runs.
    this._pendingUnfoldScroll = false;
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
    for (const d of this.deps || []) {
      add(d.task_id, d.depends_on_id);
      add(d.depends_on_id, d.task_id);
    }
    const out = new Set([taskID]);
    const stack = [taskID];
    while (stack.length) {
      const id = stack.pop();
      for (const n of adj.get(id) || []) {
        if (!out.has(n)) {
          out.add(n);
          stack.push(n);
        }
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
    if (this._selectedSet.has(t.id)) {
      this._emitSelect(t);
      return;
    }
    this._selectTask(t.id);
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
      if (t.type !== 'feature') continue;
      seen.add(t.id);
      if (!this._knownFeatureIDs.has(t.id)) {
        next.add(t.id);
        this._knownFeatureIDs.add(t.id);
      }
    }
    // Forget features that no longer exist.
    for (const id of [...next]) if (!seen.has(id)) next.delete(id);
    for (const id of [...this._knownFeatureIDs])
      if (!seen.has(id)) this._knownFeatureIDs.delete(id);
    this.foldedFeatures = next;
  }

  _toggleFold(featureID) {
    // Expanding/collapsing reflows the bars below this point;
    // dismiss the hover popup so it doesn't hang in space anchored
    // to a coordinate that no longer matches the bar the user
    // clicked.
    this._hover = null;
    this._pointerBarID = null;
    const next = new Set(this.foldedFeatures);
    const isUnfold = next.has(featureID);
    if (isUnfold) {
      next.delete(featureID);
      // Mark the (now revealed) children for the appear animation,
      // and request a scroll-to-leftmost pass on the next paint.
      // Refold has no motion — we intentionally only animate the
      // open direction (design call, see task `5245b74c`).
      const children = (this.tasks || [])
        .filter((t) => t.parent_task_id === featureID)
        .map((t) => t.id);
      this._justAppeared = new Set(children);
      this._pendingUnfoldScroll = true;
      // Drop the class after the animation has had time to finish
      // so the next interaction doesn't re-fire it. 320 ms ≈ the
      // 240 ms animation + a small buffer.
      clearTimeout(this._appearTimer);
      this._appearTimer = setTimeout(() => {
        this._justAppeared = new Set();
      }, 320);
      // Highlight the feature's whole sub-tree (the feature itself
      // and every descendant) by reusing the selection mechanism.
      // The user just unfolded one box and several new bars + arrows
      // appeared inside it; dimming everything else makes "what just
      // came in" unmistakable. The highlight auto-clears after a few
      // seconds so it doesn't sit there forever.
      const childrenByParent = new Map();
      for (const t of this.tasks || []) {
        if (!t.parent_task_id) continue;
        if (!childrenByParent.has(t.parent_task_id)) childrenByParent.set(t.parent_task_id, []);
        childrenByParent.get(t.parent_task_id).push(t.id);
      }
      const sub = new Set([featureID]);
      const walk = (id) => {
        for (const c of childrenByParent.get(id) || []) {
          if (sub.has(c)) continue;
          sub.add(c);
          walk(c);
        }
      };
      walk(featureID);
      this._selectedAnchor = featureID;
      this._selectedSet = sub;
    } else {
      next.add(featureID);
    }
    this.foldedFeatures = next;
  }

  connectedCallback() {
    super.connectedCallback();
    this.load();
    this._subscribe();
    // Keep the "now" line live; once per minute is enough for the UI.
    this._tick = setInterval(() => {
      this.now = new Date();
    }, 60 * 1000);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    clearInterval(this._tick);
    this._unsub?.();
    this._stageResizeObs?.disconnect();
  }

  // Measure the .stage's visible width so the SVG can stretch to
  // cover the viewport even when the backlog is sparse. Without
  // this the band-bg stripes end where the rightmost task lives and
  // the stage's white background shows through on the right.
  _observeStageWidth() {
    const stage = this.shadowRoot?.querySelector('.stage');
    if (!stage) return;
    if (this._stageResizeObs) return; // already wired
    const update = () => {
      const w = stage.clientWidth;
      if (w && Math.abs(w - (this._stageWidth || 0)) > 1) {
        this._stageWidth = w;
      }
      // If the initial centring never completed (e.g. the stage was
      // still 0px wide when updated() last ran), retry as soon as the
      // stage gains a real width. updated() won't fire again unless a
      // reactive property changes, so the ResizeObserver is the only
      // signal we get when layout finally settles after navigation.
      if (!this._initialCenterDone && w && this.tasks && this.tasks.length) {
        this._centerOnNow();
      }
    };
    update();
    if (typeof ResizeObserver !== 'undefined') {
      this._stageResizeObs = new ResizeObserver(update);
      this._stageResizeObs.observe(stage);
    }
  }

  updated(c) {
    if (c && c.has && c.has('projectId')) {
      this.load();
      this._subscribe();
      this._initialCenterDone = false; // re-centre when project changes
    }
    if (c && c.has && c.has('cycleId') && !c.has('projectId')) {
      // Switching cycle within the same project: just re-fetch tasks.
      this.load();
    }
    // Once the SVG is in the DOM and has its first computed layout,
    // scroll the stage so the "now" line sits in the middle. We only
    // do this once per project so we don't fight the user's scroll.
    if (!this._initialCenterDone && this.tasks && this.tasks.length) {
      this._centerOnNow();
    }
    this._repositionHoverCard();
    if (this._pendingUnfoldScroll) {
      this._pendingUnfoldScroll = false;
      this._scrollToJustAppeared();
    }
    this._observeStageWidth();
  }

  // Smoothly scroll the .stage so the leftmost just-appeared child
  // sits a comfortable distance from the left edge. Optionally
  // scrolls vertically too when the child's lane falls outside the
  // current viewport. Snaps under prefers-reduced-motion.
  _scrollToJustAppeared() {
    const stage = this.shadowRoot?.querySelector('.stage');
    if (!stage || !this._justAppeared || this._justAppeared.size === 0) return;
    // Read every just-appeared task's bounding box in stage-content
    // coords (its DOM position relative to .stage padding box, plus
    // current scroll). Pick the leftmost.
    const rects = [...this._justAppeared]
      .map((id) => {
        const el = this.shadowRoot.querySelector(`[data-task-id="${CSS.escape(id)}"]`);
        if (!el) return null;
        const br = el.getBoundingClientRect();
        const sr = stage.getBoundingClientRect();
        return {
          left: br.left - sr.left + stage.scrollLeft,
          top: br.top - sr.top + stage.scrollTop,
          height: br.height,
        };
      })
      .filter(Boolean);
    if (!rects.length) return;
    rects.sort((a, b) => a.left - b.left);
    const left = rects[0].left;
    const top = rects[0].top;
    const stageW = stage.clientWidth;
    const stageH = stage.clientHeight;
    // 32px gutter from the left edge so the bar isn't flush.
    let targetLeft = Math.max(0, left - 32);
    // Don't scroll right if the bar is already visible comfortably.
    if (left > stage.scrollLeft && left < stage.scrollLeft + stageW - 100) {
      targetLeft = stage.scrollLeft;
    }
    // Vertical: only scroll when the child's lane is outside the
    // visible band. Otherwise keep scrollTop where it was.
    let targetTop = stage.scrollTop;
    if (top < stage.scrollTop || top + rects[0].height > stage.scrollTop + stageH) {
      targetTop = Math.max(0, top - 32);
    }
    const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
    if (reduce) {
      stage.scrollLeft = targetLeft;
      stage.scrollTop = targetTop;
      return;
    }
    const startL = stage.scrollLeft,
      deltaL = targetLeft - startL;
    const startT = stage.scrollTop,
      deltaT = targetTop - startT;
    if (Math.abs(deltaL) < 1 && Math.abs(deltaT) < 1) return;
    const t0 = performance.now();
    const duration = 220;
    const ease = (t) => 1 - Math.pow(1 - t, 4); // ease-out-quart
    const step = (now) => {
      const t = Math.min(1, (now - t0) / duration);
      const k = ease(t);
      stage.scrollLeft = startL + deltaL * k;
      stage.scrollTop = startT + deltaT * k;
      if (t < 1) requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  }

  // After every render, measure the hover card's real width/height
  // and reposition it. Content-driven width (220–320 in CSS) and
  // height mean the template's estimate can be off until we measure
  // — without this the left/up flip math opens an oversized gap.
  //
  // The positioning has three phases:
  //   1. Cap the card's max-width / max-height to fit inside the
  //      stage (with an 8px gutter). On a one-lane Gantt the stage
  //      can be ~400px wide; the CSS default max-width: 320px would
  //      otherwise produce a card too wide to flip into.
  //   2. Pick a tentative position next to the cursor (right + below
  //      preferred) and flip to the other side when the natural slot
  //      doesn't fit.
  //   3. Final clamp: force the card inside [scrollLeft+8,
  //      scrollLeft+stageW-w-8] horizontally and analogously
  //      vertically, so a near-edge cursor without a winning flip
  //      can never leave the card hanging outside the stage.
  _repositionHoverCard() {
    const card = this.shadowRoot?.querySelector('.hover-card');
    if (!card || !this._hover) {
      this._lastCardW = null;
      this._lastCardH = null;
      return;
    }
    const stage = this.shadowRoot.querySelector('.stage');
    const scrollLeft = stage?.scrollLeft || 0;
    const stageW = stage?.clientWidth || 800;
    const scrollTop = stage?.scrollTop || 0;
    const stageH = stage?.clientHeight || 600;
    // Phase 1: constrain to the stage. The 8px gutter mirrors the
    // minimum margin used elsewhere in this function.
    const maxW = Math.max(120, stageW - 16);
    const maxH = Math.max(80, stageH - 16);
    card.style.maxWidth = `${maxW}px`;
    card.style.maxHeight = `${maxH}px`;
    // When the stage is narrower than the CSS min-width (220), lift
    // that floor too so the card can actually shrink.
    if (maxW < 220) card.style.minWidth = `${maxW}px`;
    else card.style.minWidth = '';
    const measuredW = card.offsetWidth;
    const measuredH = card.offsetHeight;
    const dwOK = Math.abs((this._lastCardW || 0) - measuredW) < 1;
    const dhOK = Math.abs((this._lastCardH || 0) - measuredH) < 1;
    if (dwOK && dhOK) return;
    this._lastCardW = measuredW;
    this._lastCardH = measuredH;
    const { barX, barY, cursor } = this._hover;
    const anchorX = cursor ? cursor.x : barX;
    const anchorY = cursor ? cursor.y : barY;
    const offX = cursor ? 14 : 8;
    const offYBelow = cursor ? 16 : -8;
    const offYAbove = cursor ? 16 : 8;
    // Phase 2: tentative slot.
    let left = anchorX + offX;
    if (left + measuredW > scrollLeft + stageW) {
      left = anchorX - measuredW - offX;
    }
    let top = anchorY + offYBelow;
    if (top + measuredH > scrollTop + stageH) {
      top = anchorY - measuredH - offYAbove;
    }
    // Phase 3: final clamp. If measuredW > stageW the right bound
    // becomes < the left bound, in which case Math.min wins and we
    // pin to the left gutter; the card can still overflow on the
    // right but at least the title is visible.
    left = Math.max(scrollLeft + 8, Math.min(left, scrollLeft + stageW - measuredW - 8));
    top = Math.max(scrollTop + 8, Math.min(top, scrollTop + stageH - measuredH - 8));
    card.style.left = `${left}px`;
    card.style.top = `${top}px`;
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
    if (reduce) {
      stage.scrollLeft = target;
      return;
    }
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
    //
    // If after two frames the stage still has no width (the topbar
    // or sidebar is still settling after a navigation), retry on the
    // next frame up to ~10 times before giving up. The ResizeObserver
    // wired in _observeStageWidth() is the other safety net.
    if (this._centerInFlight) return;
    this._centerInFlight = true;
    let attempts = 0;
    const tryCenter = () => {
      const stage = this.shadowRoot?.querySelector('.stage');
      const nowLine = this.shadowRoot?.querySelector('.now-line');
      if (!stage || !nowLine) {
        this._centerInFlight = false;
        return;
      }
      if (!stage.clientWidth) {
        attempts++;
        if (attempts < 10) {
          requestAnimationFrame(tryCenter);
        } else {
          this._centerInFlight = false;
        }
        return;
      }
      const nowX = parseFloat(nowLine.getAttribute('x1') || '0');
      const target = Math.max(0, nowX - stage.clientWidth / 2);
      stage.scrollLeft = target;
      this._initialCenterDone = true;
      this._centerInFlight = false;
    };
    requestAnimationFrame(() => requestAnimationFrame(tryCenter));
  }

  _subscribe() {
    this._unsub?.();
    if (!this.projectId) return;
    this._unsub = subscribe(this.projectId, (ev) => {
      // Tasks and dependencies are what the gantt draws; reload on
      // any related event. 'realtime.reconnected' catches events that
      // happened while EventSource was reconnecting.
      if (
        ev.type === 'realtime.reconnected' ||
        ev.type?.startsWith('task.') ||
        ev.type?.startsWith('cycle.')
      )
        this.load();
    });
  }

  _roleLabel(id) {
    const r = (this.roles || []).find((x) => x.id === id);
    return r ? r.label : '';
  }

  _featureRoles(featureID) {
    // List the distinct roles of the feature's non-feature descendants,
    // separated by commas, in the project's role order.
    const taskByID = new Map((this.tasks || []).map((t) => [t.id, t]));
    const childrenByParent = new Map();
    for (const t of this.tasks || []) {
      if (!t.parent_task_id) continue;
      if (!childrenByParent.has(t.parent_task_id)) childrenByParent.set(t.parent_task_id, []);
      childrenByParent.get(t.parent_task_id).push(t.id);
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
      if (t && t.type !== 'feature' && t.target_role_id) roleIDs.add(t.target_role_id);
    }
    const sortedRoles = [...(this.roles || [])]
      .filter((r) => roleIDs.has(r.id))
      .sort((a, b) => (a.position ?? 0) - (b.position ?? 0))
      .map((r) => r.label.toLowerCase());
    return sortedRoles.length ? sortedRoles.join(', ') : 'no roles';
  }

  _priorityLabel(value) {
    if (this.priorities && this.priorities.length) {
      const exact = this.priorities.find((p) => p.value === value);
      if (exact) return exact.key;
    }
    return `p${value}`;
  }

  async load() {
    if (!this.projectId) return;
    try {
      const cycleParam = this.cycleId ? `&cycle_id=${encodeURIComponent(this.cycleId)}` : '';
      const [tr, rr, dr, mr, qr] = await Promise.all([
        fetch(`/api/projects/${this.projectId}/tasks?include_children=true${cycleParam}`),
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
    const order = [...this.roles].sort((a, b) => (a.position ?? 0) - (b.position ?? 0));
    const general = {
      ID: '__general__',
      Key: 'general',
      Label: 'General',
      Color: 'var(--fg-muted)',
    };
    const result = order.map((r) => ({ role: r, tasks: [] }));
    result.push({ role: general, tasks: [] });
    const byID = new Map(order.map((r) => [r.id, result.find((b) => b.role.id === r.id)]));
    for (const t of this.tasks || []) {
      const band = t.target_role_id ? byID.get(t.target_role_id) : result[result.length - 1];
      (band || result[result.length - 1]).tasks.push(t);
    }
    // Drop bands with no tasks except the general one if it has any.
    return result.filter((b) => b.tasks.length > 0);
  }

  // Compute topological depth across ALL todo tasks in the project,
  // not per band. A task depending on a task in another band still
  // ends up further to the right by columns, so dependency arrows
  // never collapse to perfectly vertical lines.
  //
  // Depth = 0 when the task has no `todo`/`doing` predecessors;
  // otherwise = max(predecessor.depth) + 1.
  computeTopoDepths(tasks) {
    const taskByID = new Map(tasks.map((t) => [t.id, t]));
    const incoming = new Map();
    for (const t of tasks) incoming.set(t.id, []);
    for (const d of this.deps) {
      if (incoming.has(d.task_id) && taskByID.has(d.depends_on_id)) {
        incoming.get(d.task_id).push(d.depends_on_id);
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
        if (t.state === 'done') continue;
        d = Math.max(d, visit(pre) + 1);
      }
      depth.set(id, d);
      return d;
    };
    for (const t of tasks) visit(t.id);
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
    const bandPad = 8; // vertical padding inside a band
    const labelWidth = 120;
    const pastSlotW = minBarWidth + 6; // 146 — one done task per ordinal slot
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
      if (t.state === 'done' && t.actual_end) doneByID.set(t.id, t);
    }
    // A past task is "anchored" when it touches at least one dependency
    // edge that will be drawn as an arrow on the chart. Anchored tasks
    // keep their own column. "Free" past tasks (no arrows in or out)
    // tuck vertically into the previous column's empty lanes, which
    // shrinks the band's horizontal footprint without altering any
    // arrow geometry.
    const depTouched = new Set();
    for (const d of this.deps) {
      if (doneByID.has(d.task_id)) depTouched.add(d.task_id);
      if (doneByID.has(d.depends_on_id)) depTouched.add(d.depends_on_id);
    }
    // A done task whose ANCESTOR feature touches a dep edge inherits
    // that anchored status. Otherwise leaf children of an anchored
    // feature have succession=0, gravitate to NOW and stretch the
    // parent's aggregate all the way to the present.
    const taskByIDForSucc = new Map();
    for (const t of this.tasks || []) taskByIDForSucc.set(t.id, t);
    for (const t of this.tasks || []) {
      if (!doneByID.has(t.id)) continue;
      let p = taskByIDForSucc.get(t.parent_task_id);
      while (p) {
        if (depTouched.has(p.id)) {
          depTouched.add(t.id);
          break;
        }
        p = taskByIDForSucc.get(p.parent_task_id);
      }
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
    const futureLanesPerBand = bands.map((b) => {
      const cellCounts = new Map();
      let doingCount = 0;
      for (const t of b.tasks) {
        if (t.state === 'doing') {
          doingCount++;
        } else if (t.state === 'todo') {
          const d = globalDepths.get(t.id) || 0;
          const key = `${d}:${t.priority}`;
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
    // Build depSuccessors and inherit each feature's outgoing edges
    // onto every descendant. Semantically: if X depends_on feature F,
    // X also "depends on" every leaf of F finishing (F isn't done
    // until they are). Without this propagation, F's leaf children
    // have no successors of their own → succession 0 → slot pinned
    // to NOW → F's aggregate stretches to the present even when F
    // itself sits far in the past.
    const depSuccessors = new Map();
    for (const d of this.deps) {
      if (doneByID.has(d.task_id) && doneByID.has(d.depends_on_id)) {
        if (!depSuccessors.has(d.depends_on_id)) depSuccessors.set(d.depends_on_id, []);
        depSuccessors.get(d.depends_on_id).push(d.task_id);
      }
    }
    for (const t of this.tasks || []) {
      if (!doneByID.has(t.id)) continue;
      let p = taskByIDForSucc.get(t.parent_task_id);
      while (p) {
        const parentSuccs = depSuccessors.get(p.id);
        if (parentSuccs && parentSuccs.length) {
          if (!depSuccessors.has(t.id)) depSuccessors.set(t.id, []);
          const list = depSuccessors.get(t.id);
          for (const s of parentSuccs) if (!list.includes(s)) list.push(s);
        }
        p = taskByIDForSucc.get(p.parent_task_id);
      }
    }
    const bandSuccessor = new Map();
    for (const b of bands) {
      const anchoredChronoSorted = b.tasks
        .filter((t) => t.state === 'done' && t.actual_end && depTouched.has(t.id))
        .sort((x, y) => new Date(x.actual_end).getTime() - new Date(y.actual_end).getTime());
      for (let i = 0; i < anchoredChronoSorted.length - 1; i++) {
        bandSuccessor.set(anchoredChronoSorted[i].id, anchoredChronoSorted[i + 1].id);
      }
    }
    // Children indexed by parent so a feature's succession can floor
    // to its deepest descendant's succession (see the "feature floor"
    // step inside computeSucc below).
    const childrenByParentForSucc = new Map();
    for (const t of this.tasks || []) {
      if (!t.parent_task_id) continue;
      if (!childrenByParentForSucc.has(t.parent_task_id))
        childrenByParentForSucc.set(t.parent_task_id, []);
      childrenByParentForSucc.get(t.parent_task_id).push(t.id);
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
      // Feature floor: a feature's succession must be at least as
      // high as its deepest descendant's. Otherwise the parent's slot
      // lands RIGHT of its leftmost child (children run on a deeper
      // dep chain than the parent's own outgoing edges), so any
      // precondition of the feature ends up between the feature's
      // children and the feature itself, producing backward arrows
      // when the feature is rendered unfolded.
      for (const c of childrenByParentForSucc.get(id) || []) {
        s = Math.max(s, computeSucc(c));
      }
      visiting.delete(id);
      succession.set(id, s);
      return s;
    };
    for (const id of doneByID.keys()) computeSucc(id);
    const maxPastSlots = doneByID.size ? Math.max(0, ...succession.values()) + 1 : 0;
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
        if (t.state !== 'done' || !t.actual_end) continue;
        if (!depTouched.has(t.id)) continue;
        const s = globalPastSlot.get(t.id);
        slotOccupants.set(s, (slotOccupants.get(s) || 0) + 1);
      }
      // Newest free first: closer-to-NOW priority gets the rightmost
      // available slot.
      const freeSorted = b.tasks
        .filter((t) => t.state === 'done' && t.actual_end && !depTouched.has(t.id))
        .sort((x, y) => new Date(y.actual_end).getTime() - new Date(x.actual_end).getTime());
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
        globalPastSlot.set(t.id, bestSlot);
        slotOccupants.set(bestSlot, (slotOccupants.get(bestSlot) || 0) + 1);
        usedGlobalSlots.add(bestSlot);
      }
    }
    // Compact: drop slot indices that no VISIBLE task occupies. A
    // task is hidden when an ancestor feature is folded (the feature
    // aggregate renders at the parent's own slot, not at the
    // children's). Slots whose only occupants are hidden children
    // would leave empty columns in the past zone, pushing visible
    // bars left for no visual reason. We drop those slots while
    // preserving the slot ORDER, so cross-band dep arrows stay
    // forward. Hidden tasks themselves snap to their nearest visible
    // ancestor slot (≤ original) so any geometry that still reads
    // their slot lands on the folded parent's column.
    {
      const isHiddenByFold = (taskID) => {
        let cur = taskByIDForSucc.get(taskID);
        while (cur && cur.parent_task_id) {
          if (this.foldedFeatures.has(cur.parent_task_id)) return true;
          cur = taskByIDForSucc.get(cur.parent_task_id);
        }
        return false;
      };
      const visibleSlots = new Set();
      for (const [id, s] of globalPastSlot) {
        if (!isHiddenByFold(id)) visibleSlots.add(s);
      }
      const usedSlots = [...visibleSlots].sort((a, b) => a - b);
      const remap = new Map();
      usedSlots.forEach((s, i) => {
        remap.set(s, i);
      });
      for (const [id, s] of globalPastSlot) {
        if (remap.has(s)) {
          globalPastSlot.set(id, remap.get(s));
          continue;
        }
        // Hidden child whose own slot was dropped. Snap to the nearest
        // visible slot at or below its original; that lands the child
        // on the same column as its folded parent aggregate.
        let target = -1;
        for (const us of usedSlots) {
          if (us <= s) target = us;
          else break;
        }
        globalPastSlot.set(id, target >= 0 ? remap.get(target) : 0);
      }
    }
    const compactedPastSlots = (() => {
      let m = -1;
      for (const s of globalPastSlot.values()) if (s > m) m = s;
      return m + 1;
    })();
    const pastSlotPerBand = bands.map((b) => {
      const m = new Map();
      for (const t of b.tasks) {
        if (globalPastSlot.has(t.id)) m.set(t.id, globalPastSlot.get(t.id));
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
    const futurePriorityBuckets = []; // [{ depth, priority, x }] for labels
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
        if (t.parent_task_id) hasDescendants.add(t.parent_task_id);
      }
      const tasksByDepth = new Map();
      for (const t of this.tasks || []) {
        if (t.state !== 'todo') continue;
        if (t.type === 'feature' && hasDescendants.has(t.id)) continue;
        const d = globalDepths.get(t.id) || 0;
        if (!tasksByDepth.has(d)) tasksByDepth.set(d, []);
        tasksByDepth.get(d).push(t);
      }
      // For each depth, build a sorted list of distinct priorities and
      // record the X for each priority bucket.
      const depthsSorted = Array.from(tasksByDepth.keys()).sort((a, b) => a - b);
      let cursor = futureStartX;
      for (const d of depthsSorted) {
        const ts = tasksByDepth.get(d);
        const distinctPriorities = Array.from(new Set(ts.map((t) => t.priority))).sort(
          (a, b) => b - a,
        ); // DESC
        for (let i = 0; i < distinctPriorities.length; i++) {
          const p = distinctPriorities[i];
          const x = cursor + i * futureColumnWidth + 12;
          futurePriorityBuckets.push({ depth: d, priority: p, x });
          for (const t of ts) {
            if (t.priority === p) futureSubColumnX.set(t.id, x);
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
    const taskByID = new Map((this.tasks || []).map((t) => [t.id, t]));
    const childrenByParent = new Map();
    for (const t of this.tasks || []) {
      if (!t.parent_task_id) continue;
      if (!childrenByParent.has(t.parent_task_id)) childrenByParent.set(t.parent_task_id, []);
      childrenByParent.get(t.parent_task_id).push(t.id);
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
      if (!f || f.type !== 'feature') continue;
      collectDescendants(fid, hiddenByFold);
    }

    // ---- Raw positions for every task (independent of fold state) ----
    // Computed in the task's "natural" band so we can build aggregates
    // even when descendants live in a band other than the feature's.
    const bandIndexByTaskID = new Map();
    bands.forEach((b, bi) => {
      for (const t of b.tasks) bandIndexByTaskID.set(t.id, bi);
    });
    const rawPositions = new Map(); // taskID -> {from, to, bi}
    bands.forEach((b, bi) => {
      const depths = futureDepthsPerBand[bi];
      const slots = pastSlotPerBand[bi];
      for (const t of b.tasks) {
        const pastSlot = t.state === 'done' ? (slots.get(t.id) ?? null) : null;
        const futureX = t.state === 'todo' ? futureSubColumnX.get(t.id) : undefined;
        const x = this.taskX({
          t,
          labelWidth,
          pastSlotW,
          pastSlot,
          presentX,
          presentWidth,
          futureStartX,
          futureColumnWidth,
          futureX,
          depths,
          minBarWidth,
        });
        if (!x) continue;
        rawPositions.set(t.id, { from: x.from, to: x.to, bi });
      }
    });

    // ---- Aggregate positions for folded features ----
    // First pass collects from/to plus the distinct set of role bands
    // each feature's non-feature descendants live in. If a feature has
    // descendants in 2+ role bands it's "cross-role" — we'll hoist it
    // into a dedicated Features lane (decided below). Single-role
    // features stay inside their natural role band.
    const featureAggregates = new Map(); // featureID -> {from, to, bi, crossRole, roleColors}
    for (const fid of this.foldedFeatures) {
      const feat = taskByID.get(fid);
      if (!feat || feat.type !== 'feature') continue;
      const desc = collectDescendants(fid, new Set());
      if (!desc.size) continue;
      // Aggregate position uses the feature's OWN slot, not the
      // envelope of its descendants. Descendants live in their own
      // role bands at slots that follow each task's own succession
      // depth; the envelope of those would stretch the aggregate
      // across the entire chronology of the descendants and overlap
      // arrows between sibling features. bandsSeen still walks the
      // descendants so the role-dot stack and crossRole flag stay
      // correct.
      const pFeat = rawPositions.get(fid);
      const lo = pFeat?.from ?? Infinity;
      const hi = pFeat?.to ?? -Infinity;
      const bandsSeen = new Set();
      const bandVotes = new Map();
      for (const did of desc) {
        const d = taskByID.get(did);
        if (!d || d.type === 'feature') continue;
        const p = rawPositions.get(did);
        if (!p) continue;
        bandsSeen.add(p.bi);
        bandVotes.set(p.bi, (bandVotes.get(p.bi) || 0) + 1);
      }
      if (lo === Infinity) continue;
      const crossRole = bandsSeen.size > 1;
      // Natural band (used when not cross-role): feature's own
      // target_role if set, else the band with the most descendants.
      let naturalBi = bandIndexByTaskID.get(fid);
      if (naturalBi == null) {
        let best = -1,
          max = -1;
        for (const [k, v] of bandVotes)
          if (v > max) {
            max = v;
            best = k;
          }
        naturalBi = best >= 0 ? best : 0;
      }
      // Capture the role colours of the involved bands, ordered by the
      // band's display position so the dots read top→bottom by role.
      const roleColors = [...bandsSeen]
        .sort((a, b) => a - b)
        .map((bi) => bands[bi].role.color || 'var(--gray-5)');
      featureAggregates.set(fid, { from: lo, to: hi, bi: naturalBi, crossRole, roleColors });
    }

    // ---- Optional Features lane (only when there's something to put in it) ----
    // When a folded feature spans 2+ role bands, we hoist its aggregate
    // into a synthetic lane at the top of the chart. The display order
    // of bands becomes [Features?, ...roleBands]; everything keyed by
    // band index is shifted by the offset.
    const featuresBand =
      featureAggregates.size > 0
        ? {
            role: {
              ID: '__features__',
              Key: 'features',
              Label: 'Features',
              Color: 'var(--fg-subtle)',
            },
            tasks: [],
          }
        : null;
    const displayBands = featuresBand ? [featuresBand, ...bands] : bands;
    const bandOffset = featuresBand ? 1 : 0;

    // ---- Visible entries per (display) band ----
    const visiblePerBand = displayBands.map(() => []);
    for (const t of this.tasks || []) {
      if (hiddenByFold.has(t.id)) continue;
      if (t.type === 'feature') {
        if (this.foldedFeatures.has(t.id) && featureAggregates.has(t.id)) {
          const agg = featureAggregates.get(t.id);
          const targetBi = 0;
          visiblePerBand[targetBi].push({
            task: t,
            from: agg.from,
            to: agg.to,
            kind: 'feature-agg',
            crossRole: agg.crossRole,
            roleColors: agg.roleColors,
          });
        } else if (!childrenByParent.has(t.id)) {
          const p = rawPositions.get(t.id);
          if (p)
            visiblePerBand[p.bi + bandOffset].push({
              task: t,
              from: p.from,
              to: p.to,
              kind: 'normal',
            });
        }
        // unfolded feature with children: feature itself hidden, kids show through
        continue;
      }
      const p = rawPositions.get(t.id);
      if (p)
        visiblePerBand[p.bi + bandOffset].push({ task: t, from: p.from, to: p.to, kind: 'normal' });
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
          if (laneEnds[li] + overlapGap <= e.from) {
            placed = li;
            break;
          }
        }
        if (placed < 0) {
          placed = laneEnds.length;
          laneEnds.push(e.to);
        } else {
          laneEnds[placed] = e.to;
        }
        positions.push({
          task: e.task,
          bi,
          lane: placed,
          from: e.from,
          to: e.to,
          kind: e.kind,
          roleColors: e.roleColors,
        });
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
        const h = isEmpty ? 0 : lanesPerBand[bi] * laneHeight + bandPad * 2 - laneGap;
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
    // running cursor stashed in _futureEndX. We also floor it at the
    // stage's visible width (measured after first paint) so the
    // band-bg stripes never end before the right edge of the viewport
    // when the backlog is sparse.
    const intrinsicWidth =
      Math.max(futureStartX + futureColumnWidth, this._futureEndX || futureStartX) + 16;
    const width = Math.max(intrinsicWidth, this._stageWidth || 0);

    // Build an index for dependency arrows.
    const posByTaskID = new Map(positions.map((p) => [p.task.id, p]));

    // When a dep endpoint is a feature that the user expanded, the
    // feature itself has no position (it disappears in favour of its
    // children). Redirect such endpoints to an extremal descendant
    // so the arrow still has somewhere to land. The source side
    // (precondition) snaps to the LAST visible descendant (max
    // `.to`); the target side (dependent) snaps to the FIRST visible
    // descendant (min `.from`).
    const gatherDescendantPositions = (id) => {
      const out = [];
      const visit = (nid) => {
        const direct = posByTaskID.get(nid);
        if (direct) {
          out.push(direct);
          return;
        }
        for (const cid of childrenByParent.get(nid) || []) visit(cid);
      };
      for (const cid of childrenByParent.get(id) || []) visit(cid);
      return out;
    };
    const resolveSourceEndpoint = (id) => {
      const direct = posByTaskID.get(id);
      if (direct) return direct;
      const candidates = gatherDescendantPositions(id);
      if (!candidates.length) return null;
      return candidates.reduce((best, p) => (p.to > best.to ? p : best), candidates[0]);
    };
    const resolveTargetEndpoint = (id) => {
      const direct = posByTaskID.get(id);
      if (direct) return direct;
      const candidates = gatherDescendantPositions(id);
      if (!candidates.length) return null;
      return candidates.reduce((best, p) => (p.from < best.from ? p : best), candidates[0]);
    };

    // Find tasks whose state is not 'done' but at least one task that
    // depends on them is already 'done'. That's a logical
    // inconsistency: the dependent completed before its precondition.
    // We surface it on the bar with a solid red border.
    const stateByID = new Map((this.tasks || []).map((t) => [t.id, t.state]));
    const dependentsByID = new Map();
    for (const d of this.deps) {
      // d.task_id depends on d.depends_on_id — so d.task_id is a dependent
      // (later task) of d.depends_on_id.
      if (!dependentsByID.has(d.depends_on_id)) dependentsByID.set(d.depends_on_id, []);
      dependentsByID.get(d.depends_on_id).push(d.task_id);
    }
    const inconsistentIDs = new Set();
    for (const t of this.tasks || []) {
      if (t.state === 'done') continue;
      for (const dependentID of dependentsByID.get(t.id) || []) {
        if (stateByID.get(dependentID) === 'done') {
          inconsistentIDs.add(t.id);
          break;
        }
      }
    }

    // Index members by user id so we can render the assignee avatar
    // inside the task box. A user appears multiple times in the
    // memberships list (once per role), so dedupe by UserID.
    const usersById = new Map();
    for (const m of this.members || []) {
      if (!usersById.has(m.user_id)) {
        usersById.set(m.user_id, {
          AvatarURL: m.avatar_url,
          DisplayName: m.display_name,
          GithubLogin: m.github_login,
        });
      }
    }

    // Band fill: a single lane background. Rhythm comes from a 1px
    // hairline between bands, not from zebra alternation (cleaner,
    // tableroom density à la GitHub). The features band still has
    // its own slightly darker fill so it reads as a parent row.
    const bandFill = displayBands.map((b) =>
      b.role.id === '__features__' ? 'var(--gantt-band-features)' : 'var(--gantt-band-1)',
    );

    // Selection-aware predicates: when nothing is selected every bar
    // and arrow renders at full strength. Otherwise the connected
    // set is bright and its arrows promote on top of the task rects.
    const hasSelection = this._selectedSet.size > 0;
    const isPromotedEdge = (d) =>
      hasSelection && (this._selectedSet.has(d.task_id) || this._selectedSet.has(d.depends_on_id));

    return html`
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      <div class="stage" @keydown=${(e) => this._onStageKey(e)}
           @click=${() => this._clearSelection()}>
        ${this._renderHoverCard()}
        <svg width=${width} height=${totalHeight}
             role="img"
             aria-label=${`Gantt chart with ${(this.tasks || []).length} tasks`}>
          <defs>
            <marker id="dep-arrowhead" viewBox="0 0 10 10" refX="0" refY="5"
                    markerWidth="8" markerHeight="8" orient="auto-start-reverse">
              <path d="M 0 0 L 10 5 L 0 10 z" style="fill: var(--gantt-arrow)"></path>
            </marker>
            <clipPath id="gantt-avatar-clip">
              <circle cx=${avatarSize / 2} cy=${avatarSize / 2} r=${avatarSize / 2}></circle>
            </clipPath>
          </defs>

          <!-- Zone labels. The NOW zone gets its own pill below,
               so the only labels needed here are PAST and FUTURE. -->
          <text class="zone-label" x=${labelWidth + 4} y="14">PAST · chronological</text>
          <text class="zone-label" x=${futureStartX + 4} y="14">FUTURE · topological + priority</text>

          <!-- Priority labels at the top of each future sub-column -->
          ${futurePriorityBuckets.map(
            (b) => svg`
            <text class="priority-label" x=${b.x + 4} y=${headerH - 4}>${this._priorityLabel(b.priority)}</text>
          `,
          )}

          <!-- Band rows (empty bands collapse to height 0 and skip
               render). No alternation: a single fill carries the lane;
               a hairline between consecutive visible bands gives the
               rhythm. Features keeps its own slightly darker class. -->
          ${(() => {
            this._visibleBandIdx = 0;
            return null;
          })()}
          ${displayBands.map((b, bi) => {
            if (bandHeights[bi] === 0) return null;
            const isFeatures = b.role.id === '__features__';
            const cls = isFeatures ? 'band-bg features' : 'band-bg';
            if (!isFeatures) this._visibleBandIdx++;
            const showSeparator = this._visibleBandIdx > 1 && !isFeatures;
            return svg`
              <rect class=${cls}
                    x="0" y=${bandTops[bi]}
                    width=${width} height=${bandHeights[bi]}></rect>
              ${
                showSeparator
                  ? svg`<line class="band-separator"
                            x1="0" y1=${bandTops[bi]}
                            x2=${width} y2=${bandTops[bi]}></line>`
                  : null
              }
              <text class="band-label"
                    x="8" y=${bandTops[bi] + bandHeights[bi] / 2 + 4}>
                ${isFeatures ? 'Features' : b.role.label}
              </text>
            `;
          })}

          <!-- Past-zone tint: a subtle wash painted on top of the
               band fills so done work reads as archive. The 4%
               opacity keeps the bars beneath legible. -->
          <rect class="zone-past-tint"
                x=${labelWidth} y="0"
                width=${Math.max(0, presentX - labelWidth)}
                height=${totalHeight}></rect>

          <!-- Zone dividers: thin solid hairlines (no dashed). The
               past-zone tint above already communicates the past/now
               border ambiently. -->
          <line class="zone-divider" x1=${labelWidth} y1="0" x2=${labelWidth} y2=${totalHeight}></line>
          <line class="zone-divider" x1=${presentX} y1="0" x2=${presentX} y2=${totalHeight}></line>
          <line class="zone-divider" x1=${futureStartX} y1="0" x2=${futureStartX} y2=${totalHeight}></line>

          <!-- "now" marker: the original thin red line, topped by a
               small NOW pill anchored to the time-axis row. The pill
               replaces the legacy "now" text label so the marquee
               reads at a glance. -->
          <line class="now-line"
                x1=${presentX + presentWidth / 2} y1=${headerH - 6}
                x2=${presentX + presentWidth / 2} y2=${totalHeight}>
            <title>${this.now ? this.now.toLocaleString() : ''}</title>
          </line>
          ${(() => {
            const pillW = 30;
            const pillH = 14;
            const pillX = presentX + presentWidth / 2 - pillW / 2;
            const pillY = 4;
            return svg`
              <rect class="now-pill-bg"
                    x=${pillX} y=${pillY}
                    width=${pillW} height=${pillH}
                    rx="3" ry="3"></rect>
              <text class="now-pill-text"
                    x=${presentX + presentWidth / 2}
                    y=${pillY + pillH / 2}
                    text-anchor="middle">
                <title>${this.now ? this.now.toLocaleString() : ''}</title>
                NOW
              </text>
            `;
          })()}

          <!-- Dependency arrows, FIRST PASS: rendered before the task
               rects so the bars paint on top of them. Selection-incident
               edges are deliberately SKIPPED here and re-rendered after
               the bars (promoted pass) so they read above any rect they
               cross. -->
          ${this.deps.map((d) => {
            if (isPromotedEdge(d)) return null;
            return this._renderArrowPath(
              d,
              resolveSourceEndpoint,
              resolveTargetEndpoint,
              taskCenterY,
              taskHeight,
              hasSelection,
            );
          })}

          <!-- Tasks placed in their lane -->
          ${positions.map((p) => {
            const t = p.task;
            const color = displayBands[p.bi].role.color || 'var(--fg-muted)';
            const y = taskY(p.bi, p.lane);
            const w = Math.max(8, p.to - p.from);
            const user = t.assignee_user_id ? usersById.get(t.assignee_user_id) : null;
            const avatarX = p.from + avatarPad;
            const avatarY = y + (taskHeight - avatarSize) / 2;
            const labelX = user ? avatarX + avatarSize + 6 : p.from + 8;
            const labelMaxChars = Math.max(6, Math.floor((p.to - labelX - 6) / 7));
            if (p.kind === 'feature-agg') {
              const childCount = (childrenByParent.get(t.id) || []).length;
              const dots = p.roleColors || [];
              const dotR = 4;
              const dotGap = 4;
              const dotPadRight = 10;
              // Reserve room for the dots so the label doesn't run under them.
              const dotsBlockW = dots.length * (dotR * 2) + Math.max(0, dots.length - 1) * dotGap;
              const dotsStartX = p.to - dotPadRight - dotsBlockW + dotR;
              const labelRoom = Math.max(6, Math.floor((dotsStartX - dotR - labelX - 6) / 7));
              const aggDimmed = hasSelection && !this._selectedSet.has(t.id);
              const aggAppeared = this._justAppeared?.has(t.id) ? ' just-appeared' : '';
              return svg`
                <rect class=${`task-rect feature-agg${aggDimmed ? ' dim' : ''}${hasSelection && this._selectedSet.has(t.id) ? ' promoted' : ''}${aggAppeared}`}
                      data-task-id=${t.id}
                      x=${p.from} y=${y}
                      width=${w} height=${taskHeight}
                      rx="6" ry="6"
                      stroke-dasharray="6 3"
                      style=${`cursor: pointer; fill: var(--gantt-bar-feature-fill); stroke: ${color}`}
                      @click=${(e) => {
                        e.stopPropagation();
                        this._toggleFold(t.id);
                      }}
                      @mouseenter=${(e) => this._onBarHover(e, t, p.from, y)}
                      @mousemove=${(e) => this._onBarMove(e, t, p.from, y)}
                      @mouseleave=${() => this._onBarLeave()}
                      @focus=${(e) => this._onBarHover(e, t, p.from, y)}
                      @blur=${() => this._onBarBlur()}
                      tabindex="0">
                </rect>
                <text class="task-label" x=${labelX} y=${y + taskHeight / 2 + 4}
                      style="pointer-events:none">
                  ▸ ${this._truncate(t.title, labelRoom)}
                </text>
                ${dots.map(
                  (c, i) => svg`
                  <circle cx=${dotsStartX + i * (dotR * 2 + dotGap)}
                          cy=${y + taskHeight / 2}
                          r=${dotR}
                          fill=${c}
                          stroke="rgba(0,0,0,0.18)" stroke-width="0.5"
                          style="pointer-events:none"></circle>
                `,
                )}
              `;
            }
            const isFeatureUnfolded = t.type === 'feature' && !this.foldedFeatures.has(t.id);
            // Childless features and (defensive) any feature reaching here render as normal.
            const taskDimmed = hasSelection && !this._selectedSet.has(t.id);
            const taskAppeared = this._justAppeared?.has(t.id) ? ' just-appeared' : '';
            // Role-derived paint. Tint = 14% role over white (a soft
            // pastel that lets multiple bars share a lane without
            // shouting). Ink = role colour darkened 75% toward black
            // so labels stay AAA-readable on top of the tint.
            // CSS color-mix is the runtime engine; `color` may be a
            // hex from the DB or a `var()` fallback — both work.
            const tint = `color-mix(in srgb, ${color} 14%, white)`;
            const ink = `color-mix(in srgb, ${color} 75%, black)`;
            // Done bars defer to the CSS class so the monochrome
            // paint survives any per-project role colour. Doing /
            // todo bars get the role tint inline.
            const barStyle = t.state === 'done' ? '' : `fill: ${tint}; stroke: ${color}`;
            const labelOnTint = t.state !== 'done';
            const labelStyle = labelOnTint
              ? `pointer-events: none; fill: ${ink}`
              : 'pointer-events: none';
            return svg`
              <rect class=${`task-rect ${t.state}${inconsistentIDs.has(t.id) ? ' inconsistent' : ''}${taskDimmed ? ' dim' : ''}${hasSelection && this._selectedSet.has(t.id) ? ' promoted' : ''}${taskAppeared}`}
                    data-task-id=${t.id}
                    x=${p.from} y=${y}
                    width=${w} height=${taskHeight}
                    rx="6" ry="6"
                    style=${barStyle}
                    @click=${(e) => {
                      e.stopPropagation();
                      this._onTaskClick(t);
                    }}
                    @dblclick=${(e) => {
                      e.stopPropagation();
                      this._emitSelect(t);
                    }}
                    @mouseenter=${(e) => this._onBarHover(e, t, p.from, y)}
                    @mousemove=${(e) => this._onBarMove(e, t, p.from, y)}
                    @mouseleave=${() => this._onBarLeave()}
                    @focus=${(e) => this._onBarHover(e, t, p.from, y)}
                    @blur=${() => this._onBarBlur()}
                    tabindex="0">
              </rect>
              ${
                user && user.avatar_url
                  ? svg`
                <g transform=${`translate(${avatarX}, ${avatarY})`} style="pointer-events:none">
                  <image href=${user.avatar_url}
                         width=${avatarSize} height=${avatarSize}
                         clip-path="url(#gantt-avatar-clip)"></image>
                  <circle cx=${avatarSize / 2} cy=${avatarSize / 2} r=${avatarSize / 2}
                          style="fill: none; stroke: var(--gray-3); stroke-width: 1"></circle>
                </g>
              `
                  : null
              }
              <text class=${`task-label${labelOnTint ? ' on-tint' : ''}${t.type === 'bug' ? ' bug' : ''}`}
                    x=${labelX} y=${y + taskHeight / 2 + 4}
                    style=${labelStyle}>
                ${t.type === 'bug' ? 'bug: ' : ''}${this._truncate(t.title, labelMaxChars - (t.type === 'bug' ? 5 : 0))}
              </text>
            `;
          })}

          <!-- Dependency arrows, SECOND PASS: only the arrows incident
               to the currently-selected subgraph, rendered after the
               task rects so they sit on top of every box they cross. -->
          ${this.deps.map((d) => {
            if (!isPromotedEdge(d)) return null;
            return this._renderArrowPath(
              d,
              resolveSourceEndpoint,
              resolveTargetEndpoint,
              taskCenterY,
              taskHeight,
              hasSelection,
            );
          })}
        </svg>
      </div>
      <div class="legend">
        <span><span class="swatch" style="background:var(--gantt-bar-done-fill);border:1px solid var(--gantt-bar-done-stroke)"></span> done</span>
        <span><span class="swatch" style="background:color-mix(in srgb, var(--brand-blue) 14%, white);border:1.5px solid var(--brand-blue)"></span> doing</span>
        <span><span class="swatch" style="background:color-mix(in srgb, var(--brand-blue) 14%, white);border:1.5px dashed var(--brand-blue)"></span> todo</span>
        <span><span class="bug-mark">bug:</span> italic title — bug</span>
        <span><span class="swatch" style="border:2px solid var(--danger)"></span> inconsistent — dependent already done</span>
        <span><span class="swatch" style="background:var(--gantt-now-line);width:2px"></span> now</span>
      </div>
    `;
  }

  // Compute the (from, to) X coordinates of a task on the timeline.
  //
  // Past tasks use their ordinal slot index (passed in as pastSlot).
  // Doing tasks are anchored to a fixed slot in the present zone.
  // Todo tasks are placed by their topological depth in the future
  // zone. Returns null when the task cannot be placed.
  taskX({
    t,
    labelWidth,
    pastSlotW,
    pastSlot,
    presentX,
    presentWidth,
    futureStartX,
    futureColumnWidth,
    futureX,
    depths,
    minBarWidth,
  }) {
    if (t.state === 'done') {
      if (pastSlot == null) return null; // no actual_end ⇒ not on past axis
      const from = labelWidth + pastSlot * pastSlotW + 6;
      return { from, to: from + minBarWidth };
    }
    if (t.state === 'doing') {
      const from = presentX + 12;
      return { from, to: from + minBarWidth };
    }
    // todo — futureX (sub-column X) computed by the priority grouping
    // pass. Fall back to depth-only if we somehow didn't precompute.
    let from = futureX;
    if (from == null) {
      const d = depths.get(t.id) || 0;
      from = futureStartX + d * futureColumnWidth + 12;
    }
    return { from, to: from + minBarWidth };
  }

  // Render one dependency arrow as an SVG <path>. Used in two passes
  // by the main render: the default (non-promoted) pass before the
  // task rects, and the promoted pass after, so selection-incident
  // arrows always land on top of every box they cross.
  _renderArrowPath(d, resolveSource, resolveTarget, taskCenterY, taskHeight, hasSelection) {
    const from = resolveSource(d.depends_on_id);
    const to = resolveTarget(d.task_id);
    if (!from || !to) return null;
    const markerW = 8;
    const promoted =
      hasSelection && (this._selectedSet.has(d.task_id) || this._selectedSet.has(d.depends_on_id));
    const dimmed = hasSelection && !promoted;
    const cls = `arrow${promoted ? ' promoted' : ''}${dimmed ? ' dim' : ''}`;
    const onClick = (e) => {
      e.stopPropagation();
      this._onArrowClick(d.depends_on_id);
    };
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
    this.dispatchEvent(
      new CustomEvent('task-selected', {
        detail: { task: t },
        bubbles: true,
        composed: true,
      }),
    );
  }

  _truncate(s, n) {
    if (!s) return '';
    return s.length > n ? s.slice(0, Math.max(0, n - 1)) + '…' : s;
  }

  // _onBarHover stashes the task plus the bar's top-left in SVG
  // (scrolled-content) coords. The bar anchor is the keyboard-focus
  // fallback when no cursor is available. For pointer hover the
  // popup follows the mouse via _onBarMove. If a cursor was already
  // tracked for this same task (mouseenter then click → focus), we
  // keep it so the popup doesn't jump back to the bar's top-left
  // just because the rect received keyboard focus.
  _onBarHover(_e, task, barX, barY) {
    const prior = this._hover;
    const cursor = prior && prior.task && prior.task.id === task.id ? prior.cursor : null;
    this._hover = { task, barX, barY, cursor };
    this._pointerBarID = task.id;
  }
  // Convert a pointer event into scrolled-content coords (the same
  // frame `position: absolute` children of `.stage` live in) and
  // stash them so the popup can track the cursor on mousemove.
  _onBarMove(e, task, barX, barY) {
    const stage = this.shadowRoot?.querySelector('.stage');
    if (!stage) return;
    const r = stage.getBoundingClientRect();
    const cursor = {
      x: e.clientX - r.left + stage.scrollLeft,
      y: e.clientY - r.top + stage.scrollTop,
    };
    this._hover = { task, barX, barY, cursor };
    this._pointerBarID = task.id;
  }
  // Pointer left the bar: drop the hover entirely.
  _onBarLeave() {
    this._pointerBarID = null;
    this._hover = null;
  }
  // Keyboard focus left a bar (also fires on click-induced focus
  // shift between two bars). Only clear when the pointer isn't
  // sitting on a bar — otherwise the new bar's mouseenter has
  // already populated `_hover` with a valid cursor and we don't
  // want a stale blur from the previous bar to wipe it.
  _onBarBlur() {
    if (this._pointerBarID == null) {
      this._hover = null;
    }
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
    const { task: t, barX, barY, cursor } = this._hover;
    const stage = this.shadowRoot?.querySelector('.stage');
    const stageW = stage?.clientWidth || 800;
    const scrollLeft = stage?.scrollLeft || 0;
    // The card is `position: absolute` inside `.stage` (which is
    // `position: relative` + `overflow: auto`), so its coordinates
    // are in the scrolled SVG content space, not the viewport.
    // Width is content-driven (min 220, max 320 in CSS) — we cache
    // the previous render's measured width in `_lastCardW` so the
    // flip math matches reality; first render falls back to the
    // min width so the gap can only ever come out smaller, not
    // wider, than intended.
    const cardW = this._lastCardW || 220;
    const cardH = this._lastCardH || 80;
    const stageH = stage?.clientHeight || 600;
    const scrollTop = stage?.scrollTop || 0;
    const anchorX = cursor ? cursor.x : barX;
    const anchorY = cursor ? cursor.y : barY;
    // Same numeric gap on both sides — the previous left-flip drift
    // was a width-estimate bug, not a perception issue.
    const offX = cursor ? 14 : 8;
    const offY = cursor ? 16 : -8;
    let left = anchorX + offX;
    let top = Math.max(8, anchorY + offY);
    // Flip to the left side of the anchor if the card would extend
    // past the visible viewport's right edge.
    const viewportRight = scrollLeft + stageW;
    if (left + cardW > viewportRight) {
      left = Math.max(scrollLeft + 8, anchorX - cardW - offX);
    }
    // Flip above the cursor if the card would extend past the
    // visible viewport's bottom edge — last-row tasks would
    // otherwise push the popup outside the stage. Symmetrical gap
    // so above-cursor placement feels the same as below.
    const viewportBottom = scrollTop + stageH;
    if (top + cardH > viewportBottom) {
      top = Math.max(scrollTop + 8, anchorY - cardH - (cursor ? 16 : 8));
    }

    const role = t.target_role_id ? this.roles.find((r) => r.id === t.target_role_id) : null;
    const user = t.assignee_user_id
      ? this.members.find((m) => m.user_id === t.assignee_user_id)
      : null;
    const isFolded = t.type === 'feature' && this.foldedFeatures.has(t.id);
    const childCount = isFolded
      ? (this.tasks || []).filter((x) => x.parent_task_id === t.id).length
      : 0;

    // Single template for every bar — features and tasks share the
    // same chrome (chip row + optional assignee). Nottario doesn't
    // model calendar dates for tasks (Gantt positions are by zone +
    // priority + topological depth, not dates), so the popup
    // intentionally omits any date row.
    return html`
      <div class="hover-card" role="tooltip"
           style=${`left:${left}px; top:${top}px`}>
        <div class="title">${t.title}</div>
        <div class="row">
          <span class=${`chip state-${t.state}`}>${t.state}</span>
          ${
            t.type === 'bug'
              ? html`<span class="chip type-bug">bug</span>`
              : t.type !== 'task'
                ? html`<span class="chip">${t.type}</span>`
                : null
          }
          <span class="chip">${this._priorityLabel(t.priority)}</span>
          ${
            role
              ? html`
            <span class="chip role" style=${(() => {
              const c = role.color || 'var(--fg-muted)';
              return `background: color-mix(in srgb, ${c} 18%, white); color: color-mix(in srgb, ${c} 70%, black)`;
            })()}>${role.label}</span>
          `
              : null
          }
        </div>
        ${
          user
            ? html`
          <div class="assignee">
            ${user.avatar_url ? html`<img src=${user.avatar_url} alt="">` : null}
            <span>${user.display_name || user.github_login}</span>
          </div>
        `
            : null
        }
        ${
          isFolded
            ? html`
          <div class="meta">${childCount} task${childCount === 1 ? '' : 's'} · ${this._featureRoles(t.id)}</div>
        `
            : null
        }
        <div class="meta hint">${isFolded ? 'Click to expand' : 'Double-click to open'}</div>
      </div>
    `;
  }
}

customElements.define('nottario-gantt', NottarioGantt);
