# Score-set editor patterns — research

Scope: current `ScoreSetEditorModal` (string[] via `watch`/`setValue`), reorder UX, paste-list mode, Radix radio-cards, responsive table.

## 1. useFieldArray vs current watch/setValue on `string[]`

Current code (`score-set-editor-modal.tsx:56-83`): `components` is `form.watch("components")` (a `string[]`), mutated via `setComponents()` → `form.setValue(..., {shouldValidate, shouldDirty})`. Schema (`grading.ts:21-38`) validates `z.array(z.string()...)` with a `superRefine` for dup-detection, errors surfaced at `errors.components?.[index]` and `errors.components?.root`.

- **`useFieldArray` requires object rows**, not primitives — RHF field arrays are typed as `{ id, ...T }[]`; a schema of `z.array(z.string())` doesn't map directly. Adopting it means changing the schema to `z.array(z.object({ value: z.string()... }))` (or wrapping/unwrapping at submit), which ripples into the zod schema, API mapping (`components.map(c => c.value.trim())`), and existing tests. [react-hook-form.com/docs/usefieldarray](https://react-hook-form.com/docs/usefieldarray)
- **Stable keys**: RHF's own docs state `field.id` (not `index`) must be the React `key` "to prevent re-renders breaking the fields" — current code uses `index` as `key` (`score-set-editor-modal.tsx:145`), which is fine only because rows are plain `<Input value=.../>` with no internal uncontrolled state; `useFieldArray` fixes this generically but the current code doesn't need it since nothing keeps hidden state per row.
- **`move`/`remove`/`append`** are built-in and avoid manual splice logic (current `moveComponent` hand-rolls splice, ~10 LOC) — modest LOC savings, not a functional gap.
- **Per-row errors**: with `useFieldArray`, path becomes `errors.components?.[index]?.value?.message` (extra `.value` hop) vs current `errors.components?.[index]?.message` — marginally noisier.
- **Re-seeding on reopen**: current `form.reset(toDefaults(scoreSet))` in a `useEffect` already handles this correctly for `string[]`. With `useFieldArray`, `reset()` also resyncs the field array (this is the documented pattern), so no behavior change — but per RHF docs, don't chain a mutation immediately after `reset()` in the same tick; add a render in between (already true here since reset happens on `open` transition, not adjacent to a mutation).

**Trade-off verdict**: `useFieldArray` is the "more idiomatic RHF" choice for object arrays, but here it forces a schema/shape change for a max-10-item primitive list where `watch`+`setValue` already works, is simpler (KISS/YAGNI), and the RHF discussion threads note `setValue`-driven `replace`-style updates can unmount/remount only when using field-array's own `replace`, not applicable here since there's no field array today. **Recommendation: keep current `watch`/`setValue` pattern.** Only migrate to `useFieldArray` if the row shape grows to include more than a name (e.g., weight, id) — then object rows become natural and the `.id` stability actually matters. [react-hook-form.com/docs/usewatch](https://react-hook-form.com/docs/usewatch), [github.com/orgs/react-hook-form/discussions/11014](https://github.com/orgs/react-hook-form/discussions/11014)

## 2. Reorder: move-up/down buttons vs `@dnd-kit`

Current: 44px-ish icon buttons (↑/↓) per row, disabled at edges, `aria-label` per direction (`score-set-editor-modal.tsx:155-172`). No DnD lib is installed (`package.json` confirmed — no `@dnd-kit/*` dep).

- **dnd-kit** ships built-in keyboard sensor + ARIA live-region announcements, described by its own docs as accessible by design. [docs.dndkit.com/guides/accessibility](https://docs.dndkit.com/guides/accessibility)
- Bundle cost: dnd-kit trims non-English strings specifically to control bundle size, implying core+sortable is still non-trivial (`@dnd-kit/core` + `@dnd-kit/sortable` + `@dnd-kit/utilities`, ~10-15KB gzip combined) — a new dependency surface (touch handling, sensors, collision detection) for a **max-10-item, keyboard-first form list**.
- General guidance found: "Up/down buttons per row are acceptable for a11y, but provide poor UX compared to drag-and-drop and require more work for large reorders" — the "large reorders" caveat doesn't apply at n≤10.

**Recommendation: keep move-up/down buttons.** For ≤10 items the UX gap vs drag is small, buttons need zero new deps, work identically on touch/keyboard/screen readers, and match this repo's YAGNI bias (no DnD lib exists anywhere in the codebase today). Reach for dnd-kit only if a future list grows large/reorder-heavy (e.g., >20 items) or touch drag becomes an explicit request. [docs.dndkit.com/concepts/sortable](https://docs.dndkit.com/concepts/sortable)

## 3. "Paste a list" textarea mode

No installed precedent found in repo for this pattern (not in the 4 read files); this is general engineering guidance, not sourced from external docs — treat as design reasoning, not literature-verified.

- **Parsing**: split on `/[\n,;]+/`, `.map(s => s.trim())`, `.filter(Boolean)`, then `.slice(0, 10)`. Order in the "cap 10" step matters — cap *after* filtering blanks so blank lines don't consume slots.
- **Live duplicate detection**: reuse the same `key = name.trim().toLowerCase()` logic already in `scoreSetInputSchema`'s `superRefine` (`grading.ts:27-28`) — do not reimplement; extract it to a shared helper (e.g., `findDuplicateIndexes(names: string[])`) called both by the zod schema and by a textarea-mode live preview, to keep single source of truth (DRY).
- **Syncing per-row ↔ textarea**: treat them as two *views* over the same `components: string[]` state, not two independent state slices. Textarea mode should be a toggle that either (a) derives its textarea value from `components.join("\n")` when switching in, and (b) on switching back to per-row (or on blur/apply), re-parses and calls the existing `setComponents()` — never keep both mutable simultaneously. This avoids divergent-edit bugs; only one is "live" at a time.
- Edge case: warn (not block) if paste exceeds 10 — show a truncation notice rather than silently dropping, since silent data loss on paste is a common UX complaint.

## 4. Radix RadioGroup (`radix-ui` package) for radio-cards

Import: `import { RadioGroup } from "radix-ui"` (confirmed in `package.json`: `"radix-ui": "^1.6.7"` — unified package, matches current usage elsewhere presumably via `HvModal`/`Field` primitives).

- **Markup**: `RadioGroup.Root` (accepts `value`/`onValueChange`, `orientation`, `loop`) wraps `RadioGroup.Item` (renders hidden `<input type=radio>` for form semantics) wraps `RadioGroup.Indicator` (checked-state visual). Custom card content (title + chips) goes *inside* `Item`, alongside or replacing the `Indicator` — Item is a generic container, not restricted to a dot. [radix-ui.com/primitives/docs/components/radio-group](https://www.radix-ui.com/primitives/docs/components/radio-group)
- **Keyboard**: roving tabindex — `Tab` focuses the checked item (or first if none selected); arrow keys move focus *and* select (unlike native radios' HTML-only "arrow selects" behavior, this is a documented WAI-ARIA Radio Group pattern); `Space` checks focused item. `loop` (default `true`) wraps arrow nav at ends.
- **Labeling a card**: give each `Item` an explicit `aria-label` (e.g., set name + component count) if the visible card has multiple text nodes (title + chip list) that wouldn't concatenate into a sensible accessible name automatically; otherwise the accessible name is computed from the Item's text content per standard accname algorithm, which can read oddly with chips mixed into title text.
- **Testing Library query**: `screen.getByRole("radio", { name: /vd: giữa kỳ/i })` — Radix's `Item` exposes native `role="radio"` via the hidden input pairing, so RTL's accessible-name matching works the same as any native radio; use `user.click(radio)` or `user.keyboard("{ArrowDown}")` after focusing the group for interaction tests.

## 5. Responsive table → card list (class → assigned-set, 2-column)

- **CSS-only reflow (`display:block`/`grid` on `table/tr/th/td`)** is the common approach but a real accessibility regression: it strips native `table`/`row`/`cell` roles from the accessibility tree below the breakpoint, silently breaking header/cell association for screen-reader users unless every `<td>` is given a matching `aria-label` (verbose, easy to drift from the `<th>` text as columns change). [smashingmagazine.com — Accessible Responsive Tables (Part 2)](https://www.smashingmagazine.com/2022/12/accessible-front-end-patterns-responsive-tables-part2/), [css-tricks.com/accessible-simple-responsive-tables](https://css-tricks.com/accessible-simple-responsive-tables/)
- **Two markups (render table above breakpoint, card list below via `hidden md:block`/`md:hidden`)** avoids the aria-label duplication problem entirely — each markup keeps its own native semantics (table keeps `<th scope="col">` + optional `<caption>`; the card list is just a `<ul>`/`<dl>` with visible labels). Cost: duplicated JSX for one component, or a shared row-renderer function called from two thin wrapper components — reasonable at 2 columns.
- **Testing implication**: RTL renders both markups into jsdom regardless of CSS breakpoint (jsdom ignores media queries), so tests must assert against one deliberately (e.g., query within a `data-testid="score-set-table"` / `"score-set-card-list"` scope, or the smaller markup should use `<caption class="sr-only">` + `scope="col"` on `<th>` to stay screen-reader-parseable even though only one is visually shown per breakpoint). Prefer testing the table variant for structural assertions (`getAllByRole("columnheader")`) since it carries real semantics; card variant tests focus on content presence (`getByText`).

**Recommendation**: two markups, single shared row-mapping function, no CSS-only reflow — matches existing repo convention of a real `<table>` elsewhere (not verified in the 4 files given, but is safer default given a11y regression risk documented above).

## Unresolved questions

- Whether the codebase has an existing responsive-table precedent elsewhere (out of scope per task's 4-file read limit) — worth checking before deciding two-markup vs CSS reflow, to match convention.
- Whether "paste a list" is an accepted design requirement or a hypothetical — no existing schema/API support for bulk-paste was found in `grading.ts`; the 10-cap and per-name validation already exist server-side-mirrored, so paste mode is purely a client input-affordance, no API change needed.

Status: DONE
Summary: Recommend keeping `watch`/`setValue` (not `useFieldArray`, avoids schema shape change) and move-up/down buttons (not dnd-kit, avoids new dep for ≤10 items); documented Radix RadioGroup card markup/testing and a two-markup (not CSS-reflow) responsive table approach for a11y.
Concerns/Blockers: none — all citations from official docs (react-hook-form.com, radix-ui.com, docs.dndkit.com) or established a11y sources (Smashing Magazine, CSS-Tricks); paste-list section is design reasoning, not externally sourced.
