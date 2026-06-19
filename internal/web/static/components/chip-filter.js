import { LitElement, html, css } from '/static/vendor/lit/lit.js';
import { popoverStyles } from '/static/components/surfaces.js';
import { chevronDownIcon } from '/static/components/icons.js';

// <nottario-chip-filter> — pill-shaped filter control used to narrow
// a list view. Two flavours, picked by the `mode` attribute:
//
//   mode="toggle"      Single binary on/off. No menu. .checked drives
//                      the active visual. Emits `change` with
//                      detail = { checked: bool }.
//   mode="multi"       Multi-select via a popover of checkboxes.
//                      `.options` is an array of { value, label }.
//                      `.values` is the currently-selected array of
//                      values; the component emits `change` with
//                      detail = { values: [...] } on every flip.
//
// Single-select is rare enough that the simplest path is multi with
// `max=1` — kept off the API surface until something asks for it.
//
// The component DOES NOT own filter state across reloads; the parent
// keeps `.values` in sync and pipes them back in. This stays
// trivially testable and keeps the parent in control of URL syncing.
//
// Visual contract (matches the previous inline .filter-chip rules
// verbatim — same height, padding, count-pill, accent on active):
//   - 26px-tall pill, 1px border, white fill.
//   - Active: tinted blue background + accent border.
//   - Numeric count pill on the right when N>0 (multi mode).
//   - Chevron icon on the right (multi mode only).
//   - Popover anchored under the chip with the standard popoverStyles
//     shadow + radius (no per-chip drift).
class NottarioChipFilter extends LitElement {
  static properties = {
    label: { type: String },
    mode: { type: String }, // 'toggle' | 'multi'
    checked: { type: Boolean }, // toggle mode
    values: { type: Array }, // multi mode
    options: { type: Array }, // multi mode: [{value, label}]
    _open: { state: true },
  };

  static styles = [
    popoverStyles,
    css`
      :host { display: inline-block; position: relative; }
      *, *::before, *::after { box-sizing: border-box; }

      .chip {
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
      .chip:hover { border-color: var(--border-strong); }
      .chip.active {
        background: var(--tint-blue);
        border-color: var(--accent);
        color: var(--tint-blue-fg);
      }
      .chip .count {
        background: var(--accent);
        color: #fff;
        border-radius: 999px;
        padding: 1px 6px;
        font-size: 10px;
        font-weight: 600;
      }
      .chip svg { width: 10px; height: 10px; }

      .menu {
        top: calc(100% + 4px);
        left: 0;
        min-width: 180px;
        padding: 4px;
      }
      .menu label {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 6px 8px;
        border-radius: 4px;
        cursor: pointer;
        font-weight: 400;
        font-size: 13px;
      }
      .menu label:hover { background: var(--bg-hover); }
      .menu input[type="checkbox"] { margin: 0; }
    `,
  ];

  constructor() {
    super();
    this.label = '';
    this.mode = 'toggle';
    this.checked = false;
    this.values = [];
    this.options = [];
    this._open = false;
  }

  // Close the popover when the user clicks outside the chip. Attached
  // at the document level to catch clicks that land on other parts of
  // the host page (toolbar buttons, columns, …).
  connectedCallback() {
    super.connectedCallback();
    this._docClick = (e) => {
      if (this._open && !e.composedPath().includes(this)) {
        this._open = false;
      }
    };
    document.addEventListener('mousedown', this._docClick);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    document.removeEventListener('mousedown', this._docClick);
  }

  _onChipClick() {
    if (this.mode === 'toggle') {
      this.checked = !this.checked;
      this.dispatchEvent(
        new CustomEvent('change', {
          detail: { checked: this.checked },
          bubbles: true,
          composed: true,
        }),
      );
      return;
    }
    this._open = !this._open;
  }

  _toggleValue(v) {
    const next = (this.values || []).includes(v)
      ? this.values.filter((x) => x !== v)
      : [...(this.values || []), v];
    this.values = next;
    this.dispatchEvent(
      new CustomEvent('change', {
        detail: { values: next },
        bubbles: true,
        composed: true,
      }),
    );
  }

  render() {
    const isToggle = this.mode === 'toggle';
    const count = isToggle ? 0 : (this.values || []).length;
    const active = isToggle ? this.checked : count > 0;
    return html`
      <button class=${`chip${active ? ' active' : ''}`}
              type="button"
              aria-pressed=${isToggle ? String(this.checked) : null}
              aria-haspopup=${isToggle ? null : 'true'}
              aria-expanded=${isToggle ? null : String(this._open)}
              @click=${() => this._onChipClick()}>
        ${this.label}
        ${count > 0 ? html`<span class="count">${count}</span>` : null}
        ${isToggle ? null : chevronDownIcon()}
      </button>
      ${
        this._open && !isToggle
          ? html`
            <div class="popover menu" role="listbox">
              ${(this.options || []).map(
                (o) => html`
                  <label>
                    <input type="checkbox"
                           ?checked=${(this.values || []).includes(o.value)}
                           @change=${() => this._toggleValue(o.value)}>
                    ${o.label}
                  </label>
                `,
              )}
            </div>
          `
          : null
      }
    `;
  }
}

customElements.define('nottario-chip-filter', NottarioChipFilter);
