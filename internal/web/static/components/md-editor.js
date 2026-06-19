import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { buttonStyles } from '/static/components/buttons.js';
import '/static/components/markdown.js';

// <nottario-md-editor> is the GitHub-style markdown editor used for
// task descriptions and comment bodies. Two tabs (Write / Preview),
// no formatting toolbar, auto-resizing textarea. Designed to be
// embedded inline where the source text used to render — not a modal.
//
// Props:
//   .value         current buffer (string)
//   .placeholder   placeholder shown when empty
//   .disabled      blocks interaction without dimming everything
//   .saving        same as disabled but also flips the submit label
//   .projectId     forwarded to <nottario-markdown> so [[chips]] resolve
//
// Events:
//   submit  CustomEvent<{ value: string }>  — Save or Ctrl/Cmd+Enter
//   cancel  CustomEvent                     — Cancel or Esc
//
// The component does NOT call the network itself. The parent owns the
// PATCH/POST flow and pipes `saving` back in while it's in flight.
class NottarioMdEditor extends LitElement {
  static properties = {
    value: { type: String },
    placeholder: { type: String },
    disabled: { type: Boolean },
    saving: { type: Boolean },
    projectId: { type: String, attribute: 'project-id' },
    _tab: { state: true },
  };

  static styles = [
    buttonStyles,
    css`
      :host {
        display: block;
        box-sizing: border-box;
      }
      * { box-sizing: border-box; }

      .frame {
        border: 1px solid var(--border);
        border-radius: 6px;
        background: #fff;
        overflow: hidden;
      }
      .tabs {
        display: flex;
        gap: 0;
        padding: 6px 6px 0;
        background: var(--bg-subtle);
        border-bottom: 1px solid var(--border);
      }
      .tab {
        appearance: none;
        background: transparent;
        border: 1px solid transparent;
        border-bottom: none;
        padding: 6px 12px;
        font: inherit;
        font-size: 12px;
        color: var(--fg-muted);
        cursor: pointer;
        border-radius: 6px 6px 0 0;
        margin-bottom: -1px;
      }
      .tab[aria-selected="true"] {
        background: #fff;
        color: var(--fg);
        border-color: var(--border);
      }
      .tab:focus-visible {
        outline: 2px solid var(--accent);
        outline-offset: 1px;
      }

      .pane {
        padding: 10px 12px;
      }
      textarea {
        width: 100%;
        min-height: 6em;
        max-height: 30em;
        resize: none;
        border: 0;
        outline: 0;
        padding: 0;
        font: inherit;
        font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
        font-size: 13px;
        line-height: 1.5;
        color: var(--fg);
        background: transparent;
      }
      textarea::placeholder { color: var(--fg-muted); opacity: 0.7; }

      .preview-empty {
        color: var(--fg-muted);
        font-style: italic;
        font-size: 13px;
      }

      .actions {
        display: flex;
        align-items: center;
        gap: 10px;
        padding: 8px 12px;
        border-top: 1px solid var(--border);
        background: var(--bg-subtle);
      }
      .hint {
        flex: 1;
        font-size: 11px;
        color: var(--fg-muted);
      }
      .hint kbd {
        font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace;
        font-size: 10px;
        padding: 1px 5px;
        border-radius: 3px;
        border: 1px solid var(--border);
        background: #fff;
        color: var(--fg);
      }
    `,
  ];

  constructor() {
    super();
    this.value = '';
    this.placeholder = '';
    this.disabled = false;
    this.saving = false;
    this.projectId = '';
    this._tab = 'write';
  }

  // Focus the textarea after first render so the editor is keyboard-
  // ready as soon as it swaps in. Re-focus on tab change back to Write.
  firstUpdated() {
    this._focusTextarea();
  }

  updated(changed) {
    if (changed.has('_tab') && this._tab === 'write') {
      this._focusTextarea();
    }
  }

  _focusTextarea() {
    const ta = this.renderRoot?.querySelector('textarea');
    if (ta) {
      ta.focus();
      // Move caret to end so re-opens after a 409 don't dump the user
      // back at position 0 in the middle of their prose.
      const len = ta.value.length;
      ta.setSelectionRange(len, len);
      this._autoresize(ta);
    }
  }

  _autoresize(ta) {
    // Auto-grow up to max-height, then let the textarea scroll
    // internally. Reset height to 'auto' first so it can shrink too.
    ta.style.height = 'auto';
    ta.style.height = ta.scrollHeight + 'px';
  }

  _onInput(e) {
    this.value = e.target.value;
    this._autoresize(e.target);
  }

  _onKeyDown(e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      this._emitCancel();
      return;
    }
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      this._emitSubmit();
    }
  }

  _selectTab(tab) {
    this._tab = tab;
  }

  _emitSubmit() {
    if (this.disabled || this.saving) return;
    this.dispatchEvent(
      new CustomEvent('submit', {
        detail: { value: this.value },
        bubbles: true,
        composed: true,
      }),
    );
  }

  _emitCancel() {
    if (this.saving) return;
    this.dispatchEvent(new CustomEvent('cancel', { bubbles: true, composed: true }));
  }

  render() {
    const writeSelected = this._tab === 'write';
    return html`
      <div class="frame">
        <div class="tabs" role="tablist">
          <button class="tab" role="tab"
                  type="button"
                  aria-selected=${writeSelected ? 'true' : 'false'}
                  ?disabled=${this.saving}
                  @click=${() => this._selectTab('write')}>Write</button>
          <button class="tab" role="tab"
                  type="button"
                  aria-selected=${!writeSelected ? 'true' : 'false'}
                  ?disabled=${this.saving}
                  @click=${() => this._selectTab('preview')}>Preview</button>
        </div>
        <div class="pane">
          ${
            writeSelected
              ? html`<textarea
                       .value=${this.value}
                       placeholder=${this.placeholder}
                       ?disabled=${this.disabled || this.saving}
                       @input=${this._onInput}
                       @keydown=${this._onKeyDown}></textarea>`
              : this.value
                ? html`<nottario-markdown
                          project-id=${this.projectId}
                          .source=${this.value}></nottario-markdown>`
                : html`<div class="preview-empty">Nothing to preview yet.</div>`
          }
        </div>
        <div class="actions">
          <div class="hint">
            <kbd>${navigator.platform.includes('Mac') ? '⌘' : 'Ctrl'}</kbd>+<kbd>Enter</kbd> save
            ·
            <kbd>Esc</kbd> cancel
          </div>
          <button class="btn secondary"
                  type="button"
                  ?disabled=${this.saving}
                  @click=${this._emitCancel}>Cancel</button>
          <button class="btn primary"
                  type="button"
                  ?disabled=${this.disabled || this.saving}
                  @click=${this._emitSubmit}>
            ${this.saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    `;
  }
}

customElements.define('nottario-md-editor', NottarioMdEditor);
