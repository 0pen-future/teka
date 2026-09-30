---
phase: 4
title: "Sidebar và route"
status: completed
priority: P1
effort: "3h"
dependencies: [3]
---

# Phase 4: Sidebar và route

## Goal

Sidebar có một mục "Học sinh" đứng đầu nhóm "Lớp học" (đổi tên từ "Giảng dạy"),
bỏ ba mục cũ, ẩn nhóm rỗng; nhãn route cập nhật.

## Context

- `apps/web/src/layouts/dashboard-layout.tsx:69-196` (`useNavGroups`), `:204-225` (`OVERFLOW_LABELS`), `:232-249` (`OVERFLOW_PATH_PREFIXES`).
- `apps/web/src/features/roster/routes.tsx` (route `contacts`, `contacts/:id`, `students`, `students/import`).
- `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx` (các test "Phụ huynh nav entry", "Nhập từ Excel", "Quản trị học sinh", "Giảng dạy nav group").
- `docs/frontend-guidelines.md:37, 157` mô tả nhóm "Giảng dạy".

## Implementation Steps

1. `useNavGroups`:
   - Đổi header "Giảng dạy" → "Lớp học"; thêm `{ label: "Học sinh", to: "/students", Icon: HvUsersIcon, perm: "students.list" }` làm mục đầu. Không đổi icon các mục khác.
   - Mục "Hồ sơ học sinh" (`/records`) giữ nguyên — là trang khác (hồ sơ học tập), không thuộc phạm vi.
   - Xoá "Phụ huynh" (Dạy học), "Nhập từ Excel" và "Quản trị học sinh" (Trung tâm). Bỏ import `BookUserIcon`, `FileSpreadsheetIcon` nếu không còn dùng.
   - Sau bộ lọc `perm`, lọc tiếp `groups.filter((g) => g.entries.length > 0)` — ẩn nhóm rỗng chung cho mọi nhóm.
2. `OVERFLOW_LABELS`: bỏ "Phụ huynh", "Quản trị học sinh", "Nhập từ Excel"; thêm "Học sinh". `OVERFLOW_PATH_PREFIXES`: giữ `/students`, `/contacts` (route vẫn tồn tại). Cập nhật comment trên `OVERFLOW_LABELS`.
3. `routes.tsx`:
   - Redirect `/contacts` đã làm ở phase 3.
   - `students` title → "Học sinh".
   - `contacts/:id`: giữ route; gate trong trang ở phase 3.
4. Kiểm tra mọi link tới `/contacts` (không có `:id`) trong `apps/web/src` và đổi sang `/students?tab=contacts` khi là link nội bộ.
5. `dashboard-layout.test.tsx`: viết lại các test liên quan:
   - Nhóm "Lớp học" có "Học sinh" đứng đầu, link `/students`.
   - Thành viên có `students.list` thấy "Học sinh"; không có thì không thấy.
   - Không còn "Phụ huynh", "Quản trị học sinh", "Nhập từ Excel" ở sidebar và sheet Thêm.
   - Nhóm có mọi mục bị lọc không render header (dựng member chỉ có một key để kiểm tra).
   - Thay test "keeps only Nhập từ Excel active on /students/import" bằng: trên `/students/import`, "Học sinh" active (parent entry).
6. E2E (`apps/web/e2e/`): cập nhật selector/nhãn nav và luồng mới:
   - `class-staff-read.spec.ts:30-33` (giáo viên giờ mở được `/students`, không thấy nút ghi)
   - `roster.spec.ts:29-53` (tab mới, `/contacts` redirect)
   - `courses.spec.ts:107,133`, `audit.spec.ts:36`, `helpers/ux-routes.ts:42` (nhãn/route "Phụ huynh", "Quản trị học sinh")
   - Chạy trên stack e2e cô lập (`compose -p teka-e2e`), không đụng stack prod.
7. `docs/frontend-guidelines.md`: cập nhật danh sách nhóm sidebar (Giảng dạy → Lớp học) và câu mô tả liên quan.

## Todo

- [x] Nav groups + empty-group filter
- [x] OVERFLOW_LABELS
- [x] Tests layout
- [x] E2E specs
- [x] Docs frontend-guidelines

## Verification

- `cd apps/web && npx vitest run src/layouts/__tests__/dashboard-layout.test.tsx`
- E2E các spec trên (stack `teka-e2e`)
- `npm run lint && npm run typecheck`

## Risk Assessment

- Dashboard mobile: "Học sinh" nằm trong sheet Thêm; tab Thêm vẫn active trên `/students` nhờ prefix.
- Giáo viên trước đây không có mục nào tới `/students`; giờ có. Phase này phụ thuộc phase 3 (bỏ owner-guard, tab Người liên hệ) — không merge riêng trước phase 3.
