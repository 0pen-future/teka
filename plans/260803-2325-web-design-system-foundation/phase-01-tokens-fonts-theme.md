---
phase: 1
title: "Tokens, Fonts, Tailwind Theme"
status: pending
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: Tokens, Fonts, Tailwind Theme

## Overview

Land the design system's raw material inside `apps/web`: the token CSS files
copied verbatim from the DS package, self-hosted Baloo 2 + Nunito, a Tailwind
v4 `@theme` bridge so the tokens exist as utilities, and a remap of the shadcn
semantic variables so every existing primitive picks up the "Dịu Mát" palette.

## Requirements

- Functional:
  - [ ] All DS custom properties from `tokens/{colors,typography,spacing,effects}.css`
        resolve on `:root` at runtime, byte-identical values to the source files.
  - [ ] `base.css` behaviors applied: body on `--cream-100` with `--ink-700`
        Nunito 600 at 17px, headings Baloo 2 `--ink-900`, global
        `prefers-reduced-motion` kill-switch.
  - [ ] Baloo 2 (600/700/800) and Nunito (400/600/700/800) served from the app
        bundle via Fontsource — **no Google Fonts CDN request** (the DS
        `fonts.css` explicitly says to replace its CDN import when self-hosting).
  - [ ] Tailwind utilities exist for every color step (`bg-mint-50`…`bg-mint-600`,
        `text-ink-900`, `bg-cream-100`, `bg-surface-dark`, sky/sun/coral ramps),
        both font families (`font-display`, `font-body`), press shadows
        (`shadow-press-mint|sky|sun|coral|line`), soft shadows, and DS radii.
  - [ ] shadcn semantic vars remapped: `--background→cream-100`,
        `--foreground→ink-700`, `--primary→mint-400`,
        `--primary-foreground→white`, `--secondary→sky-300`,
        `--destructive→coral-400`, `--muted→cream-200`,
        `--muted-foreground→ink-400`, `--border/--input→line-200`,
        `--ring→mint-200`, `--radius→20px` (DS `--radius-lg`, so shadcn
        `rounded-lg` = DS button radius).
- Non-functional:
  - [ ] Existing auth pages and their vitest suites pass unchanged (palette
        swap only, no layout contract change).
  - [ ] No FOUT worse than today: Fontsource files are imported in
        `globals.css` so Vite inlines/preloads them like the current Geist
        import.

## Architecture

```
globals.css
├─ @import "tailwindcss" (+ tw-animate-css, shadcn/tailwind.css)   — unchanged
├─ @import "@fontsource/baloo-2/{600,700,800}.css"                 — new
├─ @import "@fontsource/nunito/{400,600,700,800}.css"              — new
├─ @import "./tokens/colors.css"      ┐
├─ @import "./tokens/typography.css"  │ copied verbatim from DS package,
├─ @import "./tokens/spacing.css"     │ minus the Google Fonts @import in
├─ @import "./tokens/effects.css"     ┘ fonts.css (replaced by Fontsource)
├─ @theme inline { …maps DS vars → Tailwind namespace… }
├─ :root { …shadcn semantic remap onto DS vars… }
└─ base layer: body/heading rules + reduced-motion kill-switch (from DS base.css)
```

Rules of the bridge:

1. **DS token files are the single source of truth.** They are copied as files,
   not transcribed. The `@theme inline` block references `var(--mint-400)`
   etc.; it never repeats a hex value. Verification is `diff` against the
   decoded copies of the DS files.
2. **`fonts.css` is the one file not copied verbatim**: its Google CDN
   `@import` is replaced by Fontsource imports — exactly the substitution the
   DS file's own comment instructs for production.
3. **Geist is removed** (`@fontsource-variable/geist` import and the
   `--font-sans` mapping): the DS defines the complete typography and no screen
   keeps the old face. `font-sans` maps to Nunito so untouched shadcn internals
   degrade gracefully.
4. Tailwind v4 namespace mapping (CSS-first, no JS config — the app has no
   `tailwind.config`; `apps/web/package.json` uses `@tailwindcss/vite`):
   - `--color-mint-50…600: var(--mint-50…600)` and likewise sky/sun/coral/
     cream/ink/line, plus `--color-surface-dark`, `--color-white`.
   - `--font-display: var(--font-display)`, `--font-body: var(--font-body)`,
     `--font-sans: var(--font-body)`.
   - `--shadow-press-mint: var(--press-mint)` (+ sky/sun/coral/line),
     `--shadow-soft-xs…xl: var(--shadow-xs…xl)`.
   - Radii utilities come from the remapped `--radius` plus arbitrary values
     (`rounded-[var(--radius-xl)]`) where the kit needs 24px; phase 2 documents
     this convention in the kit instead of inventing extra names.

## Related Code Files

- Create: `apps/web/src/styles/tokens/colors.css`
- Create: `apps/web/src/styles/tokens/typography.css`
- Create: `apps/web/src/styles/tokens/spacing.css`
- Create: `apps/web/src/styles/tokens/effects.css`
- Modify: `apps/web/src/styles/globals.css` — fonts, token imports, `@theme`
  bridge, shadcn remap, base layer
- Modify: `apps/web/package.json` — add `@fontsource/baloo-2`,
  `@fontsource/nunito`; remove `@fontsource-variable/geist`
- Modify: `apps/web/index.html` — `<html lang="vi">`, title "Sổ Lớp",
  `theme-color` `#f4f8f3`
- Delete: none

## Implementation Steps

1. `npm --prefix apps/web install @fontsource/baloo-2 @fontsource/nunito` and
   remove `@fontsource-variable/geist`.
2. Copy the four token files from the DS package into
   `apps/web/src/styles/tokens/` (source of record: the design project's
   `_ds/h-c-vui-m-i-ng-y-design-system-…/tokens/` directory). Keep the file
   comments — they carry usage guidance.
3. Rewrite `globals.css` per the architecture block: font imports, token
   imports, `@theme inline` additions (keep the existing shadcn `@theme`
   mappings), shadcn `:root` remap, and the base layer (body, headings,
   `:focus-visible { box-shadow: var(--ring) }`, reduced-motion kill-switch).
   The DS defines no dark mode: leave the `.dark` selector present but unused
   (plan 07 phase 1 removes the theme toggle from the teacher shell).
4. Update `index.html` (lang, title, theme-color; favicon can stay until plan
   07 replaces it).
5. Verify: `npm --prefix apps/web run dev`, confirm computed styles on `body`
   (`background-color: rgb(244,248,243)`, `font-family: Nunito…`), confirm the
   network panel shows only same-origin font requests.
6. Run typecheck, lint, vitest (auth suites must pass), and `diff` the copied
   token files against the DS originals.

## Success Criteria

- [ ] `diff` of the copied token files vs the DS sources shows no drift.
- [ ] Zero requests to `fonts.googleapis.com` / `fonts.gstatic.com`.
- [ ] `bg-mint-400 text-white font-display shadow-press-mint` compiles into a
      chunky mint sample in a scratch component (removed before merge).
- [ ] shadcn `Button`/`Input` render mint/cream/ink without per-component edits.
- [ ] Auth vitest suites green; typecheck and lint green.

## Risk Assessment

| Risk | Mitigation |
|---|---|
| Fontsource `baloo-2` package misses weight 800 | Check package contents before wiring; fall back to the variable-font package or add the static 800 file. |
| shadcn oklch values linger and mix with DS hex | The remap replaces values wholesale inside `:root`; grep for `oklch(` after the edit — remaining occurrences must be justified (charts, `.dark`). |
| `--radius: 20px` changes existing dialogs' look mid-flight | That is the intended DS look; visual check on the auth screens is the phase's verification, not a regression. |

**Rollback:** revert `globals.css`, `package.json`, `index.html`; delete
`styles/tokens/`. No component code depends on this phase until phase 2 lands.
