import { css } from '/static/vendor/lit/lit.js';

// Shared styles for the "add row" pattern: a small form rendered
// directly below a data table, with fields aligned end-to-end and
// a primary Add button on the right. Used to live inline in
// pages/project-settings.js (Roles, Priorities, Members); extracted
// so any page can compose them into its `static styles`:
//
//   static styles = [addRowStyles, css`…`];
//
// Mark-up template:
//
//   <form class="add-row" @submit=${...}>
//     <nottario-field label="Key">
//       <input name="key" placeholder="…" required>
//     </nottario-field>
//     <nottario-field label="Label">
//       <input name="label" required>
//     </nottario-field>
//     <div class="add-action">
//       <button type="submit" class="btn primary">Add …</button>
//     </div>
//   </form>
//
// The fields stretch to fill the row by default; add the `narrow`
// modifier to lock one to a fixed width (e.g. a numeric value cell).
export const addRowStyles = css`
  .add-row {
    display: flex;
    gap: 12px;
    margin-top: 16px;
    align-items: flex-end;
    flex-wrap: wrap;
  }
  .add-row .field { margin-bottom: 0; flex: 1; min-width: 120px; }
  .add-row .field.narrow { flex: 0 0 110px; }
  .add-row nottario-field { margin-bottom: 0; flex: 1; min-width: 120px; }
  .add-row nottario-field.narrow { flex: 0 0 110px; }
  /* Auto-sized field used for the colour-swatches grid: stays at
     its intrinsic width instead of stretching to fill. */
  .add-row nottario-field.auto { flex: 0 0 auto; min-width: 0; }
  .add-row .add-action { display: flex; align-items: center; height: 32px; }
`;
