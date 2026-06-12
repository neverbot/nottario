import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-segmented-control> renders a row of mutually-exclusive
// options (Kanban/Gantt, Diagram/Tree). The active state is its own
// visual treatment — neutral light fill + bottom border — and never
// borrows the primary CTA green. ARIA: a `radiogroup` of `radio`-role
// buttons with `aria-checked`, so screen readers report it correctly.
//
// Properties:
//   options: [{ value, label, title? }]
//   value:   currently selected `value`
//
// Fires a bubbling `change` event with detail `{ value }` when the
// user picks a new option. The host stays the source of truth: it
// listens to `change`, runs its routing, then updates `value`.
class NottarioSegmentedControl extends LitElement {
  static properties = {
    options: { type: Array },
    value: { type: String },
  };

  static styles = css`
    :host { display: inline-flex; box-sizing: border-box; }
    * { box-sizing: border-box; }
    .group {
      display: inline-flex;
      align-items: stretch;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: var(--bg-subtle);
      padding: 2px;
      gap: 2px;
    }
    button {
      height: 26px;
      padding: 0 10px;
      border: 1px solid transparent;
      background: transparent;
      border-radius: 4px;
      font: inherit;
      font-size: 13px;
      font-weight: 500;
      color: var(--fg-muted);
      cursor: pointer;
      line-height: 1;
    }
    button:hover { color: var(--fg); }
    button[aria-checked="true"] {
      background: #ffffff;
      color: var(--fg);
      border-color: var(--border);
      box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04);
    }
    button:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 1px;
    }
  `;

  constructor() {
    super();
    this.options = [];
    this.value = '';
  }

  _pick(v) {
    if (v === this.value) return;
    this.dispatchEvent(
      new CustomEvent('change', {
        detail: { value: v },
        bubbles: true,
        composed: true,
      }),
    );
  }

  render() {
    return html`
      <div class="group" role="radiogroup">
        ${(this.options || []).map(
          (o) => html`
          <button role="radio"
                  aria-checked=${this.value === o.value ? 'true' : 'false'}
                  title=${o.title || o.label}
                  @click=${() => this._pick(o.value)}>${o.label}</button>
        `,
        )}
      </div>
    `;
  }
}

customElements.define('nottario-segmented-control', NottarioSegmentedControl);
