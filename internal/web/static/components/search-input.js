import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-search-input
//   placeholder="Filter or search..."
//   value="..."
//   @input=${e => host.query = e.detail.value}
//   @clear=${() => host.query = ''}>
//   <span slot="hint">...</span>  <!-- optional keyboard hint row -->
// </nottario-search-input>
//
// Single-line filter/search input with an integrated SVG clear button
// that only appears when there's a value. The clear button uses an
// inline SVG (not the Unicode "×" character) because that glyph has a
// low optical center in system fonts and never centers nicely in a
// 22px button.
//
// Emits:
//   - `input` ({ detail: { value } }) on every keystroke
//   - `clear`                          when the X is clicked
//   - `enter` ({ detail: { value } }) when the user presses Enter
//
// The slot 'hint' is for the small keyboard-shortcuts line that
// some consumers (docs rail) show below the input.
//
// Lifted ahead of strict 3+ rule because the alternatives (Unicode
// character, `type="search"` native clear) are visibly inconsistent
// across browsers and the existing custom version was clipping its
// glyph. A single tested component beats three slightly different
// hand-rolled inputs.
class NottarioSearchInput extends LitElement {
  static properties = {
    value:       { type: String },
    placeholder: { type: String },
    autofocus:   { type: Boolean },
  };

  static styles = css`
    :host {
      display: block;
      box-sizing: border-box;
      position: relative;
    }
    .wrap { position: relative; }
    input {
      width: 100%;
      padding: 6px 30px 6px 10px;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      font: inherit;
      font-size: 13px;
      background: #ffffff;
      color: #1f2328;
      box-sizing: border-box;
    }
    input::placeholder { color: #8b949e; }
    input:focus {
      outline: 2px solid #0969da;
      outline-offset: 0;
      border-color: #0969da;
    }
    .clear {
      position: absolute;
      right: 4px;
      top: 50%;
      transform: translateY(-50%);
      width: 22px;
      height: 22px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      background: transparent;
      border: 1px solid transparent;
      cursor: pointer;
      color: #59636e;
      padding: 0;
      border-radius: 4px;
      font: inherit;
    }
    .clear:hover {
      color: #1f2328;
      background: #f6f8fa;
      border-color: #d0d7de;
    }
    .clear:focus-visible {
      outline: 2px solid #0969da;
      outline-offset: 0;
    }
    .clear svg { display: block; }
    ::slotted([slot="hint"]) {
      display: block;
      margin-top: 4px;
      font-size: 11px;
      color: #8b949e;
      padding-left: 2px;
    }
  `;

  constructor() {
    super();
    this.value = '';
    this.placeholder = '';
    this.autofocus = false;
  }

  // Public: forwarded to the inner <input>. Consumers like the docs
  // page bind `/` to focus the search bar.
  focus() {
    this.shadowRoot?.querySelector('input')?.focus();
  }

  select() {
    this.shadowRoot?.querySelector('input')?.select?.();
  }

  _onInput(e) {
    // The native InputEvent IS composed: true (unlike most native
    // events you'd expect to stop at the shadow boundary). If we let
    // it bubble out, consumers listening for `input` on this host
    // receive TWO events per keystroke: our CustomEvent (with
    // detail.value) and the native InputEvent (with detail = 0,
    // because UIEvent.detail is a number). Reading `e.detail.value`
    // on the native one yields undefined and corrupts host state.
    // Stop the native event here so only our CustomEvent reaches
    // the host.
    e.stopPropagation();
    this.value = e.target.value;
    this.dispatchEvent(new CustomEvent('input', {
      detail: { value: this.value },
      bubbles: true, composed: true,
    }));
  }

  _onKey(e) {
    if (e.key === 'Enter') {
      this.dispatchEvent(new CustomEvent('enter', {
        detail: { value: this.value },
        bubbles: true, composed: true,
      }));
    }
  }

  _clear() {
    this.value = '';
    this.dispatchEvent(new CustomEvent('clear', {
      bubbles: true, composed: true,
    }));
    this.dispatchEvent(new CustomEvent('input', {
      detail: { value: '' },
      bubbles: true, composed: true,
    }));
    this.focus();
  }

  render() {
    return html`
      <div class="wrap">
        <input
          type="search"
          aria-label=${this.placeholder || 'Search'}
          placeholder=${this.placeholder}
          .value=${this.value}
          ?autofocus=${this.autofocus}
          @input=${this._onInput}
          @keydown=${this._onKey}>
        ${this.value ? html`
          <button class="clear" title="Clear (Esc)" aria-label="Clear search"
                  @click=${this._clear}>
            <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
              <path d="M3 3 L9 9 M9 3 L3 9" stroke="currentColor"
                    stroke-width="1.6" stroke-linecap="round"/>
            </svg>
          </button>
        ` : null}
      </div>
      <slot name="hint"></slot>
    `;
  }
}

customElements.define('nottario-search-input', NottarioSearchInput);
