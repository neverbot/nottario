# Nottario palette

Light-theme product palette. Dark theme is parked until v0.1.0+.

The single source of truth is `internal/web/static/styles.css`: the
`:root` block defines every colour Nottario uses. This document
explains the *why* — anchor, ramps, semantic aliases, when to reach
for what — so future changes preserve the system instead of growing
new one-off hex literals.

## Anchor

Two brand hex values seed the system:

| Token           | Hex       | OKLCH approx       | Where it shows up                          |
|-----------------|-----------|--------------------|--------------------------------------------|
| `--brand-blue`  | `#1f6feb` | `0.55 0.20 254`    | Topbar mark, accent, focus ring, Backend role |
| `--brand-green` | `#2da44e` | `0.62 0.17 145`    | Topbar mark (gradient end), success, Frontend role |

The topbar's logo carries them as a 135° gradient (`brand-green` →
`brand-blue`). Promoting them to root tokens means the rest of the
app inherits the same hue family — accents, success buttons and the
focus ring all read as one with the brand.

## Neutrals

A 10-step grey ramp, hue ≈ 240 (slightly cool), chroma ≤ 0.01. Pick
the step that matches the *intent*, not the exact GitHub value:

| Token     | Hex       | Used for                                  |
|-----------|-----------|-------------------------------------------|
| `--gray-0`| `#ffffff` | Page background, top-of-card fill         |
| `--gray-1`| `#f6f8fa` | Subtle surfaces, button rest, code blocks |
| `--gray-2`| `#eaeef2` | Hover wash, hairlines between rows        |
| `--gray-3`| `#d0d7de` | Default borders, "done" bar fill in Gantt |
| `--gray-4`| `#afb8c1` | Strong borders, arrowheads, "done" stroke |
| `--gray-5`| `#8c959f` | Decorative dots, low-priority text        |
| `--gray-6`| `#6e7781` | Tertiary text                             |
| `--gray-7`| `#57606a` | Muted body text (`--fg-muted`)            |
| `--gray-8`| `#424a53` | Heading on subtle background              |
| `--gray-9`| `#1f2328` | Body text (`--fg`)                        |

**Never use raw `#fff` or `#000`.** Reach for a ramp step instead;
the cool tint keeps neutrals coherent with the brand.

## Functional aliases

Names that describe *role*, not appearance. Prefer these over the
ramp whenever the value has a semantic meaning.

```
--fg            body text
--fg-muted      secondary text
--fg-subtle     tertiary text
--fg-on-accent  text painted on top of an accent fill
--bg            page background
--bg-subtle     panel / inset background
--border        default border
--border-muted  internal hairlines
--border-strong stronger border / divider
--accent        primary action, links, focus ring
--accent-hover  accent on :hover
--success       primary CTA / "done" state
--warning       caution chip, QA accent
--danger        destructive action, "now" line, error state
```

## Status tints

Soft chip backgrounds, paired with a darker token of the same hue
for foreground text. Use for state pills, inline annotations and
empty-state callouts. **Do not use as page background** — they
saturate quickly when extended.

| Tint               | Foreground          | Pairs with               |
|--------------------|---------------------|--------------------------|
| `--tint-blue`      | `--tint-blue-fg`    | `doing`, informational   |
| `--tint-green`     | `--tint-green-fg`   | `done`, success message  |
| `--tint-yellow`    | `--tint-yellow-fg`  | warning, admin badge     |
| `--tint-red`       | `--tint-red-fg`     | `bug`, destructive warn  |
| `--tint-purple`    | `--tint-purple-fg`  | design role, prefer over neon purples |

## Chrome accents

Surface-specific colours that earned their own token slot because at
least two files share the value, but the meaning doesn't fit cleanly
under a general functional alias.

| Token                      | Hex       | Used by                                  |
|----------------------------|-----------|------------------------------------------|
| `--bg-hover`               | `#f3f4f6` | Button / menu / list hover wash          |
| `--tint-red-border`        | `#ffc1ba` | Companion border for `--tint-red`        |
| `--badge-warning-border`   | `#d4a72c` | Admin / chore / note badges              |
| `--topbar-active`          | `#ff8c42` | Topbar / tab active underline (the orange marquee) |
| `--search-hit`             | `#8ec0ff` | Search match highlight outline           |
| `--kind-external`          | `#bc4c00` | Default colour for the arch "external" kind |

## Roles

Each project stores its own role colours in the DB. The defaults
descend from the same hue family the rest of the palette uses, so
even when a project sticks with the defaults nothing clashes:

| Role default      | Token              | Hex       |
|-------------------|--------------------|-----------|
| Backend           | `--role-backend`   | `#1f6feb` |
| Frontend          | `--role-frontend`  | `#2da44e` |
| QA                | `--role-qa`        | `#bf8700` |
| Design            | `--role-design`    | `#8250df` |

A project that customises a role colour stops using these defaults;
the value flows straight from the DB. Custom values should still
respect the rule that they look right when tinted at L≈92 (for
"todo" / "doing" Gantt bars) and L≈25 (for the hover-card chip
text).

## Gantt semantic tokens

The Gantt is the marquee view; it reads colour from a small dedicated
set of slots so the palette can re-skin it without touching `gantt.js`.

| Token                       | Description                                  |
|-----------------------------|----------------------------------------------|
| `--gantt-band-1`            | Default lane fill                            |
| `--gantt-band-features`     | Features (rolled-up parent) band fill        |
| `--gantt-band-separator`    | Hairline between lane rows                   |
| `--gantt-zone-divider`      | Vertical past/now/future divider             |
| `--gantt-zone-past-tint`    | Subtle translucent wash over the past zone   |
| `--gantt-now-line`          | The vertical "now" marker                    |
| `--gantt-now-glow`          | Wider translucent column behind `now-line`   |
| `--gantt-bar-done-fill`     | Monochrome fill for done bars                |
| `--gantt-bar-done-stroke`   | Stroke for done bars                         |
| `--gantt-bar-feature-fill`  | Fill for rolled-up feature aggregates        |
| `--gantt-arrow`             | Dependency arrow default stroke              |
| `--gantt-arrow-promoted`    | Selection-incident arrow stroke              |
| `--gantt-label`             | Default task-label text                      |
| `--gantt-label-muted`       | Subtle / "done" label text                   |
| `--gantt-label-on-tint`     | Label when painted on a role-coloured tint   |

For task bars that vary by role, the colour is computed in `gantt.js`
from the role's hex via OKLCH-style tinting at runtime — see
`_roleTint()` and `_roleInk()` helpers.

## Migration map

Old literals that survived ad-hoc paint, and where they go now:

| Old hex   | Token                          | Notes                              |
|-----------|--------------------------------|------------------------------------|
| `#0969da` | `--accent`                     | Now resolves to `--brand-blue`     |
| `#1f883d` | `--success`                    | Now resolves to `--brand-green`    |
| `#1f2328` | `--fg`                         |                                    |
| `#59636e` | `--fg-muted`                   |                                    |
| `#6e7781` | `--fg-subtle`                  |                                    |
| `#8b949e` | `--gray-5`                     | Decorative greys → ramp step       |
| `#d0d7de` | `--border`                     |                                    |
| `#d1d9e0` | `--border`                     | Was indistinguishable from d0d7de  |
| `#afb8c1` | `--border-strong`              |                                    |
| `#eaeef2` | `--gray-2` / `--border-muted`  |                                    |
| `#f6f8fa` | `--bg-subtle`                  |                                    |
| `#ffffff` | `--bg`                         | Or keep `#fff` for SVG fills only  |
| `#cf222e` | `--danger`                     |                                    |
| `#ffebe9` | `--tint-red`                   |                                    |
| `#ddf4ff` | `--tint-blue`                  |                                    |
| `#dafbe1` | `--tint-green`                 |                                    |
| `#fff8c5` | `--tint-yellow`                |                                    |
| `#0550ae` | `--tint-blue-fg`               |                                    |
| `#1a7f37` | `--success-hover`              |                                    |
| `#9a6700` | `--warning`                    |                                    |
| `#7d4e00` | `--warning-text`               |                                    |
| `#82071e` | `--danger-text`                |                                    |
| `#1f6feb` | `--brand-blue` / `--accent`    | Topbar mark, Backend role          |
| `#2da44e` | `--brand-green` / `--success`  | Topbar mark, Frontend role         |
| `#a371f7` | `--role-design`                | Replaced with calmer `#8250df`     |

## When to add a new token

Add one when the value carries semantic weight (a new state, a new
role, a new chart family) and at least two surfaces will share it.

Don't add one for a one-off accent (e.g. a single illustration's
shadow). Inline literals are fine when they're decorative and local.

Don't add one for a per-instance value that already lives in the
database (custom role colour, custom arch kind colour). Those are
data, not design tokens.

## Rationale snapshot

- **One blue, not two.** `#1f6feb` (brand) replaces `#0969da`
  (Primer) as the single accent. The brand mark, the topbar, the
  focus ring and every link now read as one colour.
- **One green, not two.** `#2da44e` (brand) replaces `#1f883d`
  (Primer) as the `--success` value.
- **Calmer purple.** Design role default moves from `#a371f7` (neon)
  to `#8250df` (closer to Primer purple, less attention-grabbing in
  a list).
- **OKLCH thinking, hex storage.** The hex values committed are
  derived from intentional OKLCH targets; future re-tunes
  (lightening for dark theme, adjusting chroma) edit the OKLCH and
  re-export the hex. Keep this discipline.

## Out of scope (filed separately)

- Dark theme.
- Density variants.
- Branded chart palettes for the architecture view.
