---
phase: 2
title: "Core Components"
status: completed
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: Core Components

## Overview

Build the reusable "Học Vui Mỗi Ngày" component kit in
`apps/web/src/components/hv/` — the chunky press Button, Card family, status
pills, StatPill, ProgressBar, modal shell, and toast that every screen in plans
07 and 08 composes from. Recipes come from the DS bundle
(`_ds_bundle.js`, namespace `HCVuiMINgYDesignSystem_0bd86b`) and the prototype's
inline styles; this phase ports them to React + Tailwind utilities from phase 1.

## Requirements

- Functional (each component matches the DS/prototype recipe exactly):
  - [x] `HvButton` — variants `primary` (mint-400 bg, white ink, press-mint),
        `secondary` (sky-300, press-sky), `reward` (sun-400 bg, sun-600 ink,
        press-sun), `danger` (coral-400, press-coral), `ghost` (white bg, inset
        line border, press-line); sizes `sm` 44px / `md` 56px / `lg` 64px
        min-height; optional `block`; optional leading/trailing icon slots;
        `font-display` bold, wide tracking, `--radius-lg` 20px;
        `box-shadow: 0 5px 0 <press>`; `:active` → `translateY(5px)` and shadow
        collapse; `:focus-visible` → `--ring`.
  - [x] `HvCard` — white bg, `--radius-xl` 24px, `--pad-card` 20px; variants
        `raised` (default, `--shadow-sm`), `flat` (border line-200, no shadow),
        `sunken` (cream-200 bg, inset feel); `interactive` adds hover lift
        (translateY(-2px) + `--shadow-md`, duration 140ms `--ease-out`).
  - [x] `StatusPill` — the collections trio used across close/pay/students
        screens: `paid` mint-50 bg / mint-600 text ("Đã đóng"), `partial`
        sun-100 / sun-600 ("Đóng thiếu"), `unpaid` coral-100 / coral-600
        ("Chưa đóng"); `--radius-pill`, `font-display` 700, 13px.
  - [x] `HvBadge` — small rounded label matching the DS badge recipe (subject
        variants mint/sky, `sm` size, `solid` option) for class tags and
        display-note markers.
  - [x] `StatPill` / `ProgressBar` — DS recipes: StatPill = icon + number
        (Baloo 2) + label on white pill; ProgressBar = line-200 track,
        mint-400 fill (coral-400 when the semantic is "missing"), radius-pill,
        360ms `--ease-soft` width transition.
  - [x] `HvModal` — shell over the existing shadcn `Dialog`: overlay
        `rgba(28,58,49,.4)`, panel white `--radius-xl`+ (prototype uses 24px),
        `popIn` 220ms `--ease-soft` (scale .96→1 + fade), header `font-display`
        700 `--ink-900`, close affordance ≥48px. Responsive built in: bottom
        sheet (full-width, rounded top corners only, slide-up) under `sm`;
        centered panel `max-w-md` (≤480px) from `sm` up — consumers in plans
        07/08 get this behavior for free.
  - [x] `HvToast` — fixed bottom-center pill, ink-900 bg, white `font-display`
        text, `toastIn` 250ms rise+fade, auto-dismiss ~2600ms, respects
        reduced motion; exposed via a tiny `useHvToast()` (or wraps the
        existing sonner/toaster if present — reuse before adding a dependency).
  - [x] Icons — re-export the prototype's icon set (home, check, users, file,
        send, wallet, x, plus…) from `lucide-react` with DS defaults (2px
        stroke, round caps, 20px) as `HvIcon`/named exports.
- Non-functional:
  - [x] Every interactive element ≥ `--touch-min` 48px hit area (44px `sm`
        button is DS-sanctioned for dense secondary actions only — documented
        on the prop).
  - [x] All animations gated by the phase 1 reduced-motion kill-switch.
  - [x] Hover-only affordances (`HvCard interactive` lift, ghost hover tints)
        wrapped in `@media (hover: hover)` so touch devices (phone/tablet)
        never stick a hover state.
  - [x] Kit exports one barrel `apps/web/src/components/hv/index.ts`;
        components typed with explicit variant/size unions (cva or plain maps,
        following `apps/web/src/components/ui` conventions).

## Architecture

`components/hv/` sits beside `components/ui/` (shadcn): `ui/` keeps machinery
primitives (Dialog, Popover, Form, Select…), `hv/` owns everything with a
visible DS identity. Screens import from `@/components/hv`; they touch `ui/`
directly only for form machinery. Variant styling uses `class-variance-authority`
(already a shadcn dependency) with Tailwind utilities from phase 1 — no inline
hex, no new CSS files except `hv-animations.css` for the `popIn`/`toastIn`
keyframes.

## Related Code Files

- Create: `apps/web/src/components/hv/hv-button.tsx`
- Create: `apps/web/src/components/hv/hv-card.tsx`
- Create: `apps/web/src/components/hv/status-pill.tsx`
- Create: `apps/web/src/components/hv/hv-badge.tsx`
- Create: `apps/web/src/components/hv/stat-pill.tsx`
- Create: `apps/web/src/components/hv/progress-bar.tsx`
- Create: `apps/web/src/components/hv/hv-modal.tsx`
- Create: `apps/web/src/components/hv/hv-toast.tsx`
- Create: `apps/web/src/components/hv/hv-icon.tsx`
- Create: `apps/web/src/components/hv/hv-animations.css`
- Create: `apps/web/src/components/hv/index.ts`
- Create: `apps/web/src/components/hv/__tests__/hv-button.test.tsx`
- Create: `apps/web/src/components/hv/__tests__/status-pill.test.tsx`
- Create: `apps/web/src/components/hv/__tests__/hv-modal.test.tsx`
- Modify: `apps/web/src/styles/globals.css` — import `hv-animations.css` if not
  co-located via the component import
- Delete: none

## Implementation Steps

1. Port `HvButton` from the DS bundle's `hv-btn` CSS (variant custom-property
   pattern `--_bg/--_press/--_ink` translates to cva variant maps). Assert the
   three min-heights and the press transform in a component test.
2. Port `HvCard` + `interactive` hover from the bundle's card recipe.
3. Build `StatusPill`, `HvBadge`, `StatPill`, `ProgressBar` from the bundle +
   prototype styles (`PILLS` / `STATUS_L` maps in the prototype are the
   canonical color/label pairs).
4. Build `HvModal` wrapping shadcn `Dialog` with DS overlay/panel/keyframes;
   verify the existing dialog a11y (focus trap, esc) still works.
5. Build `HvToast` (reuse an existing toaster dependency if one is installed;
   otherwise a ~40-line context + portal is enough — YAGNI over adding a lib).
6. Build `HvIcon` re-exports from `lucide-react` (add the dependency if absent
   — check `package.json` first).
7. Write the three test files: variant classes, labels ("Đã đóng"/"Đóng
   thiếu"/"Chưa đóng"), press/hover class presence, modal open/close, toast
   auto-dismiss (fake timers).
8. Run typecheck, lint, vitest.

## Success Criteria

- [x] A kitchen-sink render (test-only) shows all five button variants × three
      sizes matching the prototype's look: chunky shadow, correct pressed
      color, Baloo 2 labels.
- [x] StatusPill colors/labels byte-match the prototype `PILLS`/`STATUS_L`
      maps.
- [x] Modal and toast animate with `--ease-soft` and stay static under
      `prefers-reduced-motion: reduce`.
- [x] No raw hex colors in `components/hv/` (grep gate) — tokens/utilities
      only.
- [x] typecheck, lint, vitest pass.

## Risk Assessment

| Risk | Mitigation |
|---|---|
| Kit drifts from prototype because recipes were re-typed | Source values verified against `_ds_bundle.js` extracts; the grep gate bans raw hex so any color must route through a token. |
| shadcn Dialog styling fights the DS panel look | `HvModal` owns the panel classes; only behavior (portal, focus trap) is inherited. |
| Toast implementation duplicates an existing notification path | Step 5 checks for an installed toaster first; reuse wins. |

**Rollback:** delete `components/hv/`; nothing else references it until plans
07/08 consume it.
