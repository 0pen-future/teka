---
title: Web Design System Foundation completed — Học Vui Mỗi Ngày kit
date: 2026-08-04
summary: "Integrated the Dịu Mát design system into apps/web: verbatim token contract bridged into Tailwind v4, self-hosted fonts, and a chunky-press HV component kit (button, card, pills, progress, modal, toast, icons) with tests."
---

# Web Design System Foundation completed — Học Vui Mỗi Ngày kit

## What happened

Completed the two-phase Web Design System Foundation plan (`teka/260803-1625`),
the shared base that plans 07 and 08 build every screen from.

**Phase 1 — tokens, fonts, theme.** Copied the four DS token files
(`colors/typography/spacing/effects.css`) verbatim into
`apps/web/src/styles/tokens/`. Rewrote `globals.css` to bridge every DS custom
property into Tailwind v4 utilities via `@theme inline` (color ramps, press +
soft shadows, fonts, radii), remapped the shadcn semantic layer onto the "Dịu
Mát" palette so existing primitives inherit the system, and applied the base
layer (cream page, Baloo headings, focus ring, reduced-motion kill-switch).
Fonts self-hosted through `@fontsource` (Baloo 2 600/700/800, Nunito
400/600/700/800) with vietnamese subset — zero third-party requests.

**Phase 2 — component kit.** Built `apps/web/src/components/hv/`: HvButton
(5 variants × 3 sizes, chunky 5px press), HvCard, StatusPill (collections trio),
HvBadge, StatPill, ProgressBar, HvModal (responsive bottom-sheet<sm /
centered>=sm over radix), HvToast (reuses the mounted sonner Toaster), HvIcon
(lucide re-exports), a barrel, and the keyframes file. Every color/size/shadow
routes through a token utility or `var(--token)` — a grep gate enforces no raw
hex.

Code review (DONE_WITH_CONCERNS) drove four post-build fixes: ProgressBar
`missing`→coral fill, HvModal always-accessible-name + suppressed Description
warning, HvToast 2600ms auto-dismiss, and responsive slideUp/popIn modal
animation. Final: typecheck + lint clean, 9 test files / 49 tests pass, grep
gate clean, scope additive (no regression to existing auth/users suites).

## Decision

When the phase spec's prose and the design project's `_ds_bundle.js` recipe
disagree, the bundle is authoritative (100%-design-system rule). Reconciliations
(HvCard shadow level, ProgressBar easing/track, ghost-button 5px press, the
ring/focus-shadow collision resolution, HvModal wrapping DialogPrimitive
directly) are recorded in `adr.md`.

## Next steps

- Plan 07 (web teacher app) — mobile-first screens on the HV kit.
- Plan 08 (web parent statement page) — public statement using StatusPill +
  ProgressBar `missing`.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
