import { css } from '/static/vendor/lit/lit.js';

// Shared row-action styles for data tables: the small ✕ delete and
// ✎ edit icon buttons that sit on the right edge of each row. Used
// to live inline in pages/project-settings.js; extracted so any
// page can compose them into its `static styles`:
//
//   static styles = [tableActionStyles, css`…`];
//
// Mark-up template (admin-only branch):
//
//   <td class="row-actions">
//     <button class="edit" @click=${...}>
//       <svg ...></svg>
//     </button>
//     <button class="delete" @click=${...}>✕</button>
//   </td>
//
// Both buttons sit on a quiet ghost state at rest; hover/focus
// surfaces the semantic colour (accent for edit, danger for delete).
export const tableActionStyles = css`
  .row-actions {
    text-align: right;
  }
  .row-actions button { margin-left: 4px; }

  /* Pencil-icon Edit button: quiet at rest, picks up the accent on
     hover/focus. Use for non-destructive in-place edit affordances. */
  .row-actions .edit {
    width: 26px;
    height: 26px;
    padding: 0;
    color: var(--gray-5);
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font: inherit;
  }
  .row-actions .edit:hover {
    color: var(--accent);
    border-color: var(--accent);
    background: var(--tint-blue);
  }
  .row-actions .edit:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 1px;
  }

  /* Quieter destructive button in table rows: at rest a small ghost
     ✕; armed/hover swaps to the loud red. Keeps tables visually
     calm while still putting the destructive affordance one click
     away. */
  .row-actions .delete {
    width: 26px;
    height: 26px;
    padding: 0;
    font-size: 12px;
    line-height: 1;
    color: var(--gray-5);
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    cursor: pointer;
    font-family: inherit;
  }
  .row-actions .delete:hover {
    color: var(--danger);
    border-color: rgba(207, 34, 46, 0.4);
    background: var(--tint-red);
  }
  .row-actions .delete:focus-visible {
    outline: 2px solid var(--danger);
    outline-offset: 1px;
  }
`;
