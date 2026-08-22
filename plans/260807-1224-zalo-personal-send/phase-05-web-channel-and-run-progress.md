---
phase: 5
title: "Web channel + run progress"
status: completed
priority: P1
effort: "1d"
dependencies: [3, 4]
---

# Phase 5: Web channel + run progress

## Overview

Trang notifications: chọn kênh `zalo_personal` khi đã link, confirm trước khi
gửi, theo dõi run bằng poll, hiển thị fallback/failed trung thực, resume run
bị gián đoạn. Giữ nguyên flow copy-paste manual.

<!-- Updated: Validation Session 1 - pre-send confirm dialog + interrupted/resume UI -->

## Requirements

- Functional:
  - Chọn kênh ở action bulk-send: mặc định `zalo_manual` như hiện tại; option
    `Gửi qua Zalo (tự động)` chỉ enable khi profile status = linked (dùng query
    `GET /me/zalo` sẵn có của profile feature); tooltip/ghi chú khi disabled.
  - **Confirm trước khi gửi** kênh personal: dialog "X phụ huynh gửi tự động ·
    Y dùng copy thủ công — tiếp tục?" (counts tính client-side từ
    `zalo_user_id` trên contact data đã fetch — không cần endpoint preview);
    xác nhận xong mới gọi bulk-send.
  - Sau bulk-send kênh personal: banner tiến độ `Đang gửi x/y…` poll
    `GET /billing-periods/:id/notifications/run` (TanStack Query
    `refetchInterval` ~2s khi `active`, dừng khi `done`); đồng thời invalidate
    ledger list để row status cập nhật.
  - Hiển thị: `fallback_manual_count` ("N contact chưa liên kết — dùng
    copy-paste bên dưới", các row manual vẫn vào flow copy/mark-sent hiện tại);
    row failed → lý do (`error_message` từ ledger); run xong → summary
    `Đã gửi x · Lỗi y`.
  - Bulk-send khi run đang chạy → API 409 → toast/message "Đang có lượt gửi
    chạy, đợi xong đã".
  - Run `interrupted` (restart giữa run): banner "Lượt gửi bị gián đoạn —
    còn N chưa gửi" + nút "Gửi tiếp" →
    `POST /billing-periods/:id/notifications/run/resume`, rồi poll tiếp như
    run thường.
  - Session expired (API 4xx pre-check) → message + link tới profile re-scan.
- Non-functional: poll dừng hẳn khi run done/không có run (không poll nền vô
  hạn); copy tiếng Việt; tests MSW mô phỏng run tiến triển.

## Architecture

- Schemas (`collections-schemas.ts`): mở rộng bulk-send response
  (`run_id`, `personal_queued_count`, `fallback_manual_count`), schema run
  snapshot; `notifications-api.ts` thêm `getNotificationRun(periodId)`.
- Hook mới `useNotificationRun(periodId, {enabled})`: `refetchInterval` trả
  2000 khi status `running`, `false` khi terminal
  (`completed`/`expired`/`interrupted`); done → invalidate `listNotifications`.
  Mutation `useResumeNotificationRun(periodId)` cho nút "Gửi tiếp".
- UI: channel select (radio/segmented trong khu action `notifications-page.tsx`),
  confirm dialog trước khi gửi (dùng `confirm-dialog` pattern sẵn có, counts từ
  contact mapping), component `run-progress-banner.tsx` (progress + counts +
  trạng thái interrupted/nút resume + failed link xuống ledger).
  `message-card.tsx` giữ nguyên cho manual rows.

## Related Code Files

- Create: `apps/web/src/features/collections/components/run-progress-banner.tsx` (+ test)
- Modify: `apps/web/src/features/collections/pages/notifications-page.tsx`,
  `api/notifications-api.ts`, `hooks/use-notifications.ts`,
  `schemas/collections-schemas.ts`, `__tests__/collections-handlers.ts` + tests
- Modify: `apps/web/src/test/msw/handlers.ts` (nếu handlers dùng chung)

## Implementation Steps

1. Schemas + API + MSW (run snapshot có kịch bản tiến triển theo số lần gọi).
2. `useNotificationRun` với refetchInterval động + invalidation.
3. Channel select gating theo `GET /me/zalo` status + confirm dialog trước khi
   gửi (counts client-side).
4. `run-progress-banner.tsx` + tích hợp page; handle 409/expired/interrupted +
   nút "Gửi tiếp" (resume mutation).
5. Tests: linked/unlinked gating, confirm dialog counts, poll tiến triển →
   done, fallback count, failed row hiển thị lý do, 409 khi run đang chạy,
   interrupted → resume.
6. E2E hiện có (`apps/web/e2e/statement.spec.ts` khu notifications) — cập nhật
   nếu copy/DOM đổi.

## Success Criteria

- [x] Chưa link → option personal disabled kèm giải thích; linked → confirm
      dialog hiển thị đúng "X tự động · Y thủ công" rồi mới gửi.
- [x] Poll hiển thị x/y tăng dần, dừng khi terminal, ledger rows đổi trạng thái
      không cần reload; run interrupted → banner + "Gửi tiếp" hoạt động.
- [x] Fallback count + manual flow không thay đổi hành vi cũ (tests cũ xanh).
- [x] `npm test`, `eslint`, `tsc`, `prettier`, `npm run build` xanh.

## Risk Assessment

- **Poll mồ côi** (bật interval khi không có run): enabled gate theo response
  `active`; test chốt "không run → không refetch".
- **Đóng tab giữa run:** by design run vẫn chạy server-side; mở lại trang →
  banner khôi phục từ snapshot endpoint (test case reload-mid-run).
