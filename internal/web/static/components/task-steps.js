import { LitElement, html, css, svg } from '/static/vendor/lit/lit.js';
import { buttonStyles } from '/static/components/buttons.js';
import { trashIcon } from '/static/components/icons.js';
import {
  MAX_STEP_LEN,
  MAX_STEPS,
  moveIndex,
  stepProgress,
  stepTextsFromPaste,
} from '/static/steps.js';

// <nottario-task-steps> is the checklist inside the task dialog: tick,
// add (one line, or several pasted at once), rename in place, delete
// and reorder (drag the grip, or Alt+Up / Alt+Down from the row).
//
// The board passes the steps it loaded with the task detail; every
// change goes straight to the API and is reflected locally first, so a
// tick never waits on the network. A refused change rolls back and
// says why. After each change the component fires `steps-change`
// with the list it now shows, so the board can update the card count
// without reloading.
//
// Properties: projectId, taskId, steps ([{id, position, text, done}])
// and stepsAt, the performance.now() at which the board requested them.

const gripIcon = html`<svg width="12" height="12" viewBox="0 0 16 16" aria-hidden="true">${svg`
  <circle cx="5.5" cy="3.5" r="1.2" fill="currentColor"/><circle cx="10.5" cy="3.5" r="1.2" fill="currentColor"/>
  <circle cx="5.5" cy="8" r="1.2" fill="currentColor"/><circle cx="10.5" cy="8" r="1.2" fill="currentColor"/>
  <circle cx="5.5" cy="12.5" r="1.2" fill="currentColor"/><circle cx="10.5" cy="12.5" r="1.2" fill="currentColor"/>`}</svg>`;

export class NottarioTaskSteps extends LitElement {
  static properties = {
    projectId: { type: String, attribute: 'project-id' },
    taskId: { type: String },
    steps: { type: Array },
    stepsAt: { type: Number },
    _steps: { state: true },
    _editingId: { state: true },
    _draft: { state: true },
    _adding: { state: true },
    _error: { state: true },
    _announce: { state: true },
    _dragId: { state: true },
    _dropIndex: { state: true },
  };

  static styles = [
    buttonStyles,
    css`
      :host { display: block; }
      *, *::before, *::after { box-sizing: border-box; }

      .eyebrow {
        font-size: 11px;
        text-transform: uppercase;
        letter-spacing: 0.06em;
        color: var(--gray-5);
        font-weight: 600;
        margin: 0 0 6px;
      }
      .eyebrow .count {
        margin-left: 2px;
        text-transform: none;
        letter-spacing: 0;
        font-weight: 500;
        color: var(--fg-muted);
      }

      /* The grip column hangs into the dialog's side padding, so the
         checkboxes line up with the text of the other sections. */
      ol {
        list-style: none;
        margin: 0 0 0 -20px;
        padding: 0;
      }

      /* One row: grip, checkbox, text, delete. The grip and the delete
         button stay quiet until the row is hovered or holds focus, so
         a long list reads as a list, not a toolbar. */
      .step {
        display: grid;
        grid-template-columns: 14px 16px 1fr 28px;
        align-items: start;
        column-gap: 6px;
        padding: 3px 0;
        border-radius: var(--radius-sm);
        position: relative;
      }
      .step:hover { background: var(--bg-subtle); }
      .step.dragging { opacity: 0.4; }
      /* Drop target: a thin accent rule across the gap the step will
         land in. */
      .step.drop-before { box-shadow: inset 0 2px 0 var(--accent); }
      .step.drop-after { box-shadow: inset 0 -2px 0 var(--accent); }

      .grip {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        height: 22px;
        color: var(--gray-4);
        cursor: grab;
        opacity: 0;
      }
      .step:hover .grip,
      .step:focus-within .grip { opacity: 1; }
      .grip:active { cursor: grabbing; }

      input[type="checkbox"] {
        width: 14px;
        height: 14px;
        margin: 4px 0 0 1px;
        accent-color: var(--success);
        cursor: pointer;
      }
      input[type="checkbox"]:focus-visible {
        outline: 2px solid var(--accent);
        outline-offset: 2px;
      }

      /* The text is a button: clicking it edits the step. It reads as
         plain text until hovered. */
      .text {
        display: block;
        width: 100%;
        min-height: 22px;
        padding: 2px 4px;
        margin: 0;
        border: 1px solid transparent;
        border-radius: var(--radius-sm);
        background: none;
        font: inherit;
        font-size: 13px;
        line-height: 18px;
        color: var(--fg);
        text-align: left;
        overflow-wrap: anywhere;
        cursor: text;
      }
      .text:focus-visible {
        outline: 2px solid var(--accent);
        outline-offset: 0;
      }
      .step.done .text {
        color: var(--fg-muted);
        text-decoration: line-through;
        text-decoration-color: var(--gray-4);
      }

      .field {
        width: 100%;
        height: 24px;
        padding: 2px 6px;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        background: var(--bg);
        font: inherit;
        font-size: 13px;
        color: var(--fg);
      }
      .field:focus {
        outline: none;
        border-color: var(--accent);
        box-shadow: 0 0 0 3px var(--tint-blue);
      }
      .step .field { margin-top: -1px; }

      .step .icon-btn {
        width: 24px;
        height: 22px;
        opacity: 0;
      }
      .step:hover .icon-btn,
      .step:focus-within .icon-btn { opacity: 1; }

      /* Touch screens have no hover: keep the controls visible. */
      @media (hover: none) {
        .grip, .step .icon-btn { opacity: 1; }
      }

      /* The add field sits in the text column, under the list, so an
         empty checklist costs one quiet line. */
      .add {
        margin-left: -20px;
        display: grid;
        grid-template-columns: 14px 16px 1fr 28px;
        column-gap: 6px;
        padding: 3px 0;
      }
      .add .plus {
        grid-column: 2;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        height: 24px;
        color: var(--gray-5);
        font-size: 15px;
        line-height: 1;
      }
      .add .field {
        grid-column: 3;
        border-color: transparent;
        background: transparent;
      }
      .add .field:hover { border-color: var(--border); }
      .add .field:focus { background: var(--bg); }
      .add .field::placeholder { color: var(--gray-5); }

      .note,
      .error {
        margin: 4px 0 0 22px;
        font-size: 12px;
      }
      .note { color: var(--fg-muted); }
      .error { color: var(--danger); }

      .sr-only {
        position: absolute;
        width: 1px;
        height: 1px;
        padding: 0;
        margin: -1px;
        overflow: hidden;
        clip: rect(0 0 0 0);
        white-space: nowrap;
        border: 0;
      }
    `,
  ];

  constructor() {
    super();
    this.steps = [];
    this._steps = [];
    this._editingId = null;
    this._draft = '';
    this._adding = false;
    this._error = '';
    this._announce = '';
    this._dragId = null;
    this._dropIndex = null;
    // Writes go out one at a time, in the order the user made them, so
    // a quick tick and untick cannot land on the server swapped.
    this._queue = Promise.resolve();
    this._inflight = 0;
    this._savedAt = 0;
  }

  willUpdate(changed) {
    if (changed.has('taskId')) {
      this._editingId = null;
      this._error = '';
      this._onDragEnd();
    }
    // A reload requested before our last write finished may not hold
    // it, and would undo what the user sees. Take only lists the board
    // asked for after that (stepsAt), with nothing still in flight;
    // until then the list on screen is the newer one.
    const fresh =
      this._inflight === 0 && (this.stepsAt ?? Number.POSITIVE_INFINITY) >= this._savedAt;
    if (changed.has('taskId') || (changed.has('steps') && fresh)) {
      this._steps = [...(this.steps || [])];
    }
    if (this._dragId && !this._steps.some((s) => s.id === this._dragId)) this._onDragEnd();
  }

  // ---- API ----------------------------------------------------------

  _url(stepId = '', suffix = '') {
    const base = `/api/projects/${this.projectId}/tasks/${this.taskId}/steps`;
    return stepId ? `${base}/${stepId}${suffix}` : base;
  }

  async _send(method, url, body) {
    const r = await fetch(url, {
      method,
      headers: body ? { 'Content-Type': 'application/json' } : {},
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!r.ok) {
      const j = await r.json().catch(() => ({}));
      throw new Error(j.error || r.statusText || 'request failed');
    }
    return r.status === 204 ? null : r.json();
  }

  // _apply shows a new list and tells the board.
  _apply(steps) {
    this._steps = steps;
    this.dispatchEvent(
      new CustomEvent('steps-change', { detail: { steps }, bubbles: true, composed: true }),
    );
  }

  // _write shows `next` at once and queues the request behind any
  // earlier one. On failure `revert` undoes this change alone on
  // whatever the list is by then, so other changes that went through
  // stay on screen. `settle` folds the response into the list.
  // Resolves to whether the write was saved.
  _write({ next, request, revert, settle, fail = "Couldn't save" }) {
    this._error = '';
    if (next) this._apply(next);
    this._inflight++;
    const run = this._queue.then(async () => {
      try {
        const res = await request();
        if (settle) this._apply(settle(this._steps, res));
        return true;
      } catch (err) {
        if (revert) this._apply(revert(this._steps));
        this._error = `${fail}: ${err.message}`;
        return false;
      } finally {
        this._inflight--;
        this._savedAt = performance.now();
      }
    });
    this._queue = run;
    return run;
  }

  // _say announces a message, even the same one twice in a row.
  async _say(msg) {
    this._announce = '';
    await this.updateComplete;
    this._announce = msg;
  }

  async _add(texts) {
    const clean = texts.map((t) => t.trim()).filter(Boolean);
    if (!clean.length || this._adding) return false;
    const tooLong = clean.find((t) => t.length > MAX_STEP_LEN);
    if (tooLong) {
      this._error = `A step is one line of up to ${MAX_STEP_LEN} characters. Longer work is a subtask.`;
      return false;
    }
    const room = MAX_STEPS - this._steps.length;
    if (clean.length > room) {
      this._error = `Only ${room} more ${room === 1 ? 'step fits' : 'steps fit'}; a task holds ${MAX_STEPS}.`;
      return false;
    }
    this._adding = true;
    // The new steps are appended to the list as it stands, not swapped
    // for the server's copy, which would drop changes still queued.
    const ok = await this._write({
      request: () => this._send('POST', this._url(), { texts: clean }),
      settle: (cur, j) => {
        const known = new Set(cur.map((s) => s.id));
        const added = (j.steps || []).filter((s) => !known.has(s.id));
        return [...cur, ...added].map((s, i) => ({ ...s, position: i }));
      },
      fail: "Couldn't add",
    });
    this._adding = false;
    if (ok) this._say(clean.length === 1 ? 'Step added.' : `${clean.length} steps added.`);
    return ok;
  }

  _toggle(step, done) {
    const set = (list, value) => list.map((s) => (s.id === step.id ? { ...s, done: value } : s));
    return this._write({
      next: set(this._steps, done),
      request: () => this._send('PATCH', this._url(step.id), { done }),
      revert: (cur) => set(cur, !done),
    });
  }

  async _remove(step) {
    const index = this._steps.findIndex((s) => s.id === step.id);
    const saved = this._write({
      next: this._steps.filter((s) => s.id !== step.id).map((s, i) => ({ ...s, position: i })),
      request: () => this._send('DELETE', this._url(step.id)),
      revert: (cur) => {
        if (cur.some((s) => s.id === step.id)) return cur;
        const out = [...cur];
        out.splice(Math.min(index, out.length), 0, step);
        return out.map((s, i) => ({ ...s, position: i }));
      },
    });
    // Keep the keyboard where it was: the next step, else the previous
    // one, else the add field.
    await this.updateComplete;
    const target = this._steps[index] || this._steps[index - 1];
    this._keepFocus(target ? `#cb-${target.id}` : '.add .field');
    if (await saved) this._say('Step deleted.');
  }

  // _move reorders at once and resolves when the server has it.
  _move(step, to) {
    const from = this._steps.findIndex((s) => s.id === step.id);
    const clamped = Math.max(0, Math.min(to, this._steps.length - 1));
    if (from < 0 || clamped === from) return Promise.resolve(false);
    const saved = this._write({
      next: moveIndex(this._steps, from, clamped),
      request: () => this._send('PUT', this._url(step.id, '/position'), { position: clamped }),
      revert: (cur) => {
        const at = cur.findIndex((s) => s.id === step.id);
        return at < 0 ? cur : moveIndex(cur, at, from);
      },
    });
    saved.then((ok) => {
      if (ok) this._say(`Moved to position ${clamped + 1} of ${this._steps.length}.`);
    });
    return saved;
  }

  // _saveEdit stores the edited text. `refocus` puts the keyboard back
  // on the step, for Enter; a blur means the user went elsewhere.
  async _saveEdit(step, refocus) {
    if (this._editingId !== step.id) return;
    const text = this._draft.trim();
    this._editingId = null;
    // An emptied step keeps its text: deleting is the trash button's
    // job, not a side effect of clearing a field.
    if (text && text !== step.text) {
      if (text.length > MAX_STEP_LEN) {
        this._error = `A step is one line of up to ${MAX_STEP_LEN} characters.`;
        return;
      }
      const set = (list, value) => list.map((s) => (s.id === step.id ? { ...s, text: value } : s));
      this._write({
        next: set(this._steps, text),
        request: () => this._send('PATCH', this._url(step.id), { text }),
        revert: (cur) => set(cur, step.text),
      });
    }
    if (refocus) {
      await this.updateComplete;
      this._focus(`#text-${step.id}`);
    }
  }

  async _beginEdit(step) {
    this._editingId = step.id;
    this._draft = step.text;
    this._error = '';
    await this.updateComplete;
    const input = this.renderRoot.querySelector(`#edit-${step.id}`);
    input?.focus();
    input?.select();
  }

  _focus(selector) {
    this.renderRoot.querySelector(selector)?.focus();
  }

  // _keepFocus moves focus only when it is still inside the list or was
  // lost with a removed row, never when the user has moved on.
  _keepFocus(selector) {
    const page = document.activeElement;
    if (this.renderRoot.activeElement || !page || page === document.body) this._focus(selector);
  }

  // ---- Keyboard and pointer -----------------------------------------

  async _onRowKey(e, step, index) {
    if (!e.altKey || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown')) return;
    e.preventDefault();
    // Rows are rendered by index, so after the move this element shows
    // another step: remember which control of which step had focus and
    // find it again by id, as soon as the list is redrawn, so a second
    // Alt+Arrow acts on the same step.
    const kind = e.target.id.startsWith('cb-') ? 'cb' : 'text';
    this._move(step, index + (e.key === 'ArrowUp' ? -1 : 1));
    await this.updateComplete;
    this._keepFocus(`#${kind}-${step.id}`);
  }

  _onEditKey(e, step) {
    if (e.key === 'Enter') {
      e.preventDefault();
      this._saveEdit(step, true);
    } else if (e.key === 'Escape') {
      // Cancel the edit only; the dialog must not close with it.
      e.preventDefault();
      e.stopPropagation();
      this._editingId = null;
      this.updateComplete.then(() => this._focus(`#text-${step.id}`));
    }
  }

  async _onAddKey(e) {
    if (e.key === 'Escape' && e.target.value) {
      e.stopPropagation();
      e.target.value = '';
      return;
    }
    if (e.key !== 'Enter' || e.isComposing) return;
    e.preventDefault();
    const field = e.target;
    if (await this._add([field.value])) field.value = '';
  }

  // Several pasted lines become several steps, list markers stripped.
  // A refused paste leaves the field as it was; the clipboard still
  // holds the list.
  async _onAddPaste(e) {
    const text = e.clipboardData?.getData('text') || '';
    if (!/[\r\n]/.test(text)) return;
    e.preventDefault();
    const field = e.target;
    const texts = stepTextsFromPaste(text);
    if (field.value.trim()) texts.unshift(field.value.trim());
    if (await this._add(texts)) field.value = '';
  }

  _onDragStart(e, step) {
    e.stopPropagation();
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', step.text);
    const row = e.target.closest('.step');
    if (row) e.dataTransfer.setDragImage(row, 12, 12);
    this._dragId = step.id;
  }

  _onDragOver(e, index) {
    if (!this._dragId) return;
    e.preventDefault();
    e.stopPropagation();
    const rect = e.currentTarget.getBoundingClientRect();
    const after = e.clientY > rect.top + rect.height / 2;
    this._dropIndex = index + (after ? 1 : 0);
  }

  _onDrop(e) {
    if (!this._dragId) return;
    e.preventDefault();
    e.stopPropagation();
    const step = this._steps.find((s) => s.id === this._dragId);
    const from = this._steps.indexOf(step);
    // _dropIndex is a gap (0..n); removing the step first shifts every
    // gap below it up by one.
    const to = this._dropIndex > from ? this._dropIndex - 1 : this._dropIndex;
    this._onDragEnd();
    if (step && to !== null) this._move(step, to);
  }

  _onDragEnd() {
    this._dragId = null;
    this._dropIndex = null;
  }

  // ---- Render -------------------------------------------------------

  _renderStep(step, index) {
    const editing = this._editingId === step.id;
    const dragging = this._dragId === step.id;
    const dropClass =
      this._dragId && !dragging
        ? this._dropIndex === index
          ? ' drop-before'
          : this._dropIndex === index + 1 && index === this._steps.length - 1
            ? ' drop-after'
            : ''
        : '';
    return html`
      <li class=${`step${step.done ? ' done' : ''}${dragging ? ' dragging' : ''}${dropClass}`}
          @dragover=${(e) => this._onDragOver(e, index)}
          @drop=${(e) => this._onDrop(e)}>
        <span class="grip" draggable="true" title="Drag to reorder" aria-hidden="true"
              @dragstart=${(e) => this._onDragStart(e, step)}
              @dragend=${() => this._onDragEnd()}>${gripIcon}</span>
        <input type="checkbox" id=${`cb-${step.id}`}
               .checked=${step.done}
               aria-label=${step.text}
               aria-describedby="steps-help"
               @change=${(e) => this._toggle(step, e.target.checked)}
               @keydown=${(e) => this._onRowKey(e, step, index)}>
        ${
          editing
            ? html`<input class="field" id=${`edit-${step.id}`}
                     aria-label="Step text"
                     maxlength=${MAX_STEP_LEN}
                     .value=${this._draft}
                     @input=${(e) => {
                       this._draft = e.target.value;
                     }}
                     @keydown=${(e) => this._onEditKey(e, step)}
                     @blur=${() => this._saveEdit(step, false)}>`
            : html`<button class="text" id=${`text-${step.id}`}
                     title="Edit step"
                     aria-label=${`Edit step: ${step.text}`}
                     aria-describedby="steps-help"
                     @click=${() => this._beginEdit(step)}
                     @keydown=${(e) => this._onRowKey(e, step, index)}>${step.text}</button>`
        }
        <button class="icon-btn danger" title="Delete step" aria-label=${`Delete step: ${step.text}`}
                @click=${() => this._remove(step)}>${trashIcon(13)}</button>
      </li>
    `;
  }

  render() {
    const steps = this._steps;
    const { done, total } = stepProgress(steps);
    const full = total >= MAX_STEPS;
    return html`
      <h4 class="eyebrow" id="steps-title">
        Steps${total ? html` <span class="count">${done} of ${total} done</span>` : null}
      </h4>
      <span class="sr-only" id="steps-help">Alt+Up and Alt+Down move a step.</span>
      ${
        total
          ? html`<ol aria-labelledby="steps-title"
                     @dragleave=${(e) => {
                       if (!e.currentTarget.contains(e.relatedTarget)) this._dropIndex = null;
                     }}>${steps.map((s, i) => this._renderStep(s, i))}</ol>`
          : null
      }
      ${
        full
          ? html`<p class="note">${MAX_STEPS} steps is the limit. Split the rest into subtasks.</p>`
          : html`
            <div class="add">
              <span class="plus" aria-hidden="true">+</span>
              <input class="field"
                     aria-label="Add a step"
                     placeholder=${total ? 'Add a step' : 'Add a step, or paste a list'}
                     maxlength=${MAX_STEP_LEN}
                     ?readonly=${this._adding}
                     aria-busy=${this._adding ? 'true' : 'false'}
                     @keydown=${(e) => this._onAddKey(e)}
                     @paste=${(e) => this._onAddPaste(e)}>
            </div>
          `
      }
      ${this._error ? html`<p class="error" role="alert">${this._error}</p>` : null}
      <span class="sr-only" role="status" aria-live="polite">${this._announce}</span>
    `;
  }
}

customElements.define('nottario-task-steps', NottarioTaskSteps);
