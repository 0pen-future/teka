---
phase: 3
title: "Web — hub Kho học liệu (3 tab v5) + nhóm sidebar"
status: completed
priority: P1
effort: "2d"
dependencies: [1, 2]
---

# Phase 3: Web — hub Kho học liệu + nhóm sidebar

## Context Links
- [plan.md](./plan.md) · D3, D4, D5, D12, **D13** · v5 màn `lib`.
- Sidebar: `apps/web/src/layouts/dashboard-layout.tsx` (`useNavGroups` :69, nhóm "Giảng dạy" :92-111, `OVERFLOW_LABELS`/
  `OVERFLOW_PATH_PREFIXES` :196-230, `useNavActive` :258), test `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx`
  (`describe("Giảng dạy nav group")` :544 là khuôn), tiền lệ thêm nhóm: `plans/260923-0715-giang-day-menu/phase-01-nav-danh-sach-chi-tiet-lop.md`.
- Routes: `apps/web/src/features/library/routes.tsx` (`library`, `library/templates/new`, `library/templates/:id`, `prep…`).
- Hiện trạng: `apps/web/src/features/library/pages/library-page.tsx` (tabs `templates|lessons|materials|exercises`,
  `HvSegmented variant="tabs"`, `?tab=`), `components/items-tabs.tsx` (bảng Học liệu/Bài tập + `MaterialDialog`/`ExerciseDialog`).
- Design system: `apps/web/src/components/hv/` (`HvCard`, `HvChip`, `HvBadge`, `StatusPill`, `HvButton`, `HvSegmented`, `HvToast`, `HvNotice`).

## Goal
Hub `/library` hiển thị đúng v5: subtitle ba cấp, 3 tab, thẻ chương trình mẫu có đếm + chip phiên bản, hai bảng ngân
hàng có LOẠI/ĐỊNH DẠNG/DÙNG TRONG/TRẠNG THÁI và nút ngừng/kích hoạt. Sidebar có nhóm riêng **KHO HỌC LIỆU**
(D13) mà mỗi mục trỏ thẳng vào một tab hub hoặc trang Chuẩn bị tài liệu.

## UI theo v5 (`lib`)
- Header: "Kho học liệu" + subtitle *"Ba cấp: ngân hàng nội dung, ngân hàng bài tập và chương trình mẫu. Chương trình mẫu
  chỉ tham chiếu tới hai ngân hàng — không giữ bản sao."*
- Tabs (`HvSegmented`): **Chương trình mẫu** · **Ngân hàng nội dung** · **Ngân hàng bài tập** — là **route con**
  `/library` · `/library/materials` · `/library/exercises` (D13). `LibraryPage` nhận prop `tab: "templates"|"materials"|"exercises"`,
  chọn tab = `navigate()` sang route tương ứng (giữ `HvSegmented variant="tabs"` cho khớp design và e2e). Tại `/library`,
  nếu còn `?tab=materials|exercises` → `<Navigate to="/library/<tab>" replace />`; `?tab=lessons` hoặc giá trị lạ → templates
  (bỏ query). Xoá `tabSchema`/`useSearchParams` khỏi page sau khi redirect xong.
- **Tab Chương trình mẫu**: ô tìm (giữ role `searchbox` tên "Tìm chương trình mẫu" cho e2e), nút "+ Chương trình mẫu"
  (giữ wizard `/library/templates/new`). Lưới `HvCard` (2 cột ≥ md): tên (link tới chi tiết), `StatusPill`
  (`templateStatusLabel`), nút "Sửa" (mở `TemplateDialog`), hàng đếm `{lesson_count} buổi mẫu · {version_count} phiên bản ·
  {class_count} lớp đang gắn`, hàng chip phiên bản (`HvChip` theo `versions[]`: "v{n}" + variant theo status, click →
  `/library/templates/:id?v=:vid`). Empty state `HvStateBlock`.
- **Tab Ngân hàng nội dung**: bảng LOẠI (icon + `materialKindLabel`) · TIÊU ĐỀ (title, mô tả mờ) · ĐỊNH DẠNG
  (`materialFormatLabel(url)`) · DÙNG TRONG (`{lesson_count} buổi · {template_count} CT mẫu`, "—" khi 0) · TRẠNG THÁI
  (`StatusPill` Hoạt động/Ngừng) · thao tác **Sửa** + toggle "Ngừng"/"Kích hoạt" (`useSetMaterialStatus`, không confirm —
  reversible). Ghi chú dưới bảng: *"Không xóa cứng nội dung còn được phiên bản đang hoạt động tham chiếu — chỉ ngừng hoạt
  động. Cờ 'chia sẻ cho học viên' nằm ở từng buổi, không nằm ở file."* Nút "Xoá" hiện có giữ, disabled khi `lesson_count > 0`
  (tooltip lý do) để khớp 409.
- **Tab Ngân hàng bài tập**: bảng MÃ (mono + nút copy → `hvToast("Đã sao chép mã")`) · BÀI TẬP (title, tags) · KỸ NĂNG ·
  CẤP ĐỘ · DÙNG TRONG · TRẠNG THÁI · Sửa + toggle. Tìm kiếm theo tên **hoặc mã** (server `q` đã phủ `code`).
- Bộ lọc trạng thái mỗi bảng: `HvSegmented` nhỏ "Tất cả / Hoạt động / Ngừng" → `active` param (default Tất cả).
- `MaterialDialog`: select LOẠI 7 giá trị (bỏ "Khác" khỏi lựa chọn; giữ hiển thị khi item cũ là `other`),
  URL placeholder theo loại (ghi chú `note` cho phép URL trống? — **không**: giữ URL bắt buộc, `note` dùng description).
- `ExerciseDialog`: thêm Mã (placeholder "Tự sinh nếu bỏ trống"), Kỹ năng, Cấp độ; giữ Độ khó 1–5 và tags.

## Nhóm sidebar KHO HỌC LIỆU (D13)
- `dashboard-layout.tsx` · `useNavGroups`: bỏ 2 entry "Kho học liệu" và "Chuẩn bị tài liệu" khỏi nhóm "Giảng dạy"; chèn
  ngay sau nhóm đó:
  ```ts
  {
    header: "Kho học liệu", // CSS uppercase → "KHO HỌC LIỆU"
    entries: [
      { label: "Chương trình mẫu", to: "/library", Icon: LibraryBigIcon, perm: "library.read" },
      { label: "Ngân hàng nội dung", to: "/library/materials", Icon: BookOpenIcon, perm: "library.read" },
      { label: "Ngân hàng bài tập", to: "/library/exercises", Icon: ClipboardCheckIcon, perm: "library.read" },
      { label: "Chuẩn bị tài liệu", to: "/prep", Icon: ClipboardListIcon, perm: "library.read" },
    ],
  },
  ```
  `BookOpenIcon`/`ClipboardCheckIcon` đã import từ `lucide-react` trong file; đổi icon khác của lucide nếu trùng mục khác
  quá gần — không thêm lib.
- `OVERFLOW_LABELS`: thay "Kho học liệu" bằng 3 nhãn mới, giữ "Chuẩn bị tài liệu"; `OVERFLOW_PATH_PREFIXES` đã có `/library`,
  `/prep` → không đổi (prefix phủ cả route con). Cập nhật doc comment đầu file mô tả các nhóm.
- Active-state: không thêm logic. `useNavActive` chọn tiền tố dài nhất trong `NavPathsContext`, nên `/library/materials` chỉ
  sáng "Ngân hàng nội dung"; `/library/templates/:id` và `/library/templates/new` sáng "Chương trình mẫu".
- `features/library/routes.tsx`: thêm `library/materials` và `library/exercises` (lazy cùng `LibraryPage`, truyền `tab`).
- Test `dashboard-layout.test.tsx`: `describe("Kho học liệu nav group")` theo khuôn "Giảng dạy": nhóm `role="group"`
  tên "Kho học liệu" đứng ngay sau "Giảng dạy"; 4 link đúng href; ẩn cả 4 khi `/centers/me` thiếu `library.read`;
  "Giảng dạy" không còn 2 link cũ; sheet "Thêm" liệt kê 4 mục; active đúng mục tại `/library/materials` và
  `/library/templates/1`. Sửa kỳ vọng ở test "Giảng dạy" nếu đang đếm số link.
- e2e: `library.spec.ts:36,171,244` đổi `goto("/library?tab=…")` → `/library/<tab>`; comment `library.spec.ts:84`
  "nav entry lives in the Giảng dạy group" → "Kho học liệu group"; `courses.spec.ts:50` comment "next to Kho học liệu" sửa lại.
  Thêm 1 bước e2e: click "Ngân hàng bài tập" ở sidebar → URL `/library/exercises`, tab active.

## Components
- Modify `apps/web/src/layouts/dashboard-layout.tsx`, `apps/web/src/features/library/routes.tsx` (mục trên).
- Modify `pages/library-page.tsx`: tabs, header, render `TemplateCards`.
- Create `components/template-cards.tsx` (`TemplateCards` + `TemplateCard`), thay bảng cũ; giữ `useTemplatesList` + `useSearch`.
- Modify `components/items-tabs.tsx`: tách thành `components/materials-bank.tsx` và `components/exercises-bank.tsx`
  (mỗi bảng ~150 dòng, đủ lý do tách); hook `useSearch` đang là hàm cục bộ ở `items-tabs.tsx:29` → chuyển ra
  `hooks/use-search.ts` để hai bảng và Phase 5 dùng chung; `items-tabs.tsx` xoá.
- Modify `components/material-dialog.tsx`, `components/exercise-dialog.tsx`: field mới.
- Create `components/copy-code-button.tsx` (nhỏ, dùng lại ở Phase 4/5 tab Bài tập).

## Tests (`__tests__/`)
- `library-page.test.tsx`: 3 tab theo route con (`renderPage("/library/materials")` thay `?tab=materials`); `?tab=materials` →
  redirect `/library/materials`; `?tab=lessons` → `/library`; card hiển thị đếm + chip; toggle Ngừng gọi PATCH và pill đổi;
  copy mã gọi `navigator.clipboard.writeText` (mock) + toast; filter Ngừng gọi `?active=false`.
- Cập nhật fixture trong `library-handlers.ts` nếu Phase 2 chưa đủ.

## Files
- Create: `components/template-cards.tsx`, `components/materials-bank.tsx`, `components/exercises-bank.tsx`, `components/copy-code-button.tsx`, `hooks/use-search.ts`.
- Modify: `pages/library-page.tsx`, `routes.tsx`, `components/material-dialog.tsx`, `components/exercise-dialog.tsx`,
  `__tests__/library-page.test.tsx`, `apps/web/src/layouts/dashboard-layout.tsx`,
  `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx`, `apps/web/e2e/library.spec.ts` (goto + comment),
  `apps/web/e2e/courses.spec.ts:50` (comment).
- Delete: `components/items-tabs.tsx`.

## Verification
```bash
cd apps/web && npx vitest run src/features/library/__tests__/library-page.test.tsx src/layouts/__tests__/dashboard-layout.test.tsx && npx tsc --noEmit
make lint-web
```

## Success Criteria
- [x] Sidebar có nhóm "Kho học liệu" sau "Giảng dạy" với 4 mục D13; "Giảng dạy" còn 4 mục; ẩn khi thiếu `library.read`; sheet "Thêm" đủ mục.
- [x] Hub có đúng 3 tab tại 3 route con; `?tab=lessons|materials|exercises` cũ redirect, không vỡ.
- [x] Tại `/library/materials` chỉ mục "Ngân hàng nội dung" active; tại `/library/templates/:id` mục "Chương trình mẫu" active.
- [x] Thẻ chương trình mẫu hiện `{n} buổi mẫu · {n} phiên bản · {n} lớp đang gắn` và chip phiên bản điều hướng đúng.
- [x] Ngừng/Kích hoạt đổi pill ngay (optimistic hoặc invalidate) và item ngừng biến mất khỏi picker ở Phase 5.
- [x] Copy mã bài tập hoạt động, tìm kiếm theo mã ra kết quả.
- [x] Selector e2e hiện có ("Tìm chương trình mẫu", "Xoá chương trình") vẫn tồn tại.
