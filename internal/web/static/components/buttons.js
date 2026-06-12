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
    background: #ffffff;
    color: var(--fg);
    font: inherit;
    font-size: 13px;
    font-weight: 500;
    line-height: 1;
    cursor: pointer;
    transition: background-color 60ms ease-out, border-color 60ms ease-out;
    box-sizing: border-box;
  }
  .btn:hover { background: #f3f4f6; border-color: var(--border-strong); }
  .btn:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
  .btn:disabled { opacity: 0.55; cursor: not-allowed; }

  .btn.primary {
    background: var(--success);
    color: #ffffff;
    border-color: rgba(31, 35, 40, 0.15);
  }
  .btn.primary:hover { background: var(--success-hover); border-color: rgba(31, 35, 40, 0.2); }

  .btn.secondary { /* alias of base — explicit name for readability */ }

  .btn.ghost {
    background: transparent;
    border-color: transparent;
    color: var(--fg);
  }
  .btn.ghost:hover { background: #f3f4f6; border-color: transparent; }

  .btn.danger {
    color: var(--danger);
    border-color: rgba(207, 34, 46, 0.35);
    background: #ffffff;
  }
  .btn.danger:hover { background: var(--tint-red); border-color: var(--danger); }

  .btn.icon {
    width: 30px;
    padding: 0;
  }
`;
