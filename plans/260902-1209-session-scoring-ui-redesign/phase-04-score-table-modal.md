---
phase: 4
title: "Full score table modal (xl)"
status: completed
priority: P1
effort: "1d"
dependencies: [3]
---

# Phase 4: Full score table modal (xl)

## Overview

Thêm nút "Mở bảng đầy đủ" ở tab Điểm buổi, mở `HvModal size="xl"` chứa bảng
học sinh × cột (report §4.2): cột tên dính trái 180px, tiêu đề dính trên với
`line-clamp-2`, cột TB tính tại chỗ, Enter xuống / Shift+Enter lên / Tab phải,
hàng vắng gộp một dòng cuối. Dùng chung `useScoreDraft` và thanh trạng thái
của Phase 3, nên nháp trong panel và trong bảng là một.

## Requirements

- Functional:
  - Bảng `<table>` thường (D9), `border-separate`; `th` cột tên `sticky left-0 top-0 z-20`, `th` cột điểm `sticky top-0 z-10`, `td` tên `sticky left-0 z-10`, mọi ô dính có nền đục.
  - Cột điểm rộng 72px; tiêu đề `line-clamp-2`; `th` **luôn** có `aria-label` = tên cột đầy đủ (không đo cắt ở runtime).
  - Cột "TB" `<th scope="col">` cuối, giá trị `avg` từ `useScoreDraft` (tính trên `raw` hợp lệ, một chữ số thập phân, "—" khi chưa có).
  - Điều hướng: Enter → cùng cột hàng dưới (bỏ qua hàng vắng), Shift+Enter → hàng trên, Tab → trình duyệt. Refs matrix theo `cellKey`.
  - Hàng vắng: một `<tr>` cuối với `<td colspan={components.length + 2}>Vắng (n): tên, tên…</td>`.
  - Footer modal: cùng thanh trạng thái "k/N học sinh đã chấm · m ô chưa lưu" + `HvButton` "Lưu điểm" + `HvButton secondary` "Đóng". Đóng khi dirty → `UnsavedScoresGuard`.
  - Modal mở từ panel bằng `HvButton variant="secondary" size="sm" icon={<HvIcon name="table" />}` "Mở bảng đầy đủ"; đóng modal quay về panel với nháp còn nguyên (state ở panel, không ở modal).
- Non-functional: tối đa 10 cột: 180 + 10×72 + 72 = 972px < ~1032px nội dung của modal 1080px (trừ padding 24px hai bên) → không cuộn ngang ở ≥1280px; hàng bọc `React.memo` với props nguyên thuỷ để gõ một ô không re-render 200 input; cuộn dọc bên trong body modal; dưới `sm` modal là bottom sheet 95dvh và **cho phép** cuộn ngang (bảng là fallback mobile, không phải đường chính).

## Architecture

```
features/teaching/components/
  score-table-modal.tsx     HvModal size=xl + <table>; props: open, onOpenChange, draft (từ useScoreDraft), components, rosterRows, sessionLabel
  score-entry-by-student.tsx  thêm nút mở; giữ `useScoreDraft` là chủ state, truyền `draft` xuống modal
  score-entry-footer.tsx    dùng lại từ P3
```

`useScoreDraft` không đổi API. `ScoreTableModal` chỉ đọc `draft.cells`, gọi
`draft.setRaw/commit/flush`. Refs matrix: `useRef(new Map<string, HTMLInputElement>())`;
`onNavigate(key, dir)` → tìm `studentIdx` trong `editableStudents`, ±1, cùng `componentId`.

## Related Code Files

| Action | File | Ghi chú |
|--------|------|---------|
| Create | `apps/web/src/features/teaching/components/score-table-modal.tsx` | |
| Modify | `apps/web/src/features/teaching/components/score-entry-by-student.tsx` | nút "Mở bảng đầy đủ" + state `tableOpen` |
| Create | `apps/web/src/features/teaching/__tests__/score-table-modal.test.tsx` | dùng fixture của `score-entry-by-student.test.tsx` |

## Implementation Steps

1. `score-table-modal.tsx`: dựng bảng với class sticky theo công thức ở research (corner z-20, header z-10, cột tên z-10, nền `bg-white`); ô dùng `HvScoreInput size="sm"` với `inputRef` vào refs matrix.
2. Điều hướng Enter/Shift+Enter; Tab không can thiệp.
3. Cột TB và hàng vắng.
4. Footer + guard; `onOpenChange(false)` khi dirty → guard với "Lưu và đóng"/"Bỏ thay đổi"/"Tiếp tục chấm" — dùng `HvConfirmDialog` hai nút + huỷ bằng Escape.
5. Nút mở trong panel; test; lint/typecheck.
6. Kiểm tra tay ở 1280px/1080px/390px; kiểm tra tiêu đề dính khi cuộn 20 hàng.

## Test scenarios

| Case | Assertion |
|------|-----------|
| Bấm "Mở bảng đầy đủ" | `role="dialog"` có `columnheader` cho từng cột + "TB"; hàng học sinh present/late có input; hàng "Vắng (1)" cuối |
| Gõ 7 và 8 cho hai cột của An | ô TB của An hiển thị "7,5" |
| Enter ở ô (An, 15 phút) | focus ô (Bình, 15 phút), bỏ qua học sinh vắng |
| Shift+Enter ở hàng đầu | focus không đổi |
| Gõ ở bảng, đóng modal bằng "Đóng" khi dirty | không có guard: modal đóng, panel hiển thị ô `dirty` và "1 ô chưa lưu"; không PUT (nháp dùng chung, autosave vẫn chạy) |
| Gõ ở bảng, bấm "Lưu điểm" rồi Esc | PUT một lần; panel hiển thị `saved` |
| Tiêu đề cột dài 50 ký tự | `th` có `aria-label` = tên đầy đủ, span bên trong có class `line-clamp-2` |
| Sticky | `th` đầu có class `sticky left-0 top-0`; `td` tên có `sticky left-0` |

## Success Criteria

- [x] Bảng đầy đủ và panel dùng chung một nháp: gõ trong bảng, đóng, panel thấy ô dirty (test "closing keeps the unsaved cell visible there").
- [x] Không cuộn ngang ở ≥1280px với **10 cột** (kiểm tra tay, ghi ảnh vào PR). — *`score-table-1280.png`, `score-table-1080.png` trong `plans/reports/screenshots-260902-scoring-ui/`: 12 columnheader (tên + 10 cột + TB), body và modal không cuộn ngang; header `position: sticky; top: 0`, cột tên `sticky; left: 0`, tiêu đề cột `line-clamp: 2` ("Thuyết trình" xuống 2 dòng). Ở 390px (`score-table-390.png`, `-scrolled.png`) bảng 972px cuộn ngang **trong thân modal**, cột tên vẫn dính trái, body 390/390.*
- [x] Enter/Shift+Enter/Tab đúng; test xanh; lint/typecheck xanh.

## Risk Assessment

- **Radix Dialog focus trap và Enter**: Enter trong input không submit vì không có `<form>`; đảm bảo không bọc bảng trong `<form>`.
- **Sticky cell z-index và nền**: dùng `bg-white` trên mọi ô dính; ô dirty giữ sun-100 chỉ ở input, không ở `td`.
- **Nhiều `HvScoreInput` (200) trong modal**: không virtualize, nhưng `useScoreDraft` trả `cells` là Map mới mỗi keystroke nên hàng phải `React.memo` và chỉ nhận raw/state của các ô trong hàng; đo bằng React Profiler khi kiểm tra tay.
