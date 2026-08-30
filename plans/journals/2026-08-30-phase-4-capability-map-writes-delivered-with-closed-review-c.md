---
title: Phase 4 capability-map writes delivered with closed review cycle
date: 2026-08-30
summary: "Class-staff write gates shipped web+API; review found real RunSnapshot authz gap; stale tests diagnosed, no redeploy needed"
---

# Phase 4 capability-map writes delivered with closed review cycle

## What happened

Executed phase 4 of `plans/260830-0938-gh-260830-class-staff-roles-phone-privacy/` (TDD, auto mode): write endpoints now enforce the capability map (`authctx/class_staff.go`) — attendance writes for giao_vien + tro_giang, scores/remarks/lesson-plan/enrollment/sessions for giao_vien, statement send for hoc_vu, owner passes all; 403 honest khi readable-but-incapable, 404 neutral khi không có stint. Web mirrors per-capability qua `features/roster/lib/class-permissions.ts` (`canWriteClass`, `canRecordAttendance`, `canSendClassReports`) thay vì một boolean chung.

Review cycle (code-reviewer) trả 12 findings, 11 fixed in-slice, 1 info-only:

- **Fix authz thật**: `RunSnapshot` chiều class thiếu `AuthorizeClassSend` — mọi member cùng center poll được tiến độ gửi của lớp bất kỳ (repo lookup chỉ scope theo center). Đã gate trong `notifications/service.go` + pin integration test (hoc_vu OK, owner OK, tro_giang 403, outsider 404).
- Web: `accessResolved = Boolean(klass)` để nút xác nhận điểm danh chờ ở trạng thái "Đang tải…" thay vì flash nhãn từ chối; enroll button gate theo `canWriteClass`; cancel session là sessions.write (giao_vien/owner only).
- e2e: afterEach restore idempotent cho spec flip attendance (capture `{sheetUrl, pressed}` trước khi mutate, chỉ restore khi mismatch).
- Billing: test edge cases cho `ListPeriodsByClass` (ended stint vẫn list, handoff vẫn list cho teacher cũ, foreign class 404, voided invoices → empty).
- Info-only: `classInvoiceLinesExist` EXISTS-chain chưa có index hỗ trợ — ghi nhận, không fix trong slice.

## Diagnosis wins

- Hai test attendance fail sau deploy là **stale tests** (assert behavior cũ đã bị plan này thay đổi có chủ đích), không phải bug code → không cần redeploy production.
- Full `make test-api` rerun fail ở package `migrations` (150s, coverage 0.2%) trong khi chạy song song với teardown e2e — chạy isolated (`go -C apps/api test -tags integration -count=1 -p 1 ./migrations/ ./seeds/`) pass trong 7s → testcontainer resource contention, không phải regression. Bài học: full suite cần máy rảnh; verify package nghi ngờ bằng `-count=1 -p 1` trước khi kết luận.

## Verification

- Web: tsc -b sạch, vitest 449 passed / 68 files, eslint 0 error.
- API: build + vet sạch; notifications + billing integration xanh; migrations + seeds xanh isolated.
- e2e: 31 passed (3.7m) trên stack cô lập `teka-e2e`, seed mới, `down -v` teardown.
- Swagger regen qua `make api-docs`.

## Next steps

- Phase 3 bước 4: dry-run queries trên prod DB (user-blocked).
- M5 open question: member POST /payments → 404 hay 403 (+ release note).
- Release note write-freeze sau handoff (D8) khi công bố.
- Prod đang chạy image build từ tree chưa commit — commit + rebuild khi tiện (~195 file trên `teka/260830-0506`).
- Phase 5 cleanup chờ soak.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
