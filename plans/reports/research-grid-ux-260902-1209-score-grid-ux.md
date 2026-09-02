# Research: score-entry grid UX (20 students × ≤10 cols)

Context read: `component-score-grid.tsx` (plain `<table>`, `type="number"` inputs,
manual "Lưu" button, sticky *first column only*, no keyboard nav, no
dirty/saved/invalid cell states), `use-debounced-save.ts` (trailing debounce +
flush-on-unmount, reused elsewhere for per-keystroke autosave), `use-component-scores.ts`
(PUT is a wholesale replace of the session's score set, echoes full rows back).

## 1. Keyboard nav: table-of-inputs vs role="grid"

**Recommendation: keep a plain `<table>` of labelled inputs, not `role="grid"`.**
A native table gives Tab-moves-right for free (DOM order); you only need to
add Enter/Shift+Enter for vertical movement. `role="grid"` requires reimplementing
the *entire* interaction model yourself — single tab-stop, roving `tabindex`,
arrow-key 2-D navigation, `aria-rowindex`/`aria-colindex` — because assistive
tech treats a grid as one stop, not N focusable inputs.[^1][^2] For a
teacher-only internal tool (not a spreadsheet product), that cost isn't
justified — YAGNI. Plain inputs are also far simpler to query in Testing
Library (`getByLabelText`), whereas a grid pattern needs role/coordinate-based
queries and manual key-simulation for every test.

**Implementation for Enter/Shift+Enter:** use a `useRef<Record<string, HTMLInputElement | null>>`
keyed by the same `cellKey(studentId, componentId)` already in the component,
not `document.querySelector` (avoids DOM coupling, works with virtualization
later). On `onKeyDown`: Enter → focus the ref for `(nextRow, sameCol)`,
Shift+Enter → `(prevRow, sameCol)`, Tab is left to the browser. Precompute a
`studentId` order array once (`rosterRows` is already ordered) so "next/prev
row" is an index lookup, not a DOM walk.[^3]

Trade-off: refs matrix (chosen) vs `data-row`/`data-col` + delegated
`querySelector` — refs avoid string-parsing and stray-DOM-node bugs, cost is
one extra ref per cell (200 refs max here, trivial).

## 2. Decimal input on mobile

**Recommendation: `<input type="text" inputmode="decimal" pattern="[0-9]*[.,]?[05]?">`**
(pattern loosely allows the 0.5-step shapes; keep JS-side `parseScoreInput`
as the real validator, pattern is just a mobile-keyboard/UX hint, not the
source of truth). Reasons to move off `type="number"`, confirmed by 3
independent sources:

- Chrome/desktop shows spinner arrows that fire on accidental scroll-wheel
  over a focused cell — exactly the kind of unintended edit a 20×10 grid
  invites.[^4]
- `type="number"` rejects the comma decimal separator in comma-locale
  keyboards and behaves inconsistently across browsers for decimals.[^4][^5]
- `inputmode="decimal"` still opens the numeric-with-separator keyboard on
  iOS/Android while keeping `type="text"` semantics (no spinner, consistent
  comma/dot handling left to app code).[^5][^6]

Current code (`component-score-grid.tsx:142`) uses `type="number" step={0.5}`
— this is the one concrete regression risk to fix if this grid gets a
mobile/tablet audience (teachers marking from a phone is plausible for this
product).

## 3. Sticky header + sticky first column

Current grid only sticks the first column; header scrolls away in a tall
roster. Minimal correct recipe, cross-checked against 4 sources:[^7][^8][^9]

- `border-collapse` breaks sticky borders (the border "unglues" from the
  cell once it's pinned) — must use `border-separate` (already the case here:
  `border-spacing-y-1`). Good, no change needed.
- Sticky must be applied to `<th>`/`<td>` directly, not `<thead>`/`<tr>` — the
  header row's stickiness comes from `sticky top-0` on every `<th>` in it.
- The **corner cell** (row-0/col-0) needs sticky in both axes and the
  **highest** `z-index` of all sticky cells (header cells above body cells
  for vertical scroll; first-column cells above body for horizontal scroll;
  corner above both).
- Every sticky cell needs an opaque `background-color` or content scrolls
  through it illegibly.
- The scroll container needs `overflow: auto/auto` (already `overflow-x-auto
  overflow-y-auto` on the wrapper `div` at line 109) — sticky positioning is
  relative to *that* scrollport, not the table.

Tailwind recipe (extending what's already there):
```html
<div class="max-h-[280px] overflow-auto">
  <table class="border-separate border-spacing-y-1">
    <thead>
      <tr>
        <th class="sticky top-0 left-0 z-20 bg-white">...</th>  <!-- corner -->
        <th class="sticky top-0 z-10 bg-white">...</th>          <!-- header -->
      </tr>
    </thead>
    <tbody>
      <tr>
        <td class="sticky left-0 z-10 bg-white">...</td>         <!-- 1st col -->
        <td>...</td>
      </tr>
    </tbody>
  </table>
</div>
```

## 4. Autosave-on-blur + explicit Save, avoiding double-submit

- **Coexistence pattern:** blur triggers `schedule()` (debounced ~800ms, per
  `use-debounced-save.ts`), the visible "Lưu" button calls `flush()`
  immediately — both funnel into the *same* pending-payload ref, so there is
  one in-flight mutation path, not two competing writers.[^10]
- **Double-submit guard:** gate on `saveMutation.isPending` — disable the Save
  button and skip scheduling a new debounce timer while a mutation is in
  flight; TanStack Query's `mutate` already queues/replaces via the hook's own
  `pendingRef`, but the *button* still needs a disabled state so a user can't
  fire a second overlapping PUT mid-flight (current code has no such guard —
  `saveScores` can be called again while `saveMutation.isPending`).[^11]
- **Batching multiple dirty cells into one PUT:** already implemented
  correctly — `draft` is a single dict accumulating all touched cells,
  flushed as one array of `PutSessionScoreEntryInput`. This is the right
  shape; keep per-field-level debounce OFF (i.e., don't debounce per
  keystroke here) since blur-per-cell + explicit flush is coarser and
  matches how `useSaveSessionScores` treats the PUT as a wholesale
  replace-batch, not a per-cell endpoint.
- **Toast cadence:** once per flush (on mutation `onSuccess`), never per cell
  — current code already does this (`saveScores`'s `onSuccess` toasts once
  with a cell count). Keep it.
- **Unsaved-cells protection:** add a `beforeunload` listener gated on
  `dirty` for tab-close/refresh, *and* an in-app confirm (e.g. a Radix
  `AlertDialog`) when the panel/modal that hosts this grid is closed/routed
  away from while `dirty` — `beforeunload` cannot intercept in-app
  navigation, only actual browser unload, so both are needed.[^12][^11]

## 5. Visual cell states (dirty / just-saved / invalid)

- **Dirty:** border or background tint change (e.g., amber/sun ring) — purely
  additive to existing `border-2 border-line-200 focus:border-mint-400`.
- **Just-saved flash:** brief (~1s) transition to a success tint then back to
  neutral, triggered off the mutation's `onSuccess` per saved key — cosmetic,
  skip if it adds meaningful complexity per KISS; the single toast already
  confirms the save adequately for a 20×10 form.
- **Invalid:** `aria-invalid="true"` on the input plus visible marker (icon or
  border color) — **never color alone**, per WCAG 1.4.1 and 3 independent
  sources.[^13][^14][^15] Pair with `aria-describedby` pointing at a
  visually-present (not just visually-hidden) inline message when the score
  fails `parseScoreInput`, since a silently-dropped invalid entry (current
  behavior: unparsable input is dropped on save, line 82-85) is worse UX than
  telling the teacher why nothing saved.

## Fit for this codebase

- Reuse `useDebouncedSave` for the blur-driven flush (already the project's
  debounce primitive, used elsewhere) — don't add a new debounce utility.
- Reuse `hvToast` for the single post-flush toast — already wired.
- Reuse Radix (`AlertDialog` via `hv-modal` presumably) for the "unsaved
  changes" in-app confirm — already a dependency, don't add a new modal lib.
- No new dependency needed for any of the 5 items — all achievable with
  existing React 19 + Tailwind v4 + TanStack Query + Radix stack.

## Sources
[^1]: https://www.w3.org/TR/2016/WD-wai-aria-practices-1.1-20161214/examples/grid/dataGrids.html
[^2]: https://accessibility.build/guides/accessible-data-grid
[^3]: https://www.uxpin.com/studio/blog/keyboard-navigation-patterns-complex-widgets/
[^4]: https://n8d.at/inputtypenumber-and-why-it-isnt-good-for-your-user-experience/
[^5]: https://css-tricks.com/everything-you-ever-wanted-to-know-about-inputmode/
[^6]: https://css-tricks.com/finger-friendly-numerical-inputs-with-inputmode/
[^7]: https://css-tricks.com/a-table-with-both-a-sticky-header-and-a-sticky-first-column/
[^8]: https://clothiernamedjeremiah.medium.com/deep-dive-tables-with-sticky-headers-and-columns-9cbbeb286e73
[^9]: https://css-tricks.com/position-sticky-and-table-headers/
[^10]: https://kannanravi.medium.com/implementing-efficient-autosave-with-javascript-debounce-techniques-463704595a7a
[^11]: https://github.com/jaredpalmer/formik/issues/172
[^12]: https://en.wikipedia.org/wiki/Autosave
[^13]: https://webaim.org/techniques/formvalidation/
[^14]: https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Reference/Attributes/aria-invalid
[^15]: https://www.w3.org/WAI/WCAG21/Techniques/aria/ARIA21

## Adoption risk / maturity
All techniques are stable web platform features (CSS `position: sticky` since
2017, `inputmode` since 2019 broad support, ARIA `aria-invalid` since ARIA 1.0)
— no experimental APIs, no new npm packages, zero abandonment risk.

## Unresolved questions
- Is mobile/tablet score entry actually a real use case for this product, or
  desktop-only? Determines whether the `type="number"` → `inputmode="decimal"`
  change is worth the churn now vs. deferred.
- Is a per-cell "just-saved flash" wanted, or does the existing single toast
  suffice? (Recommended: skip, per KISS, unless a maintainer wants it.)

Status: DONE
Summary: Plain table + refs-matrix keyboard nav (not role=grid) is the right
fit; switch cells to `type="text" inputmode="decimal"`; sticky corner cell
needs highest z-index; reuse existing `useDebouncedSave`/`hvToast` for
autosave-on-blur + explicit flush with an `isPending` double-submit guard and
`beforeunload` + in-app confirm for unsaved cells; invalid cells need
`aria-invalid` + visible text, not color alone.
Concerns/Blockers: none — all recommendations are additive to existing code
with no new dependencies.
