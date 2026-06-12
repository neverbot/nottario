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
    border: 1px solid var(--border);
    background: #ffffff;
    color: var(--fg);
    line-height: 1.4;
    vertical-align: 2px;
  }

  /* task types */
  .badge.bug     { background: var(--tint-red); border-color: #ffabab; color: var(--danger); }
  .badge.feature { background: var(--tint-blue); border-color: #8ec0ff; color: var(--accent); }
  .badge.chore   { background: var(--tint-yellow); border-color: #d4a72c; color: var(--warning-text); }
  .badge.spike   { background: #ddf4d1; border-color: #95d57e; color: var(--success-hover); }
  .badge.task    { /* base — neutral; explicit so the class can always be applied */ }

  /* doc kinds */
  .badge.skill   { background: var(--tint-blue); border-color: #8ec0ff; color: var(--accent); }
  .badge.context { background: var(--bg-subtle); border-color: var(--border); color: var(--fg); }
  .badge.note    { background: var(--tint-yellow); border-color: #d4a72c; color: var(--warning-text); }

  /* user-level marker (orthogonal to type/kind) */
  .badge.admin   { background: var(--tint-yellow); border-color: #eac54f; color: var(--warning); font-weight: 600; }
`;
