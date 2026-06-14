import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-confirm-dialog>: in-app confirmation dialog.
//
// Don't instantiate directly — use the `confirm` API below, which
// lazily creates and reuses a single element in `document.body` so
// any page can call `await confirm({...})` without having to mount
// the element in its template.
//
// Visual baseline matches the existing GitHub-style compact panel
// that lived inline in `pages/board.js::_renderConfirmDelete` before
// this component was extracted. Replaces the four native
// `window.confirm()` calls scattered across the pages.
//
// Usage:
//
//   import { confirm } from '/static/components/confirm-dialog.js';
//
//   const ok = await confirm({
//     title: 'Delete role?',
//     body: 'The role will be removed from this project.',
//     confirmLabel: 'Delete',
//     danger: true,
//   });
//   if (ok) { ... }
class NottarioConfirmDialog extends LitElement {
  static properties = {
    _open: { state: true },
    _title: { state: true },
    _body: { state: true },
    _confirmLabel: { state: true },
    _cancelLabel: { state: true },
    _danger: { state: true },
  };

  static styles = css`
    :host {
      box-sizing: border-box;
    }
    * { box-sizing: border-box; }
    .backdrop {
      position: fixed;
      inset: 0;
      background: rgba(31, 35, 40, 0.40);
      z-index: 70;
      display: flex;
      align-items: center;
      justify-content: center;
    }
    .panel {
      width: 360px;
      max-width: calc(100vw - 32px);
      background: var(--bg);
      border: 1px solid var(--border);
      border-radius: 8px;
      box-shadow: 0 8px 24px rgba(31, 35, 40, 0.16);
      padding: 18px 20px;
      font-family: var(--font);
    }
    h3 {
      margin: 0 0 8px;
      font-size: 15px;
      color: var(--fg);
    }
    p {
      margin: 0 0 16px;
      color: var(--fg-muted);
      font-size: 13px;
      line-height: 1.5;
    }
    .actions {
      display: flex;
      gap: 8px;
      justify-content: flex-end;
    }
    button {
      font: inherit;
      font-size: 14px;
      cursor: pointer;
      border: 1px solid var(--border);
      background: var(--bg-subtle);
      color: var(--fg);
      padding: 5px 14px;
      border-radius: var(--radius, 6px);
    }
    button:hover { background: var(--gray-2); border-color: var(--border-strong); }
    button.primary {
      background: var(--success);
      border-color: rgba(31, 35, 40, 0.15);
      color: var(--fg-on-accent);
    }
    button.primary:hover { background: var(--success-hover); }
    button.danger {
      background: var(--danger);
      border-color: rgba(31, 35, 40, 0.15);
      color: var(--fg-on-accent);
    }
    button.danger:hover { background: var(--danger-hover); }
    button:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 2px;
    }
  `;

  constructor() {
    super();
    this._open = false;
    this._title = '';
    this._body = '';
    this._confirmLabel = 'OK';
    this._cancelLabel = 'Cancel';
    this._danger = false;
    this._resolve = null;
    this._onKey = (e) => {
      if (!this._open) return;
      if (e.key === 'Escape') {
        e.preventDefault();
        this._answer(false);
      } else if (e.key === 'Enter') {
        e.preventDefault();
        this._answer(true);
      }
    };
  }

  connectedCallback() {
    super.connectedCallback();
    document.addEventListener('keydown', this._onKey);
  }
  disconnectedCallback() {
    document.removeEventListener('keydown', this._onKey);
    super.disconnectedCallback();
  }

  render() {
    if (!this._open) return null;
    return html`
      <div class="backdrop"
           role="presentation"
           @click=${(e) => {
             if (e.target.classList.contains('backdrop')) this._answer(false);
           }}>
        <div class="panel"
             role="alertdialog"
             aria-modal="true"
             aria-labelledby="confirm-title">
          <h3 id="confirm-title">${this._title}</h3>
          ${this._body ? html`<p>${this._body}</p>` : null}
          <div class="actions">
            <button type="button" @click=${() => this._answer(false)}>${this._cancelLabel}</button>
            <button type="button"
                    class=${this._danger ? 'danger' : 'primary'}
                    autofocus
                    @click=${() => this._answer(true)}>${this._confirmLabel}</button>
          </div>
        </div>
      </div>
    `;
  }

  // Public API. Opens the dialog and returns a promise resolving to
  // true / false.
  ask(opts = {}) {
    this._title = opts.title || 'Are you sure?';
    this._body = opts.body || '';
    this._confirmLabel = opts.confirmLabel || 'OK';
    this._cancelLabel = opts.cancelLabel || 'Cancel';
    this._danger = !!opts.danger;
    this._open = true;
    this.requestUpdate();
    return new Promise((res) => {
      this._resolve = res;
    });
  }

  _answer(value) {
    const r = this._resolve;
    this._open = false;
    this._resolve = null;
    this.requestUpdate();
    if (r) r(value);
  }
}

customElements.define('nottario-confirm-dialog', NottarioConfirmDialog);

// Lazy singleton: the first call to `confirm({...})` creates the
// element and parks it in `document.body`. Subsequent calls reuse
// the same element.
let _singleton = null;
function el() {
  if (!_singleton) {
    _singleton = document.createElement('nottario-confirm-dialog');
    document.body.appendChild(_singleton);
  }
  return _singleton;
}

export function confirm(opts = {}) {
  return el().ask(opts);
}
