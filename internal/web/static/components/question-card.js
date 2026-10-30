import { LitElement, html, css, nothing } from '/static/vendor/lit/lit.js';
import { badgeStyles } from '/static/components/badges.js';
import { buttonStyles } from '/static/components/buttons.js';
import { toast } from '/static/components/toast.js';
import '/static/components/avatar.js';
import '/static/components/markdown.js';
import '/static/components/md-editor.js';
import { resumeCommand, sessionName } from '/static/live-model.js';
import { formatDateTime, formatRelativeTime } from '/static/time.js';
import { copyToClipboard } from '/static/utils/clipboard.js';

// <nottario-question-card> shows one question an agent filed for a
// human, with the form to answer it in place. Shared by the Questions
// page, the Live view's "Waiting for you" block and the task dialog.
//
// Properties:
//   question  the question view from the API (GET /api/questions, the
//             task detail's questions, or an answer response)
//   projectId the project the card lives in. Global views leave it
//             empty: the card then answers through question.project.id
//             and, with showTask, leads the header with a project chip.
//   compact   no frame and a header folded to one line (state, task
//             link, age), for Live rows and the task dialog, where the
//             surrounding row is the frame
//   showTask  name the task (as a link to it) in the header
//
// Parts: card, the frame, focusable by script (focus() on the element
// moves there); a list that draws its own separators (the
// Questions page) drops the border and padding through ::part(card).
//
// Events (bubbling, composed):
//   question-answered  { question }        the answer was saved; detail
//                                          carries the updated view. The
//                                          card already replaced its own
//                                          question with it; parents
//                                          should replace their copy too,
//                                          or their next render puts the
//                                          stale one back
//   question-stale     { question, code }  the server refused the answer
//                                          because the question moved on
//                                          (409: already_delivered or
//                                          withdrawn); the parent should
//                                          reload it, as there is no
//                                          single-question endpoint
//
// Keyboard: Cmd/Ctrl+Enter submits from anywhere in the card; with
// focus inside a choice question, keys 1-9 pick the matching option
// (ignored while typing in a text field).

let nextId = 0;

// One shared 15-second tick refreshes every card's relative times.
const cards = new Set();
let ticker = null;
function watch(card) {
  cards.add(card);
  if (ticker) return;
  ticker = setInterval(() => {
    for (const c of cards) c.requestUpdate();
  }, 15_000);
}
function unwatch(card) {
  cards.delete(card);
  if (cards.size === 0 && ticker) {
    clearInterval(ticker);
    ticker = null;
  }
}

// typingTarget reports whether a keydown comes from a place where
// digits are text, not shortcuts.
function typingTarget(el) {
  if (!el) return false;
  if (el.isContentEditable) return true;
  if (el.tagName === 'TEXTAREA' || el.tagName === 'SELECT') return true;
  if (el.tagName === 'INPUT') return !['radio', 'checkbox', 'button', 'submit'].includes(el.type);
  return false;
}

export class NottarioQuestionCard extends LitElement {
  static properties = {
    question: { type: Object },
    projectId: { type: String, attribute: 'project-id' },
    compact: { type: Boolean, reflect: true },
    showTask: { type: Boolean, attribute: 'show-task' },
    _editing: { state: true },
    _keys: { state: true },
    _other: { state: true },
    _otherText: { state: true },
    _text: { state: true },
    _note: { state: true },
    _noteOpen: { state: true },
    _saving: { state: true },
    _error: { state: true },
  };

  static styles = [
    buttonStyles,
    badgeStyles,
    css`
      :host {
        display: block;
        color: var(--fg);
        font-size: 14px;
      }
      *, *::before, *::after { box-sizing: border-box; }

      .card {
        border: 1px solid var(--border);
        border-radius: var(--radius);
        background: var(--bg);
        padding: 12px 16px;
      }
      :host([compact]) .card {
        border: 0;
        border-radius: 0;
        padding: 0;
        background: transparent;
      }
      .card.withdrawn { opacity: 0.6; }

      /* Header: who asked about what, and when. One wrapping line of
         muted metadata, the task title in the foreground. */
      header {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 4px 8px;
        font-size: 12px;
        line-height: 20px;
        color: var(--fg-muted);
        margin-bottom: 8px;
      }
      :host([compact]) header { margin-bottom: 6px; }
      header.folded { flex-wrap: nowrap; min-width: 0; }
      header.folded .task {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
      header.folded .state,
      header.folded time { flex: none; }
      header .sep { color: var(--gray-4); }
      header .task {
        color: var(--fg);
        font-size: 13px;
        font-weight: 600;
        text-decoration: none;
        overflow-wrap: anywhere;
      }
      header .task:hover { color: var(--accent); text-decoration: underline; }
      header .task:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; border-radius: 2px; }
      header .step { overflow-wrap: anywhere; }
      header .session { font-family: var(--mono); font-size: 11px; }
      header time { margin-left: auto; white-space: nowrap; }

      .project {
        display: inline-block;
        padding: 0 8px;
        border: 1px solid var(--border);
        border-radius: 999px;
        background: var(--bg-subtle);
        color: var(--fg);
        font-size: 11px;
        font-weight: 500;
        line-height: 18px;
      }

      /* State pill: amber only while the question waits on a person. */
      .state {
        display: inline-block;
        padding: 0 7px;
        border-radius: 2em;
        border: 1px solid var(--border);
        font-size: 11px;
        font-weight: 500;
        line-height: 18px;
        color: var(--fg-muted);
        background: var(--bg-subtle);
      }
      .state.open {
        color: var(--attention-fg);
        background: var(--attention-subtle);
        border-color: var(--attention-muted);
      }
      .state.delivered {
        color: var(--tint-green-fg);
        background: var(--tint-green);
        border-color: rgba(26, 127, 55, 0.3);
      }

      /* The header spans the card, its age on the right; what follows
         is capped to a readable measure. */
      .content { max-width: 75ch; }
      .card:focus { outline: none; }
      .body { margin-bottom: 12px; }
      :host([compact]) .body nottario-markdown { font-size: 13px; }
      .excerpt { margin: 0; overflow-wrap: anywhere; }

      /* Choice options: native inputs with their labels, one row per
         option, the description and the recommendation reason muted
         underneath. */
      .options {
        display: flex;
        flex-direction: column;
        gap: 2px;
        margin: 0 0 8px;
      }
      .opt {
        display: grid;
        grid-template-columns: 16px 1fr;
        column-gap: 8px;
        padding: 6px 8px;
        border-radius: var(--radius);
        cursor: pointer;
      }
      .opt:hover { background: var(--bg-subtle); }
      .opt input {
        width: 14px;
        height: 14px;
        margin: 3px 0 0;
        accent-color: var(--accent);
        cursor: pointer;
      }
      .opt input:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
      .opt-main { min-width: 0; line-height: 20px; overflow-wrap: anywhere; }
      .opt-key {
        font-family: var(--mono);
        font-size: 11px;
        color: var(--fg-subtle);
        margin-right: 6px;
      }
      .opt-label { font-weight: 500; }
      .opt-main .badge { margin-left: 6px; }
      .badge.recommended {
        background: var(--tint-blue);
        border-color: var(--tint-blue-border);
        color: var(--tint-blue-fg);
      }
      .opt-desc,
      .opt-reason {
        grid-column: 2;
        margin: 2px 0 0;
        font-size: 12px;
        line-height: 18px;
        color: var(--fg-muted);
      }
      .opt-reason { font-style: italic; }

      /* The Other field sits under its option, aligned with the labels. */
      .other .field {
        display: block;
        width: calc(100% - 32px);
        margin: 2px 0 4px 32px;
      }
      .field {
        width: 100%;
        height: 28px;
        padding: 3px 8px;
        border: 1px solid var(--border);
        border-radius: var(--radius);
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

      .note { margin: 4px 0 8px; }
      .note-toggle {
        padding: 0;
        border: 0;
        background: none;
        font: inherit;
        font-size: 12px;
        color: var(--accent);
        cursor: pointer;
      }
      .note-toggle:hover { text-decoration: underline; }
      .note-toggle:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 2px; }

      .actions {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 8px;
        margin-top: 8px;
      }
      .actions .grow,
      .status .grow { flex: 1; }
      .hint { font-size: 11px; color: var(--fg-muted); }
      .hint kbd {
        font-family: var(--mono);
        font-size: 10px;
        padding: 1px 5px;
        border-radius: 3px;
        border: 1px solid var(--border);
        background: var(--bg);
        color: var(--fg);
      }
      .error {
        margin: 8px 0 0;
        font-size: 12px;
        color: var(--danger);
      }

      /* The answer, once given. */
      .answer {
        border-top: 1px solid var(--border-muted);
        padding-top: 10px;
      }
      .answer-meta {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 4px 8px;
        font-size: 12px;
        line-height: 20px;
        color: var(--fg-muted);
        margin-bottom: 6px;
      }
      .answer-meta nottario-avatar { vertical-align: middle; }
      .answer-meta .who { color: var(--fg); font-weight: 500; }
      .picked {
        margin: 0 0 6px;
        padding: 0;
        list-style: none;
      }
      .picked li { line-height: 22px; overflow-wrap: anywhere; }
      .answer-note { margin-top: 6px; }
      .answer-note .label {
        display: block;
        font-size: 12px;
        color: var(--fg-muted);
        margin-bottom: 2px;
      }
      .status {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: 8px;
        margin-top: 8px;
        font-size: 12px;
        color: var(--fg-muted);
      }
      .status .btn { height: 26px; padding: 0 10px; font-size: 12px; }
      .withdrawn-reason {
        margin: 0;
        font-size: 13px;
        color: var(--fg-muted);
        overflow-wrap: anywhere;
      }
    `,
  ];

  constructor() {
    super();
    this.question = null;
    this.projectId = '';
    this.compact = false;
    this.showTask = false;
    this._uid = `q${++nextId}`;
    this._formFor = null;
    this._resetForm(null);
    this.addEventListener('keydown', (e) => this._onKeydown(e));
  }

  // focus() lands on the card's frame (the article, which carries the
  // question as its name), so lists can move focus between questions
  // and keys 1-9 reach the card without the host being a tab stop.
  focus(options) {
    const article = this.renderRoot?.querySelector('article');
    if (article) article.focus(options);
    else super.focus(options);
  }

  connectedCallback() {
    super.connectedCallback();
    watch(this);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    unwatch(this);
  }

  willUpdate(changed) {
    // A different question starts from a clean form. The same question
    // refreshed by the parent keeps whatever is being typed.
    if (changed.has('question') && this.question?.id !== this._formFor) {
      this._formFor = this.question?.id ?? null;
      this._editing = false;
      this._resetForm(null);
    }
  }

  // _resetForm empties the form, or fills it from an answer to edit.
  _resetForm(answer) {
    const keys = answer?.keys || [];
    const text = answer?.text || '';
    const note = answer?.note || '';
    const choice = this.question?.kind === 'choice';
    this._keys = choice ? [...keys] : [];
    this._other = choice && text !== '';
    this._otherText = choice ? text : '';
    this._text = choice ? '' : text;
    this._note = note;
    this._noteOpen = note !== '';
    this._saving = false;
    this._error = '';
  }

  get _pid() {
    return this.projectId || this.question?.project?.id || '';
  }

  get _formOpen() {
    const q = this.question;
    if (!q) return false;
    return q.state === 'open' || (this._editing && q.state === 'answered' && !q.delivered_at);
  }

  // ---- input ----

  _onKeydown(e) {
    if (!this._formOpen || this._saving) return;
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      // The md-editor already turned this into a submit event.
      if (e.defaultPrevented) return;
      e.preventDefault();
      this._submit();
      return;
    }
    if (this.question.kind !== 'choice') return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (!/^[1-9]$/.test(e.key)) return;
    if (typingTarget(e.composedPath()[0])) return;
    const index = Number(e.key) - 1;
    const opt = this.question.options?.[index];
    if (!opt) return;
    e.preventDefault();
    this._pick(opt.key);
    this.updateComplete.then(() => {
      this.renderRoot.querySelector(`input[data-index="${index}"]`)?.focus();
    });
  }

  _pick(key) {
    if (this.question.multi) {
      this._keys = this._keys.includes(key)
        ? this._keys.filter((k) => k !== key)
        : [...this._keys, key];
    } else {
      this._keys = [key];
      this._other = false;
    }
    this._error = '';
  }

  _pickOther(checked) {
    if (this.question.multi) {
      this._other = checked;
    } else {
      this._other = true;
      this._keys = [];
    }
    this._error = '';
    if (this._other) {
      this.updateComplete.then(() => this.renderRoot.querySelector('.field')?.focus());
    }
  }

  async _openNote() {
    this._noteOpen = true;
    await this.updateComplete;
    const editor = this.renderRoot.querySelector('.note nottario-md-editor');
    if (!editor) return;
    await editor.updateComplete;
    editor.focus();
  }

  _closeNote() {
    if (this._note.trim() !== '') return;
    this._noteOpen = false;
  }

  _startEdit() {
    this._resetForm(this.question.answer);
    this._editing = true;
  }

  _cancelEdit() {
    this._editing = false;
    this._resetForm(null);
  }

  _payload() {
    const q = this.question;
    if (q.kind !== 'choice') return { text: this._text.trim() };
    // Options keep their order on the question, not the click order.
    const order = (q.options || []).map((o) => o.key);
    const keys = order.filter((k) => this._keys.includes(k));
    const text = this._other ? this._otherText.trim() : '';
    return { keys, text, note: this._noteOpen ? this._note.trim() : '' };
  }

  _validate(body) {
    if (this.question.kind !== 'choice') return body.text ? '' : 'Write an answer first.';
    if (this._other && !body.text) return 'Write your answer in the Other field.';
    if (body.keys.length === 0 && !body.text) return 'Pick an option or write an answer.';
    return '';
  }

  async _copyResume() {
    const cmd = resumeCommand(this.question?.session);
    if (!cmd) return;
    if (await copyToClipboard(cmd)) toast.success('Copied');
    else toast.error('Could not copy to the clipboard.');
  }

  async _submit() {
    const q = this.question;
    if (!q || this._saving) return;
    const body = this._payload();
    const problem = this._validate(body);
    if (problem) {
      this._error = problem;
      return;
    }
    const pid = this._pid;
    if (!pid) {
      this._error = 'This question has no project to answer it in.';
      return;
    }
    this._saving = true;
    this._error = '';
    const method = q.state === 'answered' ? 'PUT' : 'POST';
    let res;
    let data = {};
    try {
      res = await fetch(`/api/projects/${pid}/questions/${q.id}/answer`, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      data = await res.json().catch(() => ({}));
    } catch (_) {
      this._saving = false;
      this._error = 'Could not reach the server. Your answer is still here; try again.';
      return;
    }
    this._saving = false;
    if (res.ok && data.question) {
      this._editing = false;
      this.question = data.question;
      this._formFor = data.question.id;
      this._resetForm(null);
      this.dispatchEvent(
        new CustomEvent('question-answered', {
          detail: { question: data.question },
          bubbles: true,
          composed: true,
        }),
      );
      return;
    }
    if (res.status === 409) {
      toast.error(data.error || 'This question changed in the meantime.');
      this._editing = false;
      this.dispatchEvent(
        new CustomEvent('question-stale', {
          detail: { question: q, code: data.code || '' },
          bubbles: true,
          composed: true,
        }),
      );
      return;
    }
    this._error = data.error || `Could not save the answer (HTTP ${res.status}).`;
  }

  // ---- render ----

  render() {
    const q = this.question;
    if (!q) return nothing;
    return html`
      <article class="card ${q.state}" part="card" tabindex="-1"
               aria-label=${q.excerpt || 'Question'}>
        ${this._renderHeader(q)}
        <div class="content">
          <div class="body">
            ${
              q.body_html
                ? html`<nottario-markdown .html=${q.body_html}></nottario-markdown>`
                : html`<p class="excerpt">${q.excerpt}</p>`
            }
          </div>
          ${this._renderState(q)}
        </div>
      </article>
    `;
  }

  _renderHeader(q) {
    if (this.compact) return this._renderFoldedHeader(q);
    const parts = [];
    if (this.showTask && !this.projectId && q.project) {
      parts.push(html`<span class="project">${q.project.name}</span>`);
    }
    if (this.showTask && q.task) {
      const href = `/projects/${this._pid}/board/kanban#task=${q.task.id}`;
      parts.push(html`<a class="task" href=${href}>${q.task.title}</a>`);
    }
    if (q.step) parts.push(html`<span class="step">${q.step.text}</span>`);
    if (q.session) {
      parts.push(
        html`<span class="session" title=${q.session.key || ''}>${sessionName(q.session)}</span>`,
      );
    }
    const joined = parts.flatMap((p, i) =>
      i === 0 ? [p] : [html`<span class="sep" aria-hidden="true">·</span>`, p],
    );
    return html`
      <header>
        ${this._renderStatePill(q)}
        ${joined}
        <time datetime=${q.created_at} title=${formatDateTime(q.created_at)}>
          ${formatRelativeTime(q.created_at)}
        </time>
      </header>
    `;
  }

  // _renderFoldedHeader is the one-line header of compact cards: the
  // state, the task (when shown) and the age; the row around the card
  // already says the rest.
  _renderFoldedHeader(q) {
    const href = q.task ? `/projects/${this._pid}/board/kanban#task=${q.task.id}` : '';
    return html`
      <header class="folded">
        ${this._renderStatePill(q)}
        ${this.showTask && q.task ? html`<a class="task" href=${href}>${q.task.title}</a>` : nothing}
        <time datetime=${q.created_at} title=${formatDateTime(q.created_at)}>
          ${formatRelativeTime(q.created_at)}
        </time>
      </header>
    `;
  }

  _renderStatePill(q) {
    if (q.state === 'open') return html`<span class="state open">Waiting for answer</span>`;
    if (q.state === 'withdrawn') return html`<span class="state">Withdrawn</span>`;
    if (q.delivered_at) return html`<span class="state delivered">Delivered</span>`;
    return html`<span class="state">Answered</span>`;
  }

  _renderState(q) {
    if (q.state === 'withdrawn') {
      return html`<p class="withdrawn-reason">
        ${q.withdrawn_reason ? `Withdrawn: ${q.withdrawn_reason}` : 'Withdrawn'}
      </p>`;
    }
    if (this._formOpen) return this._renderForm(q);
    if (q.state === 'answered') return this._renderAnswer(q);
    return nothing;
  }

  _renderForm(q) {
    const editing = q.state === 'answered';
    const mac = typeof navigator !== 'undefined' && navigator.platform.includes('Mac');
    return html`
      ${q.kind === 'choice' ? this._renderChoices(q) : this._renderTextEditor()}
      ${this._error ? html`<p class="error" role="alert">${this._error}</p>` : nothing}
      <div class="actions">
        <span class="hint grow"><kbd>${mac ? '⌘' : 'Ctrl'}</kbd>+<kbd>Enter</kbd> to answer</span>
        ${
          editing
            ? html`<button class="btn secondary" type="button" ?disabled=${this._saving}
                     @click=${this._cancelEdit}>Cancel</button>`
            : nothing
        }
        <button class="btn primary" type="button" ?disabled=${this._saving} @click=${this._submit}>
          ${this._saving ? 'Saving…' : editing ? 'Update answer' : 'Answer'}
        </button>
      </div>
    `;
  }

  _renderTextEditor() {
    return html`
      <nottario-md-editor
        bare
        no-autofocus
        project-id=${this._pid}
        placeholder="Write your answer…"
        .value=${this._text}
        ?saving=${this._saving}
        @input=${(e) => {
          this._text = e.target.value;
          this._error = '';
        }}
        @submit=${() => this._submit()}
        @cancel=${() => this._editing && this._cancelEdit()}></nottario-md-editor>
    `;
  }

  _renderChoices(q) {
    const multi = !!q.multi;
    const type = multi ? 'checkbox' : 'radio';
    const name = `${this._uid}-opt`;
    const options = q.options || [];
    return html`
      <div class="options"
           role=${multi ? 'group' : 'radiogroup'}
           aria-label=${q.excerpt || 'Answer options'}>
        ${options.map((o, i) => {
          const recommended = o.key === q.recommended_key;
          return html`
            <label class="opt">
              <input type=${type} name=${name} value=${o.key} data-index=${i}
                     .checked=${this._keys.includes(o.key)}
                     ?disabled=${this._saving}
                     @change=${() => this._pick(o.key)}>
              <span class="opt-main">
                <span class="opt-key">${o.key}</span><span class="opt-label">${o.label}</span>
                ${recommended ? html`<span class="badge recommended">Recommended</span>` : nothing}
              </span>
              ${o.description ? html`<span class="opt-desc">${o.description}</span>` : nothing}
              ${
                recommended && q.recommended_reason
                  ? html`<span class="opt-reason">${q.recommended_reason}</span>`
                  : nothing
              }
            </label>
          `;
        })}
        <div class="other">
        <label class="opt">
          <input type=${type} name=${name} value="" data-other
                 .checked=${this._other}
                 ?disabled=${this._saving}
                 @change=${(e) => this._pickOther(e.target.checked)}>
          <span class="opt-main"><span class="opt-label">Other</span></span>
        </label>
          ${
            this._other
              ? html`<input class="field" type="text" aria-label="Other answer"
                       placeholder="Your answer"
                       .value=${this._otherText}
                       ?disabled=${this._saving}
                       @input=${(e) => {
                         this._otherText = e.target.value;
                         this._error = '';
                       }}
                       @keydown=${(e) => {
                         if (e.key === 'Enter' && !e.metaKey && !e.ctrlKey) {
                           e.preventDefault();
                           this._submit();
                         }
                       }}>`
              : nothing
          }
        </div>
      </div>
      <div class="note">
        ${
          this._noteOpen
            ? html`<nottario-md-editor
                     bare
                     no-autofocus
                     project-id=${this._pid}
                     placeholder="Add a note for the agent…"
                     .value=${this._note}
                     ?saving=${this._saving}
                     @input=${(e) => {
                       this._note = e.target.value;
                     }}
                     @submit=${() => this._submit()}
                     @cancel=${() => this._closeNote()}></nottario-md-editor>`
            : html`<button class="note-toggle" type="button" @click=${this._openNote}>Add a note</button>`
        }
      </div>
    `;
  }

  _renderAnswer(q) {
    const a = q.answer || { keys: [], text: '', note: '' };
    const by = q.answered_by;
    const cmd = resumeCommand(q.session);
    return html`
      <div class="answer">
        <div class="answer-meta">
          ${
            by
              ? html`<nottario-avatar size="16" .src=${by.avatar_url || ''}
                       .name=${by.display_name || by.login}></nottario-avatar>
                     <span class="who">${by.display_name || by.login}</span>`
              : nothing
          }
          <span title=${formatDateTime(q.answered_at)}>answered ${formatRelativeTime(q.answered_at)}</span>
        </div>
        ${q.kind === 'choice' ? this._renderPicked(q, a) : this._renderAnswerText(q, a)}
        ${
          a.note
            ? html`<div class="answer-note">
                <span class="label">Note</span>
                ${
                  q.answer_note_html
                    ? html`<nottario-markdown .html=${q.answer_note_html}></nottario-markdown>`
                    : html`<p class="excerpt">${a.note}</p>`
                }
              </div>`
            : nothing
        }
        ${
          q.delivered_at
            ? html`<div class="status">
                <span title=${formatDateTime(q.delivered_at)}>Delivered ${formatRelativeTime(q.delivered_at)}</span>
              </div>`
            : html`<div class="status">
                <span class="grow">Waiting for the agent to pick it up</span>
                ${
                  cmd
                    ? html`<button class="btn secondary" type="button" title=${cmd}
                             @click=${this._copyResume}>Copy resume command</button>`
                    : nothing
                }
                <button class="btn ghost" type="button" @click=${this._startEdit}>Edit</button>
              </div>`
        }
      </div>
    `;
  }

  _renderPicked(q, a) {
    const byKey = new Map((q.options || []).map((o) => [o.key, o]));
    return html`
      <ul class="picked">
        ${(a.keys || []).map((k) => {
          const o = byKey.get(k);
          return html`<li><span class="opt-key">${k}</span><span class="opt-label">${o ? o.label : k}</span></li>`;
        })}
        ${a.text ? html`<li><span class="opt-key">Other</span>${a.text}</li>` : nothing}
      </ul>
    `;
  }

  _renderAnswerText(q, a) {
    if (q.answer_text_html)
      return html`<nottario-markdown .html=${q.answer_text_html}></nottario-markdown>`;
    return html`<p class="excerpt">${a.text}</p>`;
  }
}

customElements.define('nottario-question-card', NottarioQuestionCard);
