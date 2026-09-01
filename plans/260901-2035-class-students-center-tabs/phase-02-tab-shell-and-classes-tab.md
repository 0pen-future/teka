---
phase: 2
title: "Tab shell and classes tab"
status: completed
priority: P1
effort: "5h"
dependencies: [1]
---

# Phase 2: Tab shell and classes tab

## Overview

Dựng khung tab underline (Lớp học | Học sinh | Chưa ghi danh) cho
`StudentsPageContent` theo pattern `permission-matrix.tsx`, state qua URL
`?tab=` với quy tắc suy tab bảo toàn deep-link cũ, và xây tab **Lớp học**
(UI mới). Hai tab còn lại tạm render nội dung hiện có (dọn ở phase 3).

## Requirements

- Functional: quy tắc chọn tab (xem Architecture) bảo toàn mọi link
  `?class_id=` hiện có; tab Lớp học đủ danh sách/tạo lớp/link cài đặt.
- Non-functional: a11y — tablist trang `aria-label="Khu vực"`, mỗi tab
  `role="tab"` + `aria-selected`, focus-visible ring; ở tab "Lớp học"
  **không** phát sinh request `/students`, `/sessions`, `/enrollments`;
  không horizontal scroll mobile.

## Architecture

- **Tab strip**: sao markup từ `permission-matrix.tsx:215-246` (button
  underline 3px mint-400 active / transparent inactive), KHÔNG copy logic
  dirty-dot (trang này không có draft). Khai báo tĩnh:
  ```tsx
  const PAGE_TABS = [
    { id: "classes", label: "Lớp học" },
    { id: "students", label: "Học sinh" },
    { id: "unenrolled", label: "Chưa ghi danh" },
  ] as const;
  type PageTabId = (typeof PAGE_TABS)[number]["id"];
  ```
- **Quy tắc chọn tab** (bảo toàn deep-link cũ — các link đang tồn tại đều
  mang `class_id` không kèm `tab`: `class-overview-cards.tsx:35`,
  `class-settings-page.tsx:105`, e2e `class-staff-write.spec.ts:228`):
  1. `?tab=` hợp lệ → thắng tuyệt đối.
  2. Không có `tab`: `class_id === "none"` → `unenrolled`;
     `class_id` khác rỗng → `students`; còn lại → `classes` (mặc định).
  Giá trị `tab` lạ → fallback quy tắc 2. Không cần đổi link nào ở
  dashboard/class-settings nhờ quy tắc này.
- **Tab state = URL** (khác permission-matrix dùng local state — lý do:
  trang đã round-trip `class_id`/`q` qua URL, không có unsaved drafts):
  đọc `searchParams.get("tab")`, ghi bằng functional updater với
  `{ replace: true }` (nhất quán mọi write hiện có tại `students-page.tsx:88,187`;
  hệ quả chấp nhận: Back không undo đổi tab). Khi rời tab Học sinh không
  xoá `class_id`/`q` (quay lại còn nguyên filter). Helper chung
  `setTab(tab, extraParams?)` dùng cùng một functional updater.
- **Gate query theo tab**: `useStudentsList` không có option `enabled`
  (`hooks/use-students.ts:16-22`) và `effectiveClassId` fallback
  `classes[0].id` (`students-page.tsx:102-104`) ⇒ nếu không gate, tab Lớp
  học vẫn bắn 3 query roster của lớp đầu. Fix: chỉ tính `selectedClassId`
  khi tab active là `students`/`unenrolled`, thêm option `enabled` cho
  `useStudentsList` (mở rộng signature hook, không đổi query key) hoặc
  render lười nhánh students. Test bắt buộc: vào tab Lớp học → zero
  request roster (mẫu `server.events` như phase 1).
- **Tab Lớp học**: dùng `useClassesList({ status: "active", per_page: 100 })`
  sẵn có (React Query dedupe). Data đã xác minh trên `type Class`
  (`roster-schemas.ts:177-190`): `schedules` (`:185`), `default_unit_price`
  (`:183`). Render mỗi row: tên lớp, tóm tắt lịch qua
  `formatScheduleSummary` (`roster/lib/roster-format.ts:41`), đơn giá qua
  `formatMoney` (`@/lib/utils`) — dùng y như `class-overview-cards.tsx:55-57`;
  link `⚙ Cài đặt` → `/classes/${id}/settings`; nút `+ Tạo lớp mới` mở
  `ClassDialog` (di chuyển từ header). KHÔNG hiển thị sĩ số (`Class` không
  có field đếm — tránh N+1). Desktop: bảng trong card
  `rounded-[20px] shadow-soft-md`; mobile (`sm:hidden`): stacked `HvCard` —
  theo cặp pattern đã có trong trang. Empty state "Chưa có lớp nào" + nút
  tạo lớp.
- **Header trang**: giữ `h1` "Lớp & học sinh"; action buttons chuyển vào
  tab tương ứng (hoàn tất ở phase 3); link header `⚙ Cài đặt lớp` bị bỏ
  (đã có per-row trong tab Lớp học).

## Related Code Files

- Modify: `apps/web/src/features/roster/pages/students-page.tsx`
- Create: `apps/web/src/features/roster/components/classes-tab.tsx`
  (UI mới độc lập data-wise; page đã ~530 dòng)
- Modify: `apps/web/src/features/roster/hooks/use-students.ts` (thêm
  `enabled` option nếu chọn hướng gate bằng option)
- Modify: `apps/web/src/features/roster/__tests__/students-page.test.tsx` —
  các test vỡ vì va chạm `role="tab"`: `:202-206`, `:210-215`, `:224-230`
  (`getAllByRole("tab")` sẽ đếm cả 3 tab trang) và test header `:92-120`
  (nút dời vào tab). Mọi truy vấn pill phải scope
  `within(screen.getByRole("tablist", { name: "Lớp" }))`; tab trang truy
  vấn qua `tablist` tên "Khu vực".

## Implementation Steps

1. Thêm tab strip + hàm suy tab theo quy tắc trên + `setTab` helper.
2. Chia render theo `activeTab` (chưa refactor sâu nhánh students —
   phase 3); gate `selectedClassId`/queries theo tab.
3. Viết `classes-tab.tsx`: props nhận `classes`, callback mở ClassDialog.
4. Chuyển nút `+ Tạo lớp mới` từ header vào tab Lớp học; bỏ link header
   `⚙ Cài đặt lớp`.
5. Tests: (a) mặc định (URL trần) mở tab Lớp học, thấy tên lớp + lịch +
   đơn giá + link Cài đặt đúng href; (b) click tab đổi URL (`replace`) và
   panel; (c) `?class_id=X` không kèm tab → tab Học sinh đúng lớp;
   (d) `?class_id=none` → tab Chưa ghi danh; (e) `?tab=classes&class_id=X`
   → tab Lớp học (tab thắng); (f) tab Lớp học zero request roster;
   (g) tạo lớp từ tab hoạt động; (h) viết lại các test va chạm role="tab"
   đã kiểm kê.

## Success Criteria

- [x] 3 tab đúng pattern underline, keyboard-focusable, hai tầng tablist
      phân biệt bằng aria-label.
- [x] Quy tắc suy tab pass đủ 5 case test (a,c,d,e + tab lạ).
- [x] Tab Lớp học zero request roster; đủ nội dung + empty state.
- [x] Test mới + test viết lại xanh, `npm run typecheck` xanh.

## Risk Assessment

- **Trộn hai hệ "tab"** (tab trang vs pill chọn lớp): bắt buộc `aria-label`
  phân biệt ("Khu vực" vs "Lớp") + style khác nhau (underline vs pill);
  test scope bằng `within()` để không phụ thuộc thứ tự DOM.
- **URL state kép** (`tab` + `class_id` + `q`): mọi write qua cùng một
  functional updater (bẫy stale-params đã được comment tại
  `students-page.tsx:67-92` — giữ nguyên bài học đó).
- **Mở rộng signature `useStudentsList`**: chỉ thêm `enabled`, không đổi
  query key — các caller khác không ảnh hưởng.
- **Rollback**: revert commit phase; không state ngoài URL.
