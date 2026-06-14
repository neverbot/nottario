import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-color-swatches>: brand-palette colour picker.
//
// A small radiogroup of round swatches, one per palette colour. The
// selected swatch picks up a focus-ring style accent. Used in place
// of `<input type="color">` so every project stays inside the
// brand-anchored set documented in docs/design/palette.md — no
// free-form hex entry.
//
// Usage:
//
//   import '/static/components/color-swatches.js';
//   import { BRAND_ROLE_PALETTE } from '/static/components/color-swatches.js';
//
//   <nottario-color-swatches
//     .palette=${BRAND_ROLE_PALETTE}
//     .value=${this._color}
//     @change=${(e) => (this._color = e.detail.value)}>
//   </nottario-color-swatches>
//
// The default palette covers the role / kind colour vocabulary the
// rest of the system uses today.

export const BRAND_ROLE_PALETTE = [
  '#1f6feb', // brand blue
  '#2da44e', // brand green
  '#bf8700', // gold (qa)
  '#8250df', // purple (design)
  '#cf222e', // danger red
  '#bc4c00', // kind-external orange
];

class NottarioColorSwatches extends LitElement {
  static properties = {
    palette: { type: Array },
    value: { type: String },
    ariaLabel: { type: String, attribute: 'aria-label' },
  };

  static styles = css`
    :host {
      box-sizing: border-box;
      display: inline-block;
    }
    * { box-sizing: border-box; }
    .row {
      display: inline-flex;
      gap: 6px;
      align-items: center;
      padding: 4px 6px;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: var(--bg);
    }
    .swatch {
      width: 22px;
      height: 22px;
      padding: 0;
      border: 0;
      border-radius: 50%;
      cursor: pointer;
      box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.1);
    }
    .swatch.selected {
      box-shadow:
        inset 0 0 0 1px rgba(0, 0, 0, 0.15),
        0 0 0 2px var(--bg),
        0 0 0 4px var(--accent);
    }
    .swatch:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 2px;
    }
  `;

  constructor() {
    super();
    this.palette = BRAND_ROLE_PALETTE;
    this.value = '';
    this.ariaLabel = 'Colour';
  }

  render() {
    const value = (this.value || '').toLowerCase();
    return html`
      <div class="row" role="radiogroup" aria-label=${this.ariaLabel}>
        ${(this.palette || []).map((c) => {
          const selected = value === c.toLowerCase();
          return html`
            <button type="button"
                    class=${`swatch${selected ? ' selected' : ''}`}
                    role="radio"
                    aria-checked=${selected ? 'true' : 'false'}
                    aria-label=${c}
                    title=${c}
                    style=${`background:${c}`}
                    @click=${() => this._pick(c)}></button>
          `;
        })}
      </div>
    `;
  }

  _pick(c) {
    this.value = c;
    this.dispatchEvent(
      new CustomEvent('change', {
        detail: { value: c },
        bubbles: true,
        composed: true,
      }),
    );
  }
}

customElements.define('nottario-color-swatches', NottarioColorSwatches);
