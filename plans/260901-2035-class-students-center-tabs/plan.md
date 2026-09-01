---
title: "class-students-center-tabs"
description: "Chuyển trang Lớp & học sinh sang nhóm nav Trung tâm, gate owner-only ở web, tái cấu trúc thành 3 tab (Lớp học | Học sinh | Chưa ghi danh) theo pattern trang Phân quyền vai trò"
status: completed
priority: P1
effort: "2.5-3d"
tags: [web, rbac, ui, navigation]
created: 2026-09-01
---

# class-students-center-tabs

## Overview

Trang "Lớp & học sinh" (`/students`, `students-page.tsx`) hiện nằm trong nhóm
nav "Dạy học" với gate `perm: "students.list"`, dùng pill chọn lớp để lọc một
bảng học sinh × liên hệ, kèm tab sentinel "Chưa ghi danh" (`class_id=none`).

Plan này chuyển trang thành một trang quản trị trung tâm:

1. **Nav**: chuyển entry sang nhóm "Trung tâm", chỉ hiện cho owner (spread
   `isResolved && isOwner`, cùng pattern với "Phân quyền vai trò" tại
   `dashboard-layout.tsx:135-148`); trên mobile chuyển vào sheet "Thêm"
   (`OVERFLOW_LABELS`).
2. **Guard**: trang owner-only hoàn toàn — non-owner bị redirect về `/`
   (pattern component-vỏ của `center-permissions-page.tsx:13-21`).
3. **Tabs**: tái cấu trúc thành 3 tab underline theo pattern
   `permission-matrix.tsx:215-246` (`role="tablist"`, gạch chân mint-400):
   **Lớp học** (danh sách lớp + tạo lớp + link cài đặt lớp) | **Học sinh**
   (bảng roster hiện tại, vẫn lọc theo lớp bằng pill) | **Chưa ghi danh**
   (học sinh chưa vào lớp, thay sentinel).
4. **Quét điểm chạm member**: mọi nơi đang trỏ member vào `/students`
   (dashboard card "Lớp mới", trang nhập Excel, back-link cài đặt lớp,
   3 e2e spec) được sửa để không dead-end (chi tiết theo phase).

## Decisions (accepted 2026-09-01, user-confirmed)

| Quyết định | Lựa chọn | Lý do |
|---|---|---|
| Mức truy cập | Owner-only hoàn toàn (ẩn nav + redirect) | Trang trở thành surface quản trị; member thao tác lớp/HS qua các màn Dạy học |
| Cấu trúc tab | Lớp học \| Học sinh \| Chưa ghi danh | Tách quản lý lớp khỏi roster; tab Lớp học là UI mới, 2 tab còn lại từ nội dung hiện có |
| API enforcement | **Giữ nguyên permission keys** — không đổi route policy | Không phá teacher flows (điểm danh, classbook, ghi danh); không đảo quyết định RBAC của plan [[260829-1640-gh-260829-flexible-center-rbac]]; owner vẫn thu/cấp quyền member qua Phân quyền vai trò |
| Route path | Giữ `/students` | Không churn deep-link (`/students/:id`, `/students/import` giữ nguyên); nhóm nav chỉ là presentation |
| Tab state | URL search param `?tab=`, ghi với `{ replace: true }` | Trang đã round-trip `class_id`/`q` qua URL; không có draft chưa lưu như permission-matrix nên không cần local state |
| E2e member roster | Chuyển assert sang `/records?class_id=` | Giữ coverage acceptance của plan handoff-roster-visibility, chỉ đổi surface (records-page enrollment-based, member đọc được) |
| Card "Lớp mới" (dashboard) | **Ẩn card với non-owner** | User chọn ẩn thay vì đổi target theo vai trò; member không thấy card, tránh dead-end |
| Mobile bottom bar | Vào sheet "Thêm" (`OVERFLOW_LABELS`) | Nhất quán với mọi entry nhóm Trung tâm; bottom bar hai vai trò đồng nhất |

**Không đổi permission catalog**: không thêm/bớt key, không tăng
`CatalogVersion` (msw mirror giữ = 3), không migration.
`docs/adding-permissions.md` đã được đối chiếu với code (route_policy.go,
catalog.go, permissions.go, msw handlers) ngày 2026-09-01 — chính xác,
không cần sửa.

## Accepted consequences (hệ quả của owner-only, có bằng chứng)

- **Member mất surface ghi danh học sinh có sẵn vào lớp mình phụ trách**:
  `EnrollExistingStudentDialog` chỉ được dùng tại `students-page.tsx:474-479`
  (gate `canWriteClass`). Đường còn lại là `/students/:id` → "Ghi danh vào
  lớp" (`student-detail-page.tsx:76-80`), nhưng member chỉ tới được qua
  `/contacts/:id` — cần quyền `contacts.list`. Member không có
  `contacts.list` sẽ phải nhờ owner ghi danh. Đây là hệ quả được chấp nhận
  cùng quyết định owner-only; không mở surface mới trong plan này.
- **Học vụ (class-role) mất hẳn đường gửi báo cáo class-scoped qua UI**
  (phát hiện ở phase 4, user chốt 2026-09-01: chấp nhận bỏ): khẳng định ban
  đầu "học vụ còn entry /reports và notifications-page" là **sai** — nav
  `/reports` yêu cầu quyền center-wide `can_send_reports` mà học vụ
  class-role không có, và notifications-page cần period id chỉ khám phá được
  qua `ClassSendPeriodsDialog`, vốn chỉ mount từ nút "Gửi báo cáo" của trang
  roster (nay owner-only). Hệ quả đã chấp nhận: xoá
  `ClassSendPeriodsDialog` (dead code), trim biến thể class-scoped của
  `useReportPeriods`/`listReportPeriods`, bỏ e2e test "hoc_vu discovers the
  class period from the roster". API class-scoped send vẫn cho phép — chỉ
  UI path bị cắt.

## Related plans

- `260829-1640-gh-260829-flexible-center-rbac` (in-progress): hạ tầng RBAC +
  trang Phân quyền vai trò đã merge master — plan này chỉ tiêu thụ pattern,
  không sửa catalog nên không blocking.
- `260829-2127-class-score-config` (done): tiền lệ entry owner-only
  "Cấu hình lớp học" trong nhóm Trung tâm.
- `260830-0714-GH-260830-handoff-roster-visibility` (done): acceptance
  e2e của nó đang assert roster member tại `/students` — plan này chuyển
  assert đó sang `/records` (quyết định user 2026-09-01), giữ nguyên
  hành vi API được plan đó bảo vệ.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Entry "Lớp & học sinh" nằm trong nhóm "Trung tâm", chỉ owner thấy; biến mất khỏi "Dạy học" và bottom bar mobile (vào sheet "Thêm") | P1 |
| 2 | Non-owner vào thẳng `/students` bị redirect về `/` với **zero request** roster (guard component-vỏ) | P1 |
| 3 | Trang có 3 tab underline đúng pattern Phân quyền vai trò, state qua `?tab=`, a11y hai tầng tablist ("Khu vực" / "Lớp") | P1 |
| 4 | Tab Lớp học: danh sách lớp active (lịch + đơn giá từ `Class` schema) + tạo lớp + link `⚙ Cài đặt` từng lớp | P1 |
| 5 | Tab Học sinh giữ nguyên năng lực hiện có (lọc lớp, tìm kiếm, bảng + card mobile, dialogs) | P1 |
| 6 | Tab Chưa ghi danh thay sentinel `class_id=none`; deep-link `class_id` cũ không kèm `tab` vẫn về đúng roster (quy tắc suy tab) | P1 |
| 7 | Mọi điểm chạm member vào `/students` được sửa: ẩn card "Lớp mới" với non-owner, copy trang nhập Excel, back-link cài đặt lớp theo vai trò, 3 e2e spec chuyển sang `/records` | P1 |
| 8 | Không đổi API/permission catalog; docs/comments được rà cập nhật | P2 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Nav move + owner guard + entry-point sweep](./phase-01-start.md) | Completed |
| 2 | [Phase 2: Tab shell and classes tab](./phase-02-tab-shell-and-classes-tab.md) | Completed |
| 3 | [Phase 3: Students and unenrolled tabs](./phase-03-students-and-unenrolled-tabs.md) | Completed |
| 4 | [Phase 4: Verification, e2e rewrite, docs](./phase-04-verification-and-docs.md) | Completed |

Phase 1 độc lập triển khai được (giá trị bảo mật UI ngay). Phase 2 → 3 tuần
tự (cùng file `students-page.tsx`). Phase 4 chốt (bao gồm viết lại e2e —
bắt buộc, không tuỳ chọn).

## Success Criteria

- [x] Owner: thấy "Lớp & học sinh" trong nhóm "Trung tâm" (desktop) và sheet "Thêm" (mobile), trang render 3 tab, mọi flow (tạo lớp, thêm/sửa/xoá HS, ghi danh, wizard) hoạt động như trước. (Unit tests `students-page.test.tsx` 23 pass + e2e `roster.spec.ts` xanh trên stack cô lập.)
- [x] Member (mọi permission): không thấy entry/card nào trỏ tới `/students`; vào thẳng URL bị redirect `/` không phát sinh request roster; các màn Dạy học không hỏng. (Test guard zero-request + e2e `class-staff-read`/`class-staff-write` xanh; grep `/students` chỉ còn usage owner-context và route defs.)
- [x] Deep-link: `?tab=` tường minh thắng; không có `tab` thì `class_id=none` → Chưa ghi danh, `class_id` khác → Học sinh, còn lại → Lớp học. Refresh khôi phục đúng trạng thái. (5 case suy tab đều có assertion trong `students-page.test.tsx`.)
- [x] `npm run typecheck`, `npm run test` (web) xanh; 3 e2e spec (`class-staff-read`, `class-staff-write`, `roster`) viết lại xanh trên stack e2e; assert roster member chạy trên `/records`. (Vitest 480 pass/3 skip; e2e seed tươi 2026-09-01 — 5 fail còn lại của suite là breakage master có sẵn từ PR #39, chi tiết ở phase 4.)
- [x] Không diff nào dưới `apps/api/` và không đổi `CATALOG_VERSION`/msw catalog mirror. (`git diff --stat apps/api/` rỗng; `CATALOG_VERSION` = 3.)
- [x] Comments/docs mô tả trang được cập nhật đúng thực tế mới. (4 comment kiểm kê ở phase 3 đã sửa; grep "Lớp & học sinh" trong docs/README → 0 hit, không có docs evergreen phải đổi.)

## Non-goals

- Không đổi API route policy, permission catalog, migration.
- Không đụng nội dung `class-settings-page.tsx` ngoài back-link — vẫn là trang riêng, tab Lớp học chỉ link tới.
- Không gate lại `/students/:id`, `/contacts/*`, `/students/import` — giữ hành vi hiện tại.
- Không extract shared `HvTabs` component (2 usage chưa đủ; ghi nhận follow-up).
- Không thêm đếm sĩ số per-class — đã xác minh `Class` (`roster-schemas.ts:177-190`) không có field đếm; tránh N+1.
- Không thêm surface ghi danh mới cho member (hệ quả đã chấp nhận ở trên).

## Open questions

None — 6 quyết định sản phẩm đã được user chốt (bảng Decisions), red-team
findings đã được hấp thụ vào các phase.

<!-- slug: class-students-center-tabs -->
