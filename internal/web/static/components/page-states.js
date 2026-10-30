import { html, css } from '/static/vendor/lit/lit.js';

// The states a list page goes through around its content, shared by
// the Live and Questions pages: loading placeholders, the empty and
// error panels, the paused live-updates banner and a muted notice.
// Pages add pageStateStyles to their static styles and render the
// markup below.

export const pageStateStyles = css`
  .paused,
  .notice {
    margin: 0 0 12px;
    padding: 8px 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-subtle);
    color: var(--fg-muted);
    font-size: 13px;
  }

  .empty,
  .error {
    padding: 32px 16px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg);
    text-align: center;
    color: var(--fg-muted);
    font-size: 14px;
  }
  .error { color: var(--danger-text); }
  .error p,
  .empty p { margin: 0 0 12px; }
  .empty p:last-child { margin-bottom: 0; }

  /* Loading placeholders: static grey bars, no shimmer. */
  .skeleton { list-style: none; margin: 0; padding: 0; }
  .skeleton li {
    padding: 16px;
    border: 1px solid var(--border);
    border-top-width: 0;
    background: var(--bg);
  }
  .skeleton li:first-child {
    border-top-width: 1px;
    border-radius: var(--radius) var(--radius) 0 0;
  }
  .skeleton li:last-child { border-radius: 0 0 var(--radius) var(--radius); }
  .bar {
    height: 10px;
    border-radius: 3px;
    background: var(--gray-2);
    margin-bottom: 10px;
  }
  .bar.short { width: 30%; }
  .bar.long { width: 70%; }
  .bar.mid { width: 50%; }
  .bar:last-child { margin-bottom: 0; }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }
`;

// skeleton renders placeholder rows while a first load runs, each a
// stack of bars of the given widths ('short', 'long', 'mid'), plus a
// status line for screen readers.
export function skeleton(label, bars = ['short', 'long', 'mid'], rows = 3) {
  return html`<ul class="skeleton" aria-hidden="true">
      ${Array.from(
        { length: rows },
        () => html`<li>${bars.map((b) => html`<div class="bar ${b}"></div>`)}</li>`,
      )}
    </ul>
    <span class="sr-only" role="status">${label}</span>`;
}

// pausedBanner is the line shown while the live stream is down.
export function pausedBanner() {
  return html`<p class="paused" role="status">Live updates paused, reconnecting…</p>`;
}

// errorPanel shows a load error with a retry button.
export function errorPanel(message, retry) {
  return html`<div class="error" role="alert">
    <p>${message}</p>
    <button class="btn secondary" type="button" @click=${retry}>Try again</button>
  </div>`;
}
