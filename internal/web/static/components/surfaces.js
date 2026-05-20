import { css } from '/static/vendor/lit/lit.js';

// Shared visual primitives for shadow-DOM-isolated Lit pages.
//
// Compose into a component's `static styles` next to the page-specific
// CSS:
//
//   static styles = [surfaceStyles, tableStyles, css`…page-specific…`];
//
// One token earns its place when it shows up in 3+ pages. Anything
// rarer stays inline. The fixed tokens are inventoried in the
// project's `design-tokens.md` (Nottario context doc) — keep them in
// sync when adding to or pruning from this file.

// surfaceStyles: the standard white card chrome used by project cards,
// the kanban cards, the profile identity card, project-settings
// panels, and the empty-state callouts.
//
//   .surface         the base card (white bg, hairline border, soft shadow)
//   .surface.tinted  same shape, light grey fill (for column containers)
//   .empty           dashed border, centered muted body — for "no rows"
export const surfaceStyles = css`
  .surface {
    background: #ffffff;
    border: 1px solid #d1d9e0;
    border-radius: 8px;
    box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04);
    box-sizing: border-box;
  }
  .surface.tinted {
    background: #f6f8fa;
    box-shadow: none;
  }
  .empty {
    padding: 40px 24px;
    text-align: center;
    color: #59636e;
    background: #ffffff;
    border: 1px dashed #d1d9e0;
    border-radius: 8px;
    box-sizing: border-box;
  }
  .empty strong {
    display: block;
    color: #1f2328;
    font-size: 15px;
    margin-bottom: 4px;
  }
`;

// tableStyles: rounded data tables that actually round their corners.
//
// `border-collapse: separate` is mandatory — the previous regression
// (sharp corners on profile/users tables) came from `collapse`. The
// `overflow: hidden` on the outer table clips the inner cells against
// the radius. Header row gets the standard tinted background; rows
// separate with a thin hairline.
//
//   .data-table   the outer table element
export const tableStyles = css`
  table.data-table {
    width: 100%;
    border-collapse: separate;
    border-spacing: 0;
    background: #ffffff;
    border: 1px solid #d1d9e0;
    border-radius: 8px;
    overflow: hidden;
    box-shadow: 0 1px 0 rgba(31, 35, 40, 0.04);
    box-sizing: border-box;
  }
  table.data-table th,
  table.data-table td {
    text-align: left;
    padding: 9px 14px;
    border-bottom: 1px solid #eaeef2;
    font-size: 13px;
    vertical-align: middle;
  }
  table.data-table tbody tr:last-child td { border-bottom: none; }
  table.data-table th {
    background: #f6f8fa;
    font-weight: 600;
    color: #59636e;
    text-transform: uppercase;
    font-size: 11px;
    letter-spacing: 0.04em;
  }
`;

// dialogStyles: the standard modal overlay + panel. Used by the new
// project / new task / new token / new doc dialogs. Esc-to-close is
// already covered by the EscController; this just owns the visual.
//
//   .dialog        the full-viewport dim backdrop
//   .dialog .panel the centered white panel inside it
export const dialogStyles = css`
  .dialog {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.4);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 10;
  }
  .dialog .panel {
    background: #ffffff;
    border-radius: 8px;
    padding: 24px;
    width: 480px;
    max-width: 92vw;
    max-height: 88vh;
    overflow: auto;
    box-shadow: 0 12px 32px rgba(31, 35, 40, 0.14);
    box-sizing: border-box;
  }
  .dialog .panel h3 { margin: 0 0 16px 0; }
`;
