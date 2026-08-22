---
phase: 1
title: "Teaching foundation — store, feature scaffold, nav"
status: completed
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: Teaching foundation — store, feature scaffold, nav

## Overview

Create the `teaching` feature scaffold, the client-side teaching store (scores, giáo án, chương trình, nhận xét — per-center localStorage persistence), and wire the prototype's nav additions so later phases only build pages.

## Requirements

- Functional: new lazy feature `apps/web/src/features/teaching/` mounted in the dashboard route tree with three route stubs: `/classbook` (Quản lý lớp học), `/records` + `/records/:studentId` (Hồ sơ học sinh), `/lesson-plans` (Duyệt giáo án, owner-only).
- Functional: teaching store holds — per class: curriculum (`lessons: string[]`, `currentIndex`); per class+lesson: lesson plan `{ goal, activities[], homework, fileName?, status: "none"|"draft"|"pending"|"approved"|"redo", redoNote?, ownerComment?, submittedBy? }`; per session: general note `{ text }` and scores `Record<studentId, number>`; per student+session: personal note. Named constant `SESSION_COST_VND = 300_000`.
- Functional: nav (`useNavGroups` in `apps/web/src/layouts/dashboard-layout.tsx`) becomes — DẠY HỌC: Điểm danh, Quản lý lớp học → `/classbook`, Hồ sơ học sinh → `/records`, Lớp & học sinh → `/students` (prototype order, lines 1401–1406); TRUNG TÂM: Duyệt giáo án → `/lesson-plans` (owner only, pending-count dot when any plan status is `pending`), Cài đặt trung tâm.
- Non-functional: store state is validated with zod on read (corrupt/legacy localStorage falls back to empty state, never throws); persistence key namespaced per center (`teka.teaching.<centerId>`); no data leaks across centers; store readable via a `useSyncExternalStore`-based hook so all pages re-render on writes.

## Architecture

- **Pattern:** module-scope store singleton + `useSyncExternalStore` (same trade-off React Query makes: external cache, React subscribes). Avoids a context provider (nothing configurable per subtree) and avoids putting localStorage state into React Query (it is not server state; caching semantics would fight persistence).
- Store module `lib/teaching-store.ts`: `getSnapshot(centerId)`, `update(centerId, recipe)`, `subscribe(listener)`. Writes are immutable replacements (snapshot identity drives `useSyncExternalStore`). Persistence = `JSON.stringify` on write, zod `safeParse` on load. A thin `useTeachingStore(centerId)` hook wraps selection; nav consumers use the `usePendingPlanCount(centerId)` number-snapshot selector so store writes don't re-render the shell.
- **Storage key = center NAME, both roles** (implemented): the member shape of `/centers/me` exposes no center id, so the name is the only role-independent value — required so the phase-6 review loop (teacher submits → owner approves) resolves one store on the same device. Name collisions across centers on one device accepted (device-local, non-authoritative).
- Center id comes from the existing `GET /centers/me` query (already used by CenterCard); owner detection reuses the established `"members" in data` narrowing — expose both from one small `useCenterContext()` helper in the teaching feature to avoid re-deriving in every page.
- Route-level owner gate for `/lesson-plans`: the lazy page renders a redirect to `/classbook` for non-owners (nav hiding alone is not a guard).
- Nav pending dot: reuse the existing pending-dot mechanism used by Điểm danh; count = plans with `status === "pending"` from the store (owner only).

## Related Code Files

- Create: `apps/web/src/features/teaching/routes.tsx`
- Create: `apps/web/src/features/teaching/lib/teaching-store.ts`
- Create: `apps/web/src/features/teaching/lib/teaching-store.test.ts` (or `__tests__/` per feature convention)
- Create: `apps/web/src/features/teaching/hooks/use-center-context.ts`
- Create: `apps/web/src/features/teaching/pages/classbook-page.tsx`, `records-page.tsx`, `student-record-page.tsx`, `lesson-plans-page.tsx` (minimal shells: h1 + subtitle per prototype)
- Modify: `apps/web/src/app/router.tsx` (mount `teachingRoutes`)
- Modify: `apps/web/src/layouts/dashboard-layout.tsx` (`useNavGroups` entries + owner gating + dot)
- Modify: layout tests + msw defaults as needed (suite runs `onUnhandledRequest: "error"`)

## Implementation Steps

1. Define zod schemas + TS types for the store state; implement load/save/subscribe/update with per-center namespacing and corrupt-data fallback.
2. Add `useTeachingStore` + `useCenterContext` hooks.
3. Scaffold feature folder, page shells, `routes.tsx` following the roster feature's lazy-route pattern; register in `router.tsx`.
4. Update `useNavGroups`: reorder DẠY HỌC, add the two new entries, add owner-gated Duyệt giáo án with pending dot; keep Phụ huynh placement decision from commit 4ad5518 (stays under DẠY HỌC).
5. Owner redirect inside `lesson-plans-page.tsx`.
6. Unit tests: store persistence round-trip, corrupt JSON fallback, center isolation, pending-count selector; layout tests for new nav order + owner gating (msw `GET /centers/me` owner vs member shapes).

## Success Criteria

- [x] All four routes resolve; non-owner hitting `/lesson-plans` is redirected.
- [x] Nav order and gating match the prototype; pending dot appears when a plan is pending.
- [x] Store round-trips through localStorage, isolates centers, survives corrupt data.
- [x] Suite green, eslint 0 errors, tsc clean.

## Risk Assessment

- **localStorage schema drift in later phases** → version field in the persisted envelope; zod fallback discards unreadable state (acceptable: local-only, non-authoritative data — documented in decision #1).
- **Nav test churn** (existing layout tests assert current entries) → update assertions in the same phase, never weaken to smoke tests.
- **Owner flag flicker on load** could flash the gated entry → gate renders only after `GET /centers/me` resolves, mirroring CenterCard's height-stable loading approach.
