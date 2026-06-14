import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-toast>: single-slot toast pill anchored bottom-center.
//
// Don't instantiate directly — use the `toast` API below, which
// lazily creates and reuses a single element in `document.body` so
// any page can call `toast.success('Saved.')` without having to
// mount the element themselves.
//
// Visual baseline matches the existing GitHub-style dark pill that
// lived inline in `pages/board.js` before this component was
// extracted. Variants add a thin coloured left border (success /
// error) without changing the pill shape.
class NottarioToast extends LitElement {
  static properties = {
    _msg: { state: true },
    _variant: { state: true },
    _action: { state: true },
    _visible: { state: true },
  };

  static styles = css`
    :host {
      box-sizing: border-box;
      position: fixed;
      left: 50%;
      bottom: 16px;
      transform: translateX(-50%);
      z-index: 60;
      pointer-events: none;
    }
    * { box-sizing: border-box; }
    .pill {
      pointer-events: auto;
      display: inline-flex;
      align-items: center;
      gap: 12px;
      padding: 10px 14px;
      background: var(--fg);
      color: var(--fg-on-accent);
      border-radius: 8px;
      box-shadow: 0 8px 24px rgba(31, 35, 40, 0.24);
      font-size: 13px;
      max-width: min(560px, calc(100vw - 32px));
      transform: translateY(8px);
      opacity: 0;
      transition: opacity 160ms ease-out, transform 160ms ease-out;
    }
    .pill.show {
      opacity: 1;
      transform: translateY(0);
    }
    /* Variant accent: a 3px left bar in the variant colour, on top
       of the dark pill. Keeps the GitHub-style baseline; the colour
       is the variant signal. */
    .pill.success { box-shadow: 0 8px 24px rgba(31, 35, 40, 0.24),
                                inset 3px 0 0 var(--success); }
    .pill.error   { box-shadow: 0 8px 24px rgba(31, 35, 40, 0.24),
                                inset 3px 0 0 var(--danger); }
    .pill.success { padding-left: 17px; }
    .pill.error   { padding-left: 17px; }
    button {
      background: transparent;
      border: 0;
      color: var(--brand-blue);
      filter: brightness(1.6);
      font: inherit;
      font-weight: 600;
      cursor: pointer;
      padding: 0;
    }
    button:hover { text-decoration: underline; }
    @media (prefers-reduced-motion: reduce) {
      .pill { transition: none; }
    }
  `;

  constructor() {
    super();
    this._msg = '';
    this._variant = '';
    this._action = null;
    this._visible = false;
    this._timer = null;
  }

  render() {
    if (!this._msg) return null;
    const cls = `pill${this._visible ? ' show' : ''}${this._variant ? ` ${this._variant}` : ''}`;
    return html`
      <div class=${cls} role="status" aria-live="polite">
        <span>${this._msg}</span>
        ${
          this._action
            ? html`<button @click=${() => this._runAction()}>${this._action.label}</button>`
            : null
        }
      </div>
    `;
  }

  // Public API. Replaces any in-flight toast.
  show(message, opts = {}) {
    const variant = opts.variant || ''; // '' | 'success' | 'error'
    const duration =
      typeof opts.duration === 'number' ? opts.duration : variant === 'error' ? 6000 : 3000;
    const action = opts.undo
      ? { label: opts.undoLabel || 'Undo', fn: opts.undo }
      : opts.retry
        ? { label: opts.retryLabel || 'Retry', fn: opts.retry }
        : null;

    if (this._timer) {
      clearTimeout(this._timer);
      this._timer = null;
    }
    this._msg = message;
    this._variant = variant;
    this._action = action;
    this._visible = false;
    this.requestUpdate();
    // Defer the .show class one frame so the entry transition runs.
    requestAnimationFrame(() => {
      this._visible = true;
      this.requestUpdate();
    });
    if (duration > 0) {
      this._timer = setTimeout(() => this._dismiss(), duration);
    }
  }

  _dismiss() {
    this._visible = false;
    this._timer = null;
    this.requestUpdate();
    // Wait for fade-out before clearing the message so the DOM stays
    // stable during the transition.
    setTimeout(() => {
      this._msg = '';
      this._variant = '';
      this._action = null;
      this.requestUpdate();
    }, 200);
  }

  _runAction() {
    const fn = this._action?.fn;
    this._dismiss();
    if (fn) {
      try {
        fn();
      } catch (_) {
        // Ignore action errors — the user already saw the toast.
      }
    }
  }
}

customElements.define('nottario-toast', NottarioToast);

// Lazy singleton: the first call to `toast.show(...)` creates the
// element and parks it in `document.body`. Subsequent calls reuse
// the same element. Pages don't need to render <nottario-toast>
// in their template.
let _singleton = null;
function el() {
  if (!_singleton) {
    _singleton = document.createElement('nottario-toast');
    document.body.appendChild(_singleton);
  }
  return _singleton;
}

export const toast = {
  show(message, opts) {
    el().show(message, opts);
  },
  success(message, opts = {}) {
    el().show(message, { ...opts, variant: 'success' });
  },
  error(message, opts = {}) {
    el().show(message, { ...opts, variant: 'error' });
  },
};
