import { css } from '/static/vendor/lit/lit.js';

// Shared form-level chrome that lives OUTSIDE the per-field wrapper
// <nottario-field> (see ./field.js). Compose into a component's
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
//   - `.checkbox-label` — inline checkbox + muted label, for "Advanced"
//     style helper toggles inside forms.
//   - `.inline-field` — bare-input chrome (border + focus ring) for
//     table cells and inline rename forms that don't want the full
//     <nottario-field> labelled wrapper.
//
// The per-control chrome that <nottario-field> paints on slotted
// inputs/selects (border, padding, focus ring, normalized chevron)
// lives in field.js. For places that need the same chrome WITHOUT
// <nottario-field>, import `selectStyles` (below) and apply class
// `select`, or import `formStyles` and apply class `inline-field`.
// Keeps the chrome defined in one place even though the shadow
// boundaries prevent ::slotted from reaching outside <nottario-field>.
//
// IMPORTANT: never put backticks inside comments within a css`...`
// tagged template literal — backticks terminate the literal early and
// silently break the stylesheet. Same rule applies to html`...`. The
// CLAUDE.md "Pre-commit gate" section documents this footgun.
export const formStyles = css`
  .actions-row {
    margin-top: 16px;
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  .helper {
    color: var(--fg-muted);
    font-size: 12px;
    margin: 0;
  }
  .helper code {
    font-family: ui-monospace, SFMono-Regular, monospace;
    background: var(--bg-subtle);
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

  /* Inline checkbox + label, sized for muted helper toggles
     such as "Advanced (enables feature type)" in the new-task
     dialog. Apply to a label.checkbox-label wrapping the input
     and the visible text. */
  label.checkbox-label {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--fg-muted);
    cursor: pointer;
  }
  label.checkbox-label input { margin: 0; }

  /* Bare-input chrome — same border + focus ring as the chrome that
     nottario-field paints on slotted inputs, but applied directly to
     an input.inline-field so the same look reaches table-cell editors
     and inline rename forms WITHOUT wrapping each in nottario-field
     (which would add a label row and break the row layout). One
     canonical definition; keep this in sync with the nottario-field
     slot styles. */
  .inline-field {
    padding: 4px 8px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg);
    font: inherit;
    box-sizing: border-box;
  }
  .inline-field:focus,
  .inline-field:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 0;
    border-color: var(--accent);
  }
  .inline-field:disabled {
    color: var(--fg-muted);
    background: var(--bg-subtle);
    cursor: not-allowed;
  }
`;

// Bare-select chrome — matches the chevron + border + focus ring that
// nottario-field paints on slotted selects, but applied directly to a
// select.select so the same look reaches contexts that don't wrap in
// nottario-field (meta panels, inline editors, table-row controls).
// One canonical definition; if it needs tweaking, tweak here AND in
// field.js together.
export const selectStyles = css`
  .select {
    appearance: none;
    -webkit-appearance: none;
    padding: 4px 28px 4px 8px;
    border: 1px solid var(--border);
    border-radius: 6px;
    font: inherit;
    font-size: 12px;
    background: #fff;
    background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 12 12'><path d='M3 4.5l3 3 3-3' fill='none' stroke='%2359636e' stroke-width='1.5' stroke-linecap='round' stroke-linejoin='round'/></svg>");
    background-repeat: no-repeat;
    background-position: right 8px center;
    color: var(--fg);
    cursor: pointer;
  }
  .select:focus {
    outline: 2px solid var(--accent);
    outline-offset: 0;
    border-color: var(--accent);
  }
  .select:disabled {
    color: var(--fg-muted);
    background-color: var(--bg-subtle);
    cursor: not-allowed;
  }
`;
