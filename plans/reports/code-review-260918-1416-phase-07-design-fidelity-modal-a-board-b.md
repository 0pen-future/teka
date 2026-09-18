# Code review · Phase 7 — Khớp 100% UI Modal A + Bảng B

- Plan: `plans/260917-2142-task-modal-a-board-b` · phase 07
- Branch: `feat/task-modal-a-board-b` (chưa commit, trên `eed9992`)
- Reviewer: subagent `code-reviewer` (reviewer-phase7) · 2026-09-18
- Kết luận reviewer: DONE_WITH_CONCERNS — gap M1, M2, M4, M5, M6, B1–B11 đã khớp report; 1 lỗi a11y nghiêm trọng và 3 lỗi vừa trong modal.

## Phát hiện và xử lý

| # | Mức | Phát hiện | Xử lý |
|---|-----|-----------|-------|
| 1 | Critical | Panel xác nhận inline (`role=alertdialog`) phủ lên footer nhưng không chặn focus: Tab từ panel rơi vào "Lưu" đang bị che → có thể lưu thay vì xoá | Đặt `inert` lên `<form>` và div footer khi `confirm !== null` (`task-form-modal.tsx`). Test mới: "takes the form and footer out of the tab order while a confirm panel covers them" |
| 2 | High | `validationToast` viết đè mọi lỗi mô tả thành "vượt 2.000 ký tự", kể cả lỗi HTML > 20.000 ký tự ("bỏ bớt định dạng") → toast mâu thuẫn inline | Chỉ remap thông điệp refine 2000 ký tự, các lỗi khác đi thẳng |
| 3 | High | `maxLength={200}` khiến zod `.max(200)` và toast M7 không bao giờ chạy; dán 300 ký tự bị cắt im lặng | Bỏ `maxLength`; `register("title", { onChange })` cắt tại 200 và toast "Tiêu đề tối đa 200 ký tự" như report (dòng 1002). Hằng `TITLE_MAX_LENGTH`/`TITLE_TOO_LONG_MESSAGE` dùng chung ở `task-schemas.ts`. Test mới: "trims a pasted title at 200 characters and says so in a toast" |
| 4 | High | Chip "Bỏ hạn" (`value: ""`) luôn `aria-pressed=true` + tint mint khi chưa có hạn; report `.quick button` chỉ có hover (M3: không pressed) | Bỏ `aria-pressed` và lớp `aria-pressed:*`; chip là nút hành động thuần. Test mới: "renders the chips as plain actions with no pressed state" |
| 5 | Low | Fade hai mép bảng dùng `from-cream-50` trong khi nền shell là `cream-100` → dải sáng mờ | Đổi sang `from-cream-100` (`board-desktop.tsx`) |
| 6 | Low | `quick-done-checkbox.tsx` export `QuickDoneButton` | `git mv` → `quick-done-button.tsx`, cập nhật import |
| 7 | Low | Rail thu gọn render chevron → count → tên; B10 ghi chevron → tên → count | Đổi thứ tự trong `collapsed-column-rail.tsx` |
| 8 | Info | Tương phản chữ trắng trên avatar mint-500/sky-400/sun-500/ink-400 ≈ 2.2–2.9:1 | Giữ nguyên theo spec report (dòng 122-123); tên đầy đủ có trong `aria-label`/`title`. Ghi nhận là quyết định thiết kế cần user xác nhận |

## Reviewer đã kiểm và không có vấn đề

- Guard `event.target !== event.currentTarget` trong `onKeyDown` của card không phá điều hướng bàn phím (phím mũi tên/Home/End/`[` `]` nằm ở handler cột trong `lib/kanban`).
- `CardControlBarrier` với `display: contents` vẫn chặn kéo (board dùng MouseSensor + TouchSensor).
- Bỏ `overflow-y-auto` ở list cột khớp `.col-list` của report, hết cắt nút Xong nhanh.
- `shadow-xs`/`shadow-md`, `var(--color-mint-400)` resolve đúng token DS.
- Không còn tham chiếu `QuickDoneCheckbox`, `isAssignedToMe`, `currentUserId` ở board, `COLUMN_DOT` ở cột/rail, "Xoá lọc". Docs không cần cập nhật.

## Kiểm chứng sau sửa

- `npx vitest run src/features/tasks src/lib/kanban`: 17 file, 195 test pass.
- `npm run typecheck`: xanh. `make lint-web`: 0 lỗi (6 warning react-compiler có sẵn).
- `make e2e-isolated E2E_ARGS="tasks-board"`: xem Validation Log của plan.
