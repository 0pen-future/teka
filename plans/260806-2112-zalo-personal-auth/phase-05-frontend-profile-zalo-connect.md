---
phase: 5
title: "Frontend profile Zalo connect"
status: completed
priority: P1
effort: "1.5d"
dependencies: [4]
---

# Phase 5: Frontend profile Zalo connect

## Overview

Replace the stubbed `Kết nối Zalo` card in `profile-page.tsx` (currently a button
that only fires a toast) with the real consent → QR → linked flow. Adds a
`use-zalo` hook layer over the Phase 4 endpoints, a consent+QR `HvModal`, and the
three card states. Sending/mapping UI stays out — this is only "link my account."

## Requirements

- Functional:
  - Card reflects real state from `GET /me/zalo`: **not linked** / **linked as
    `<tên>`** (with `Ngắt kết nối`) / **expired** (`Quét lại mã`).
  - `Đăng nhập với Zalo` opens an `HvModal`, step 1 = consent (checkbox gating
    `Tiếp tục`), step 2 = QR image + ~100s countdown, polling `link/status`.
    `Tiếp tục` sends the acknowledged `consent_version` (a versioned constant
    beside the consent copy) to `link/start`.
  - On `scanned`/`confirmed`, the QR is replaced by a `Đã quét · chờ xác nhận
    trên điện thoại` state (spinner + instruction) so the teacher knows to
    approve on their phone — distinct from `qr_ready`.
  - On `linked`, modal closes, toast `Đã kết nối Zalo`, card flips to linked,
    query invalidates.
  - On `expired`/`error`, show `Tạo mã mới` (calls `link/start` again).
  - **Mobile:** under `md`, offer `Lưu ảnh QR` (download the data-URI PNG) with
    the instruction to open Zalo → quét mã → chọn ảnh từ thư viện.
  - `Ngắt kết nối` (`DELETE /me/zalo`) with a confirm, then card → not linked.
- Non-functional:
  - Vietnamese copy; `components/hv` primitives only (`HvModal`, `HvButton`,
    `hvToast`); tokens from `styles/tokens`. No new primitives.
  - Polling via TanStack Query `refetchInterval` (~1.5s) only while the modal is
    open and state is non-terminal; stop on terminal state or modal close.
  - Full keyboard + screen-reader path on the modal (consent checkbox labelled,
    QR `<img>` has alt, countdown announced politely).
  - Respect `prefers-reduced-motion` for the countdown ring.

## Architecture

New feature slice under `apps/web/src/features/profile` (co-located, since the
card lives on the profile page — no separate `features/zalo` on web is warranted
for one card):

```
features/profile/
  api/zalo-api.ts          getZaloStatus, startZaloLink, getZaloLinkStatus, unlinkZalo
  hooks/use-zalo.ts        useZaloStatus (query), useStartLink (mutation),
                           useZaloLinkStatus(linkId, enabled) (polling query), useUnlinkZalo
  schemas/zalo-schemas.ts  zod parse for each response (mirror api/envelope pattern)
  components/zalo-connect-card.tsx   the three-state card (replaces inline JSX)
  components/zalo-link-modal.tsx     consent + QR + countdown + mobile save
```

**Polling control:** `useZaloLinkStatus(linkId, { enabled })` with
`refetchInterval: (q) => isTerminal(q.state?.state) ? false : 1500`. `enabled`
= modal open && have a `linkId`. Terminal states: `linked`, `expired`, `error`.

**State machine (client):**
`idle → consent → (Tiếp tục, sends consent_version) → starting → qr_ready(polling)
→ scanned → confirmed → linked | expired | error`.
`qr_ready` shows the QR; `scanned`/`confirmed` swap it for the "đã quét · chờ xác
nhận" view; `linked` closes the modal; `expired`/`error` show `Tạo mã mới` → back
to `starting`. Terminal (stop-polling) states remain `linked | expired | error`;
`scanned`/`confirmed` keep polling.

**QR data URI:** the `qr_png` field from `link/status` is base64 PNG →
`src={`data:image/png;base64,${qrPng}`}`. `Lưu ảnh QR` builds an `<a download>`
from the same data URI.

## Related Code Files

- Create: `apps/web/src/features/profile/api/zalo-api.ts`
- Create: `apps/web/src/features/profile/hooks/use-zalo.ts`
- Create: `apps/web/src/features/profile/schemas/zalo-schemas.ts`
- Create: `apps/web/src/features/profile/components/zalo-connect-card.tsx`
- Create: `apps/web/src/features/profile/components/zalo-link-modal.tsx`
- Create: `apps/web/src/features/profile/__tests__/zalo-connect-card.test.tsx`
- Create: `apps/web/src/features/profile/__tests__/zalo-link-modal.test.tsx`
- Create: `apps/web/src/features/profile/__tests__/zalo-handlers.ts` (MSW handlers)
- Modify: `apps/web/src/features/profile/pages/profile-page.tsx` (swap inline stub card for `<ZaloConnectCard/>`)

## Implementation Steps

1. `zalo-api.ts` + `zalo-schemas.ts`: typed clients using `apiClient` and
   `parseData`, mirroring `profile-api.ts`. `startZaloLink` posts
   `{ consent_version }`; keep the consent copy + its version as a single
   versioned constant so the displayed text and the persisted version can't drift.
   The status schema's `state` union includes `scanned`/`confirmed`.
2. `use-zalo.ts`: the four hooks; `useZaloLinkStatus` with the `refetchInterval`
   terminal-state guard; invalidate `['zalo','status']` on link/unlink success.
3. `zalo-link-modal.tsx`: two-step modal, countdown timer (`useEffect` interval,
   cleared on unmount / terminal), a `scanned`/`confirmed` view that replaces the
   QR with the "đã quét · chờ xác nhận" spinner+instruction, mobile `Lưu ảnh QR`
   under `md` (Tailwind `md:hidden`/matchMedia), reduced-motion guard.
4. `zalo-connect-card.tsx`: reads `useZaloStatus`, renders the three states,
   owns modal open state and the unlink confirm (reuse `confirm-dialog.tsx`).
5. Swap the inline card in `profile-page.tsx` for `<ZaloConnectCard/>`; keep the
   bank + data-export cards and the message-footer preview untouched.
6. Tests with MSW: card renders each state; consent gates `Tiếp tục` and
   `link/start` receives the `consent_version`; a mocked `link/status`
   progressing `qr_ready → scanned → linked` shows the "đã quét" view then closes
   the modal and flips the card; unlink confirm calls `DELETE`. Follow
   `notifications-page.test.tsx` patterns (`src/features/collections/__tests__/`).
7. `npm test -- profile` and `npm run lint`/`typecheck` for touched files.

## Success Criteria

- [x] Card shows the correct state for linked / not-linked / expired from the
      real endpoint. Verified: `zalo-connect-card.tsx:53-99` renders the three
      states. A code review found a fourth case — a *failed* status query
      rendered as "not linked" to an already-linked teacher (finding H2) — fixed
      by branching on `isError` (`zalo-connect-card.tsx:18,75`) to show a retry
      instead; see `plans/reports/zalo-phase-05-code-review.md` disposition.
- [x] Consent checkbox gates `Tiếp tục`; `Tiếp tục` sends the `consent_version`;
      QR renders from the data URI; countdown runs. Verified: phase-05 code
      review criterion 2 (code review report, Direct answers §3).
- [x] A `scanned`/`confirmed` poll swaps the QR for the "đã quét · chờ xác nhận"
      view while still polling. Verified: `zalo-link-modal.tsx:145-157`; both
      states are non-terminal per `zalo-schemas.ts:51-53`.
- [x] A status poll reaching `linked` closes the modal, toasts, and flips the
      card without a manual refresh. Verified: `zalo-link-modal.tsx:69-74` →
      `zalo-connect-card.tsx:27-31`.
- [x] `Lưu ảnh QR` is present under `md` and downloads the PNG. Verified:
      `zalo-link-modal.tsx:188-194`. Caveat: iOS Safari ignores `download` on a
      `data:` URI (review finding M6); mitigated with `target="_blank"
      rel="noopener"` (`zalo-link-modal.tsx:230-231`) so the QR opens in a new
      tab instead of replacing the running attempt. A full Blob-URL fix was
      deferred as disproportionate to the remaining gap.
- [x] `Ngắt kết nối` confirms then returns the card to not-linked. Verified:
      `zalo-connect-card.tsx:104-128`.
- [x] Modal is keyboard-navigable and screen-reader labelled; polling stops on
      close/terminal. Verified: `HvModal`'s Radix focus trap, Esc/overlay close,
      labelled checkbox, QR alt text, `role="timer"` countdown
      (`components/hv/hv-modal.tsx:121-140`, `zalo-link-modal.tsx:117-129,164,177-183`).
      State-transition announcements (finding M5) and the stop-on-close test
      (deleted in an interim rewrite) were both fixed per the code review
      disposition — `aria-live="polite"` at `zalo-link-modal.tsx:175,185,217`,
      and the stop-on-close assertion restored using `unmount()`.
- [x] `npm test` green for the profile feature; lint/typecheck clean. Verified:
      30 files / 146 tests pass, `eslint` 0 errors (4 pre-existing
      `react-hook-form` warnings unrelated to this feature), `tsc -b --noEmit`
      exit 0, `prettier --check` clean, `npm run build` succeeds — final state
      after the phase-05 code review's fixes.

## Execution Notes

- Built as designed at `apps/web/src/features/profile/{api,hooks,schemas,components}`;
  file names match the plan's Related Code Files, plus one addition:
  `__tests__/zalo-polling-errors.test.tsx` for the error-path tests added
  during the test-quality pass (`plans/reports/zalo-phase-05-test-report.md`).
- A code review (`plans/reports/zalo-phase-05-code-review.md`) found the
  happy path solid but the unhappy path incomplete: a poll that never
  succeeds looped forever with no visible error or stop condition (H1), a
  failed status query rendered as "not linked" to an already-linked teacher
  (H2), and the one screen a teacher sees on failure was in English, the
  server's fixed diagnostic string rather than client-owned Vietnamese copy
  (H3). All three fixed:
  - H1 — `useZaloLinkStatus` stops polling after `ZALO_MAX_POLL_ERRORS = 3`
    consecutive failures (`use-zalo.ts:14,49`); the modal declares the attempt
    lost at the same threshold so the UI doesn't say "failed" while the hook
    is still retrying.
  - H2 — the card branches on `isError` and offers a retry instead of
    drawing an unreadable status as "not connected" (`zalo-connect-card.tsx:18,75`).
  - H3 — all failure copy is now client-owned Vietnamese; the server's
    `error_message` carries nothing a teacher could act on, so it is no
    longer rendered.
  Four of six medium findings were also fixed: closing the modal now
  invalidates the status query (M1); a failed unlink raises a danger toast
  (M2); an explicit `phase` state replaces the derived one so a retry can't
  flash the consent screen (M3); the countdown reaching zero is treated as
  local expiry so a dead QR doesn't linger when polls can't land (M4). M6
  (iOS Safari QR download) was partially mitigated, not fully fixed — see the
  Success Criteria note above. The `ATTEMPT_TTL_SECONDS` duplication (a low
  finding) was left as-is: removing it means returning the deadline from
  `link/start`, an API contract change outside this phase's scope.
- The test report (`plans/reports/zalo-phase-05-test-report.md`) originally
  claimed TypeScript compiled because `vitest` ran; that inference was wrong
  (`vitest` doesn't typecheck) and the added test file in fact failed `tsc`,
  `eslint`, and `prettier`. The report's own correction section documents the
  fix: unused declarations removed, three of seven added tests dropped as
  duplicates of existing coverage, four kept.

## Risk Assessment

- **Mobile QR trap:** a teacher on their phone can't scan a QR on the same
  screen. Mitigation: `Lưu ảnh QR` download path is a hard requirement, tested.
- **Runaway polling:** an un-guarded `refetchInterval` keeps hitting the API.
  Mitigation: terminal-state + modal-open guards; assert in test that polling
  stops after `linked`.
- **Copy-paste fallback must survive:** this phase must not remove or regress the
  existing manual send path anywhere else in the app — it only changes the
  profile card. Verify the notifications page is untouched.
