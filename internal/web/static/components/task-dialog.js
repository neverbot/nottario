import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { repeat } from '/static/vendor/lit/directives/repeat.js';
import { subscribe } from '/static/realtime.js';
import { EscController } from '/static/components/esc.js';
import { formatDateTime, formatRelativeTime } from '/static/time.js';
import { toast } from '/static/components/toast.js';
import { confirm } from '/static/components/confirm-dialog.js';
import { buttonStyles } from '/static/components/buttons.js';
import { dialogStyles } from '/static/components/surfaces.js';
import { formStyles, selectStyles } from '/static/components/forms.js';
import { badgeStyles } from '/static/components/badges.js';
import '/static/components/markdown.js';
import '/static/components/md-editor.js';
import '/static/components/avatar.js';
import '/static/components/task-chip.js';
import '/static/components/task-steps.js';
import '/static/components/question-card.js';
import { closeIcon, trashIcon } from '/static/components/icons.js';

// <nottario-task-dialog .me=${me} .projectId=${pid} .taskId=${tid}>
//
// The task detail dialog, available over every page. The app shell
// mounts one instance and drives it from the URL hash (#task=<id>,
// plus &project=<pid> on pages outside the task's project; see
// utils/task-link.js). With no taskId it renders nothing.
//
// Everything the board dialog did lives here: inline edit of title,
// description and role, state / priority / assignee, steps, questions,
// related tasks, commits (with unlink), comments (add, edit, delete),
// the created-by line and the edited markers.
//
// Events (both bubble and are composed):
//   task-dialog-close  the user asked to close it (Esc, backdrop, the
//                      close button, or after deleting the task). The
//                      shell owns the URL, so it does the closing.
//   task-changed       {task, projectId, deleted?} after any write made
//                      from the dialog, so the page underneath can
//                      refresh its own data.
//
// While open it subscribes to its project's realtime stream and
// reloads itself on task / question / comment events (coalesced).

// Empty related block: what the dialog shows until the detail arrives.
const NO_RELATED = Object.freeze({ parent: null, children: [], depends_on: [], blocks: [] });

class NottarioTaskDialog extends LitElement {
  static properties = {
    me: { type: Object },
    projectId: { type: String },
    taskId: { type: String },
    // The open task's detail: { task, description_html, related,
    // steps, stepsAt, questions, commits, comments }.
    selected: { state: true },
    roles: { state: true },
    members: { state: true },
    priorities: { state: true },
    // Inline edit state. _edit.field is 'title' | 'desc' | null. The
    // drafts hold the user's in-progress buffer so a 409 stale doesn't
    // lose their work. Comment-level edit state lives in
    // _commentEditID (the comment being edited), _commentDrafts (a
    // {id: body} map), _commentSavingID, _commentDeletingID.
    // Commit rows carry the same kind of state keyed by "repo\nsha",
    // since a link has no id of its own: _commitUnlinkKey is the row
    // asking for confirmation, _commitUnlinkBusyKey the one whose
    // DELETE is in flight, _commitUnlinkError {key, message} the last
    // failure, shown on its own row.
    _edit: { state: true },
    _commentEditID: { state: true },
    _commentDrafts: { state: true },
    _commentSavingID: { state: true },
    _commentDeletingID: { state: true },
    _commitUnlinkKey: { state: true },
    _commitUnlinkBusyKey: { state: true },
    _commitUnlinkError: { state: true },
  };

  static styles = [
    buttonStyles,
    dialogStyles,
    formStyles,
    selectStyles,
    badgeStyles,
    css`
    :host { display: contents; }
    /* Universal box-sizing: the global reset does not penetrate the
       shadow boundary. */
    *, *::before, *::after { box-sizing: border-box; }
    /* Above the topbar (z-index 50), so the backdrop covers the page
       the dialog was opened from, whichever it is; below toasts (60)
       and the confirm dialog (70) it raises itself. */
    .dialog { z-index: 55; }
    .panel.detail:focus { outline: none; }
    .visually-hidden {
      position: absolute;
      width: 1px;
      height: 1px;
      margin: -1px;
      padding: 0;
      overflow: hidden;
      clip: rect(0 0 0 0);
      white-space: nowrap;
      border: 0;
    }

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
    .detail .meta .author-cell .via {
      color: var(--fg-muted);
      font-style: italic;
      font-size: 11px;
    }
    .detail .meta .author-cell .via .sep { margin-right: 4px; opacity: 0.6; }
    /* Creation time rides the creator line in the same muted weight as
       a comment's timestamp, so the task reads dated like its replies. */
    .detail .meta .author-cell .when { color: var(--gray-5); }
    .detail .meta .author-cell .when .sep { margin-right: 4px; opacity: 0.6; }
    .detail .meta .author-cell .via .token {
      font-style: normal;
      font-family: ui-monospace, SFMono-Regular, monospace;
    }
    /* Inline-control wrapper: lays out an adornment (e.g. avatar) next
       to a <select class="select">. The select chrome itself lives in
       components/forms.js → selectStyles, shared by every meta-row
       dropdown (assignee, role, priority) so they all read identical. */
    .detail .meta .inline-control {
      display: inline-flex;
      align-items: center;
      gap: 6px;
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

    /* Questions section: open questions as full cards, the rest folded
       below a disclosure. */
    .detail .questions-list {
      list-style: none;
      margin: 0;
      padding: 0;
    }
    /* Compact cards carry no frame; a rule between them keeps two
       questions from reading as one. */
    .detail .questions-list > li + li {
      margin-top: 12px;
      padding-top: 12px;
      border-top: 1px solid var(--border);
    }
    .detail .questions-list nottario-question-card { scroll-margin: 80px; }
    .detail .questions-rest { margin-top: 8px; }
    .detail .questions-rest > summary {
      cursor: pointer;
      font-size: 12px;
      color: var(--fg-muted);
      padding: 2px 0;
    }
    .detail .questions-rest > summary:hover { color: var(--accent); }
    .detail .questions-rest[open] > summary { margin-bottom: 8px; }
    .detail .eyebrow .eyebrow-count.attention { color: var(--attention-fg); }

    .detail .deps-list {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
    }
    /* Progress next to a section title, in the muted case of a count. */
    .detail .eyebrow .eyebrow-count {
      margin-left: 6px;
      text-transform: none;
      letter-spacing: 0;
      font-weight: 500;
      color: var(--fg-muted);
    }

    .detail .commits-list {
      border: 1px solid var(--border);
      border-radius: 6px;
      overflow: hidden;
      background: var(--bg);
    }
    /* One row = the commit itself plus its unlink control, side by
       side. The border moved up here so the control sits inside the
       same separated band as the link it belongs to. */
    .detail .commits-list .commit-row {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      /* baseline, not center: when the repo wraps under the sha, the
         control stays on the first line with the sha and the message. */
      align-items: baseline;
      border-top: 1px solid var(--gray-2);
      padding-right: 8px;
      transition: background-color 0.1s;
    }
    .detail .commits-list .commit-row:first-child { border-top: none; }
    .detail .commits-list .commit-row.busy { opacity: 0.6; }
    .detail .commits-list .commit {
      display: grid;
      grid-template-columns: auto 1fr auto;
      gap: 12px;
      align-items: baseline;
      min-width: 0;
      padding: 8px 12px;
      background: transparent;
      color: var(--fg);
      text-decoration: none;
      font-size: 13px;
      transition: background-color 0.1s;
    }
    /* The whole band tints, control included, so a two-line row reads
       as one unit instead of a shaded link next to a white cell. */
    .detail .commits-list .commit-row:hover { background: var(--bg-subtle); }
    /* The unlink control stays out of the way until the row is in
       play: pointer over it, keyboard inside it, or a confirm open.
       opacity rather than visibility, so the button keeps its place in
       the tab order and in the accessibility tree — hiding it outright
       would put it out of reach of anyone not using a mouse. Clicking
       it blind only opens the confirm, which is the point of having
       one. */
    .detail .commits-list .commit-actions {
      display: flex;
      align-items: center;
      gap: 4px;
      opacity: 0;
      font-size: 12px;
      transition: opacity 0.1s;
    }
    .detail .commits-list .commit-row:hover .commit-actions,
    .detail .commits-list .commit-row:focus-within .commit-actions,
    .detail .commits-list .commit-row.open .commit-actions,
    .detail .commits-list .commit-row.busy .commit-actions { opacity: 1; }
    /* No hover on a touch screen, so the control has to be there. */
    @media (hover: none) {
      .detail .commits-list .commit-actions { opacity: 1; }
    }
    .detail .commits-list .commit-actions .ask { color: var(--fg-muted); padding-right: 2px; }
    .detail .commits-list .commit-actions .link-btn {
      appearance: none;
      background: transparent;
      border: 0;
      border-radius: 4px;
      padding: 2px 6px;
      font: inherit;
      line-height: 16px;
      color: var(--fg-muted);
      cursor: pointer;
    }
    .detail .commits-list .commit-actions .link-btn:hover { color: var(--fg); background: var(--gray-2); }
    /* Inside the confirm the destructive choice reads as one before the
       pointer reaches it, so it cannot be mistaken for Cancel. */
    .detail .commits-list .commit-row.open .link-btn.danger { color: var(--danger); }
    .detail .commits-list .commit-actions .link-btn.danger:hover {
      color: var(--danger);
      background: color-mix(in srgb, var(--danger) 10%, transparent);
    }
    .detail .commits-list .commit-actions .link-btn:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 0;
    }
    .detail .commits-list .commit-actions .link-btn[disabled] { cursor: default; background: transparent; }
    /* A failed removal keeps the row and says why, under it. */
    .detail .commits-list .commit-error {
      grid-column: 1 / -1;
      margin: 0;
      padding: 0 12px 8px;
      font-size: 12px;
      color: var(--danger);
    }
    .detail .commits-list a.commit:hover .sha { text-decoration: underline; }
    .detail .commits-list .commit .sha {
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 12px;
      color: var(--accent);
    }
    .detail .commits-list .commit .msg {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      color: var(--fg);
    }
    .detail .commits-list .commit .when {
      font-size: 11px;
      color: var(--fg-muted);
      font-variant-numeric: tabular-nums;
    }
    /* Repo suffix in the section eyebrow when all commits share one repo. */
    .detail section .eyebrow .eyebrow-repo {
      font-family: ui-monospace, SFMono-Regular, monospace;
      text-transform: none;
      letter-spacing: 0;
      color: var(--fg-muted);
      font-weight: 500;
    }
    /* Per-row repo suffix (only rendered when commits mix repos). */
    .detail .commits-list .commit .row-repo {
      grid-column: 1 / -1;
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 11px;
      color: var(--fg-muted);
      margin-top: -2px;
    }

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

    /* Ghost-edit buttons: quiet by default, surface on row hover or on
       keyboard focus. Same chrome for title and section heads. */
    .detail .ghost-edit {
      appearance: none;
      background: transparent;
      border: 1px solid transparent;
      color: var(--fg-muted);
      font: inherit;
      font-size: 12px;
      padding: 2px 8px;
      border-radius: 4px;
      cursor: pointer;
      opacity: 0;
      transition: opacity 80ms ease-out;
    }
    .detail .title-row:hover .ghost-edit,
    .detail .section-head:hover .ghost-edit,
    .detail .ghost-edit:focus-visible {
      opacity: 1;
    }
    .detail .ghost-edit:hover {
      color: var(--fg);
      background: var(--bg-hover);
      border-color: var(--border);
    }

    /* Title edit form */
    .detail .title-edit { flex: 1; display: flex; flex-direction: column; gap: 6px; }
    .detail .title-input {
      width: 100%;
      font-size: 18px;
      font-weight: 600;
      padding: 6px 10px;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: #fff;
      color: var(--fg);
      box-sizing: border-box;
    }
    .detail .title-input:focus {
      outline: 0;
      border-color: var(--accent);
    }
    .detail .title-edit-actions {
      display: flex;
      gap: 6px;
      justify-content: flex-end;
    }

    /* Description section head: holds eyebrow + ghost-edit on one line */
    .detail .section-head {
      display: flex;
      align-items: baseline;
      gap: 6px;
      margin: 0 0 6px;
    }
    .detail .section-head .eyebrow { margin: 0; }

    /* "(edited 3m ago by @name)" marker — tiny, muted */
    .detail .edited-mark {
      color: var(--fg-muted);
      font-size: 11px;
      margin-top: 4px;
    }

    /* Comment per-item actions: link-style buttons aligned with the
       meta line, hidden until hover/focus so they don't add noise */
    .detail .comment .comment-actions {
      margin-left: auto;
      display: inline-flex;
      gap: 8px;
      opacity: 0;
      transition: opacity 80ms ease-out;
    }
    .detail .comment:hover .comment-actions,
    .detail .comment .comment-actions:focus-within {
      opacity: 1;
    }
    .detail .comment .link-btn {
      appearance: none;
      background: transparent;
      border: 0;
      padding: 0;
      font: inherit;
      font-size: 12px;
      color: var(--fg-muted);
      cursor: pointer;
    }
    .detail .comment .link-btn:hover { color: var(--fg); text-decoration: underline; }
    .detail .comment .link-btn.danger:hover { color: var(--danger); }

    /* Inline delete confirm — replaces the meta-line, body stays */
    .detail .comment-confirm {
      display: flex;
      align-items: center;
      gap: 8px;
      font-size: 13px;
      color: var(--fg);
      padding: 4px 0 6px;
    }
    .detail .comment-confirm span { margin-right: auto; }
  `,
  ];

  constructor() {
    super();
    this.me = null;
    this.projectId = '';
    this.taskId = '';
    this.selected = null;
    this.roles = [];
    this.members = [];
    this.priorities = [];
    this._resetEditState();
    new EscController(this, (e) => this._onEsc(e));
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this._unsub?.();
    this._unsub = null;
    clearTimeout(this._refreshTimer);
    this._refreshTimer = null;
  }

  willUpdate(c) {
    if (c.has('projectId') && this.projectId !== this._loadedProject) {
      // Never show another project's roles, members or priorities,
      // not even for the round trip.
      this.roles = [];
      this.members = [];
      this.priorities = [];
      this._loadedProject = null;
      if (this.projectId) this._loadProject();
    }
    if (c.has('taskId') || c.has('projectId')) {
      if (this.taskId && this.projectId) this._open();
      else this._close();
    }
  }

  updated(c) {
    // Move focus into the dialog when it opens, so keyboard users land
    // in it and Tab starts from its first control. The shell gives
    // focus back to where it was when the dialog closes.
    if (c.has('taskId') && this.taskId && !c.get('taskId')) {
      this.renderRoot.querySelector('.panel.detail')?.focus({ preventScroll: true });
    }
  }

  _resetEditState() {
    this._answeredHere = new Set();
    this._edit = { field: null, titleDraft: '', descDraft: '' };
    this._commentEditID = null;
    this._commentDrafts = {};
    this._commentSavingID = null;
    this._commentDeletingID = null;
    this._commitUnlinkKey = null;
    this._commitUnlinkBusyKey = null;
    this._commitUnlinkError = null;
  }

  // _open (re)starts the dialog on taskId: a placeholder renders while
  // the detail is in flight, so the panel shows up at once.
  _open() {
    const id = this.taskId;
    if (this.selected?.task.id === id && this._subscribedProject === this.projectId) return;
    this._closing = false;
    this._resetEditState();
    // A failed project load is retried the next time a task opens.
    if (this._loadedProject !== this.projectId && !this._projectLoading) this._loadProject();
    this.selected = {
      task: { id, title: '' },
      related: NO_RELATED,
      commits: [],
      comments: [],
    };
    this._subscribe();
    this.loadDetail(id);
  }

  _close() {
    this._unsub?.();
    this._unsub = null;
    this._subscribedProject = null;
    clearTimeout(this._refreshTimer);
    this._refreshTimer = null;
    this.selected = null;
    this._closing = false;
    this._resetEditState();
  }

  // Roles, members and priorities of the task's project: the meta
  // strip's dropdowns and the names on comments come from them.
  // _loadedProject is set only once all three arrived, so a failure
  // is retried on the next open.
  async _loadProject() {
    const pid = this.projectId;
    this._projectLoading = pid;
    try {
      const [rr, mr, qr] = await Promise.all([
        fetch(`/api/projects/${pid}/roles`),
        fetch(`/api/projects/${pid}/members`),
        fetch(`/api/projects/${pid}/priorities`),
      ]);
      if (!rr.ok || !mr.ok || !qr.ok) throw new Error('project data');
      const [rj, mj, qj] = await Promise.all([rr.json(), mr.json(), qr.json()]);
      if (this.projectId !== pid) return;
      this.roles = rj.roles || [];
      this.members = mj.members || [];
      this.priorities = qj.priorities || [];
      this._loadedProject = pid;
    } catch (_) {
      // The dialog still works without them; the dropdowns stay empty
      // until the next open retries.
    } finally {
      if (this._projectLoading === pid) this._projectLoading = null;
    }
  }

  _subscribe() {
    this._unsub?.();
    this._subscribedProject = this.projectId;
    this._unsub = subscribe(this.projectId, (ev) => {
      if (!ev.type || !this.selected) return;
      const id = this.selected.task.id;
      // 'realtime.reconnected' fires after EventSource recovers from a
      // disconnect: any events during the gap were lost, so reload.
      // task.comment.* only matter for this task. Any other task.*
      // event may change a related task's state, so it reloads too.
      if (ev.type.startsWith('task.comment.')) {
        if (ev.task_id === id) this._queueRefresh();
      } else if (ev.type === 'realtime.reconnected' || ev.type.startsWith('task.')) {
        this._queueRefresh();
      } else if (ev.type.startsWith('question.')) {
        // The Questions section and the step markers follow the
        // task's questions. Answering from the dialog relies on this
        // too: the answer's own event refreshes the detail.
        if (!ev.task_id || ev.task_id === id) this._queueRefresh();
      }
    });
  }

  // _queueRefresh coalesces event-driven reloads: a burst of events (an
  // answer, a task closing with several questions, a reconnect) costs
  // one loadDetail() instead of one per event. The reload is skipped
  // when the dialog moved to another task or closed meanwhile.
  _queueRefresh() {
    const id = this.selected?.task.id;
    if (!id) return;
    this._refreshID = id;
    if (this._refreshTimer) return;
    this._refreshTimer = setTimeout(() => {
      const target = this._refreshID;
      this._refreshTimer = null;
      this._refreshID = null;
      if (target && this.selected?.task.id === target) this.loadDetail(target);
    }, 150);
  }

  async loadDetail(id) {
    // When the request left, so the steps list can tell a reply that
    // predates its own latest change.
    const requestedAt = performance.now();
    const pid = this.projectId;
    // The dialog may have closed, or moved to another task, while the
    // request was out: never reopen it or swap its contents.
    const stale = () => this.selected?.task.id !== id || this.projectId !== pid;
    let r;
    let j;
    try {
      r = await fetch(`/api/projects/${pid}/tasks/${id}`);
      if (stale()) return;
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      j = await r.json();
      if (!j?.task) throw new Error('no task in the response');
    } catch (_) {
      if (stale()) return;
      // Only a dialog still showing its placeholder gives up: a refresh
      // that fails keeps the task already on screen.
      if (this.selected.task.updated_at) return;
      toast.error(
        r?.status === 404
          ? 'That task does not exist in this project.'
          : "Couldn't load the task. Try again in a moment.",
      );
      this.closeDetail();
      return;
    }
    if (stale()) return;
    this.selected = {
      task: j.task,
      description_html: j.description_html || '',
      related: { ...NO_RELATED, ...(j.related || {}) },
      steps: j.steps || [],
      stepsAt: requestedAt,
      questions: j.questions || [],
      commits: j.commits || [],
      comments: j.comments || [],
    };
  }

  // closeDetail asks the shell to close the dialog; the shell owns the
  // URL (and the history entry), so the dialog does not clear itself.
  closeDetail() {
    // Once per open: Esc, the backdrop and a failed load may all ask.
    if (!this.selected || this._closing) return;
    this._closing = true;
    this.dispatchEvent(new CustomEvent('task-dialog-close', { bubbles: true, composed: true }));
  }

  // kind tells the page what changed: 'task' (fields, state, deletion:
  // the board reloads), 'step' (carries the steps, so a card's progress
  // can be patched), 'comment' or 'commit' (nothing on a card).
  _changed(kind, extra = {}) {
    this.dispatchEvent(
      new CustomEvent('task-changed', {
        detail: { kind, task: this.selected?.task || null, projectId: this.projectId, ...extra },
        bubbles: true,
        composed: true,
      }),
    );
  }

  // The one Esc path (document keydown). Esc inside an editor (the
  // title input, a description or comment md-editor) cancels that edit
  // and marks the event handled, so the dialog stays. An overlay above
  // the dialog (the delete confirm) handles Esc on document too, after
  // this listener, so the close waits for the event to finish and backs
  // off when someone else consumed it.
  _onEsc(e) {
    if (!this.selected || e.defaultPrevented) return;
    // An open unlink confirm is the innermost thing on screen: Esc
    // backs out of it, not out of the whole task.
    if (this._commitUnlinkKey && !this._commitUnlinkBusyKey) {
      this.cancelUnlinkCommit();
      return;
    }
    setTimeout(() => {
      if (!e.defaultPrevented) this.closeDetail();
    });
  }

  async setState(taskID, state) {
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${taskID}/state`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ state }),
    });
    if (r.ok) {
      if (this.selected) await this.loadDetail(taskID);
      this._changed('task');
    } else {
      const j = await r.json().catch(() => ({}));
      toast.error(`Couldn't change the state: ${j.error || 'failed'}`);
    }
  }

  roleByID(id) {
    return this.roles.find((r) => r.id === id);
  }

  // Find the priority bucket whose Value is closest to `value`. Used
  // to pre-select the dropdown when the stored priority happens to
  // land between buckets (e.g. someone set a raw integer via SQL or
  // the legacy number input).
  _nearestBucketKey(value) {
    if (!this.priorities?.length) return '';
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

  // --- Inline edit of task title / description / role ---------------
  //
  // beginEditTitle / beginEditDesc swap the title / description block
  // to its editor form, seeded with the current value so the user can
  // tweak rather than retype.
  beginEditTitle() {
    const t = this.selected?.task;
    if (!t) return;
    this._edit = { field: 'title', titleDraft: t.title || '', descDraft: '' };
  }

  beginEditDesc() {
    const t = this.selected?.task;
    if (!t) return;
    this._edit = { field: 'desc', titleDraft: '', descDraft: t.description || '' };
  }

  cancelEditText() {
    this._edit = { field: null, titleDraft: '', descDraft: '' };
  }

  // saveTaskText pushes the staged title / description to the backend
  // with optimistic concurrency. On 409 stale the editor stays open
  // with the user's buffer and we refresh the underlying selected.task
  // so the next attempt has a fresh expected_updated_at.
  async saveTaskText({ title, description, target_role_id, unset_target_role } = {}) {
    const t = this.selected?.task;
    if (!t) return;
    const body = { expected_updated_at: t.updated_at };
    if (title !== undefined) body.title = title;
    if (description !== undefined) body.description = description;
    if (target_role_id !== undefined) body.target_role_id = target_role_id;
    if (unset_target_role) body.unset_target_role = true;
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${t.id}/text`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (r.ok) {
      this._edit = { field: null, titleDraft: '', descDraft: '' };
      await this.loadDetail(t.id);
      this._changed('task');
      toast.success('Saved.');
      return true;
    }
    if (r.status === 409) {
      const j = await r.json().catch(() => ({}));
      if (j.current) {
        this.selected = { ...this.selected, task: j.current };
      }
      toast.error(
        'This was just edited by someone else. Your draft is preserved — review the latest and save again.',
      );
      return false;
    }
    const j = await r.json().catch(() => ({}));
    toast.error(`Couldn't save: ${j.error || r.statusText}`);
    return false;
  }

  async setRole(roleID) {
    // Empty string means "unassign" — same convention the assignee
    // dropdown uses. Admin-only at the backend.
    if (roleID === '') {
      await this.saveTaskText({ unset_target_role: true });
    } else {
      await this.saveTaskText({ target_role_id: roleID });
    }
  }

  // --- Inline edit / delete of comments -----------------------------
  _canModifyComment(c) {
    if (!this.me) return false;
    if (this.me.is_admin) return true;
    return !!c.author_user_id && c.author_user_id === this.me.id;
  }

  beginEditComment(c) {
    this._commentEditID = c.id;
    this._commentDrafts = { ...this._commentDrafts, [c.id]: c.body || '' };
    this._commentDeletingID = null;
  }

  cancelEditComment() {
    this._commentEditID = null;
  }

  beginDeleteComment(c) {
    this._commentDeletingID = c.id;
    this._commentEditID = null;
  }

  cancelDeleteComment() {
    this._commentDeletingID = null;
  }

  // A commit link is identified by (repo, sha) — there is no id — so
  // that pair is the key for every piece of per-row state.
  _commitKey(c) {
    return `${(c.repo || '').trim()}\n${(c.sha || '').trim()}`;
  }

  beginUnlinkCommit(c) {
    this._commitUnlinkKey = this._commitKey(c);
    this._commitUnlinkError = null;
  }

  cancelUnlinkCommit() {
    this._commitUnlinkKey = null;
    this._commitUnlinkError = null;
  }

  // Removing a commit link is a correction a human makes by hand: the
  // web has no way to link one back, so the row asks first and reports
  // in place rather than vanishing on a failed request.
  async confirmUnlinkCommit(c) {
    const t = this.selected?.task;
    if (!t) return;
    const key = this._commitKey(c);
    this._commitUnlinkBusyKey = key;
    this._commitUnlinkError = null;
    try {
      const r = await fetch(`/api/projects/${this.projectId}/tasks/${t.id}/commits`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ repo: c.repo, sha: c.sha }),
      });
      if (r.status === 204 || r.status === 404) {
        // 404 means somebody got there first. The row is gone either
        // way, so reload rather than leaving a card the server no
        // longer knows about.
        this._commitUnlinkKey = null;
        await this.loadDetail(t.id);
        this._changed('commit');
        toast.success(r.status === 204 ? 'Commit unlinked.' : 'That commit was already unlinked.');
        return;
      }
      const j = await r.json().catch(() => ({}));
      this._commitUnlinkError = { key, message: j.error || r.statusText || 'Request failed' };
    } catch (e) {
      this._commitUnlinkError = { key, message: e.message };
    } finally {
      this._commitUnlinkBusyKey = null;
    }
  }

  async saveComment(commentID, body) {
    const t = this.selected?.task;
    if (!t) return;
    const cur = (this.selected.comments || []).find((x) => x.id === commentID);
    if (!cur) return;
    this._commentSavingID = commentID;
    try {
      const r = await fetch(`/api/projects/${this.projectId}/tasks/${t.id}/comments/${commentID}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ body, expected_updated_at: cur.updated_at }),
      });
      if (r.ok) {
        this._commentEditID = null;
        await this.loadDetail(t.id);
        this._changed('comment');
        toast.success('Comment saved.');
        return;
      }
      if (r.status === 409) {
        const j = await r.json().catch(() => ({}));
        if (j.current) {
          const updated = (this.selected.comments || []).map((x) =>
            x.id === commentID ? j.current : x,
          );
          this.selected = { ...this.selected, comments: updated };
        }
        toast.error(
          'This comment was just edited. Your draft is preserved — review the latest and save again.',
        );
        return;
      }
      if (r.status === 404) {
        this._commentEditID = null;
        await this.loadDetail(t.id);
        toast.error('That comment was deleted.');
        return;
      }
      const j = await r.json().catch(() => ({}));
      toast.error(`Couldn't save comment: ${j.error || r.statusText}`);
    } finally {
      this._commentSavingID = null;
    }
  }

  async confirmDeleteComment(commentID) {
    const t = this.selected?.task;
    if (!t) return;
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${t.id}/comments/${commentID}`, {
      method: 'DELETE',
    });
    if (r.status === 204) {
      this._commentDeletingID = null;
      await this.loadDetail(t.id);
      this._changed('comment');
      toast.success('Comment deleted.');
      return;
    }
    const j = await r.json().catch(() => ({}));
    toast.error(`Couldn't delete: ${j.error || r.statusText}`);
  }

  async deleteTask(id) {
    const t = this.selected?.task.id === id ? this.selected.task : null;
    const label = t?.title ? `"${t.title.slice(0, 32)}${t.title.length > 32 ? '…' : ''}"` : 'Task';
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${id}`, { method: 'DELETE' });
    if (r.ok) {
      this._changed('task', { deleted: true });
      this.closeDetail();
      toast.success(`${label} deleted.`);
    } else {
      const error = (await r.json().catch(() => ({}))).error || 'failed';
      toast.error(`Couldn't delete: ${error}`);
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
      this._changed('comment');
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
      if (this.selected) await this.loadDetail(taskID);
      this._changed('task');
    }
  }

  async setAssignee(taskID, userID) {
    const r = await fetch(`/api/projects/${this.projectId}/tasks/${taskID}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ assignee_user_id: userID }),
    });
    if (r.ok) {
      if (this.selected) await this.loadDetail(taskID);
      this._changed('task');
    } else {
      const error = (await r.json().catch(() => ({}))).error || 'failed';
      toast.error(`Couldn't change the assignee: ${error}`);
    }
  }

  // The steps component changed the open task's checklist: keep the
  // dialog's copy in step without a reload, and tell the page so a
  // board card's progress follows.
  _onStepsChange(taskID, steps) {
    if (this.selected?.task.id === taskID) {
      this.selected = { ...this.selected, steps };
    }
    this._changed('step', { steps });
  }

  // ---- Questions in the task dialog --------------------------------
  //
  // Open questions come first as full cards; the rest (answered,
  // delivered, withdrawn) fold under a disclosure. A question answered
  // from this dialog stays among the open ones until the dialog closes,
  // so the confirmation does not jump away from under the cursor.
  _renderQuestions() {
    const all = this.selected?.questions || [];
    if (!all.length) return null;
    const here = this._answeredHere || new Set();
    // Oldest first, so a card answered here keeps its place.
    const open = all
      .filter((q) => q.state === 'open' || here.has(q.id))
      .sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at));
    const rest = all.filter((q) => q.state !== 'open' && !here.has(q.id));
    const openCount = all.filter((q) => q.state === 'open').length;
    // Amber only when one of them waits on the viewer.
    const waitsOnMe = all.some((q) => q.state === 'open' && q.for_me);
    const card = (q) => html`<li><nottario-question-card
        compact
        data-id=${q.id}
        project-id=${this.projectId}
        .question=${q}></nottario-question-card></li>`;
    return html`
      <section @question-answered=${(e) => this._onQuestionAnswered(e)}
               @question-stale=${() => this.selected && this.loadDetail(this.selected.task.id)}>
        <h4 class="eyebrow">
          Questions
          ${
            openCount
              ? html`<span class=${`eyebrow-count${waitsOnMe ? ' attention' : ''}`}>${openCount} open</span>`
              : html`<span class="eyebrow-count">none open</span>`
          }
        </h4>
        ${open.length ? html`<ol class="questions-list">${repeat(open, (q) => q.id, card)}</ol>` : null}
        ${
          rest.length
            ? html`
          <details class="questions-rest" ?open=${!open.length}>
            <summary>${rest.length} earlier question${rest.length === 1 ? '' : 's'}</summary>
            <ol class="questions-list">${repeat(rest, (q) => q.id, card)}</ol>
          </details>`
            : null
        }
      </section>
    `;
  }

  _focusQuestionCard(id) {
    const card = this.renderRoot.querySelector(
      `nottario-question-card[data-id="${CSS.escape(id)}"]`,
    );
    if (!card) return;
    card.scrollIntoView({
      block: 'nearest',
      behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
    });
    card.focus();
  }

  _onQuestionAnswered(e) {
    const q = e.detail?.question;
    if (!q || !this.selected) return;
    this._answeredHere = new Set(this._answeredHere || []).add(q.id);
    this.selected = {
      ...this.selected,
      questions: (this.selected.questions || []).map((x) => (x.id === q.id ? q : x)),
    };
    // No explicit reload: the answer's question event refreshes the
    // board and this detail through _queueRefresh.
  }

  // Look up a member by UserID. Members carry display name + avatar
  // URL; comments and the task assignee link to one of them.
  _memberByID(uid) {
    if (!uid) return null;
    return (this.members || []).find((m) => m.user_id === uid) || null;
  }

  // A related task as the detail lists it. The summary comes with the
  // detail, which reloads on every task event of the project, so a
  // state change made elsewhere shows without reopening the dialog.
  _relatedChip(summary) {
    return html`<nottario-task-chip
      project-id=${this.projectId}
      .task=${summary}></nottario-task-chip>`;
  }

  _relatedState(summary) {
    return summary.state;
  }

  // "Created by" field-line on the task detail header. Shows the
  // creator's avatar + display name, the agent badge / "via {token}"
  // suffix when the task came through an MCP token, and when it was
  // created — the same "name · 3d ago" shape every comment's meta line
  // already uses, so the original post is dated like its replies.
  // Relative on screen, absolute on hover, matching the "(edited …)"
  // marker. Rows that predate creator tracking still get the date.
  _renderCreatedByLine(task) {
    const creator = task.created_by_user_id ? this._memberByID(task.created_by_user_id) : null;
    const when = task.created_at
      ? html`<span class="when" title=${formatDateTime(task.created_at)}><span class="sep">·</span>${formatRelativeTime(task.created_at)}</span>`
      : null;
    if (!creator && !task.via_mcp) {
      if (!when) return null;
      return html`
        <div class="field-line">
          <span class="lbl">Created</span>
          <span class="val author-cell">${formatRelativeTime(task.created_at)}</span>
        </div>
      `;
    }
    const name = creator?.display_name || creator?.github_login || 'unknown';
    return html`
      <div class="field-line">
        <span class="lbl">Created by</span>
        <span class="val author-cell">
          <nottario-avatar size="20"
            src=${creator?.avatar_url || ''}
            name=${name}
            .agent=${task.via_mcp || null}></nottario-avatar>
          <span>${name}</span>
          ${
            task.via_mcp
              ? html`<span class="via"><span class="sep">·</span>via <span class="token">${task.via_mcp.name || 'MCP'}</span></span>`
              : null
          }
          ${when}
        </span>
      </div>
    `;
  }

  // Render one commit row inside the commits list. `showRepo` is passed
  // by the section: when all linked commits share a single repo we hoist
  // that label to the section eyebrow and drop it from each row (the
  // usual case — nearly every task links commits from one repo). When
  // repos mix we render the repo as a small suffix on each row so the
  // reader can tell them apart.
  _renderCommit(c, showRepo) {
    const sha = (c.sha || '').trim();
    const repo = (c.repo || '').trim();
    const shortSha = sha.slice(0, 7);
    // Only link when repo looks like a clean GitHub `owner/repo` —
    // anything else (extra slashes, protocol prefix, empty repo) falls
    // back to a plain row so we never inject untrusted text into a URL.
    const repoOK = /^[A-Za-z0-9._-]+\/[A-Za-z0-9._-]+$/.test(repo);
    const url = repoOK && sha ? `https://github.com/${repo}/commit/${sha}` : null;
    const when = c.added_at ? formatRelativeTime(c.added_at) : '';
    const whenTitle = c.added_at ? formatDateTime(c.added_at) : '';
    const inner = html`
      <span class="sha" title=${sha}>${shortSha}</span>
      <span class="msg" title=${c.message || ''}>${c.message || '—'}</span>
      <span class="when" title=${whenTitle}>${when}</span>
      ${showRepo && repo ? html`<span class="row-repo">${repo}</span>` : null}
    `;
    // The link and the remove control are siblings: a button nested in
    // an anchor is invalid, and the whole row stays clickable through
    // to GitHub.
    const body = url
      ? html`<a class="commit" href=${url} target="_blank" rel="noopener noreferrer">${inner}</a>`
      : html`<div class="commit">${inner}</div>`;
    const key = this._commitKey(c);
    const confirming = this._commitUnlinkKey === key;
    const busy = this._commitUnlinkBusyKey === key;
    const failure = this._commitUnlinkError?.key === key ? this._commitUnlinkError.message : '';
    return html`
      <div class=${`commit-row${confirming ? ' open' : ''}${busy ? ' busy' : ''}`}>
        ${body}
        <span class="commit-actions">
          ${
            confirming
              ? html`
                <span class="ask">Unlink?</span>
                <button class="link-btn"
                        ?disabled=${busy}
                        @click=${() => this.cancelUnlinkCommit()}>Cancel</button>
                <button class="link-btn danger"
                        ?disabled=${busy}
                        @click=${() => this.confirmUnlinkCommit(c)}>${busy ? 'Unlinking…' : 'Unlink'}</button>
              `
              : html`
                <button class="link-btn danger"
                        title=${`Unlink commit ${shortSha}`}
                        aria-label=${`Unlink commit ${shortSha}`}
                        @click=${() => this.beginUnlinkCommit(c)}>Unlink</button>
              `
          }
        </span>
        ${failure ? html`<p class="commit-error" role="alert">Couldn't unlink: ${failure}</p>` : null}
      </div>
    `;
  }

  // Renders "(edited 5m ago by @name)" below a task field or comment
  // body. Returns nothing when the field was never edited.
  _renderEditedMarker(editedAt, editedByUserID) {
    if (!editedAt) return null;
    const editor = this._memberByID(editedByUserID);
    const name = editor?.display_name || editor?.github_login || 'someone';
    const abs = formatDateTime(editedAt);
    return html`<div class="edited-mark" title=${abs}>(edited ${formatRelativeTime(editedAt)} by ${name})</div>`;
  }

  // Renders a single comment, including the per-comment action menu
  // (Edit / Delete, when the caller is the author or an admin), the
  // inline delete confirm, and the inline edit using the md-editor.
  _renderComment(c) {
    const author = this._memberByID(c.author_user_id);
    const editing = this._commentEditID === c.id;
    const deleting = this._commentDeletingID === c.id;
    const saving = this._commentSavingID === c.id;
    const canModify = this._canModifyComment(c);
    return html`
      <div class="comment">
        <div class="ava">
          <nottario-avatar size="24"
            src=${author?.avatar_url || ''}
            name=${author?.display_name || author?.github_login || 'agent'}
            .agent=${c.via_mcp || null}></nottario-avatar>
        </div>
        <div>
          ${
            deleting
              ? html`
                <div class="comment-confirm" role="alert">
                  <span>Delete this comment?</span>
                  <button class="btn secondary"
                          @click=${() => this.cancelDeleteComment()}>Cancel</button>
                  <button class="btn danger"
                          @click=${() => this.confirmDeleteComment(c.id)}>Delete</button>
                </div>
              `
              : html`
                <div class="meta-line">
                  <span class="name">${author?.display_name || author?.github_login || 'agent'}</span>
                  ${
                    c.via_mcp
                      ? html`<span class="via"><span class="sep">·</span>via <span class="token">${c.via_mcp.name || 'MCP'}</span></span>`
                      : null
                  }
                  <span class="when" title=${formatDateTime(c.created_at)}>${formatRelativeTime(c.created_at)}</span>
                  ${
                    canModify && !editing
                      ? html`
                        <span class="comment-actions">
                          <button class="link-btn"
                                  title="Edit comment"
                                  @click=${() => this.beginEditComment(c)}>Edit</button>
                          <button class="link-btn danger"
                                  title="Delete comment"
                                  @click=${() => this.beginDeleteComment(c)}>Delete</button>
                        </span>
                      `
                      : null
                  }
                </div>
              `
          }
          ${
            editing
              ? html`
                <nottario-md-editor
                  project-id=${this.projectId}
                  .value=${this._commentDrafts[c.id] ?? c.body ?? ''}
                  .saving=${saving}
                  placeholder="Edit comment…"
                  @input=${(e) => {
                    this._commentDrafts = { ...this._commentDrafts, [c.id]: e.target.value };
                  }}
                  @submit=${(e) => this.saveComment(c.id, e.detail.value)}
                  @cancel=${() => this.cancelEditComment()}>
                </nottario-md-editor>
              `
              : html`<nottario-markdown
                       project-id=${this.projectId}
                       .html=${c.body_html || ''}
                       .source=${c.body || ''}></nottario-markdown>`
          }
          ${this._renderEditedMarker(c.edited_at, c.edited_by_user_id)}
        </div>
      </div>
    `;
  }

  renderDetail() {
    const { task, related, commits, comments } = this.selected;
    const role = task.target_role_id ? this.roleByID(task.target_role_id) : null;
    const assignee = this._memberByID(task.assignee_user_id);
    const shortID = (task.id || '').slice(0, 7);
    return html`
      <div class="dialog"
           role="dialog"
           aria-modal="true"
           aria-labelledby="task-dialog-label"
           @click=${(e) => e.target.classList.contains('dialog') && this.closeDetail()}>
        <div class="panel detail" tabindex="-1">
          <span id="task-dialog-label" class="visually-hidden">${task.title || 'Task'}</span>
          <header class="head">
            <div class="title-row">
              ${
                this._edit.field === 'title'
                  ? html`
                    <div class="title-edit">
                      <input class="title-input"
                             .value=${this._edit.titleDraft}
                             aria-label="Task title"
                             @input=${(e) => {
                               this._edit = { ...this._edit, titleDraft: e.target.value };
                             }}
                             @keydown=${(e) => {
                               if (e.key === 'Escape') {
                                 e.preventDefault();
                                 this.cancelEditText();
                               } else if (e.key === 'Enter' && !e.shiftKey) {
                                 e.preventDefault();
                                 this.saveTaskText({ title: this._edit.titleDraft });
                               }
                             }}>
                      <div class="title-edit-actions">
                        <button class="btn secondary"
                                @click=${() => this.cancelEditText()}>Cancel</button>
                        <button class="btn primary"
                                @click=${() => this.saveTaskText({ title: this._edit.titleDraft })}>Save</button>
                      </div>
                    </div>
                  `
                  : html`
                    <h3 id="task-dialog-title">${task.title}</h3>
                    <button class="ghost-edit"
                            title="Edit title"
                            aria-label="Edit title"
                            @click=${() => this.beginEditTitle()}>Edit</button>
                  `
              }
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
                  ${trashIcon()}
                </button>
                <button class="icon-btn" title="Close (Esc)" aria-label="Close"
                        @click=${() => this.closeDetail()}>
                  ${closeIcon()}
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
                <select class="select"
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
                ${
                  this.me?.is_admin
                    ? html`
                      <span class="val">
                        <select class="select"
                                @change=${(e) => this.setRole(e.target.value)}>
                          <option value="" ?selected=${!task.target_role_id}>— none —</option>
                          ${(this.roles || []).map(
                            (r) => html`
                              <option value=${r.id} ?selected=${r.id === task.target_role_id}>${r.label}</option>
                            `,
                          )}
                        </select>
                      </span>
                    `
                    : html`<span class="val">${role ? role.label : html`<span class="muted">none</span>`}</span>`
                }
              </div>

              <div class="field-line">
                <span class="lbl">Assignee</span>
                <span class="val inline-control">
                  ${
                    assignee?.avatar_url
                      ? html`<nottario-avatar size="20"
                              src=${assignee.avatar_url}
                              name=${assignee.display_name || assignee.github_login || ''}></nottario-avatar>`
                      : null
                  }
                  <select class="select"
                          @change=${(e) => this.setAssignee(task.id, e.target.value)}>
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

              ${
                related.parent
                  ? html`
                    <div class="field-line">
                      <span class="lbl">Part of</span>
                      <span class="val">${this._relatedChip(related.parent)}</span>
                    </div>
                  `
                  : null
              }

              ${this._renderCreatedByLine(task)}
            </div>
          </header>

          <div class="body">
            <section class="desc-section">
              <div class="section-head">
                <h4 class="eyebrow">Description</h4>
                ${
                  this._edit.field !== 'desc'
                    ? html`<button class="ghost-edit"
                              title="Edit description"
                              aria-label="Edit description"
                              @click=${() => this.beginEditDesc()}>Edit</button>`
                    : null
                }
              </div>
              ${
                this._edit.field === 'desc'
                  ? html`
                    <nottario-md-editor
                      project-id=${this.projectId}
                      .value=${this._edit.descDraft}
                      placeholder="Describe the task in markdown…"
                      @input=${(e) => {
                        // The editor emits @submit on save, but we keep the
                        // draft in sync so a 409 stale re-render preserves
                        // it. The <textarea> bubbles 'input' through the
                        // shadow boundary because composed:true is the
                        // browser default for input events.
                        this._edit = { ...this._edit, descDraft: e.target.value };
                      }}
                      @submit=${(e) => this.saveTaskText({ description: e.detail.value })}
                      @cancel=${() => this.cancelEditText()}>
                    </nottario-md-editor>
                  `
                  : task.description
                    ? html`<nottario-markdown
                              project-id=${this.projectId}
                              .html=${this.selected.description_html || ''}
                              .source=${task.description}></nottario-markdown>`
                    : html`<p class="empty">No description.</p>`
              }
              ${this._renderEditedMarker(task.edited_at, task.edited_by_user_id)}
            </section>

            <section>
              <nottario-task-steps
                project-id=${this.projectId}
                .taskId=${task.id}
                .steps=${this.selected.steps || []}
                .stepsAt=${this.selected.stepsAt}
                .questions=${this.selected.questions || []}
                @step-question=${(e) => this._focusQuestionCard(e.detail.questionId)}
                @steps-change=${(e) => this._onStepsChange(task.id, e.detail.steps)}></nottario-task-steps>
            </section>

            ${this._renderQuestions()}


            ${
              related.children.length
                ? html`
              <section>
                <h4 class="eyebrow">
                  Subtasks
                  <span class="eyebrow-count">${related.children.filter((c) => this._relatedState(c) === 'done').length} of ${related.children.length} done</span>
                </h4>
                <div class="deps-list">${related.children.map((c) => this._relatedChip(c))}</div>
              </section>
            `
                : null
            }

            ${
              related.depends_on.length
                ? html`
              <section>
                <h4 class="eyebrow">Depends on</h4>
                <div class="deps-list">${related.depends_on.map((d) => this._relatedChip(d))}</div>
              </section>
            `
                : null
            }

            ${
              related.blocks.length
                ? html`
              <section>
                <h4 class="eyebrow">Blocks</h4>
                <div class="deps-list">${related.blocks.map((b) => this._relatedChip(b))}</div>
              </section>
            `
                : null
            }

            ${(() => {
              // Hoist the repo label to the section eyebrow when every
              // linked commit shares a single repo (the common case).
              // Falls back to per-row repo suffixes when the set is
              // mixed. Empty set = no repo label, empty state below.
              const repos = new Set(commits.map((c) => c.repo).filter(Boolean));
              const singleRepo = repos.size === 1 ? [...repos][0] : null;
              const showRepoPerRow = repos.size > 1;
              return html`
            <section>
              <h4 class="eyebrow">
                Commits${singleRepo ? html`<span class="eyebrow-repo"> — ${singleRepo}</span>` : ''}
              </h4>
              ${
                commits.length === 0
                  ? html`<p class="empty">No commits linked.</p>`
                  : html`
                  <div class="commits-list">
                    ${commits.map((c) => this._renderCommit(c, showRepoPerRow))}
                  </div>
                `
              }
            </section>
            `;
            })()}

            <section>
              <h4 class="eyebrow">Comments</h4>
              ${
                comments.length === 0
                  ? html`<p class="empty">No comments yet.</p>`
                  : comments.map((c) => this._renderComment(c))
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

  render() {
    return this.selected ? this.renderDetail() : null;
  }
}

customElements.define('nottario-task-dialog', NottarioTaskDialog);
