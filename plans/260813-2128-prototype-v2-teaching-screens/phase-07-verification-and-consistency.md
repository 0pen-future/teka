---
phase: 7
title: "Verification & consistency"
status: completed
priority: P1
effort: "0.5d"
dependencies: [2, 3, 4, 5, 6]
---

# Phase 7: Verification & consistency

## Overview

Cross-cutting verification pass over all four screens: full test suite, static checks, responsive behavior, accessibility, design-system fidelity audit, and delivery journal.

## Requirements

- Functional: every earlier phase's screens work together — nav deep links, cross-screen store consistency (classbook ↔ records ↔ lesson-plans), center switching resets visible teaching data to the new center's slice.
- Non-functional gates:
  - Full web suite green (`npm test` in `apps/web`), eslint 0 errors, `tsc` clean, production build succeeds.
  - Responsive: all four screens usable at sidebar (desktop), md icon-rail, and bottom-tab (mobile) breakpoints; wide tables scroll within their card (`overflow:auto`) instead of the page; detail panels stack under tables on narrow widths (prototype uses `flex-wrap`).
  - A11y: new nav entries inside the existing `role="group"`/`aria-label` structure; modals focus-trapped + Escape-dismiss; status chips carry text (never color-only); inline note inputs and score inputs labeled (`aria-label` where the visual design omits labels); press-shadow buttons retain visible focus styles.
  - DS fidelity: grep-audit new code for hardcoded hex/rgb values — only token `var(--…)` usages (plus `#fff` on filled press buttons and the `rgba(...)` overlay/chip values the prototype itself specifies); Baloo 2 only via the existing display-font utility; radii/shadows from tokens.
- Docs: user-visible behavior changed (three new screens + nav) → update the smallest owning docs surface (discover via README/docs navigation per `documentation-management.md`); write the delivery journal entry in `plans/journals/`.

## Architecture

Verification-only phase — no new product code. Fixes discovered here land in the owning feature files; if a fix is non-trivial, reopen the owning phase's checklist rather than patching blind. Consider the repo's review capability (code-reviewer agent) for the cross-module surface: nav gating + store are shared contracts touched by five phases.

## Related Code Files

- Modify (fix-ups only): files from Phases 1–6 as findings dictate
- Modify: docs surface owning app screens/navigation (discovered, not assumed)
- Create: `plans/journals/2026-08-XX-prototype-v2-teaching-screens.md`

## Implementation Steps

1. Run the full gates: web test suite, eslint, tsc, build. Fix regressions at the source (never weaken tests).
2. Manual pass on the dev server at three breakpoints per screen; verify deep links, center switch isolation, and the full plan-review loop end-to-end.
3. Keyboard-only + screen-reader-label audit of new interactive surfaces; fix gaps.
4. Token/DS grep audit (`#[0-9a-fA-F]{3,8}` and `rgb(` outside allowed cases) over `features/teaching/` + touched roster/layout files.
5. Run code review over the whole branch surface (cross-module: layout, roster, teaching, hv components); triage findings per `review-audit-self-decision.md`.
6. Update owning docs; write the journal entry; compare the result against plan.md's success criteria before calling delivery complete.

## Success Criteria

- [x] All plan-level success criteria in `plan.md` check off against live behavior.
- [x] Suite green, eslint 0 errors, tsc clean, build passes.
- [x] Responsive + a11y checks pass at all three breakpoints; no color-only status signaling.
- [x] No stray hardcoded colors; DS audit clean.
- [x] Docs updated; journal written.

## Risk Assessment

- **Verification finds a cross-cutting design flaw late** (e.g. store shape wrong for the review loop) → the store's single transition function and pure derivation modules localize such fixes; phase-level reopen protocol above prevents blind patching.
- **Doc surface ambiguity** → discover through README/docs navigation as the documentation rule requires; do not invent a new docs tree.
