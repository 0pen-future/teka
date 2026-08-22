---
phase: 2
title: "Web class settings teacher handoff"
status: done
priority: P1
effort: "4h"
dependencies: [1]
---

# Phase 2: Web class settings teacher handoff

## Overview

Owner-only "Giáo viên phụ trách" section on the class settings page: shows the
current teacher, lets the owner pick another center member and hand the class
over via Phase 1's endpoint.

## Requirements

- Functional: current teacher name displayed; member select (from the owner's
  `GET /centers/me` body, which already carries `members`); "Bàn giao lớp"
  action calls `PUT /classes/:id/teacher`; success refreshes class-related
  queries; API errors (403/422) surface as inline message.
- Non-functional: section renders only for owners (same `"members" in center`
  narrowing used by the import page — the API's owner check remains the real
  gate); no new global state, plain React Query mutation.

## Architecture

- API layer: `reassignTeacher({classId, teacherId})` in the roster feature's
  api module (axios PUT, JSON body `{teacher_id}`), zod schema for the response.
- Hook: `useReassignTeacher(classId)` mutation; on success invalidate
  `classesKeys.all` and the sessions/attendance keys the class settings page
  already depends on (mirror `useImportRoster`'s invalidation style).
- UI: new card in `class-settings-page.tsx` — current teacher (resolve
  `class.teacher_id` against `center.members`), `<select>` of members excluding
  the current teacher, confirm button with pending state, inline error text.
  A confirm step (browser `confirm` is banned if repo avoids it — use a
  two-click "Bàn giao" → "Xác nhận bàn giao" pattern consistent with existing
  destructive actions on the page, scout before building).

## Related Code Files

- Modify: `apps/web/src/features/roster/api/` (classes or new handoff api fn)
- Modify: `apps/web/src/features/roster/hooks/use-classes.ts` (mutation hook)
- Modify: `apps/web/src/features/roster/pages/class-settings-page.tsx`
- Modify/Create: tests under `apps/web/src/features/roster/__tests__/`
  (MSW handler for the new endpoint + page test)

## Implementation Steps

1. Scout `class-settings-page.tsx` for existing card/action patterns and the
   destructive-action confirm idiom; reuse it.
2. Add api fn + zod response schema + mutation hook with invalidation.
3. Build the owner-gated card; wire pending/error states.
4. Tests: owner sees section, member does not; successful handoff calls the
   endpoint with the picked member and re-renders the new teacher; 422 shows
   the API message.
5. `tsc -b --noEmit` + focused vitest, then full web suite.

## Success Criteria

- [ ] Owner can hand a class to another member end-to-end against the live API.
- [ ] Member accounts never see the section.
- [ ] Query invalidation refreshes the class list/detail without reload.
- [ ] Web suite green.

## Risk Assessment

- Members list is owner-body-only — the section must not assume `members`
  exists (type-narrow first), or member accounts crash the settings page.
- Handoff by an owner of a class they are also teaching is legal; UI must not
  filter the owner out of the member select.
