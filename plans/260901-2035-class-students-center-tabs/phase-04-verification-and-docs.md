---
phase: 4
title: "Verification, e2e rewrite, docs"
status: completed
priority: P1
effort: "4h"
dependencies: [3]
---

# Phase 4: Verification, e2e rewrite, docs

## Overview

Viết lại 3 e2e spec đang phụ thuộc `/students` (bắt buộc — không tuỳ chọn),
chạy verification toàn diện phía web, xác nhận zero-diff phía API, rà
docs/comments.

## Requirements

- Functional: 3 e2e spec xanh trên stack e2e cô lập; assert roster member
  chạy trên `/records`; mọi gate unit/typecheck xanh.
- Non-functional: không side-effect ngoài `apps/web`.

## Architecture

Guard owner-only là web-only theo thiết kế (API vẫn permission-gated) —
e2e member phải xác nhận bị chặn ở UI (redirect), KHÔNG kỳ vọng 403 từ API
cho các key member vẫn được cấp.

**E2e rewrite (bắt buộc, đã kiểm kê từ red-team):**

- `apps/web/e2e/class-staff-read.spec.ts:29-36` — Cô Thu (hoc_vu)/Thầy
  Minh (tro_giang) `goto("/students")`, đọc roster và lấy `class_id` từ
  URL: chuyển sang `/records?class_id=` (records-page enrollment-based,
  member đọc được, có class picker `role="tab"` —
  `teaching/pages/records-page.tsx:28-47`). Thêm bước assert member
  `goto("/students")` → redirect `/`.
- `apps/web/e2e/class-staff-write.spec.ts:30-36` (`classIdFromRosterTab`)
  và `:228-229` (assert "Bé Phúc" — acceptance của plan
  handoff-roster-visibility): helper lấy class_id chuyển sang `/records`;
  ghi chú trong spec rằng acceptance handoff giờ được assert tại
  `/records` (quyết định user 2026-09-01).
- `apps/web/e2e/roster.spec.ts:52-63,118-124` (owner) — cập nhật theo
  layout mới: sau `goto("/students")` mặc định là tab "Lớp học"; các bước
  `+ Tạo lớp mới` chạy trong tab Lớp học; bước roster/tìm kiếm phải click
  tab "Học sinh" trước; selector pill scope trong tablist "Lớp".

**Phát hiện trong lúc thực thi (user chốt 2026-09-01):** e2e
`class-staff-write.spec.ts` còn test "hoc_vu discovers the class period from
the roster and sends the class copies" vào bằng nút "Gửi báo cáo" của trang
roster — entry duy nhất của học vụ class-role (không có grant center-wide
nên `/reports` không tới được, notifications-page cần period id không tự
khám phá được). User chọn **chấp nhận bỏ workflow UI này**: xoá test đó,
xoá `ClassSendPeriodsDialog` (dead code sau phase 3), trim biến thể
class-scoped của `useReportPeriods`/`listReportPeriods`. API vẫn cho phép
class-scoped send.

## Related Code Files

- Modify: `apps/web/e2e/class-staff-read.spec.ts`,
  `apps/web/e2e/class-staff-write.spec.ts`, `apps/web/e2e/roster.spec.ts`
- Delete: `apps/web/src/features/reports/components/class-send-periods-dialog.tsx`
  (kèm trim `reports/index.ts`, `reports/hooks/use-reports.ts`,
  `reports/api/reports-api.ts` — hệ quả quyết định bỏ workflow ở trên)
- Verify only: `apps/api/**` (git diff rỗng),
  `apps/web/src/test/msw/handlers.ts` (`CATALOG_VERSION` giữ nguyên = 3)
- Docs: đã grep trước (`grep -rn "Lớp & học sinh" docs/ README.md
  apps/web/README.md` → 0 hit; docs chỉ nhắc endpoint `/students` API —
  không liên quan) ⇒ không có docs evergreen phải sửa;
  `docs/adding-permissions.md` đã đối chiếu code 2026-09-01: chính xác,
  không sửa. Bước docs còn lại chỉ là re-check nhanh sau implement.

## Implementation Steps

1. Viết lại 3 e2e spec như kiểm kê trên.
2. `npm run typecheck && npm run test` trong `apps/web`.
3. `npm run e2e` trên stack e2e cô lập (compose project `teka-e2e`, port/URL
   overrides; statement specs cần seed tươi).
4. `git diff --stat apps/api/` phải rỗng; xác nhận `CATALOG_VERSION` không
   đổi; `make lint` nếu chạm shared config.
5. Re-check nhanh grep docs + comment (phase 3 đã sửa); đối chiếu Success
   Criteria của `plan.md`; cập nhật status phases qua `ak plan phase` CLI.

## Success Criteria

- [x] 3 e2e spec viết lại xanh; typecheck + unit test web xanh. (Full suite
      trên stack `teka-e2e` seed tươi 2026-09-01: 26 passed, cả 3 spec viết
      lại xanh. 5 fail còn lại — billing, collections, 3 statement — là
      breakage CÓ SẴN trên master: Web CI e2e của chính master@2efc779, base
      của nhánh, fail đúng 5 spec đó cộng test hoc_vu send cũ; CI web đỏ từ
      PR #39/b915a50, lần xanh cuối a0704ed 2026-08-30. Ngoài scope plan —
      cần bugfix riêng. Vitest: 68 files, 480 passed / 3 skipped;
      typecheck + lint 0 lỗi.)
- [x] `git diff` không có file nào dưới `apps/api/`; catalog mirror nguyên
      vẹn (`CATALOG_VERSION` = 3; tester + reviewer xác nhận độc lập).
- [x] Toàn bộ Success Criteria trong `plan.md` được tick với bằng chứng.

## Kết quả gate review (2026-09-01)

Reviewer không tìm thấy defect chặn. Một finding Medium đã sửa ngay: tab
"Lớp học" từng hiển thị lỗi tải `/classes` như danh sách rỗng (mời tạo lớp
trùng) — đã thread `isError` từ `useClassesList` xuống `ClassesTab`, render
"Không tải được danh sách lớp" theo đúng pattern `class-overview-cards`,
kèm unit test nhánh lỗi. Hai finding Low được ghi nhận là non-issue có chủ
đích, không đổi code: (1) card "Lớp mới" ẩn theo `!isOwner` chưa resolved —
hướng an toàn (member không thấy nháy card), chỉ là layout shift phía owner;
(2) tab "Thêm" mobile sáng active trên `/students/:id` với member — thuần
cosmetic, highlight sidebar không ảnh hưởng.

Reviewer cũng xác minh độc lập phía API rằng acceptance handoff chuyển sang
`/records` là tương đương (`classscope.ReadExists` gồm cả stint đã kết thúc
trên cả `GET /classes` lẫn `GET /enrollments`; đường `GET /students` vẫn có
integration test riêng phía Go). Follow-up ghi nhận, không làm trong plan:
nhánh class-mode của `notifications-page.tsx` (`?class_id=`) giờ là dead UI
sau khi bỏ workflow gửi của học vụ — dọn hay giữ là quyết định riêng; và
stub route `/students` trong `class-settings-handoff.test.tsx` giờ vestigial
(vô hại). Report đầy đủ: `plans/reports/260901-2035-phase4-code-review.md`;
completion report: `plans/reports/260901-2035-completion-report.md`.

## Risk Assessment

- **Stack e2e không sẵn**: dừng và báo (không skip âm thầm) — 3 spec này
  là acceptance surface của plan trước; plan chưa done khi e2e chưa chạy.
- **`/records` thiếu bước tương đương** (ví dụ lấy class_id từ URL): nếu
  records-page không round-trip `class_id` qua URL như kỳ vọng, dừng và
  báo user trước khi chế thêm hành vi cho records-page (ngoài scope).
- **Rollback**: e2e-only + verify; revert commit phase nếu cần.
