---
phase: 3
title: "Students and unenrolled tabs"
status: completed
priority: P1
effort: "6h"
dependencies: [2]
---

# Phase 3: Students and unenrolled tabs

## Overview

Hoàn thiện tab **Học sinh** và tab **Chưa ghi danh**, dọn các conditional
owner giờ luôn-đúng, gỡ nút gửi báo cáo member-only, và sửa hai điểm chạm
member còn lại: copy/link trang nhập Excel và back-link trang cài đặt lớp.

## Requirements

- Functional: tab Học sinh = pill chọn lớp + tìm kiếm + bảng/card như hiện
  tại; tab Chưa ghi danh = danh sách unenrolled + "Ghi danh vào lớp";
  wizard thêm học sinh (Bước 2/2, "ghi danh sau") chuyển tab đúng; member
  không còn gặp copy/link chết trỏ về `/students`.
- Non-functional: không đổi query keys/endpoints; `keepPreviousData`
  guards hiện có (map enrollment gated theo `selectedClassId`) giữ nguyên.

## Architecture

- **Tab Học sinh**: giữ pill `role="tablist"` "Lớp" (bỏ item sentinel
  "Chưa ghi danh" khỏi mảng pill — giờ là tab trang), search input, bảng
  desktop + stacked card mobile, cột Buổi T{m}/Nhập học với logic
  `monthSessionCount`/`enrollmentStartLabel` nguyên trạng. Action trong
  tab: `+ Ghi danh học sinh` (EnrollExistingStudentDialog),
  `+ Thêm học sinh` (StudentDialog wizard), Sửa/Xoá per-row.
- **Tab Chưa ghi danh**: nhánh `isUnenrolledTab` hiện tại
  (`useStudentsList({ unenrolled: true })`); giữ badge "Chưa vào lớp nào" +
  "Ghi danh vào lớp" (EnrollStudentDialog); `onSuccess` →
  `setTab("students", { class_id: enrollment.class_id })`.
- **Sentinel cleanup**: xoá hằng `UNENROLLED_TAB` và mọi nhánh
  `isUnenrolledTab` đan trong render (nhánh suy-tab của phase 2 vẫn nhận
  `class_id=none` từ link cũ). Toast wizard "ghi danh sau" →
  `setTab("unenrolled")`.
- **Dọn owner conditionals**: guard vỏ (phase 1) bảo đảm content chỉ render
  cho owner ⇒ các ternary `isOwner ? ...` trong content (hiện tại dòng
  219-229, 328-345, 434-454) thành render thẳng. Cập nhật các comment mô
  tả trang đã sai thực tế: `students-page.tsx:41-44` (mô tả "primary nav
  destination" + pill tabs), `:60-62` (member enroll-only),
  `use-class-search.ts:10`, `class-settings-page.tsx:55`.
- **Gỡ nút "Gửi báo cáo"**: `canRunSends` là cờ member-only (owner không
  bao giờ thấy nút — comment tại dashboard-layout xác nhận), member không
  còn vào trang ⇒ nút chết. Xoá button + import `ClassSendPeriodsDialog`/
  `canSendClassReports` khỏi trang. Đường gửi của học vụ còn nguyên:
  `/reports` và notifications-page.
- **`canWriteClass` per-class**: GIỮ tại chỗ gọi (hàm đã owner-bypass) để
  ngữ nghĩa per-class không mất nếu sau này mở lại cho member — không dọn
  quá tay.
- **Trang nhập Excel** (`roster-import-page.tsx`): nhánh non-owner
  (`:36-50`) đang nói member "tự thêm lớp và học sinh trong màn hình
  [Lớp & học sinh]" kèm `<Link to="/students">` — sai sự thật sau guard.
  Viết lại copy non-owner: bỏ link, chỉ còn hướng "nhờ chủ trung tâm nhập
  giúp". CTA sau commit "Xem lớp & học sinh" (`:230-238`,
  `navigate("/students")`) chỉ render cho owner (trang này member tới được
  qua entry `imports.run`).
- **Back-link cài đặt lớp** (`class-settings-page.tsx:105`, render `:174`):
  trang member đọc được (chỉ khoá ghi qua `canWriteClass`; e2e
  `class-staff-read.spec.ts:38-45` assert điều đó). `backTo` theo vai trò:
  owner → `/students?tab=students&class_id=${id}`; non-owner →
  `/records?class_id=${id}` (label đổi tương ứng, ví dụ "← Sổ lớp").

## Related Code Files

- Modify: `apps/web/src/features/roster/pages/students-page.tsx`
- Modify: `apps/web/src/features/roster/pages/roster-import-page.tsx`
- Modify: `apps/web/src/features/roster/pages/class-settings-page.tsx`
- Modify: `apps/web/src/features/roster/hooks/use-class-search.ts` (comment)
- Create: `.../components/students-tab.tsx` + `.../unenrolled-tab.tsx` CHỈ
  KHI tách xong page vẫn >400 dòng — ngược lại giữ trong page (YAGNI;
  nếu 2 tab trùng ≥70% markup bảng thì tách MỘT component bảng dùng prop
  `variant` thay vì hai file).
- Modify tests: `students-page.test.tsx` (flows + scope `within()` theo
  phase 2), `class-settings-page.test.tsx:53`,
  `class-settings-handoff.test.tsx:50` (backTo mới), test roster-import
  (copy theo vai trò), msw fixtures unenrolled nếu thiếu.

## Implementation Steps

1. Di chuyển pill lớp + search + bảng vào nhánh Học sinh; bỏ sentinel khỏi
   pill.
2. Dựng nhánh Chưa ghi danh từ logic `isUnenrolledTab` hiện có.
3. Nối wizard flows qua `setTab` helper (phase 2).
4. Dọn owner ternaries + gỡ nút Gửi báo cáo + cập nhật 4 comment đã kiểm kê.
5. Sửa roster-import-page (copy + CTA theo vai trò) và class-settings-page
   (backTo theo vai trò).
6. Tests: (a) tab Học sinh: lọc lớp, tìm kiếm debounce, cột buổi/ngày,
   sửa/xoá mở đúng dialog; (b) tab Chưa ghi danh: badge + ghi danh xong
   nhảy về tab Học sinh đúng lớp; (c) wizard 2 bước: "để sau" → tab Chưa
   ghi danh kèm toast; (d) không còn "Gửi báo cáo" trên trang; (e) import
   page: member thấy copy mới không có link, owner thấy CTA; (f) settings
   backTo đúng theo vai trò.

## Success Criteria

- [x] Mọi flow CRUD/ghi danh/wizard pass test; không đổi query key nào.
- [x] Không còn tham chiếu `UNENROLLED_TAB`/`canSendClassReports` trong trang.
- [x] Member không còn bất kỳ copy/link nào trỏ `/students` (grep
      `"/students"` trong `apps/web/src` chỉ còn usage owner-context và
      route defs).
- [x] `npm run test` (web) + `npm run typecheck` xanh.

## Risk Assessment

- **Regression flow wizard 2 bước** (thêm HS → ghi danh): nhiều state đan
  (`enrolling`, `enrollFromWizard`) — test (c) bắt buộc trước khi dọn.
- **Dọn quá tay `keepPreviousData` guards**: các comment trong file là bài
  học bug thật — di chuyển nguyên khối, không "tiện tay" viết lại.
- **Label back-link cho member**: đổi từ "Lớp & học sinh" sang đích mới —
  soát cả e2e đang match text này.
- **Rollback**: revert commit phase; phase 3 phụ thuộc phase 2 nên rollback
  theo cặp nếu đã merge chung nhánh.
