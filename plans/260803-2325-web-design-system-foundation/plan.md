---
title: "Web Design System Foundation"
description: "Integrate the 'Học Vui Mỗi Ngày' design system (direction 'Dịu Mát') into apps/web: self-hosted Baloo 2 + Nunito, full token set mapped into Tailwind v4, and the chunky-press component kit (Button, Card, pills, progress, modal shell, toast) that plans 07 and 08 build every screen from."
status: completed
priority: P1
effort: "2d"
branch: main
tags: [web, react, typescript, tailwind, design-system, tokens, components]
created: 2026-08-03
blocks: [260803-2244-07-web-teacher-app, 260803-2244-08-web-parent-statement-page]
---

# Web Design System Foundation

## Overview

The Sổ Lớp UI (plans 07 and 08) must follow the imported Claude Design system
**"Học Vui Mỗi Ngày" — direction "Dịu Mát"** 100%. The authoritative sources
were read from the design project
(`claude.ai/design/p/4a7e6c77-0971-44fb-9766-1b6429e8b126`):

- `_ds/h-c-vui-m-i-ng-y-design-system-…/tokens/{colors,typography,spacing,effects,fonts,base}.css`
  — the complete CSS-custom-property contract.
- `_ds/…/styles.css` — the single import-only entry consumers link.
- `_ds/…/_ds_bundle.js` — reference component recipes (`hv-btn`, `hv-card`,
  badge, progress, stat pill) under namespace `HCVuiMINgYDesignSystem_0bd86b`.
- `So Lop - Prototype.dc.html` — the six-screen prototype whose exact styling
  plans 07/08 replicate.

This plan owns the one-time integration so both consumer plans start from an
identical, tested base instead of re-deriving tokens per screen. It replaces the
current neutral shadcn/Geist look (`apps/web/src/styles/globals.css:1`) with the
design system while keeping the shadcn primitives (dialog, popover, form
machinery) working underneath.

Design-system character in one line: **cream paper background, ink-green text,
mint/sky/sun/coral pastel accents, Baloo 2 display + Nunito body, 20–24px radii,
soft shadows, and chunky 5px "press" buttons that physically depress on tap.**

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Every token from the DS (`colors`, `typography`, `spacing`, `effects`) available both as raw CSS custom properties and as Tailwind v4 utilities (`bg-mint-400`, `font-display`, `shadow-press-mint`, `rounded-xl2`…) | P1 |
| 2 | Baloo 2 + Nunito self-hosted via Fontsource (DS `fonts.css` ships Google CDN with an explicit "self-host for production" caveat) | P1 |
| 3 | Chunky component kit matching the prototype pixel-for-pixel: `HvButton` (primary/secondary/reward/danger/ghost × sm/md/lg × block), `HvCard` (raised/flat/sunken/interactive), status pills, `StatPill`, `ProgressBar`, modal shell, toast | P1 |
| 4 | shadcn semantic variables remapped so existing primitives (inputs, dialogs, selects) inherit the DS palette without per-usage overrides | P2 |
| 5 | `prefers-reduced-motion` kill-switch and ≥48px touch targets baked into the kit, per DS `base.css` and spacing tokens | P2 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Tokens, fonts, Tailwind theme](./phase-01-tokens-fonts-theme.md) | Completed |
| 2 | [Core components](./phase-02-core-components.md) | Completed |

Sequential: phase 2 consumes the utilities phase 1 creates.

## Token Contract (authoritative summary)

Copied values verified against the DS token files; the implementation copies the
files themselves, this table is for review only.

**Colors** — brand `--mint-400 #5cc9a7` (pressed `--mint-500`), secondary
`--sky-300 #7fc8e8`, reward/warning `--sun-400 #ffc83d`, danger `--coral-400
#ff7a66`, page `--cream-100 #f4f8f3`, text `--ink-900 #1c3a31` / `--ink-700
#27433b`, celebration surface `--surface-dark #16514c`, borders `--line-200`.
Each hue has a 50→600 ramp plus semantic aliases (`--text-*`, `--surface-*`,
`--brand-*`, `--success/--info/--warning/--danger`).

**Typography** — `--font-display: 'Baloo 2'` (headings, weights 600–800),
`--font-body: 'Nunito'` (body, default weight 600), base `--text-md: 17px`,
scale up to 60px.

**Effects** — radii `--radius-xl 24px` (cards) / `--radius-lg 20px` (buttons) /
`--radius-pill 999px`; soft shadows `--shadow-xs…xl`; press shadows
`--press-mint: 0 5px 0 var(--mint-500)` (+ sky/sun/coral/line variants,
`--press-depth: 5px`; `:active` → `translateY(5px)` + shadow collapses); focus
`--ring: 0 0 0 4px var(--mint-200)`; easing `--ease-soft
cubic-bezier(.34,1.56,.64,1)`; durations 140/220/360/600ms.

**Spacing** — 4px scale, `--touch-min 48px`, `--touch-kid 64px`, widths
`--w-phone 390px` / `--w-content 720px` / `--w-page 1080px`, `--pad-card 20px`.

**Iconography** — Lucide-style 2px-stroke round-cap SVG; the prototype's inline
`ICONS` set (home/check/users/file/send/wallet) maps directly onto the
`lucide-react` package.

## Success Criteria

- [x] `npm --prefix apps/web run dev` renders the app on `--cream-100` with
      Baloo 2 headings and Nunito body, zero Google Fonts network requests.
- [x] `bg-mint-400`, `text-ink-900`, `font-display`, `shadow-press-mint`,
      `rounded-[var(--radius-xl)]`-equivalent utilities compile and are used by
      the component kit.
- [x] `HvButton` variants visually match the prototype: chunky 5px press shadow,
      `translateY` on `:active`, min-heights 44/56/64px.
- [x] Status pills render the exact prototype palette (paid mint-50/mint-600,
      partial sun-100/sun-600, unpaid coral-100/coral-600).
- [x] `prefers-reduced-motion: reduce` disables press/pop/toast animations.
- [x] Existing auth screens still render and pass their tests after the shadcn
      variable remap (no regression in `features/auth`).
- [x] typecheck, lint, vitest pass.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Remapping shadcn `:root` variables breaks existing components' contrast (e.g. `--primary-foreground` on mint) | Medium | Medium | Phase 1 keeps the oklch shadcn block but re-derives values from DS tokens in one place; auth screens re-checked in phase 1's verification step. |
| Tailwind v4 `@theme` and DS raw custom properties drift (two names for one value) | Medium | Medium | DS token files are imported verbatim as the single source; `@theme inline` maps onto `var(--…)` references, never copies hex values. |
| Fontsource weights differ from DS assumptions (Baloo 2 600–800, Nunito 600–800) | Low | Medium | Phase 1 pins explicit weight imports and asserts rendered `font-family` in a smoke test. |
| Chunky press shadow conflicts with shadcn Button in shared flows | Medium | Low | The kit is additive (`HvButton` alongside shadcn `Button`); plans 07/08 use `HvButton` for all teacher/parent-facing actions, shadcn `Button` survives only inside unported shadcn internals. |

## Open Questions

None — the design source is fully read and the token values are fixed by the DS
files. Anything ambiguous in a specific screen is owned by plans 07/08.

<!-- slug: web-design-system-foundation -->
