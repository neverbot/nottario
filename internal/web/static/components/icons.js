import { html, svg as litSvg } from '/static/vendor/lit/lit.js';

// Inline-SVG icon registry. Returns a Lit template the caller composes
// into its own render — no shadow DOM, no extra custom element to
// instantiate per use. Keeps the visual primitives in one place so
// rolling a tiny visual tweak (stroke width, viewBox) happens here
// once instead of in every page.
//
// Conventions:
//   - 16×16 viewBox. The size at the call site uses `width` / `height`
//     attributes (most call sites use 14 or 12); icons render fine
//     down to ~10 and up to ~24.
//   - All strokes use `currentColor` so the parent's `color` cascades.
//   - Each icon takes a single optional `size` arg (number) so callers
//     that want a non-default size don't have to wrap in CSS.
//   - All icons are `aria-hidden` — the parent <button> carries the
//     `aria-label`. Don't move semantics into the SVG.
//
// Important: inner-shape templates (the `<path>` etc.) MUST be tagged
// with Lit's `svg` template literal (not `html`), otherwise the path
// element is created in the HTML namespace and the browser silently
// ignores its drawing instructions. The outer wrapper that owns the
// `<svg>` tag itself can use `html` — Lit detects the SVG tag and
// switches namespace for its direct children.
//
// Add new icons here ONLY when they appear in 2+ places; one-off
// flourishes stay where they're used so the registry doesn't grow
// into a junk drawer.

const STROKE = 1.3;

function wrap(size, body) {
  return html`<svg width=${size} height=${size} viewBox="0 0 16 16" fill="none" aria-hidden="true">${body}</svg>`;
}

// Trash can — destructive action affordance (Delete, Revoke).
export function trashIcon(size = 14) {
  return wrap(
    size,
    litSvg`<path d="M6 2.5h4M3 4.5h10M4.5 4.5l.6 8.2a1 1 0 0 0 1 .9h3.8a1 1 0 0 0 1-.9l.6-8.2M6.8 7v4M9.2 7v4"
                  stroke="currentColor" stroke-width=${STROKE} stroke-linecap="round" stroke-linejoin="round"/>`,
  );
}

// Pencil — edit affordance (rename, change inline value).
export function pencilIcon(size = 14) {
  return wrap(
    size,
    litSvg`<path d="M11.5 1.5L14.5 4.5L5 14H2V11L11.5 1.5Z"
                  stroke="currentColor" stroke-width=${STROKE + 0.1}
                  stroke-linejoin="round" stroke-linecap="round"/>`,
  );
}

// Close X — dismiss affordance (close dialog, dismiss toast). 12×12
// viewBox because the cross looks heavier than expected in a 16×16
// frame; keep this one at its own scale.
export function closeIcon(size = 12) {
  return html`<svg width=${size} height=${size} viewBox="0 0 12 12" aria-hidden="true">
    <path d="M3 3 L9 9 M9 3 L3 9" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/>
  </svg>`;
}

// Down chevron — dropdown affordance (filter chips, cycle switcher).
// Heavier stroke (2px) so it reads at small render sizes inside chip
// labels.
export function chevronDownIcon(size = 16) {
  return wrap(
    size,
    litSvg`<path d="M3 5l5 5 5-5" fill="none" stroke="currentColor" stroke-width="2"/>`,
  );
}
