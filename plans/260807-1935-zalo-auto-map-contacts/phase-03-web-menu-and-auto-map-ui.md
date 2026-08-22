---
phase: 3
title: "Web menu, more-sheet, and auto-map UI"
status: completed
priority: P1
effort: "1.5-2d"
dependencies: [2]
---

# Phase 3: Web menu, more-sheet, and auto-map UI

## Overview

Make /contacts reachable from the app shell ("Phụ huynh" entry; the mobile
bottom bar is restructured with a "Thêm" sheet so it stays at five slots) and
add the "Tự động ghép Zalo" review flow to the contacts page, including a
per-person "Kết bạn" action on not-friend rows. The manual per-contact picker
(`ZaloFriendPicker` on `/contacts/:id`) is untouched and remains the fallback.

<!-- Updated: Validation Session 1 - more-sheet and per-person friend request added -->

## Requirements

- Functional: nav entry "Phụ huynh" → `/contacts`, active-state styled like the
  other entries, present in sidebar (lg+), icon rail (md–lg), and the mobile
  "Thêm" sheet (<md).
- Functional: the bottom bar (<md) shows 4 primary tabs — Tổng quan, Điểm
  danh, Lớp & học sinh, Thu tiền — plus a fifth "Thêm" tab that opens a sheet
  listing the remaining entries (Chốt sổ, Gửi thông báo, Phụ huynh) with the
  same icons, labels, pending-dot, and disabled handling. "Thêm" shows the
  active state when the current route belongs to a sheet entry.
- Functional: on /contacts, a "Tự động ghép Zalo" action (enabled only when the
  Zalo account is linked) looks up all unmapped contacts and opens a review
  dialog with three row groups: matched friend (checkbox, default checked, shows
  avatar + Zalo name), found-but-not-friend (label "Chưa kết bạn", no mapping
  checkbox, per-row "Kết bạn" button), not found (display-only, label "Không
  tìm thấy"). Confirm writes only the checked rows and reports "Đã ghép N/M".
- Functional: "Kết bạn" sends one request via `POST /me/zalo/friends/request`
  for that row's UID; the button shows a pending state and flips to "Đã gửi"
  (disabled) on success. There is no send-all control anywhere.
- Functional: contacts already mapped are excluded from lookup; a contact list
  larger than the 200-phone cap sends only the first 200 unmapped and says so.
- Non-functional: button disabled while the match request is pending
  (the API has no double-submit guard by design); errors surface with the
  page's existing error styling.

## Architecture

- **Nav** (`apps/web/src/layouts/dashboard-layout.tsx`): one new `NavEntry`
  after "Lớp & học sinh" (`useNavEntries`, line ~38). Sidebar and rail keep
  rendering the flat entries array (7 items). Icon: pick from
  `@/components/hv` if a fitting one exists; otherwise any lucide icon
  satisfies the `ComponentType<LucideProps>` contract (`BookUser`/`Contact`
  are natural fits).
- **Bottom bar restructure**: split entries into `primary` (first 4 by a
  small partition in the layout, not a new abstraction) and `overflow`. Render
  primary as `BottomTabItem`s plus a "Thêm" tab that opens the project's
  existing sheet/drawer component listing the overflow entries. Rationale for
  the split: Tổng quan/Điểm danh/Lớp & học sinh/Thu tiền are daily actions;
  Chốt sổ/Gửi thông báo are billing-cycle actions and Phụ huynh is setup-time.
  Acceptance check at 360px.
- **Data flow** (join client-side, per phase 2's boundary decision):
  1. Collect unmapped contacts from the already-fetched contacts list (page
     through the existing paginated list API if needed, cap 200).
  2. `POST /me/zalo/friends/match` with their phones.
  3. Join rows back to contacts by the echoed phone string.
  4. On confirm, run the existing zalo-mapping mutation per accepted row
     (sequential, small N; the mutation and its cache invalidation already
     exist for the picker).
- **Code placement** (existing pattern: `zalo-friend-picker.tsx` in roster
  imports zalo hooks from `features/profile`):
  - API fns + zod schemas for the match and friend-request responses →
    `features/profile/api/zalo-api.ts` + `schemas/zalo-schemas.ts`, exported
    like `useZaloFriends`.
  - `useMatchZaloFriends` + `useSendZaloFriendRequest` mutations →
    `features/profile/hooks/use-zalo.ts`.
  - Review dialog component → `features/roster/components/zalo-auto-map-dialog.tsx`,
    wired from `contacts-page.tsx`.

## Related Code Files

- Modify: `apps/web/src/layouts/dashboard-layout.tsx` — nav entry + bottom-bar
  primary/overflow split + "Thêm" sheet.
- Modify: `apps/web/src/features/profile/api/zalo-api.ts`,
  `schemas/zalo-schemas.ts`, `hooks/use-zalo.ts`, `index.ts` — match API.
- Create: `apps/web/src/features/roster/components/zalo-auto-map-dialog.tsx`.
- Modify: `apps/web/src/features/roster/pages/contacts-page.tsx` — toolbar
  action + dialog wiring.
- Create/Modify tests: `features/roster/__tests__/zalo-auto-map.test.tsx`;
  extend the layout/nav test if one exists, otherwise add a focused render test.

## Implementation Steps (TDD)

1. Red: nav tests — entries include "Phụ huynh" linking to `/contacts`; bottom
   bar renders 4 primary tabs + "Thêm"; the sheet lists the overflow entries
   and "Thêm" is active on an overflow route.
2. Red: dialog tests with mocked match API — grouping into
   matched/not-friend/not-found; default-checked matched rows; confirm calls
   the mapping mutation only for checked rows; pending state disables the
   trigger; cap message when >200 unmapped.
3. Red: "Kết bạn" tests — one click sends one request for that row's UID;
   button flips to disabled "Đã gửi" on success; failure surfaces an error and
   re-enables.
4. Red: schema tests for the match and friend-request responses (zod parse of
   representative payloads, unknown fields tolerated).
5. Green: implement API fns, hooks, dialog, page wiring, nav entry, bottom-bar
   split + sheet.
6. Verify: vitest, eslint, tsc, build; manual pass on the three breakpoints
   (sidebar/rail/bottom-tab+sheet) against the deployed API.

## Success Criteria

- [x] "Phụ huynh" reaches /contacts from sidebar, rail, and the "Thêm" sheet;
      bottom bar holds 5 slots and is usable at 360px.
- [x] Auto-map flow: suggest → review → confirm writes only accepted rows via
      the existing mapping endpoint; summary "Đã ghép N/M" correct.
- [x] Not-friend and not-found rows are clearly labeled and never written to
      mappings; "Kết bạn" sends one request per explicit click and has no
      bulk counterpart.
- [x] Manual picker on `/contacts/:id` unchanged and green.
- [x] vitest + eslint + tsc + build all green.

## Risk Assessment

- **Bottom-bar restructure blast radius**: the "Thêm" sheet changes navigation
  for existing screens (Chốt sổ, Gửi thông báo move behind one extra tap).
  Mitigated by keeping sidebar/rail flat and by e2e/nav tests; if the
  daily-vs-cycle partition proves wrong in use, the split is one array
  boundary to move.
- **Sequential confirm writes**: N mutations for N accepted rows; fine at
  roster scale (tens). If a write fails mid-run, the summary reports the split
  and the remaining rows stay unmapped — retrying is idempotent.
- **Stale friend list**: `useZaloFriends` caches with `staleTime`; the match
  endpoint fetches its own friend list server-side, so suggestions do not
  depend on the client cache being fresh.
