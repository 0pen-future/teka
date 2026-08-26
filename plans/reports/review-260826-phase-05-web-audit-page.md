# Review — Web Audit Page (owner-only `/audit`)

Date: 2026-08-26
Reviewer: code-reviewer
Scope: `apps/web` audit feature + touchpoints (router, dashboard layout, format utils)

## Scope

Files reviewed (10 created, 5 modified, ~530 LOC new):

- `apps/web/src/features/audit/{schemas,api,hooks,components,pages}/*`, `routes.tsx`, `index.ts`
- `apps/web/src/features/audit/__tests__/{audit-handlers.ts,audit-page.test.tsx}`
- `apps/web/src/app/router.tsx`, `apps/web/src/layouts/dashboard-layout.tsx` (+ its test)
- `apps/web/src/lib/utils/format.ts`, `apps/web/src/lib/utils/index.ts`

Cross-checked against: `apps/api/internal/features/audit/{handler.go,dto.go,repository.go}`,
`apps/api/internal/features/centers/dto.go`, `apps/web/src/features/collections/*`,
`apps/web/src/features/teaching/{hooks/use-center-context.ts,pages/lesson-plans-page.tsx}`,
`apps/web/src/test/{utils.tsx,msw/handlers.ts}`, `apps/web/src/lib/api/{envelope.ts,interceptors.ts}`,
`apps/web/src/app/providers.tsx`.

Verification run locally: `npm run typecheck` (clean), `npx eslint` on the touched
files (clean), `TZ=America/New_York npx vitest run src/features/audit src/lib/utils
src/layouts` (3 files, 40 tests, all pass — i.e. no assertion is hardwired to the
developer's timezone).

## Overall Assessment

Solid, convention-faithful work. Feature structure, envelope parsing, query-key
factory, route lazy-loading, MSW strict-mode tests and HV primitives all match the
collections template. Owner gating is genuinely enforced at three layers (nav
render, page redirect, `enabled`-gated query) and the "member fires zero requests"
claim is proven by an assertion, not by inspection. No trust-boundary defect, no
breaking change, no scope drift.

Two findings are worth fixing before landing: a transient fetch error blanks
already-loaded rows, and the new shared `formatDateTime` helper has no real test
(its only assertion calls the function under test to build the expected value).
Everything else is polish.

## Critical Issues

None.

## High Priority

### H1 — A failed page-2 fetch (or background refetch) blanks the whole table

`apps/web/src/features/audit/pages/audit-page.tsx:39-53` orders the branches
`isPending → isError → empty → table`. In TanStack Query v5 a failed
`fetchNextPage()` or a failed background refetch flips `status` to `"error"` while
`data` stays populated. Result: the owner clicks "Tải thêm", the network blips, and
every row they had already loaded is replaced by "Không tải được nhật ký hoạt
động." Same on a window-focus refetch failure (`staleTime: 30_000` in
`src/app/providers.tsx` guarantees refocus refetches are common on a page people
leave open).

Fix — keep rendering what you have and surface the failure alongside it:

```tsx
{logs.length > 0 ? (
  <>
    {query.isError ? (
      <p className="text-[13px] text-coral-600">Không tải thêm được. Thử lại sau.</p>
    ) : null}
    <AuditTable logs={logs} />
  </>
) : query.isPending ? (
  …
) : query.isError ? (
  …
) : (
  <EmptyState … />
)}
```

### H2 — `formatDateTime` ships untested; its only assertion is tautological

`apps/web/src/lib/utils/format.ts:62` adds a helper to the shared `@/lib/utils`
barrel. Every other helper in that file has direct cases in
`apps/web/src/lib/utils/__tests__/format.test.ts` (`formatMoney`,
`formatSessionDate`, `formatDayMonth`, `formatPhoneLocal`, `nameInitial`);
`formatDateTime` has none — grep confirms zero references in that suite.

The one assertion that touches it,
`apps/web/src/features/audit/__tests__/audit-page.test.tsx:52`:

```tsx
expect(screen.getByText(formatDateTime("2026-08-26T10:30:00Z"))).toBeInTheDocument();
```

computes the expected string by calling the function under test. It proves the cell
went through the helper; it would pass unchanged if the helper returned the raw ISO
string, the wrong day, or a UTC reading. That is a phantom assertion for format
correctness.

Fix: add real cases to `format.test.ts` — a known instant → literal
`"dd/MM/yyyy HH:mm"`, a midnight/day-boundary case that would catch a UTC-vs-local
mix-up, and the malformed-input passthrough (which is a documented contract and is
currently unexercised). Because the helper reads local wall-clock, pin the suite's
timezone rather than asserting machine-dependently — adding `TZ: "Asia/Ho_Chi_Minh"`
to the `test.env` block in `apps/web/vitest.config.ts` makes both this helper and
the existing from/to filter assertions deterministic across dev machines and CI.

## Medium Priority

### M1 — Action group select silently desyncs from an active free-text filter

`apps/web/src/features/audit/components/audit-filters.tsx:55-57,84-103`. After
submitting free text that is not one of `ACTION_GROUPS` (e.g. `class.create`),
`groupValue` falls back to `"all"`, so the trigger reads "Tất cả hành động" while a
narrow filter is in force. An owner reading an unexpectedly short list has no
on-screen signal for why. Either render a non-selectable "Tùy chỉnh" state when
`filters.action` is set but unmatched, or drive the trigger's display from
`filters.action` directly.

### M2 — Every row's expand toggle has the identical accessible name

`apps/web/src/features/audit/components/audit-table.tsx:94-103`. `aria-expanded` is
correct, but 50 buttons all named "Chi tiết" with no `aria-controls` and no
programmatic link to the detail row make the table hard to operate non-visually.
Give the button a row-scoped `aria-label` (action + timestamp), add
`aria-controls={detailId}` and put that `id` on the detail `TableRow`. Related nit:
the trailing `<TableHead className="w-px" />` (line 52) is an empty header cell —
give it `sr-only` text.

### M3 — `entity_id` (and `actor_role`) are fetched, parsed, and never rendered

`audit-table.tsx:87` shows only `entity_type`. For an audit trail the entity id is
the actionable identifier — "student" alone rarely answers "which one". The
expanded panel already shows method/path/user-agent and is the natural home for
`entity_type · entity_id`. `actor_role` is likewise dead weight on the wire model
today; either surface it (member vs owner is meaningful in a trail) or accept it as
schema-completeness and say so.

### M4 — Date inputs hold write-only local state

`audit-filters.tsx:51-52`. `fromDate` / `toDate` are never derived from the
`filters` prop, unlike `actor_id` and `action`. Harmless today because nothing
resets `filters` from outside, but the component's contract is now half-controlled
— the first "Xóa bộ lọc" button anyone adds will leave stale dates in the inputs.
Either derive the input values from `filters.from/to` or document the asymmetry.

## Low Priority

- **L1** — `to` is sent as local `23:59:59.999`; `occurred_at` is a microsecond-precision
  `timestamptz`, so an event in the final sub-millisecond of the chosen day is excluded.
  Practically unreachable; noting it only so nobody rediscovers it as a "bug".
- **L2** — Refetch amplification: with `staleTime: 30_000` and default
  `refetchOnWindowFocus`, a refocus re-fetches *every* loaded page sequentially (K
  requests after K "Tải thêm" clicks). Correctness is safe — the API's keyset cursors
  are recomputed from freshly fetched pages — but consider `maxPages` or a longer
  `staleTime` on this query.
- **L3** — `AUDIT_PAGE_SIZE` is exported from `__tests__/audit-handlers.ts` but used
  only inside that file.
- **L4** — `src/features/audit/routes.tsx` omits the "mounted inside the protected
  dashboard layout" doc comment that `center/routes.tsx` and `collections/routes.tsx`
  both carry.
- **L5** — `src/features/audit/index.ts` exports a public surface (`auditKeys`,
  `useAuditLogs`, `auditLogSchema`) that no other feature imports. Consistent with the
  per-feature barrel convention, so fine; just be aware it is currently dead surface.
- **L6** — An owner whose `GET /centers/me` fails is redirected to `/` with no
  explanation (`audit-page.tsx:24-29`). This is deliberate parity with
  `lesson-plans-page.tsx:137-143`, so it is consistent, not a defect.

## Edge Cases Scouted (verified, no action needed)

- **Free text cannot produce a 400.** `parseListQuery`
  (`apps/api/internal/features/audit/handler.go`) validates only `actor_id` (UUID),
  `from`/`to` (RFC3339), `limit` (int) and `cursor`; `action` is taken verbatim and
  reaches SQL as a bind parameter with `LIKE ? ESCAPE '\'` and `likeEscaper`
  (`repository.go:70-72`). Junk text yields a 200 with zero items, which the page
  correctly renders as "Không có bản ghi phù hợp với bộ lọc." — not "chưa có gì".
  Every 400 vector the endpoint documents is unreachable from this UI: `actor_id`
  comes from a select of server-issued UUIDs, `from`/`to` from `toISOString()`,
  `cursor` only ever echoes a server token.
- **Fractional seconds.** `toISOString()` emits `.999Z`; Go's `time.Parse` accepts a
  fractional second after the seconds field even when the layout omits it, so
  `time.RFC3339` parses the FE's values.
- **Cursor opacity.** The MSW fake uses `offset-N`, but the only cursor assertion
  (`audit-page.test.tsx:73`) checks that the client echoed the server's token
  verbatim — a promise the real API does make. No test depends on cursor semantics
  the API does not guarantee. The `getNextPageParam` contract (`""` → `undefined`)
  matches `dto.go`'s `NextCursor`.
- **Filter change resets pagination.** Filters live in the query key
  (`use-audit-logs.ts:9,20`), and `undefined`-valued keys hash identically to absent
  ones in TanStack's `hashKey`, so clearing a filter back to "all" does not create a
  phantom second cache entry. Asserted at `audit-page.test.tsx:89`.
- **Actor identity crosses the boundary correctly.** `centers.MemberResponse.ID` is
  the teacher id, and `audit_logs.actor_user_id` joins `teachers.id`
  (`repository.go:109`), so the member select's values are the right filter domain.
- **Radix hidden native options.** The first test scopes to `within(table)` for
  actor names that also appear inside the select; the remaining assertions target
  strings (`class.create`, `auth.login`) that exist only in the table — group
  select entries render Vietnamese labels, not action values.
- **Member gating.** `useAuditLogs(filters, isResolved && isOwner)` plus the
  `expect(auditRequests).toHaveLength(0)` assertion proves no request is fired
  before the redirect. Nav entry is gated on `isResolved && isOwner`, so no flash.
- **Timezone.** The suite passes under `TZ=America/New_York`; the from/to assertions
  would fail there if the implementation had used UTC midnight, so that conversion
  is genuinely constrained (unlike `formatDateTime` — see H2).
- **XSS / data exposure.** `user_agent`, `ip`, `path` and `metadata` render as React
  text nodes / `JSON.stringify` inside `<pre>`; no `dangerouslySetInnerHTML`. The
  page is owner-only and the endpoint is center-scoped server-side, so the IP/UA
  exposure is intentional and bounded.

## Regression / Contract Check

- **Bottom-bar 4-tab invariant holds.** The new label is in `OVERFLOW_LABELS` and
  `/audit` in `OVERFLOW_PATH_PREFIXES`, so the mobile bar keeps its four direct tabs
  plus Thêm; only the sidebar/rail gain an entry. Layout tests updated accordingly
  and pass.
- **Router tree.** `auditRoutes` is spread into the `ProtectedRoute + DashboardLayout`
  children alongside `centerRoutes`; lazy-import shape matches the other feature
  route files.
- **Public contracts.** Changes are additive only: one new named export on
  `@/lib/utils`, one new feature barrel, one new route array. No existing signature,
  prop, or envelope parser changed.

## Recommended Actions

1. Fix H1 — do not drop rendered rows on a transient fetch error.
2. Fix H2 — add real `formatDateTime` cases to `format.test.ts` and pin `TZ` in
   `vitest.config.ts`.
3. Address M2 (expand-toggle accessibility) and M3 (`entity_id` in the detail panel).
4. Optional in this phase: M1 (group-select desync), M4 (controlled date inputs), L2.

## Metrics

- `npm run typecheck`: clean. No `any`, no non-null assertion outside the guarded
  `filters.action!` at `audit-filters.tsx:57`, no lint suppressions in the new code.
- `npx eslint` on the touched files: 0 errors, 0 warnings.
- Tests: 40/40 pass across `src/features/audit`, `src/lib/utils`, `src/layouts`
  (also under a non-Vietnam TZ). New behavioral coverage: 7 page tests + 2 nav
  gating tests. Coverage gap: `formatDateTime` (see H2).

## Unresolved Questions

1. Should the audit trail expose `actor_role` and `entity_id` in the UI, or is
   carrying them on the wire model purely for schema completeness (M3)?
2. Is the silent redirect-to-dashboard when `GET /centers/me` fails the intended
   behavior for a genuine owner, or should it show a retry surface? Current code
   deliberately mirrors the lesson-plans page (L6).

---

## Fixes applied after review (260826, session)

- **H1 fixed** — `audit-page.tsx` no longer routes `isError` ahead of the data
  branch: the error paragraph renders inline and the table renders whenever any
  page is loaded, so a failed `fetchNextPage`/background refetch keeps every
  row on screen (the "Tải thêm" button stays for retry). New regression test:
  "keeps loaded rows and shows an inline error when loading more fails".
- **H2 fixed** — `vitest.config.ts` pins `TZ=Asia/Ho_Chi_Minh`;
  `format.test.ts` gains four literal `formatDateTime` cases (UTC+7 rendering,
  local-midnight rollover, zero-padding with a `-05:00` offset input, and
  malformed passthrough). The page test's self-referential assertion now sits
  on top of real literal coverage.
- **M1 fixed** — the group select derives a disabled "Tùy chỉnh" state whenever
  the live action filter is free text, instead of claiming "Tất cả hành động".
- **M2 fixed** — expand toggles carry `aria-label="Chi tiết <action>"`,
  `aria-expanded`, and `aria-controls` pointing at the mounted details row id.
- **M3 fixed** — the expanded panel now shows `Đối tượng: <entity_type>
  (<entity_id>)` and `Vai trò: <actor_role>`; the expand test asserts the
  entity id round-trips.
- **M4 accepted as-is** — the raw `yyyy-mm-dd` strings are the natural source
  of truth for the date inputs; deriving them back from the RFC3339 instants
  would add parsing with no behavior change.
- **Unresolved Q2** — redirect-on-error mirrors the existing lesson-plans
  guard by design; changing that surface belongs to a layout-wide decision,
  not this feature.

Verification after fixes: `npm run lint` 0 errors (4 pre-existing warnings
elsewhere), `npm run typecheck` clean, `npm run test` 387 passed / 3 skipped
across 60 files.
