import { LitElement, html, css } from '/static/vendor/lit/lit.js';

// <nottario-field label="Name" hint="optional inline note">
//   <input name="name" required>
// </nottario-field>
//
// Slot-based form field. Renders the `<label>` (with optional muted
// hint inline) and projects the user's input/textarea/select via
// shadow DOM `<slot>`. Saves ~5 lines per field across every form on
// the site — board, projects, project-settings, tokens, docs all use
// the same `<div class="field"><label>...</label><input></div>`
// boilerplate that this replaces.
//
// The slotted control still lives in the host's light DOM, so:
//   - `form.elements.<name>` keeps working (form serialization sees
//     light-DOM descendants of <form>).
//   - the page's own `fieldStyles` still applies things like the
//     number-spinner kill (which targets pseudo-elements that
//     `::slotted` cannot reach).
//
// Styling responsibility split:
//   - <nottario-field> shadow CSS owns label typography and the
//     `::slotted(input/textarea/select)` chrome + focus ring.
//   - the page's composed `fieldStyles` keeps owning `.helper`,
//     `.actions-row`, number-spinner removal.
class NottarioField extends LitElement {
  static properties = {
    label: { type: String },
    hint:  { type: String },
  };

  static styles = css`
    :host {
      display: block;
      margin-bottom: 12px;
      box-sizing: border-box;
    }
    label {
      display: block;
      margin-bottom: 4px;
      font-weight: 500;
      font-size: 13px;
      color: #1f2328;
    }
    label .hint {
      color: #59636e;
      font-weight: 400;
      font-size: 12px;
      margin-left: 4px;
    }
    ::slotted(input),
    ::slotted(textarea),
    ::slotted(select) {
      width: 100%;
      padding: 6px 10px;
      border: 1px solid #d0d7de;
      border-radius: 6px;
      font: inherit;
      background: #ffffff;
      box-sizing: border-box;
    }
    /* Native chevron is platform-variable (macOS Chrome draws it in a
       wide internal band, leaving a generous gap to the visible right
       edge). Apaga el chevron nativo y pinta uno uniforme con un SVG
       de fondo anclado a 8px del borde derecho; el padding-right
       reserva el hueco. Tamaño y color coordinados con el texto
       muted (#59636e). */
    ::slotted(select) {
      appearance: none;
      -webkit-appearance: none;
      background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 12 12'><path d='M3 4.5l3 3 3-3' fill='none' stroke='%2359636e' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/></svg>");
      background-repeat: no-repeat;
      background-position: right 8px center;
      padding-right: 28px;
    }
    ::slotted(input:focus),
    ::slotted(textarea:focus),
    ::slotted(select:focus) {
      outline: 2px solid #0969da;
      outline-offset: 0;
      border-color: #0969da;
    }
    ::slotted(textarea) {
      resize: vertical;
      min-height: 60px;
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 12px;
    }
  `;

  constructor() {
    super();
    this.label = '';
    this.hint = '';
  }

  render() {
    return html`
      <label>
        ${this.label}
        ${this.hint ? html`<span class="hint">${this.hint}</span>` : ''}
      </label>
      <slot></slot>
    `;
  }
}

customElements.define('nottario-field', NottarioField);
