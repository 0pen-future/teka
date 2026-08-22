# Code review — Phase 1: Teaching foundation (store, feature scaffold, nav)

**Date:** 2026-08-13 · **Scope:** uncommitted working tree on `master`, `apps/web` only
**Verdict:** Phase-01 acceptance criteria met. Ship-able as a foundation, with two
architectural findings that should be resolved *before* phases 3–6 write real data.

## Scope reviewed

- New: `src/features/teaching/{lib/teaching-store.ts, hooks/use-center-context.ts, routes.tsx, pages/*.tsx, __tests__/*}`
- Modified: `src/app/router.tsx`, `src/layouts/dashboard-layout.tsx`, `src/layouts/__tests__/dashboard-layout.test.tsx`
- ~570 LOC added (≈45% tests). No `apps/api` changes — UI-only constraint respected.

## Verification run (re-run, not taken on trust)

| Check | Result |
|---|---|
| `npx vitest run` | 46 files, **270 passed** |
| `npx vitest run src/features/teaching src/layouts` | 3 files, 26 passed |
| `npm run typecheck` (`tsc -b`) | clean (`noUncheckedIndexedAccess` on) |
| `npm run lint` | **0 errors**, 4 warnings — all pre-existing `react-hooks/incompatible-library` in profile/roster (react-hook-form `watch()`), none in new files |
| `npm run build` | clean, 802ms; teaching pages emit their own lazy chunks |
| Hardcoded colors in new files | none (`grep` for hex/rgb/`bg-[` → empty); ink/mint/coral tokens only |

## Acceptance criteria (phase-01)

- [x] Four routes resolve (`/classbook`, `/records`, `/records/:studentId`, `/lesson-plans`), lazy, mirroring `roster/routes.tsx`.
- [x] Non-owner on `/lesson-plans` redirected to `/classbook` (`replace`), covered by `lesson-plans-gate.test.tsx` with a member-shaped msw override.
- [x] Nav order matches plan/prototype; TRUNG TÂM owner-gated entry with pending dot, rendered only after `/centers/me` resolves.
- [x] Store round-trip, per-center isolation, corrupt-JSON and schema-mismatch fallback — all directly asserted, not smoke-tested.
- [x] Suite/lint/tsc clean.

## Findings

### High — 1. Owner and member of the same center use *different* storage keys, silently breaking the phase-6 review loop

`use-center-context.ts:24-37` keys the store by `data.center.id` (UUID) for owners and by
`data.center_name` for members, because `MemberMeResponse` carries only `center_name`
(verified in `apps/api/internal/features/centers/dto.go:39-41`; no `center_id` is exposed
to the client anywhere — `grep center_id apps/web/src` is empty). The constraint is real
and the trade-off is documented in the hook's comment.

The undocumented consequence is what matters: a teacher writes to
`teka.teaching.Trung Tâm Bình Minh` while the owner reads `teka.teaching.30000000-…`.
Phase 6's success criterion — "Full loop verified: submit (Phase 4) → pending here →
redo note visible to teacher → resubmit → approve" — and phase 4's "feeds the Phase 1
nav dot and Phase 6 queue" therefore only hold when a **single account** plays both
sides. Cross-role on one device (the only device-local flow the plan contemplated as
working) never surfaces a submitted plan; the owner's queue and pending dot stay empty
forever, with no error.

The `snapshots` Map and the `teka.teaching.*` keys also survive logout (`useLogout`
clears the zustand store and the query cache only — `features/auth/hooks/use-auth.ts:28-39`).
Two teachers of the same center sharing a browser resolve to the *same* key, so
teacher A's per-student nhận xét and scores are readable by teacher B after a normal
logout/login. That data is exactly the kind the plan is careful about elsewhere
(decision #2 cites Nghị định 13/2023), and `auth-store.ts:8` already establishes the
"nothing sensitive in localStorage" posture for tokens.

Options (decision #1 said "keyed per center" and is user-approved — flagging the
trade-off, not overriding it):

1. **Namespace by user id** (`useAuthStore` user has `id`, `auth-schemas.ts:70`):
   `teka.teaching.<userId>` — a user has exactly one center (`GET /centers/me` returns
   one), so this loses nothing, fixes the shared-device exposure and the name-collision
   hole below, and makes the "one browser, one account" scope explicit. It does *not*
   fix the cross-role loop (nothing can, without an API change).
2. Keep the center key and clear `teka.teaching.*` + `snapshots` on logout — fixes the
   exposure only.
3. Accept and **document** it: state in phase 4/6 that the review loop is demonstrable
   single-account only, so phase 6 does not get written against an assumption that
   cannot hold.

At minimum, do (3); (1) is one line and strictly better.

### High — 2. Whole-store subscription in `useNavGroups` re-renders the entire shell on every write

`dashboard-layout.tsx:57` calls `useTeachingStore(centerId)` to derive a boolean.
Any store write — and phases 3–5 write per keystroke (điểm, nhận xét, personal notes) —
swaps the snapshot identity and re-renders `DashboardLayout`, all nav groups, every
`NavLink`, and the `Outlet` subtree. React Compiler is *not* enabled (`vite.config.ts`
uses bare `react()`; only the eslint plugin is present), so nothing memoizes this away.

Fix now while it is cheap: expose a count selector whose `getSnapshot` returns a number,
which `useSyncExternalStore` compares with `Object.is` and is therefore stable:

```ts
export function usePendingPlanCount(centerId: string | null): number {
  return useSyncExternalStore(subscribeTeaching, () =>
    centerId ? countPendingPlans(getTeachingSnapshot(centerId)) : 0,
  );
}
```

### Medium — 3. `/lesson-plans` renders a permanent blank page when `/centers/me` fails

`lesson-plans-page.tsx:12-16` returns `null` while `!isResolved`, and
`use-center-context.ts:20-23` derives `isResolved` purely from `data` being present.
On a 5xx/network failure the app retries once (`app/providers.tsx:13-19`) and then
leaves `data` undefined forever — the page is a silent blank with no error state and no
retry affordance. Surface `isError`/`isPending` from `useCenter` and render an error
state (the nav's degradation — the entry just stays hidden — is fine).

### Medium — 4. No cross-tab reconciliation; last write wins with a stale base

The store never listens for the `storage` event and `updateTeachingState` persists the
*whole* state built from the in-memory snapshot. Two open tabs (realistic: classbook +
hồ sơ học sinh) silently overwrite each other's writes wholesale. Cheap mitigation: on a
`storage` event for a `teka.teaching.` key, drop that center's snapshot and notify
listeners. Otherwise document it as a known limit before phase 3 lands real writes.

### Medium — 5. Non-scrolling sidebar/rail now carries 11 entries

`dashboard-layout.tsx:458` (sidebar) and `:486` (rail) have no `overflow-y-auto`; this
phase adds 2–3 entries to each. Rough stack at lg: logo ~90 + center card ~60 + 11
entries × ~44 + 3 headers + period card ~100 + footer ~100 ≈ 900px, so at 1366×768 the
nav no longer fits without page scroll; the md rail (~830px of discs) clips
`ProfileDisc`/`CurrentPeriodDisc` below ~830px viewport height. Not caught by jsdom
tests. Verify at 768px height before phase 7 and add `overflow-y-auto` if it regresses.

### Low

- **6.** No `features/teaching/index.ts`. Every other feature except `dashboard` has one,
  and `center/index.ts` explicitly says "Other features import ONLY from here", yet
  `dashboard-layout.tsx:27-28` deep-imports two teaching modules. Precedent exists
  (`dashboard/hooks/use-dashboard`), so this is a consistency nit — but the layout now
  reaches into a feature's `lib/` and `hooks/`, which is the weakest form of that pattern.
- **7.** `use-center-context.ts` duplicates the `"members" in data` narrowing and the
  name/role derivation that `CenterCard` (`dashboard-layout.tsx:297-300`) already does.
  Two copies of a load-bearing narrowing over an undiscriminated union will drift; the
  hook arguably belongs in `features/center` (where the schema comment documents the
  narrowing) and `CenterCard` should consume it.
- **8.** `resetTeachingStoreForTests` is exported from a production module. It is
  tree-shakeable and used by two suites, so acceptable — but `src/test/setup.ts:20-26`
  clears `localStorage` globally while *not* clearing the module-scope `snapshots` Map.
  Any future test that seeds `localStorage` directly and then renders will read a stale
  cached snapshot. Adding `resetTeachingStoreForTests()` to the global `beforeEach`
  closes that footgun (and makes the per-file `localStorage.clear()` in
  `dashboard-layout.test.tsx:38` redundant, as it already is today).
- **9.** Pending dot has no accessible name (`PendingDot` is a bare `span`), and the new
  test asserts it via `link.querySelector(".bg-coral-400")` — a CSS-class assertion
  coupled to styling. Both mirror the existing Điểm danh dot, so this is parity, not a
  regression; worth fixing centrally in phase 7.
- **10.** `OWNER_CENTER_ID` in `dashboard-layout.test.tsx:20` duplicates the msw
  handler's UUID (`test/msw/handlers.ts:442`). It fails loudly if the fixture changes,
  so acceptable, but exporting the id from the handlers module would be tighter.
- **11.** Untracked `test_cases.md` at repo root is unrelated to this phase — clean up or
  ignore before committing so it does not ride along.

## Verified OK (checked, no action)

- **No route shadowing.** `teachingRoutes` mounts before `rosterRoutes` in the protected
  children (`router.tsx:41-49`); paths `classbook`, `records`, `records/:studentId`,
  `lesson-plans` collide with nothing existing, and the `*` NotFound stays last.
- **Bottom-bar contract preserved.** With the seven overflow labels, `primaryTabs`
  resolves to exactly Tổng quan / Điểm danh / Lớp & học sinh / Thu tiền + Thêm = 5 slots,
  and it stays 5 for both roles because `Duyệt giáo án` is in `OVERFLOW_LABELS` — the tab
  count does not shift when the role resolves. `OVERFLOW_PATH_PREFIXES` gained
  `/classbook`, `/records`, `/lesson-plans`, so the Thêm tab lights up on those routes
  (including `/records/:studentId` via the `startsWith` prefix branch).
- **`useSyncExternalStore` usage is correct.** `getSnapshot` returns the cached Map value
  (identity stable between writes — asserted in the store test) and a module constant
  `NO_CENTER_STATE` for a null center; no infinite-render risk. Absent
  `getServerSnapshot` is fine for a CSR-only SPA. Unsubscribe is asserted.
- **localStorage failures are handled.** Read path catches both `JSON.parse` throw and a
  throwing/unavailable `localStorage`; write path tolerates quota/private-mode failure and
  still updates memory + notifies. Both fall back to empty state rather than throwing.
- **Owner-gate flicker.** Nav entry and page both render nothing until `/centers/me`
  resolves, matching CenterCard's height-stable approach; the member test waits on the
  "Giáo viên" role label to prove the query resolved member-shaped before asserting absence.
- **Existing layout tests were updated, not weakened.** The grouped-sidebar test still
  asserts exact per-group label lists (now longer) and its `findBy` anchor moved to the
  slowest-resolving entry rather than being relaxed; the new describe adds order, href,
  role-gating, and dot-present/absent cases.
- **No public-contract change.** No API, schema, or exported-symbol changes outside the
  new feature; zod v4 `z.record(key, value)` two-arg form used correctly.
- **Store shape matches the phase spec** (curricula, lessonPlans, sessionNotes,
  sessionScores, personalNotes, versioned envelope, `SESSION_COST_VND = 300_000`).
- **Same-name center collision** (member A's key equals another account's member key for a
  different center with the same name) is a real but low-likelihood hole in the "no data
  leaks across centers" criterion; it is subsumed by finding 1 option (1).

## Recommended actions

1. Decide finding 1 (user decision — options above); at minimum document the single-account
   limit in phases 4 and 6 so they are not built on an assumption that cannot hold.
2. Add the count-selector hook (finding 2) before phase 3 introduces per-keystroke writes.
3. Give `/lesson-plans` an error state for a failed `/centers/me` (finding 3).
4. Decide cross-tab handling (finding 4): `storage` listener or an explicit documented limit.
5. Check sidebar/rail at 768px height (finding 5); add `overflow-y-auto` if it clips.
6. Optional tidy: teaching `index.ts`, move `useCenterContext` to `features/center` and let
   `CenterCard` consume it, `resetTeachingStoreForTests()` in the global test setup.

## Unresolved questions

- Is the plan-review demo intended to work across two accounts in one browser? If yes,
  finding 1 is a blocker for phase 6, not a documentation item.
- Should teaching data be cleared on logout? Decision #1 accepted device-local storage but
  did not address session lifetime on shared devices.
