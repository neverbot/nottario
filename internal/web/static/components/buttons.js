import { css } from '/static/vendor/lit/lit.js';

// Shared button palette. Compose into a component's `static styles`:
//
//   static styles = [buttonStyles, css`…page-specific…`];
//
// Five named variants — never improvised, per the design system call:
//
//   .btn          base, used when no variant class is set
//   .btn.primary  one decisive action per surface ("Create", "New …")
//   .btn.secondary reversible default ("Cancel", "Copy"); white fill, border
//   .btn.ghost    tertiary, no border ("Edit"); text-only with hover bg
//   .btn.danger   destructive ("Delete", "Revoke"); red text + border
//   .btn.icon     compact 28px square ghost ("⚙", "↻"); for toolbars
//
// The active state of a segmented control is NOT a .primary; segmented
// controls live in their own component (components/segmented-control.js)
// so the "you're viewing Kanban" affordance never borrows the green
// save-button fill.
export const buttonStyles = css`
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    height: 30px;
    padding: 0 12px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--fg);
    font: inherit;
    font-size: 13px;
    font-weight: 500;
    line-height: 1;
    cursor: pointer;
    transition: background-color 60ms ease-out, border-color 60ms ease-out;
    box-sizing: border-box;
  }
  .btn:hover { background: var(--bg-hover); border-color: var(--border-strong); }
  .btn:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  .btn:disabled { opacity: 0.55; cursor: not-allowed; }

  .btn.primary {
    background: var(--success);
    color: var(--bg);
    border-color: rgba(31, 35, 40, 0.15);
  }
  .btn.primary:hover { background: var(--success-hover); border-color: rgba(31, 35, 40, 0.2); }

  .btn.secondary { /* alias of base — explicit name for readability */ }

  .btn.ghost {
    background: transparent;
    border-color: transparent;
    color: var(--fg);
  }
  .btn.ghost:hover { background: var(--bg-hover); border-color: transparent; }

  .btn.danger {
    color: var(--danger);
    border-color: rgba(207, 34, 46, 0.35);
    background: var(--bg);
  }
  .btn.danger:hover { background: var(--tint-red); border-color: var(--danger); }

  .btn.icon {
    width: 30px;
    padding: 0;
  }

  /* Compact pill variant — smaller than the standard .btn (28px tall
     vs 30px), fully rounded so it reads as a content-anchored
     control rather than a primary action. Used today by the cycle
     switcher in the board topbar; pair with .secondary (the default)
     for the white-fill look. */
  .btn.pill {
    height: 28px;
    border-radius: 999px;
    padding: 0 10px;
    font-size: 12px;
  }

  /* Stand-alone compact icon button. Used in dialog/section headers
     where a row of actions sits NEXT to a title (close, delete,
     refresh). Smaller (28×28) and lighter than .btn.icon so the
     title carries the visual weight. Add .danger for destructive
     actions; tint matches .btn.danger.
     Apply: <button class="icon-btn"> or <button class="icon-btn danger"> */
  .icon-btn {
    width: 28px;
    height: 28px;
    padding: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    color: var(--gray-5);
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    cursor: pointer;
    font: inherit;
  }
  .icon-btn svg { display: block; }
  .icon-btn:hover {
    color: var(--fg);
    background: var(--bg-subtle);
    border-color: var(--border);
  }
  .icon-btn:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 1px;
  }
  .icon-btn.danger:hover,
  .icon-btn.danger:focus-visible {
    color: var(--danger);
    background: var(--tint-red);
    border-color: rgba(207, 34, 46, 0.4);
  }
`;
