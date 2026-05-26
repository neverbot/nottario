import { css } from '/static/vendor/lit/lit.js';

// Shared form-field chrome. Compose into a component's `static styles`:
//
//   static styles = [fieldStyles, css`…page-specific…`];
//
// Markup convention (every page that opts in follows this):
//
//   <div class="field">
//     <label>Name <span class="muted">helper text</span></label>
//     <input name="…" required>
//   </div>
//
// The single `.field` block owns: spacing between rows, label
// typography, input/select/textarea chrome, focus ring, and a global
// kill of the browser-native number-spinner (it clashes with the
// thin border treatment and GitHub-likes hide it by convention; the
// keyboard ↑/↓ still increments).
//
// Pages with form-only edge cases (e.g. a side-by-side row, a tight
// width override) layer their own one-off rules on top of `.field`.
export const fieldStyles = css`
  .field { margin-bottom: 12px; }
  .field label {
    display: block;
    margin-bottom: 4px;
    font-weight: 500;
    font-size: 13px;
    color: #1f2328;
  }
  .field input,
  .field textarea,
  .field select {
    width: 100%;
    padding: 6px 10px;
    border: 1px solid #d0d7de;
    border-radius: 6px;
    font: inherit;
    background: #ffffff;
    box-sizing: border-box;
  }
  /* Same chevron normalisation as the slotted variant in
     components/field.js: kill the native arrow and paint a uniform
     SVG anchored 8px from the right edge. Keep both rule blocks in
     sync if either is touched. */
  .field select {
    appearance: none;
    -webkit-appearance: none;
    background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 12 12'><path d='M3 4.5l3 3 3-3' fill='none' stroke='%2359636e' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/></svg>");
    background-repeat: no-repeat;
    background-position: right 8px center;
    padding-right: 28px;
  }
  .field input:focus,
  .field textarea:focus,
  .field select:focus {
    outline: 2px solid #0969da;
    outline-offset: 0;
    border-color: #0969da;
  }
  .field textarea {
    resize: vertical;
    min-height: 60px;
    font-family: ui-monospace, SFMono-Regular, monospace;
    font-size: 12px;
  }

  /* Right-aligned button cluster at the foot of a form/dialog. Used
     by every "New X" dialog plus inline edit forms. */
  .actions-row {
    margin-top: 16px;
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  /* Small muted explanation line. Sits under fields, under tables,
     under headings. */
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

  /* Hide the browser-native number-spinner everywhere. Each browser
     paints it with its own OS chrome, which never matches our 1px
     hairline border. GitHub, Stripe, Linear all do this. */
  input[type="number"]::-webkit-inner-spin-button,
  input[type="number"]::-webkit-outer-spin-button {
    -webkit-appearance: none;
    appearance: none;
    margin: 0;
  }
  input[type="number"] { -moz-appearance: textfield; }
`;
