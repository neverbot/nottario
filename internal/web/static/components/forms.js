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
// The per-control chrome that `<nottario-field>` paints on slotted
// inputs/selects (border, padding, focus ring, normalized chevron)
// lives in `field.js`. For places that need the same chrome WITHOUT
// `<nottario-field>` — bare selects in meta panels, inline editors,
// etc. — import `selectStyles` and apply class `select` to the
// element directly. Keeps the chrome defined in one place even
// though the shadow boundaries prevent ::slotted from reaching
// outside `<nottario-field>`.
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
`;

// Bare-select chrome — matches the chevron + border + focus ring
// that `<nottario-field>` paints on slotted selects, but applied
// directly to a `<select class="select">` so the same look reaches
// contexts that don't wrap in `<nottario-field>` (meta panels, inline
// editors, table-row controls). One canonical definition; if it needs
// tweaking, tweak here AND in field.js together.
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
