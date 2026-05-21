import { css } from '/static/vendor/lit/lit.js';

// Shared badge palette. Compose into `static styles`:
//
//   static styles = [badgeStyles, css`…`];
//
// Markup: <span class="badge VARIANT">text</span>
//
// Variants come in two families: TASK TYPES used on board/Gantt cards
// (bug/feature/chore/spike) and DOC KINDS used on the docs sidebar
// (skill/context/note). Plus the orthogonal `admin` chip used on user
// rows. All share the same shape (1px border, 999px radius, 11-12px
// font) so they read as members of one family at a glance.
export const badgeStyles = css`
  .badge {
    display: inline-block;
    padding: 1px 7px;
    border-radius: 2em;
    font-size: 11px;
    font-weight: 500;
    border: 1px solid #d1d9e0;
    background: #ffffff;
    color: #1f2328;
    line-height: 1.4;
    vertical-align: 2px;
  }

  /* task types */
  .badge.bug     { background: #ffebe9; border-color: #ffabab; color: #cf222e; }
  .badge.feature { background: #ddf4ff; border-color: #8ec0ff; color: #0969da; }
  .badge.chore   { background: #fff8c5; border-color: #d4a72c; color: #7d4e00; }
  .badge.spike   { background: #ddf4d1; border-color: #95d57e; color: #1a7f37; }
  .badge.task    { /* base — neutral; explicit so the class can always be applied */ }

  /* doc kinds */
  .badge.skill   { background: #ddf4ff; border-color: #8ec0ff; color: #0969da; }
  .badge.context { background: #f6f8fa; border-color: #d1d9e0; color: #1f2328; }
  .badge.note    { background: #fff8c5; border-color: #d4a72c; color: #7d4e00; }

  /* user-level marker (orthogonal to type/kind) */
  .badge.admin   { background: #fff8c5; border-color: #eac54f; color: #9a6700; font-weight: 600; }
`;
