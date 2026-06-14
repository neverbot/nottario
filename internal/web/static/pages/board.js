import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { subscribe } from '/static/realtime.js';
import { EscController } from '/static/components/esc.js';
import { toast } from '/static/components/toast.js';
import { formButton } from '/static/components/form-button.js';
import { confirm } from '/static/components/confirm-dialog.js';
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
    // Filter chips above the columns. _filters.{mine, roles, types}.
    // roles/types are arrays so they survive Lit dirty-checks across
    // hash updates. Filter dropdowns track their own open state.
    _filters: { state: true },
    _filterOpen: { state: true },
    // Set when the user picks Advanced in the new-task dialog: only
    // then is the `feature` type selectable (features have different
    // semantics — parent roll-up — and the modal shouldn't expose
    // them as a casual option).
    _newTaskAdvanced: { state: true },
    // Confirmation dialog state for the in-app delete flow (replaces
    // the browser-native confirm()).
    _endSprintOpen: { state: true },
  };

  static styles = [
    buttonStyles,
    dialogStyles,
    formStyles,
    badgeStyles,
    css`
    :host { display: block; }
    .spacer { flex: 1; }

    /* ---- Cycle switcher (header cluster) ---- */
    .cycle-switcher {
      position: relative;
      display: inline-flex;
      align-items: center;
      gap: 10px;
      font-size: 12px;
      color: var(--fg-muted);
    }
    .cycle-switcher .pill {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      height: 28px;
      padding: 0 10px;
      border-radius: 999px;
      border: 1px solid var(--border);
      background: #fff;
      color: var(--fg);
      font: inherit;
      font-size: 12px;
      font-weight: 500;
      cursor: pointer;
    }
    .cycle-switcher .pill:hover { border-color: var(--border-strong); }
    .cycle-switcher .pill .caret { color: var(--fg-muted); font-size: 10px; }
    .cycle-switcher .pill .muted { color: var(--gray-5); font-weight: 400; }
    .cycle-dropdown {
      position: absolute;
      top: 32px;
      left: 0;
      z-index: 30;
      margin: 0;
      padding: 4px 0;
      list-style: none;
      background: #fff;
      border: 1px solid var(--border);
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
      color: var(--fg);
    }
    .cycle-dropdown li:hover { background: var(--bg-hover); }
    .cycle-dropdown li.current { font-weight: 600; background: var(--tint-blue); }
    .cycle-dropdown li .muted { color: var(--gray-5); font-size: 11px; }
    .cycle-counts {
      color: var(--fg-muted);
      font-size: 12px;
      white-space: nowrap;
    }
    .cycle-counts .sep { color: var(--border); margin: 0 6px; }

    /* ---- End-sprint dialog ---- */
    .end-sprint-dialog .panel { width: 480px; }
    .end-sprint-dialog ul {
      margin: 8px 0 16px;
      padding-left: 20px;
      font-size: 13px;
      color: var(--fg);
      line-height: 1.6;
    }
    .end-sprint-dialog ul li { margin-bottom: 2px; }

    /* Universal box-sizing — the project rule requires shadow roots
       to set this explicitly because the global reset does not
       penetrate the shadow boundary. */
    *, *::before, *::after { box-sizing: border-box; }

    /* ---- Filter row ---- */

    .filters {
      display: flex;
      align-items: center;
      flex-wrap: wrap;
      gap: 8px;
      margin-bottom: 12px;
    }
    .filter-chip {
      position: relative;
      display: inline-flex;
      align-items: center;
      gap: 6px;
      height: 26px;
      padding: 0 10px;
      border-radius: 999px;
      border: 1px solid var(--border);
      background: #fff;
      color: var(--fg);
      font: inherit;
      font-size: 12px;
      font-weight: 500;
      cursor: pointer;
    }
    .filter-chip:hover { border-color: var(--border-strong); }
    .filter-chip.active {
      background: var(--tint-blue);
      border-color: var(--accent);
      color: var(--tint-blue-fg);
    }
    .filter-chip .count {
      background: var(--accent);
      color: #fff;
      border-radius: 999px;
      padding: 1px 6px;
      font-size: 10px;
      font-weight: 600;
    }
    .filter-chip svg { width: 10px; height: 10px; }
    .filter-clear {
      background: transparent;
      border: 0;
      color: var(--fg-muted);
      font: inherit;
      font-size: 12px;
      cursor: pointer;
      padding: 4px 6px;
    }
    .filter-clear:hover { color: var(--fg); text-decoration: underline; }
    .filter-menu {
      position: absolute;
      top: calc(100% + 4px);
      left: 0;
      min-width: 180px;
      background: #fff;
      border: 1px solid var(--border);
      border-radius: 8px;
      box-shadow: 0 6px 16px rgba(31, 35, 40, 0.12);
      padding: 4px;
      z-index: 30;
    }
    .filter-menu label {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 6px 8px;
      border-radius: 4px;
      cursor: pointer;
      font-weight: 400;
    }
    .filter-menu label:hover { background: var(--bg-hover); }
    .filter-menu input[type="checkbox"] { margin: 0; }

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
      border: 1px solid var(--border);
      background: #fff;
      color: var(--fg-muted);
      font-size: 12px;
      font-weight: 500;
      cursor: pointer;
      font: inherit;
    }
    .doing-pill:hover { border-color: var(--border-strong); color: var(--fg); }
    .doing-pill .dot {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: var(--border-strong);
    }
    .col {
      background: var(--bg-subtle);
      border-radius: 8px;
      padding: 8px;
      align-self: start; /* don't stretch to match the tallest column */
    }
    .col.empty { padding: 6px 8px; }
    .col.empty.doing { padding: 8px; }
    .upnext {
      background: #fff;
      border: 1px dashed var(--border-strong);
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
      color: var(--fg-muted);
      font-weight: 600;
    }
    .upnext .title {
      font-weight: 600;
      font-size: 14px;
      color: var(--fg);
      line-height: 1.3;
    }
    .upnext .meta {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
      font-size: 12px;
      color: var(--fg-muted);
    }
    .upnext .row {
      display: flex;
      gap: 8px;
      align-items: center;
      margin-top: 4px;
    }
    .upnext .row .spacer { flex: 1; }
    .upnext button.start {
      background: var(--success);
      color: #fff;
      border: 1px solid rgba(31, 35, 40, 0.15);
      padding: 5px 12px;
      border-radius: 6px;
      font-weight: 500;
      font-size: 13px;
      cursor: pointer;
    }
    .upnext button.start:hover { background: var(--success-hover); }
    .upnext button.peek {
      background: transparent;
      color: var(--accent);
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
      color: var(--fg-muted);
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .col.empty h3 { margin-bottom: 4px; }
    .col .empty-note {
      font-size: 12px;
      color: var(--gray-5);
      padding: 0 4px 2px;
      font-style: italic;
    }
    .count {
      background: var(--gray-2);
      color: var(--fg-muted);
      border-radius: 2em;
      padding: 0 8px;
      font-size: 12px;
      font-weight: 500;
    }
    .card {
      position: relative;
      background: #fff;
      border: 1px solid var(--border);
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
    .card:hover { border-color: var(--border-strong); }
    /* DnD: the card the user is dragging fades; the column it's
       being dragged over picks up a subtle accent ring to confirm
       it will receive the drop. */
    .card[draggable="true"] { cursor: grab; }
    .card[draggable="true"]:active { cursor: grabbing; }
    .card.dragging { opacity: 0.4; }
    /* Won't-do cards live in the Done column alongside real done
       cards. Typography + opacity carry the meaning: title with a
       strikethrough, whole card dimmed to 0.55. No new colour, no
       new border style — the user's eye keeps the layout. Not
       draggable: refused transitions out of wont_do (wont_do → doing
       / wont_do → done) belong in the detail panel where the state
       buttons can disable themselves with a tooltip. */
    .card.wont-do {
      opacity: 0.55;
      cursor: pointer;
    }
    .card.wont-do .title {
      text-decoration: line-through;
      text-decoration-thickness: 1px;
    }
    .card.wont-do:hover {
      opacity: 0.85;
    }
    /* Suffix on the Done column header: "Done · 12 (3 won't do)". */
    .col h3 .wont-do-suffix {
      color: var(--fg-muted);
      font-weight: 400;
      font-size: 12px;
      margin-left: 6px;
    }
    .col.drag-over {
      outline: 2px solid var(--accent);
      outline-offset: -2px;
      background: var(--tint-blue);
    }
    .card .title { font-weight: 500; margin-bottom: 4px; }
    .card .meta {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
      font-size: 12px;
      color: var(--fg-muted);
    }
    /* Role badge: a flat-coloured pill whose hue follows the role's
       configured colour. Uses color-mix so the tint stays robust
       across short hex (#abc), long hex (#aabbcc) or named colours,
       which the previous string-concatenation approach silently
       broke on. */
    .card .role-badge,
    .upnext .role-badge {
      display: inline-flex;
      align-items: center;
      padding: 1px 8px;
      border-radius: 999px;
      border: 1px solid var(--role-color, var(--border));
      background: color-mix(in srgb, var(--role-color, var(--border)) 10%, #fff);
      font-size: 11px;
      font-weight: 500;
      color: var(--fg);
    }

    /* Priority is encoded as a coloured dot so it's the first thing
       the eye reads on a card — high → red, medium → amber, low →
       neutral. The bucket label sits next to the dot in muted text.
       Three tints reflect the three priority bands; max collapses
       into high, min into low. */
    .prio {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      font-family: inherit;
    }
    .prio .dot {
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: var(--border-strong);
    }
    .prio.high .dot { background: var(--danger); }
    .prio.medium .dot { background: var(--warning); }
    .prio.low .dot { background: var(--gray-5); }


    .error { color: var(--danger); margin-bottom: 8px; font-size: 13px; }

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
      border-bottom: 1px solid var(--gray-2);
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
      color: var(--fg);
      flex: 1;
      min-width: 0;
    }
    .detail .head .sub-line {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-top: 6px;
      font-size: 12px;
      color: var(--fg-muted);
    }
    .detail .head .short-id {
      font-family: ui-monospace, SFMono-Regular, monospace;
      color: var(--gray-5);
      font-size: 12px;
    }
    .detail .head .sub-line .dot { color: var(--border); }
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
      color: var(--gray-5);
      background: transparent;
      border: 1px solid transparent;
      border-radius: 6px;
      cursor: pointer;
      font: inherit;
    }
    .detail .head .icon-btn svg { display: block; }
    .detail .head .icon-btn:hover {
      color: var(--fg);
      background: var(--bg-subtle);
      border-color: var(--border);
    }
    .detail .head .icon-btn:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 1px;
    }
    .detail .head .icon-btn.danger:hover,
    .detail .head .icon-btn.danger:focus-visible {
      color: var(--danger);
      background: var(--tint-red);
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
      color: var(--fg-muted);
      align-items: center;
    }
    .detail .meta .field-line {
      display: inline-flex;
      align-items: center;
      gap: 6px;
    }
    .detail .meta .lbl { color: var(--gray-5); font-size: 11px; text-transform: uppercase; letter-spacing: 0.04em; font-weight: 600; }
    .detail .meta .val { color: var(--fg); }
    .detail .meta .val .muted { color: var(--gray-5); font-style: italic; font-weight: 400; }
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
      border: 1px solid var(--border);
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
      outline: 2px solid var(--accent);
      border-color: var(--accent);
    }

    /* State control as compact segmented pill — three buttons share a
       single rounded shell; the active one is the GitHub-green primary
       (matches the kanban "done" reading), the others stay neutral. */
    .detail .state-control {
      display: inline-flex;
      border: 1px solid var(--border);
      border-radius: 6px;
      overflow: hidden;
      background: var(--bg);
    }
    .detail .state-control button {
      padding: 4px 12px;
      font: inherit;
      font-size: 12px;
      background: transparent;
      border: none;
      border-right: 1px solid var(--border);
      cursor: pointer;
      color: var(--fg-muted);
    }
    .detail .state-control button:last-child { border-right: none; }
    .detail .state-control button:hover { background: var(--bg-subtle); color: var(--fg); }
    .detail .state-control button.active {
      background: var(--success);
      color: var(--bg);
      font-weight: 600;
    }
    .detail .state-control button.active:hover { background: var(--success-hover); }

    /* Priority dropdown — same chrome as the new-task dialog's
       select, narrow enough not to dominate the meta row. */
    .detail .meta select.priority {
      padding: 2px 22px 2px 8px;
      border: 1px solid var(--border);
      border-radius: 4px;
      font: inherit;
      font-size: 12px;
      background: var(--bg);
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
      color: var(--gray-5);
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
    }
    .detail .commits-list .commit {
      display: flex;
      flex-direction: column;
      gap: 2px;
      padding: 6px 10px;
      background: var(--bg-subtle);
      border: 1px solid var(--border);
      border-radius: 6px;
      color: var(--fg);
      text-decoration: none;
      font-size: 13px;
      transition: background-color 0.1s, border-color 0.1s;
    }
    .detail .commits-list a.commit:hover {
      background: var(--bg);
      border-color: var(--accent);
    }
    .detail .commits-list a.commit:hover .sha { text-decoration: underline; }
    .detail .commits-list .commit .top {
      display: flex;
      align-items: baseline;
      gap: 8px;
      min-width: 0;
    }
    .detail .commits-list .commit .sha {
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 12px;
      color: var(--accent);
      flex: 0 0 auto;
    }
    .detail .commits-list .commit .msg {
      flex: 1 1 auto;
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      color: var(--fg);
    }
    .detail .commits-list .commit .meta {
      font-size: 11px;
      color: var(--fg-muted);
      display: flex;
      gap: 6px;
      align-items: baseline;
    }
    .detail .commits-list .commit .meta .repo {
      font-family: ui-monospace, SFMono-Regular, monospace;
    }
    .detail .commits-list .commit .meta .sep { opacity: 0.5; }

    /* Comments thread — each row has a small leading avatar column. */
    .detail .comment {
      display: grid;
      grid-template-columns: 28px 1fr;
      gap: 10px;
      padding: 10px 0;
      border-top: 1px solid var(--gray-2);
    }
    .detail .comment:first-of-type { border-top: none; padding-top: 0; }
    .detail .comment .ava { padding-top: 1px; }
    .detail .comment .meta-line {
      display: flex;
      align-items: baseline;
      gap: 8px;
      font-size: 12px;
      color: var(--fg-muted);
      margin-bottom: 2px;
    }
    .detail .comment .meta-line .name { color: var(--fg); font-weight: 600; }
    .detail .comment .meta-line .when { color: var(--gray-5); }
    .detail .comment .meta-line .via {
      color: var(--fg-muted);
      font-style: italic;
      font-size: 11px;
    }
    .detail .comment .meta-line .via .sep { margin-right: 4px; opacity: 0.6; }
    .detail .comment .meta-line .via .token {
      font-style: normal;
      font-family: ui-monospace, SFMono-Regular, monospace;
    }

    .detail .add-comment {
      margin-top: 14px;
      padding-top: 14px;
      border-top: 1px solid var(--gray-2);
    }
    .detail .add-comment textarea {
      width: 100%;
      min-height: 64px;
      padding: 8px 10px;
      border: 1px solid var(--border);
      border-radius: 6px;
      font: inherit;
      font-size: 13px;
      line-height: 1.5;
      resize: vertical;
      background: var(--bg);
    }
    .detail .add-comment textarea:focus {
      outline: 2px solid var(--accent);
      outline-offset: 0;
      border-color: var(--accent);
    }
    .detail .add-comment .row {
      display: flex;
      justify-content: flex-end;
      gap: 8px;
      margin-top: 8px;
    }

    .detail .empty {
      font-size: 13px;
      color: var(--gray-5);
      font-style: italic;
    }
  `,
  ];

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
    this._filters = { mine: false, roles: [], types: [] };
    this._filterOpen = null;
    this._newTaskAdvanced = false;
    new EscController(this, (e) => this._onEsc(e));
  }

  // ---- Drag and drop between columns ----------------------------
  _onCardDragStart(e, t) {
    this._draggingID = t.id;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', t.id);
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
    const task = this.tasks.find((x) => x.id === id);
    if (!task || task.state === state) return;
    this._moveStateWithUndo(id, state, task.state);
  }

  _onEsc(e) {
    // Topmost first: the task detail panel sits over the create form
    // when both happen to be open. Stop propagation after closing so
    // an outer listener (topbar dropdown, etc.) doesn't also react.
    if (this._cycleDropdownOpen) {
      this._cycleDropdownOpen = false;
      e.stopPropagation();
      return;
    }
    if (this._endSprintOpen) {
      this._endSprintOpen = false;
      e.stopPropagation();
      return;
    }
    if (this.selected) {
      this.closeDetail();
      e.stopPropagation();
      return;
    }
    if (this.showCreate) {
      this.showCreate = false;
      e.stopPropagation();
      return;
    }
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
    // Filter chips are mirrored to the hash so deep links carry the
    // viewed slice. Comma-separated lists for multi-value chips
    // (role / type) and a "1" flag for mine.
    this._filters = {
      mine: h.get('mine') === '1',
      roles: (h.get('role') || '').split(',').filter(Boolean),
      types: (h.get('type') || '').split(',').filter(Boolean),
    };
    const taskId = h.get('task');
    if (!taskId) return;
    const t = (this.tasks || []).find((x) => x.id === taskId);
    if (t) this.open(t);
  }

  // Write the current filter state back to the URL hash so reloads
  // and deep links preserve the view. Keeps existing keys (cycle,
  // task) intact.
  _persistFilters() {
    const h = new URLSearchParams(window.location.hash.slice(1));
    const f = this._filters || {};
    if (f.mine) h.set('mine', '1');
    else h.delete('mine');
    if (f.roles?.length) h.set('role', f.roles.join(','));
    else h.delete('role');
    if (f.types?.length) h.set('type', f.types.join(','));
    else h.delete('type');
    const s = h.toString();
    const url = s ? `#${s}` : window.location.pathname + window.location.search;
    history.replaceState(null, '', url);
  }

  _toggleFilterMenu(kind) {
    this._filterOpen = this._filterOpen === kind ? null : kind;
  }

  _toggleMine() {
    this._filters = { ...this._filters, mine: !this._filters.mine };
    this._persistFilters();
  }

  _toggleFilterValue(kind, value) {
    const list = (this._filters[kind] || []).slice();
    const i = list.indexOf(value);
    if (i >= 0) list.splice(i, 1);
    else list.push(value);
    this._filters = { ...this._filters, [kind]: list };
    this._persistFilters();
  }

  _clearFilters() {
    this._filters = { mine: false, roles: [], types: [] };
    this._filterOpen = null;
    this._persistFilters();
  }

  _subscribe() {
    this._unsub?.();
    if (!this.projectId) return;
    this._unsub = subscribe(this.projectId, (ev) => {
      if (!ev.type) return;
      // 'realtime.reconnected' fires after EventSource recovers from a
      // disconnect — any events during the gap were lost, so reload.
      // task.comment.* affect only the open detail dialog; refreshing
      // the whole kanban for every comment edit would be wasteful.
      if (ev.type.startsWith('task.comment.')) {
        if (this.selected && ev.task_id === this.selected.task.id) {
          this.loadDetail(this.selected.task.id);
        }
      } else if (ev.type === 'realtime.reconnected' || ev.type.startsWith('task.')) {
        this.load();
        if (this.selected) this.loadDetail(this.selected.task.id);
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
      this.cycles = cr.ok ? (await cr.json()).cycles || [] : [];
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
    const taskByID = new Map(this.tasks.map((t) => [t.id, t]));
    const blocked = new Set();
    for (const d of this.deps || []) {
      const preID = d.depends_on_id || d.depends_on_id || d.depends_on_task_id;
      const tid = d.task_id || d.task_id;
      const pre = taskByID.get(preID);
      if (pre && pre.state !== 'done') blocked.add(tid);
    }
    const eligible = this.tasks
      .filter((t) => t.state === 'todo' && t.type !== 'feature' && !blocked.has(t.id))
      .sort((a, b) => b.priority - a.priority || new Date(a.created_at) - new Date(b.created_at));
    return eligible[0] || null;
  }

  roleByID(id) {
    return this.roles.find((r) => r.id === id);
  }

  _priorityLabel(value) {
    if (!this.priorities || !this.priorities.length) return `p${value}`;
    const exact = this.priorities.find((p) => p.value === value);
    if (exact) return exact.key;
    return `p${value}`;
  }

  // Find the priority bucket whose Value is closest to `value`. Used
  // to pre-select the dropdown when the stored priority happens to
  // land between buckets (e.g. someone set a raw integer via SQL or
  // the legacy number input).
  _nearestBucketKey(value) {
    if (!this.priorities || !this.priorities.length) return '';
    let best = this.priorities[0];
    let bestDiff = Math.abs(best.value - value);
    for (let i = 1; i < this.priorities.length; i++) {
      const d = Math.abs(this.priorities[i].value - value);
      if (d < bestDiff) {
        best = this.priorities[i];
        bestDiff = d;
      }
    }
    return best.key;
  }

  back() {
    window.nottarioNavigate('/');
  }

  _emptyCopy(state) {
    switch (state) {
      case 'todo':
        return 'Backlog clear.';
      case 'doing':
        return 'Nothing in progress.';
      case 'done':
        return 'No completed tasks yet.';
      default:
        return 'Empty.';
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
      const role = next.target_role_id ? this.roleByID(next.target_role_id) : null;
      return html`
        <div class="upnext">
          <div class="eyebrow">Up next</div>
          <div class="title">${next.title}</div>
          <div class="meta">
            <span class=${`prio ${this._priorityBucket(next.priority)}`}>
              <span class="dot"></span>
              ${this._priorityLabel(next.priority)}
            </span>
            ${
              role
                ? html`<span class="role-badge"
              style=${`--role-color:${role.color || 'var(--border)'}`}>${role.label}</span>`
                : ''
            }
          </div>
          <div class="row">
            <button class="btn primary" @click=${() => this.setState(next.id, 'doing')}>Start</button>
            <button class="btn ghost" @click=${() => this.open(next)}>Open</button>
            <div class="spacer"></div>
          </div>
        </div>
      `;
    }
    return html`<div class="empty-note">${this._emptyCopy(state)}</div>`;
  }

  byState(s) {
    // The Done column collects both `done` and `wont_do` cards: a
    // cancelled task is still closed, just closed by a different
    // decision. Real done first (sorted by actual_end desc), wont_do
    // after (also sorted desc). Other columns stay state-pure.
    let items;
    if (s === 'done') {
      items = this.tasks.filter((t) => t.state === 'done' || t.state === 'wont_do');
    } else {
      items = this.tasks.filter((t) => t.state === s);
    }
    items = this._applyFilters(items);
    if (s === 'done') {
      items.sort((a, b) => {
        // Primary: real done before wont_do.
        const aWont = a.state === 'wont_do' ? 1 : 0;
        const bWont = b.state === 'wont_do' ? 1 : 0;
        if (aWont !== bWont) return aWont - bWont;
        // Secondary: most recently finished at the top — fall back
        // to UpdatedAt when ActualEnd is null (legacy rows / manual
        // edits).
        const at = new Date(a.actual_end || a.updated_at).getTime();
        const bt = new Date(b.actual_end || b.updated_at).getTime();
        return bt - at;
      });
    }
    return items;
  }

  // Filter pipeline: mine → role any-of → type any-of. Centralised so
  // byState, the Up-next picker and any future column variant share
  // the exact same set.
  _applyFilters(items) {
    const f = this._filters || {};
    if (f.mine && this.me) {
      items = items.filter((t) => t.assignee_user_id === this.me.id);
    }
    if (f.roles && f.roles.length) {
      const set = new Set(f.roles);
      items = items.filter((t) => set.has(t.target_role_id));
    }
    if (f.types && f.types.length) {
      const set = new Set(f.types);
      items = items.filter((t) => set.has(t.type));
    }
    return items;
  }

  _filterCount() {
    const f = this._filters || {};
    return (f.mine ? 1 : 0) + (f.roles?.length || 0) + (f.types?.length || 0);
  }

  // Map a numeric priority to a coarse bucket (high / medium / low)
  // used for the card's coloured dot. The exact cutoffs follow the
  // default priority catalogue: max(100)/high(75)→high; medium(50)→
  // medium; low(25)/min(0)→low. Tasks created via SQL with off-bucket
  // values still land in the closest band.
  _priorityBucket(value) {
    if (value >= 65) return 'high';
    if (value >= 35) return 'medium';
    return 'low';
  }

  open(t) {
    this.selected = { task: t, deps: [], commits: [], comments: [] };
    this.loadDetail(t.id);
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

  closeDetail() {
    this.selected = null;
  }

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
    const t = this.tasks.find((x) => x.id === id);
    const label = t ? `"${t.title.slice(0, 32)}${t.title.length > 32 ? '…' : ''}"` : 'Task';
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${id}`, { method: 'DELETE' });
    if (r.ok) {
      this.selected = null;
      await this.load();
      toast.success(`${label} deleted.`);
    } else {
      this.error = (await r.json()).error || 'failed';
      toast.error(`Couldn't delete: ${this.error}`);
    }
  }

  async createTask(e) {
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
      await formButton(e, async () => {
        const r = await fetch(`/api/projects/${this.projectId}/tasks`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        });
        if (!r.ok) throw new Error((await r.json()).error || 'failed');
        this.showCreate = false;
        await this.load();
      });
      toast.success('Task created.');
    } catch (err) {
      this.error = err.message;
      toast.error(`Couldn't create task: ${err.message}`);
    }
  }

  async addComment(taskID, body) {
    if (!body.trim()) return;
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${taskID}/comments`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ body }),
    });
    if (r.ok) {
      await this.loadDetail(taskID);
      toast.success('Comment added.');
    } else {
      toast.error("Couldn't add comment.");
    }
  }

  // value is a priority bucket key (e.g. 'medium', 'high'). Resolve
  // it to the integer value the REST API expects via the cached
  // priorities catalogue.
  async setPriority(taskID, key) {
    const bucket = (this.priorities || []).find((p) => p.key === key);
    if (!bucket) return;
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${taskID}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ priority: bucket.value }),
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
    const role = t.target_role_id ? this.roleByID(t.target_role_id) : null;
    const assignee = t.assignee_user_id ? this._memberByID(t.assignee_user_id) : null;
    const assigneeName = assignee ? assignee.display_name || assignee.github_login || '' : '';
    const a11yLabel =
      `${t.title}, ${t.type}, ${t.state}` +
      (role ? `, role ${role.label}` : '') +
      `, priority ${this._priorityLabel(t.priority)}` +
      (assigneeName ? `, assigned to ${assigneeName}` : '');
    const dragging = this._draggingID === t.id;
    const isWontDo = t.state === 'wont_do';
    // Cancelled cards aren't draggable: every drop-target column
    // (todo / doing) is a refused transition out of wont_do. To
    // re-open, the user goes through the detail panel where the
    // state buttons can explain why.
    return html`
      <div class=${`card${dragging ? ' dragging' : ''}${isWontDo ? ' wont-do' : ''}`}
           role="button"
           tabindex="0"
           draggable=${isWontDo ? 'false' : 'true'}
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
        <div class="title">${t.title}</div>
        <div class="meta">
          <span class=${`prio ${this._priorityBucket(t.priority)}`}>
            <span class="dot"></span>
            ${this._priorityLabel(t.priority)}
          </span>
          ${
            role
              ? html`<span class="role-badge"
            style=${`--role-color:${role.color || 'var(--border)'}`}>${role.label}</span>`
              : ''
          }
        </div>
        ${
          assignee
            ? html`
          <nottario-avatar class="assignee" size="20"
                          src=${assignee.avatar_url || ''}
                          name=${assigneeName}
                          title=${assigneeName}></nottario-avatar>
        `
            : null
        }
      </div>
    `;
  }

  // ---- Cycle helpers ------------------------------------------------

  // The cycle currently being viewed: explicit selection takes priority;
  // otherwise we follow whichever cycle is active (closed_at = null).
  _currentCycle() {
    const list = this.cycles || [];
    if (this.cycleId) return list.find((c) => c.id === this.cycleId) || null;
    return list.find((c) => !c.closed_at) || null;
  }

  _toggleCycleDropdown() {
    this._cycleDropdownOpen = !this._cycleDropdownOpen;
  }

  _selectCycle(id) {
    this._cycleDropdownOpen = false;
    // Follow the active cycle when picking it (clean URL); otherwise
    // record the explicit selection in the hash so a refresh preserves
    // the view.
    const active = (this.cycles || []).find((c) => !c.closed_at);
    if (active && active.id === id) {
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
    const done = tasks.filter((t) => t.state === 'done').length;
    const doing = tasks.filter((t) => t.state === 'doing').length;
    const todo = tasks.filter((t) => t.state === 'todo').length;
    const wontDo = tasks.filter((t) => t.state === 'wont_do').length;
    const total = tasks.length;
    const wontDoSuffix = wontDo > 0 ? ` · ${wontDo} won't do` : '';
    return `${done}/${total} done · ${doing} doing · ${todo} todo${wontDoSuffix}`;
  }

  // Can the current caller end the viewed sprint? Owner or instance
  // admin. The button is also hidden when the viewed cycle is closed.
  _canEndSprint() {
    if (!this.me || !this.project) return false;
    if (this.me.is_admin) return true;
    return this.project.owner_user_id === this.me.id;
  }

  renderCycleSwitcher() {
    const current = this._currentCycle();
    if (!current) return null;
    const list = this.cycles || [];
    return html`
      <div slot="actions" class="cycle-switcher">
        <button class="pill"
                aria-haspopup="listbox"
                aria-expanded=${this._cycleDropdownOpen ? 'true' : 'false'}
                @click=${() => this._toggleCycleDropdown()}>
          ${current.name}
          ${!current.closed_at ? html`<span class="muted">(active)</span>` : html`<span class="muted">(closed)</span>`}
          <span class="caret">▾</span>
        </button>
        ${
          this._cycleDropdownOpen
            ? html`
          <ul class="cycle-dropdown" role="listbox">
            ${list.map(
              (c) => html`
              <li role="option"
                  aria-selected=${c.id === current.id ? 'true' : 'false'}
                  class=${c.id === current.id ? 'current' : ''}
                  @click=${() => this._selectCycle(c.id)}>
                <span>${c.name}</span>
                ${
                  c.closed_at
                    ? html`<span class="muted">closed ${this._relTime(c.closed_at)}</span>`
                    : html`<span class="muted">active</span>`
                }
              </li>
            `,
            )}
          </ul>`
            : null
        }
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
    const features = tasks.filter((t) => t.type === 'feature' && t.state !== 'done');
    const partialFeatureIDs = new Set(features.map((f) => f.id));
    let partialFeatureDoneChildren = 0;
    for (const t of tasks) {
      if (t.parent_task_id && partialFeatureIDs.has(t.parent_task_id) && t.state === 'done') {
        partialFeatureDoneChildren++;
      }
    }
    return {
      doing: tasks.filter((t) => t.state === 'doing').length,
      // Top-level todo only — children of a partial feature move with
      // their parent and shouldn't be double-counted.
      todo: tasks.filter((t) => t.state === 'todo' && !t.parent_task_id).length,
      partialFeatures: features.length,
      partialFeatureDoneChildren,
      standaloneDone: tasks.filter((t) => t.state === 'done' && !t.parent_task_id).length,
    };
  }

  // "sprint-3" → "sprint-4"; falls back to "<name>-next" when there is
  // no trailing number to increment.
  _defaultNextName() {
    const current = this._currentCycle();
    if (!current) return '';
    const m = current.name.match(/^(.*?)(\d+)$/);
    if (m) return m[1] + (parseInt(m[2], 10) + 1);
    return `${current.name}-next`;
  }

  _openEndSprint() {
    this._endSprintOpen = true;
  }

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
           @click=${(e) => {
             if (e.target.classList.contains('dialog')) this._endSprintOpen = false;
           }}
           @keydown=${(e) => {
             if (e.key === 'Escape') this._endSprintOpen = false;
           }}>
        <div class="panel">
          <h3 id="end-sprint-title">End ${current.name}</h3>
          <nottario-field label="Next sprint name">
            <input name="next_name" .value=${defaultName}>
          </nottario-field>
          <p>This will:</p>
          <ul>
            <li>Close <strong>${current.name}</strong> (irreversible).</li>
            <li>Move ${counts.doing} doing + ${counts.todo} todo tasks forward.</li>
            <li>Re-stamp ${counts.partialFeatures} partial features
              (incl. ${counts.partialFeatureDoneChildren} done children).</li>
            <li>Leave ${counts.standaloneDone} standalone done tasks in ${current.name}.</li>
          </ul>
          <div class="actions-row">
            <button class="btn secondary"
                    @click=${() => (this._endSprintOpen = false)}>Cancel</button>
            <button class="btn danger"
                    @click=${() => this._confirmEndSprint()}>End ${current.name}</button>
          </div>
        </div>
      </div>
    `;
  }

  render() {
    if (!this.project) return html`<p>Loading…</p>`;
    const current = this._currentCycle();
    const viewingActive = current && !current.closed_at;
    const stateLabels = { todo: 'To do', doing: 'In progress', done: 'Done' };
    return html`
      <nottario-page-header
        .title=${this.view === 'gantt' ? 'Gantt' : 'Board'}>
        ${this.renderCycleSwitcher()}
        ${
          this.view === 'gantt'
            ? html`<button slot="actions" class="btn ghost"
                         title="Scroll the Gantt back to the now line"
                         @click=${() => this.renderRoot.querySelector('nottario-gantt')?.scrollToNow()}>↻ Now</button>`
            : null
        }
        ${
          viewingActive && this._canEndSprint()
            ? html`<button slot="actions" class="btn danger"
                         title="Close this cycle and open the next"
                         @click=${() => this._openEndSprint()}>End ${current.name}</button>`
            : null
        }
        <button slot="actions" class="btn primary"
                @click=${() => (this.showCreate = true)}>New task</button>
      </nottario-page-header>
      ${this.error ? html`<div class="error">${this.error}</div>` : null}
      ${this.view === 'kanban' ? this._renderFilters() : null}
      ${
        this.view === 'gantt'
          ? html`<nottario-gantt
                  .projectId=${this.projectId}
                  .cycleId=${this.cycleId || ''}
                  @task-selected=${(e) => this.open(e.detail.task)}></nottario-gantt>`
          : html`
          <div class="columns">
            ${['todo', 'doing', 'done'].map((s) => {
              const items = this.byState(s);
              const isEmpty = items.length === 0;
              const dragOver = this._dragOverState === s && this._draggingID;
              const draggingFromThis =
                this._draggingID && this.tasks.find((x) => x.id === this._draggingID)?.state === s;
              const cls = `col${isEmpty ? ' empty' : ''}${dragOver && !draggingFromThis ? ' drag-over' : ''}`;
              return html`
                <section class=${cls}
                         role="region"
                         aria-label=${`${stateLabels[s]} (${items.length})`}
                         @dragover=${(e) => this._onColDragOver(e, s)}
                         @dragleave=${(e) => this._onColDragLeave(e, s)}
                         @drop=${(e) => this._onColDrop(e, s)}>
                  <h3>${stateLabels[s]} <span class="count">${
                    s === 'done' ? items.filter((t) => t.state === 'done').length : items.length
                  }</span>${(() => {
                    if (s !== 'done') return '';
                    const wontDoCount = items.filter((t) => t.state === 'wont_do').length;
                    return wontDoCount > 0
                      ? html`<span class="wont-do-suffix"
                                  title="${wontDoCount} task${wontDoCount === 1 ? '' : 's'} marked won't do — closed without being done">(${wontDoCount} won't do)</span>`
                      : '';
                  })()}</h3>
                  ${isEmpty ? this._renderEmptyBody(s) : items.map((t) => this.renderCard(t))}
                </section>
              `;
            })}
          </div>
        `
      }
      ${this.showCreate ? this.renderCreate() : null}
      ${this.selected ? this.renderDetail() : null}
      ${this.renderEndSprintDialog()}
    `;
  }

  _renderFilters() {
    const f = this._filters || {};
    const total = this._filterCount();
    return html`
      <div class="filters" @click=${(e) => e.stopPropagation()}>
        ${
          this.me
            ? html`
          <button class=${`filter-chip${f.mine ? ' active' : ''}`}
                  @click=${() => this._toggleMine()}>Mine</button>
        `
            : null
        }
        <div style="position:relative">
          <button class=${`filter-chip${f.roles?.length ? ' active' : ''}`}
                  @click=${() => this._toggleFilterMenu('roles')}>
            Role
            ${f.roles?.length ? html`<span class="count">${f.roles.length}</span>` : null}
            <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3 5l5 5 5-5" fill="none" stroke="currentColor" stroke-width="2"/></svg>
          </button>
          ${
            this._filterOpen === 'roles'
              ? html`
            <div class="filter-menu">
              ${this.roles.map(
                (r) => html`
                <label>
                  <input type="checkbox"
                         ?checked=${f.roles?.includes(r.id)}
                         @change=${() => this._toggleFilterValue('roles', r.id)}>
                  ${r.label}
                </label>
              `,
              )}
            </div>
          `
              : null
          }
        </div>
        <div style="position:relative">
          <button class=${`filter-chip${f.types?.length ? ' active' : ''}`}
                  @click=${() => this._toggleFilterMenu('types')}>
            Type
            ${f.types?.length ? html`<span class="count">${f.types.length}</span>` : null}
            <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M3 5l5 5 5-5" fill="none" stroke="currentColor" stroke-width="2"/></svg>
          </button>
          ${
            this._filterOpen === 'types'
              ? html`
            <div class="filter-menu">
              ${['task', 'bug', 'chore', 'spike', 'feature'].map(
                (t) => html`
                <label>
                  <input type="checkbox"
                         ?checked=${f.types?.includes(t)}
                         @change=${() => this._toggleFilterValue('types', t)}>
                  ${t}
                </label>
              `,
              )}
            </div>
          `
              : null
          }
        </div>
        ${
          total > 0
            ? html`
          <button class="filter-clear" @click=${() => this._clearFilters()}>Clear</button>
        `
            : null
        }
      </div>
    `;
  }

  // Drop a task to a new state with a transient toast that lets the
  // user undo within 6 seconds. The toast replaces the immediate
  // mutation-without-recovery that the old drag-drop had.
  _moveStateWithUndo(taskID, newState, oldState) {
    const t = this.tasks.find((x) => x.id === taskID);
    const label = t ? `"${t.title.slice(0, 32)}${t.title.length > 32 ? '…' : ''}"` : 'task';
    this.setState(taskID, newState);
    const labels = { todo: 'To do', doing: 'In progress', done: 'Done', wont_do: "Won't do" };
    toast.show(`Moved ${label} to ${labels[newState] || newState}`, {
      duration: 6000,
      undo: () => this.setState(taskID, oldState),
    });
  }

  renderCreate() {
    return html`
      <div class="dialog"
           role="dialog"
           aria-modal="true"
           aria-labelledby="new-task-title"
           @click=${(e) => {
             if (e.target.classList.contains('dialog')) this.showCreate = false;
           }}
           @keydown=${(e) => {
             if (e.key === 'Escape') this.showCreate = false;
           }}>
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
                  ${this._newTaskAdvanced ? html`<option value="feature">feature</option>` : null}
                </select>
              </nottario-field>
              <nottario-field label="Priority" style="flex:1">
                <select name="priority_key">
                  ${[...this.priorities]
                    .sort((a, b) => b.value - a.value)
                    .map(
                      (p) =>
                        html`<option value=${p.key} ?selected=${p.key === 'medium'}>${p.key} (${p.value})</option>`,
                    )}
                </select>
              </nottario-field>
              <nottario-field label="Target role" style="flex:1">
                <select name="target_role_id">
                  <option value="">— none —</option>
                  ${this.roles.map((r) => html`<option value=${r.id}>${r.label}</option>`)}
                </select>
              </nottario-field>
            </div>
            <nottario-field label="Assignee" hint="optional">
              <select name="assignee_user_id">
                <option value="">— none —</option>
                ${[...new Map((this.members || []).map((m) => [m.user_id, m])).values()].map(
                  (m) =>
                    html`<option value=${m.user_id}>${m.display_name || m.github_login}</option>`,
                )}
              </select>
            </nottario-field>
            <div class="actions-row">
              <label style="display:inline-flex;align-items:center;gap:6px;font-size:12px;color:var(--fg-muted);cursor:pointer;margin-right:auto">
                <input type="checkbox"
                       ?checked=${this._newTaskAdvanced}
                       @change=${(e) => (this._newTaskAdvanced = e.target.checked)}>
                Advanced (enables feature type)
              </label>
              <button type="button" class="btn secondary" @click=${() => (this.showCreate = false)}>Cancel</button>
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
    return (this.members || []).find((m) => m.user_id === uid) || null;
  }

  _taskByID(id) {
    return (this.tasks || []).find((t) => t.id === id) || null;
  }

  _renderCommit(c) {
    const sha = (c.sha || '').trim();
    const repo = (c.repo || '').trim();
    const shortSha = sha.slice(0, 7);
    // Only link when repo looks like a clean GitHub `owner/repo` —
    // anything else (extra slashes, protocol prefix, empty repo) falls
    // back to a plain row so we never inject untrusted text into a URL.
    const repoOK = /^[A-Za-z0-9._-]+\/[A-Za-z0-9._-]+$/.test(repo);
    const url = repoOK && sha ? `https://github.com/${repo}/commit/${sha}` : null;
    const when = c.added_at ? this._commitRelTime(c.added_at) : '';
    const meta = html`
      <div class="meta">
        ${repo ? html`<span class="repo">${repo}</span>` : null}
        ${repo && when ? html`<span class="sep">·</span>` : null}
        ${when ? html`<span class="when" title=${new Date(c.added_at).toLocaleString()}>${when}</span>` : null}
      </div>
    `;
    const inner = html`
      <div class="top">
        <span class="sha">${shortSha}</span>
        ${c.message ? html`<span class="msg" title=${c.message}>${c.message}</span>` : null}
      </div>
      ${repo || when ? meta : null}
    `;
    if (url) {
      return html`<a class="commit" href=${url} target="_blank" rel="noopener noreferrer">${inner}</a>`;
    }
    return html`<div class="commit">${inner}</div>`;
  }

  // Tiny relative-time formatter for commit added_at. Same shape as
  // the one on /projects: "5m ago", "3h ago", "2d ago", "3w ago",
  // falls back to a locale date past ~12 weeks.
  _commitRelTime(iso) {
    const then = new Date(iso).getTime();
    if (!Number.isFinite(then)) return '';
    const diff = Date.now() - then;
    if (diff < 60_000) return 'just now';
    const m = Math.floor(diff / 60_000);
    if (m < 60) return `${m}m ago`;
    const h = Math.floor(m / 60);
    if (h < 24) return `${h}h ago`;
    const d = Math.floor(h / 24);
    if (d < 7) return `${d}d ago`;
    const w = Math.floor(d / 7);
    if (w < 12) return `${w}w ago`;
    return new Date(iso).toLocaleDateString();
  }

  _relTime(iso) {
    if (!iso) return '';
    const d = new Date(iso);
    const diff = (Date.now() - d.getTime()) / 1000;
    if (diff < 60) return 'just now';
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    if (diff < 86400 * 30) return `${Math.floor(diff / 86400)}d ago`;
    return d.toLocaleDateString();
  }

  renderDetail() {
    const { task, deps, commits, comments } = this.selected;
    const role = task.target_role_id ? this.roleByID(task.target_role_id) : null;
    const assignee = this._memberByID(task.assignee_user_id);
    const shortID = (task.id || '').slice(0, 7);
    return html`
      <div class="dialog"
           role="dialog"
           aria-modal="true"
           aria-labelledby="task-dialog-title"
           @click=${(e) => e.target.classList.contains('dialog') && this.closeDetail()}
           @keydown=${(e) => {
             if (e.key === 'Escape') this.closeDetail();
           }}>
        <div class="panel detail">
          <header class="head">
            <div class="title-row">
              <h3 id="task-dialog-title">${task.title}</h3>
              <div class="title-actions">
                <button class="icon-btn danger" title="Delete task" aria-label="Delete task"
                        @click=${async () => {
                          const ok = await confirm({
                            title: 'Delete this task?',
                            body: 'The card and all of its comments will be removed. This cannot be undone.',
                            confirmLabel: 'Delete',
                            danger: true,
                          });
                          if (ok) this.deleteTask(task.id);
                        }}>
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
              <span class="badge ${task.type}">${task.type}</span>
              <span class="dot">·</span>
              <span class="short-id">#${shortID}</span>
            </div>

            <div class="meta">
              <div class="field-line">
                <span class="lbl">State</span>
                <div class="state-control">
                  ${['todo', 'doing', 'done', 'wont_do'].map((s) => {
                    // Lifecycle rules: done ↔ wont_do is refused on
                    // the backend. Disable those buttons client-side
                    // with a tooltip so the user doesn't have to read
                    // an error to understand the lifecycle.
                    const refused =
                      (task.state === 'done' && s === 'wont_do') ||
                      (task.state === 'wont_do' && s === 'done');
                    const label = s === 'wont_do' ? "won't do" : s;
                    const title = refused
                      ? `${task.state} → ${s.replace('_', ' ')} is not allowed; re-open via "todo" first to revisit the decision`
                      : `Set state to ${label}`;
                    return html`
                      <button class=${task.state === s ? 'active' : ''}
                              ?disabled=${refused}
                              title=${title}
                              @click=${() => this.setState(task.id, s)}>${label}</button>
                    `;
                  })}
                </div>
              </div>

              <div class="field-line">
                <span class="lbl">Priority</span>
                <select class="priority"
                        @change=${(e) => this.setPriority(task.id, e.target.value)}>
                  ${[...this.priorities]
                    .sort((a, b) => b.value - a.value)
                    .map(
                      (p) => html`
                    <option value=${p.key}
                            ?selected=${p.key === this._nearestBucketKey(task.priority)}>
                      ${p.key} (${p.value})
                    </option>
                  `,
                    )}
                </select>
              </div>

              <div class="field-line">
                <span class="lbl">Role</span>
                <span class="val">${role ? role.label : html`<span class="muted">none</span>`}</span>
              </div>

              <div class="field-line">
                <span class="lbl">Assignee</span>
                <span class="val assignee-edit">
                  ${
                    assignee && assignee.avatar_url
                      ? html`<nottario-avatar size="20"
                              src=${assignee.avatar_url}
                              name=${assignee.display_name || assignee.github_login || ''}></nottario-avatar>`
                      : null
                  }
                  <select @change=${(e) => this.setAssignee(task.id, e.target.value)}>
                    <option value="" ?selected=${!task.assignee_user_id}>— unassigned —</option>
                    ${[...new Map((this.members || []).map((m) => [m.user_id, m])).values()].map(
                      (m) => html`
                        <option value=${m.user_id} ?selected=${m.user_id === task.assignee_user_id}>
                          ${m.display_name || m.github_login}
                        </option>
                      `,
                    )}
                  </select>
                </span>
              </div>
            </div>
          </header>

          <div class="body">
            ${
              task.description
                ? html`
              <section>
                <nottario-markdown
                  project-id=${this.projectId}
                  .source=${task.description}></nottario-markdown>
              </section>
            `
                : null
            }

            ${
              deps.length
                ? html`
              <section>
                <h4 class="eyebrow">Depends on</h4>
                <div class="deps-list">
                  ${deps.map(
                    (id) => html`
                    <nottario-task-chip
                      project-id=${this.projectId}
                      .task=${this._taskByID(id) || { ID: id, Title: id.slice(0, 8) + ' (not loaded)' }}>
                    </nottario-task-chip>
                  `,
                  )}
                </div>
              </section>
            `
                : null
            }

            <section>
              <h4 class="eyebrow">Commits</h4>
              ${
                commits.length === 0
                  ? html`<p class="empty">No commits linked.</p>`
                  : html`
                  <div class="commits-list">
                    ${commits.map((c) => this._renderCommit(c))}
                  </div>
                `
              }
            </section>

            <section>
              <h4 class="eyebrow">Comments</h4>
              ${
                comments.length === 0
                  ? html`<p class="empty">No comments yet.</p>`
                  : comments.map((c) => {
                      const author = this._memberByID(c.author_user_id);
                      return html`
                    <div class="comment">
                      <div class="ava">
                        <nottario-avatar size="24"
                          src=${author?.avatar_url || ''}
                          name=${author?.display_name || author?.github_login || 'agent'}
                          .agent=${c.via_mcp || null}></nottario-avatar>
                      </div>
                      <div>
                        <div class="meta-line">
                          <span class="name">${author?.display_name || author?.github_login || 'agent'}</span>
                          ${
                            c.via_mcp
                              ? html`<span class="via"><span class="sep">·</span>via <span class="token">${c.via_mcp.name || 'MCP'}</span></span>`
                              : null
                          }
                          <span class="when">${this._relTime(c.created_at)}</span>
                        </div>
                        <nottario-markdown
                          project-id=${this.projectId}
                          .source=${c.body || ''}></nottario-markdown>
                      </div>
                    </div>
                  `;
                    })
              }

              <form class="add-comment"
                    @submit=${(e) => {
                      e.preventDefault();
                      const t = e.target.body;
                      this.addComment(task.id, t.value);
                      t.value = '';
                    }}>
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
