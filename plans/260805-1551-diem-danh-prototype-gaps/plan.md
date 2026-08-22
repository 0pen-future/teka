# Màn Điểm danh — khớp 100% prototype (vòng gap-closing)

Status: in-progress · Branch: master · Source: claude.ai/design project `4a7e6c77` (`So Lop - Prototype.dc.html`, attend screen dòng 103–179, view-model dòng 782–815)

## Contract

- **Outcome:** Màn Điểm danh khớp 100% prototype theo token DS. Sau 2 plan trước (260805-1435 restyle, 260805-1516 class-picker) còn 3 gap: (1) thiếu label "CHỌN LỚP" trên hàng chọn lớp, (2) subtitle 13.5px thay vì 14px, (3) nút xác nhận không phản ánh trạng thái đã xác nhận (`ĐÃ XÁC NHẬN ✓`).
- **Constraints:** Giữ nguyên chức năng hiện có (date-range, nhóm ngày, "Cần điểm danh", huỷ buổi, dirty-guard, closed-period adjust); token DS 100%; không sửa backend; test hiện có không bị yếu đi.
- **Non-goals:** Đổi font tab pill sang Nunito (DS token `--type-label-font: var(--font-display)` → Baloo cho label/button là đúng DS, prototype inline-style chỉ là fallback); band "BUỔI HỌC THÁNG 07" đơn (user đã chốt giữ nhóm ngày + date-range ở plan 260805-1435); status "✓ · vắng N" trên hàng buổi (SessionResponse không có breakdown — cần backend); khung 400px cố định + caption mô phỏng điện thoại (demo-only trong prototype).
- **Acceptance:**
  1. Label "CHỌN LỚP" 12px/800/ink-400/tracking .5px đứng trên hàng search+tabs ở màn Điểm danh; thêm cùng label ở màn Lớp & học sinh vì prototype có ở cả 2 (dòng 107 + 189) và section này là pattern chung.
  2. Subtitle màn Điểm danh 14px ink-500 (prototype dòng 106).
  3. Buổi đã xác nhận, chưa sửa gì → nút `ĐÃ XÁC NHẬN ✓`, bấm chỉ toast "Buổi này đã xác nhận rồi" (không ghi mạng — prototype dòng 814–815); chạm sửa một học sinh → nút trở lại `XÁC NHẬN BUỔI HỌC · N vắng` (hoặc `LƯU VÀ TẠO ĐIỀU CHỈNH · N vắng` khi kỳ đã chốt) và lưu bình thường.
  4. `tsc`, eslint, vitest xanh; public contract không đổi (chỉ thêm props cho `ConfirmAttendanceBar` — component nội bộ feature).

## Files

- `apps/web/src/features/attendance/pages/sessions-page.tsx` — label CHỌN LỚP + subtitle 14px.
- `apps/web/src/features/roster/pages/students-page.tsx` — label CHỌN LỚP (đồng nhất section).
- `apps/web/src/features/attendance/components/confirm-attendance-bar.tsx` — props `confirmed`/`dirty`, label trạng thái.
- `apps/web/src/features/attendance/pages/attendance-page.tsx` — truyền props, early-return toast khi đã xác nhận và không dirty.
- `apps/web/src/features/attendance/__tests__/attendance-page.test.tsx` — test mới cho vòng đời label nút xác nhận.

## Verification

- (điền sau khi chạy) tsc / eslint / vitest / code-review.
