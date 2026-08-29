---
phase: 4
title: "Web: secretary send experience"
status: done
priority: P1
effort: "1.5d"
dependencies: [2, 3]
---

# Phase 4: Web: secretary send experience

## Overview

Give a capability holder a "Gửi báo cáo" surface: center-wide period list,
reuse of the existing notifications send page, a friend-status pre-send
warning against HER Zalo friend list, and Zalo-link prompting. Hide every
send entry point from plain members (D8 — teachers only input attendance +
nhận xét; those teaching screens are untouched).

## Requirements

- Functional: nav entry "Gửi báo cáo" appears for `can_send_reports` holders
  (non-owner); it opens a period list across all center teachers; each period
  opens the existing send page (`/notifications/:periodId`) which now works for
  capability holders; pre-send dialog additionally warns "N phụ huynh đã ghép
  Zalo nhưng chưa là bạn bè của bạn" with a link to the friend-request flow;
  if her Zalo is unlinked, the page shows the existing link-Zalo prompt
  pointing to `/profile`.
- Functional (D8): plain members no longer see send entry points — the
  "Nhắc nợ" button (`contact-collection-row.tsx:83`), the notifications link
  on `billing-review-page.tsx:143`, and the send CTA on the send page itself;
  their period ledger (đã gửi gì, bởi ai) stays visible read-only.
- Non-functional: owner UX unchanged; plain-member UX unchanged EXCEPT the
  removed send affordances; read-only posture — no edit affordances (chốt sổ,
  payment recording, roster edits) appear for the secretary;
  reduced-motion/keyboard behavior per design system.

## Architecture

- **Gating:** `use-center-context.ts:24-37` gains `canSendReports` derived
  from the member shape's new flag, plus derived `canRunSends = isOwner ||
  canSendReports` mirroring the server's `ReportsOversight()` (one derived
  boolean, not scattered `||`s). The secretary nav shows for
  `!isOwner && canSendReports`; send affordances everywhere gate on
  `canRunSends`. Nav wiring in `dashboard-layout.tsx:86-104` next to the
  owner-only entries.
- **Hide plain-member send entry points (D8):** gate the "Nhắc nợ" button in
  `contact-collection-row.tsx` and the notifications `<Link>` in
  `billing-review-page.tsx` on `canRunSends`. On `notifications-page.tsx`,
  hide the send/confirm controls for `!canRunSends` while keeping the ledger
  visible (a teacher checks what the secretary sent for their period); a
  direct URL visit therefore degrades to read-only, and the server 403 is the
  real enforcement. UX-only gating — no client-side security claims.
- **Period list page:** new page `features/reports/pages/send-reports-page.tsx`
  (new small feature folder per `docs/frontend-guidelines.md` layout) listing
  billing periods center-wide (Phase 2 relaxed the list endpoint and added
  `teacher_id`/`teacher_name` to `PeriodResponse` — group by those fields),
  showing period status + statement availability; row → existing
  `/notifications/:periodId`. Reuse HvCard/status-pill/header band patterns
  (h1 font-display 26px pattern as on `center-page.tsx:56-58`).
- **Send page access:** `notifications-page.tsx:23-384` currently has no role
  check; it gains only the `canRunSends` affordance-gating above — no route
  guard, no redirect (server authorizes).
- **Pre-send buckets come from the server:** replace the client-side
  contacts×collections intersection (`notifications-page.tsx:92-106` — its own
  comment documents the 100-per-page undercount, which becomes guaranteed
  wrong center-wide) with Phase 2's
  `GET /billing-periods/:id/notifications/preview`. The confirm dialog renders
  the three server-computed buckets — auto-send (mapped+friend),
  mapped-but-not-friend (warn, still sendable, may land in stranger inbox /
  fail), unmapped (manual fallback) — and blocks confirm with a clear message
  when eligible count exceeds the returned `max_run_size`. CTA "Kết bạn trước"
  linking to the friend-request flow (POST `/me/zalo/friends/request` UI
  surface from the auto-map feature).
- **Unlinked Zalo:** reuse the `personalReady` pattern
  (`notifications-page.tsx:80`) — banner with link to `/profile`
  ZaloConnectCard.

## Related Code Files

- Create: `apps/web/src/features/reports/{api,pages,routes.tsx,index.ts}`
- Modify: `apps/web/src/features/teaching/hooks/use-center-context.ts`
- Modify: `apps/web/src/layouts/dashboard-layout.tsx`
- Modify: `apps/web/src/features/collections/pages/notifications-page.tsx`
  (pre-send dialog buckets, oversight-safe queries, unlinked banner,
  `canRunSends` affordance gating)
- Modify: `apps/web/src/features/collections/components/contact-collection-row.tsx`
  ("Nhắc nợ" gated on `canRunSends`)
- Modify: `apps/web/src/features/billing/pages/billing-review-page.tsx`
  (notifications link gated on `canRunSends`)
- Modify: `apps/web/src/app/router.tsx` (mount reports routes)
- Create/modify: vitest + MSW tests for the new page and dialog buckets

## Implementation Steps

1. Extend `use-center-context` + nav entry.
2. Build reports feature folder: API fns (periods center-wide), page, route.
3. Swap the dialog's count source to the preview endpoint (buckets +
   max_run_size guard); unlinked banner.
4. Gate the three plain-member send entry points on `canRunSends`
   (contact-collection-row, billing-review-page link, send-page controls);
   ledger stays rendered.
5. Unit tests: nav gating (owner: no entry; member: no entry; secretary:
   entry), period list grouping by teacher, dialog bucket rendering from MSW
   preview payloads incl. the max_run_size block state, unlinked state, and
   plain member sees no send affordances but still sees the ledger.

## Todo

- [x] `canSendReports` in center context + nav entry
- [x] Reports period-list page + routing
- [x] Pre-send dialog: server-computed 3-bucket warning + max_run_size guard +
      friend-request CTA
- [x] Unlinked-Zalo banner on send page for capability holders
- [x] Plain-member send entry points hidden (3 surfaces); ledger read-only
- [x] Vitest/MSW coverage per step 5

## Success Criteria

- [x] Secretary journey works on dev stack: login → nav → pick period of
      another teacher → send (manual + personal) with correct warnings
- [x] Owner UI snapshots unchanged; plain-member diffs limited to removed
      send affordances (D8)

## Risk Assessment

- Bucket math is server-owned (Phase 2 preview endpoint), so the friend-list
  contract risk moved to the API; the web only renders returned counts —
  numbers can never silently diverge from what BulkSend will actually do.
- Notifications page reuse risks hidden own-scope assumptions; mitigated by
  e2e in Phase 5 covering a cross-teacher period.
