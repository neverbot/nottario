---
target: internal/web/static/pages/search.js
total_score: 17
p0_count: 2
p1_count: 2
timestamp: 2026-06-10T16-23-27Z
slug: internal-web-static-pages-search-js
---
## Design Health Score

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of system status | 2 | "Searching…" only on first query; stale on subsequent keystrokes (line 231). |
| 2 | Match system / real world | 3 | DB jargon leaks: "arch_node", "slug", "doc_path" (lines 236, 244-245). |
| 3 | User control & freedom | 1 | No Escape, no clear button; blur-close fights panel mousedown (line 172). |
| 4 | Consistency & standards | 2 | Flat interleaved list; brief calls out grouping (line 233). |
| 5 | Error prevention | 2 | catch swallows to `[]` → looks like no matches (line 164). |
| 6 | Recognition over recall | 2 | No section headers; 10px uppercase pills do all the categorisation work. |
| 7 | Flexibility & efficiency | 1 | Only `/` shortcut. No ↑/↓/Enter in results. |
| 8 | Aesthetic & minimalist | 3 | Genuinely restrained. |
| 9 | Error recovery | 0 | No error state. |
| 10 | Help & documentation | 1 | No `/` hint; no idle teaching state. |
| **Total** | | **17/40** | Below honest band; keyboard story + grouping gap drag it down. |

## Anti-Patterns Verdict

- **LLM**: low slop, medium genericness. Real finding: `.kind-pill.document` and `.hit mark` both use `#fff8c5` — same yellow twice in one row (lines 83, 106).
- **Detector (CLI)**: clean (`[]`). No side-stripe, gradient text, glassmorphism, hero-metric, identical-grid, modal-first violations. Manual scan also clean.
- **Shared minor**: `:host` lacks explicit `box-sizing: border-box` (lines 17-27). Violates the project's shadow-root rule even though practical impact here is zero.

## Overall Impression
A small, honest dropdown with two structural gaps that drag the score: no keyboard nav inside the results, and silent network errors masquerading as empty matches. The grouping the brief calls for is missing — three coloured pills are doing the work two-pixel section headers should do.

## What's Working
- `<mark>` snippet handling + the explanatory comment about why innerHTML is used instead of unsafeHTML (lines 79-88, 177-179).
- `/` shortcut correctly skips inputs / textareas / contenteditable (lines 130-133).
- Disabled state when projectId is null (line 225).

## Priority Issues

### [P0] No keyboard navigation in results — search.js:229-250
Once results render, Tab leaves the input, ↑/↓ do nothing, Enter does nothing. Only `mousedown` opens a hit (line 234). A `/`-triggered dropdown that requires the mouse to use is broken for its target persona.
*Fix*: `selectedIndex` state; keydown ↑/↓ adjust (clamped), Enter `goto(hits[selectedIndex])`, Escape closes + blurs. Render active with `aria-selected="true"` and bg `#ddf4ff`. `role="listbox"` on `.panel`, `role="option"` on `.hit`.

### [P0] Network errors render as "No matches." — search.js:162-167, 232
`catch(_)` sets `hits = []` → empty-state branch fires. A 500 / token expiry / offline is indistinguishable from a clean miss. Users retype the query thinking it's wrong.
*Fix*: add `this.error` state. In catch set `error = 'Search failed. Retry.'` and keep `hits`. New branch above line 232 with `role="alert"`. Clear `error` at top of `runSearch()`.

### [P1] No grouping by source — search.js:233
Tasks / docs / arch nodes interleave. Three pill colours substitute for section headers.
*Fix*: partition into `{ task, document, arch_node }`. Render three `<section>`s, each with `Tasks <count>` header (12px, `#59636e`, weight 600, padding 6px 12px, bg `#f6f8fa`, bottom hairline). Drop the per-row pill — section header now carries kind.

### [P1] Empty state is a dead-end — search.js:232
Just `<div class="empty">No matches.</div>`.
*Fix*: `No matches for <strong>${query}</strong> in this project. Try a shorter term, or check spelling. Search is scoped to the current project.`

### [P2] Pill–mark colour collision — search.js:83, 106
Document pill and mark highlight both `#fff8c5`.
*Fix*: if P1 grouping lands the pill goes away. Otherwise change pill to GitHub's neutral-warning pair (`#fff1e5` / `#9a6700`).

### [P2] `updated()` is O(hits × 2) per render — search.js:180-194
Fires on every Lit update including unrelated state churn. Will replay innerHTML on every arrow keystroke once P0 lands.
*Fix*: `updated(changed) { if (!changed.has('hits')) return; ... }`.

## Persona Red Flags

**Keyboard-first power user** (`/` implies this persona): ↑/↓/Enter dead in results; no Escape; `/` hint never surfaced; post-navigate focus state undefined.

**First-time user**: doesn't know what's searchable; sees "No matches." on network errors and bounces; "arch_node" / "slug" / "doc_path" leak internals.

## Minor Observations
- Universal selector `*, *::before, *::after { box-sizing: border-box; }` at the top of `static styles` (project's shadow-root rule).
- Unfocused input border `#afb8c1` (line 32) vs panel `#d1d9e0` (line 55) — tiny consistency nit.

## Questions to Consider
1. If `/` focuses this dropdown, why does the panel below it not respond to the keyboard?
2. Three pill colours, three meta shapes, zero section headers — what is the pill communicating that a header wouldn't say once?
3. When the API 500s, the user reads "No matches." — UX bug or trust bug?
