import { css } from '/static/vendor/lit/lit.js';

// Shared form-level chrome that lives OUTSIDE the per-field wrapper
// `<nottario-field>` (see ./field.js). Compose into a component's
// `static styles`:
//
//   static styles = [formStyles, css`…page-specific…`];
//
// Owns:
//   - `.actions-row` — right-aligned button cluster at the foot of
//     every "New X" dialog.
//   - `.helper`      — small muted explanation lines under fields,
//     tables and headings.
//   - the global `<input type="number">` spinner kill (each browser
//     paints it with its own OS chrome that never matches our 1px
//     hairline border).
//
// The per-control chrome (border, padding, focus ring, normalized
// select chevron) lives in `<nottario-field>`'s shadow styles so it
// stays defined in exactly one place.
export const formStyles = css`
  .actions-row {
    margin-top: 16px;
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  .helper {
    color: #59636e;
    font-size: 12px;
    margin: 0;
  }
  .helper code {
    font-family: ui-monospace, SFMono-Regular, monospace;
    background: #f6f8fa;
    padding: 0 4px;
    border-radius: 3px;
    font-size: 11px;
  }

  input[type="number"]::-webkit-inner-spin-button,
  input[type="number"]::-webkit-outer-spin-button {
    -webkit-appearance: none;
    appearance: none;
    margin: 0;
  }
  input[type="number"] { -moz-appearance: textfield; }
`;
