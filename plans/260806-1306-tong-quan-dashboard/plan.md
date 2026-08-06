# "Tổng quan" page theo prototype Sổ Lớp

Status: done · Branch: master · Source: claude.ai/design project `4a7e6c77` (`So Lop - Prototype.dc.html`, screen `data-screen-label="Tổng quan"`)

## Contract

- **Outcome:** Trang `/` (DashboardPage) khớp visual + hành vi màn "Tổng quan" của prototype, dùng token design system sẵn có (`src/styles/tokens/`) và HV components: (1) greeting h1 Baloo 28px + ngày; (2) banner cảnh báo buổi chưa điểm danh (1 banner gộp, 1 nút "Điểm danh ngay" → buổi pending đầu tiên); (3) hàng 4 stat cards (Học sinh / Điểm danh tháng / Phải thu kỳ / Đã thu); (4) lưới "Lớp của bạn" — card mỗi lớp với pill trạng thái, lịch · đơn giá, sĩ số, tiến độ điểm danh, click mở màn điểm danh của lớp đó.
- **Constraints:** Không sửa backend — mọi số liệu gộp từ endpoint sẵn có (`/students`, `/classes`, `/classes/:id/sessions`, `/sessions/pending`, `/billing-periods` + `/preview`, `/collections/summary`). Giữ pattern feature-module hiện có; cross-feature import chỉ qua barrel `index.ts`. Giữ error-state phân biệt được lỗi tải và không có dữ liệu.
- **Non-goals:** Ô "Luồng demo" (chỉ là hướng dẫn demo của prototype); endpoint aggregation backend; đổi sidebar (đã khớp từ trước); logic search-param write.
- **Acceptance:**
  1. Header: `Chào buổi <sáng|trưa|chiều|tối>, {tên}!` (Baloo 28px/800/ink-900) + ngày `Thứ X, dd/MM/yyyy` (14px ink-400) trên cùng hàng baseline.
  2. Banner: nền coral-100 bo 20px, đĩa "!" 40px coral-400, title "Có N buổi đã dạy nhưng chưa điểm danh", text liệt kê `lớp — buổi` nối " · " + "Chưa điểm danh là chưa tính được tiền.", 1 nút danger → `/sessions/{first}/attendance`. 0 buổi pending → không render gì (theo prototype); lỗi tải → dòng lỗi coral.
  3. Stats: 4 card trắng bo 20px shadow-md; label 12.5px/800/ink-400, value Baloo 26px/800/ink-900, sub 12.5px ink-500. Số liệu: tổng học sinh + "N lớp đang chạy"; % buổi đã xác nhận trong kỳ (held / non-cancelled) + "x/y buổi đã xác nhận"; tổng phải thu kỳ hiện tại (GET preview — pure read, không draft) + trạng thái chốt sổ; đã thu (summary khi đã chốt, "—" + "Chốt sổ để bắt đầu thu" khi chưa).
  4. Class cards: lưới auto-fill minmax(280px,1fr); card bo 24px shadow-md hover shadow-lg; tên Baloo 18px; pill "Đủ điểm danh"(mint) / "Thiếu N"(coral) / "Lớp mới"(sky); dòng `lịch — giờ · giá/buổi`; `N học sinh`, `x/y buổi đã điểm danh`; progress bar 10px mint/coral; card là link → `/sessions?class_id={id}` (lớp chưa có buổi → `/students?class_id={id}`).
  5. `/sessions` đọc `?class_id=` làm lựa chọn lớp ban đầu (read-only, chỉ nhận id có trong danh sách lớp active).
  6. `npm run test`, lint, typecheck xanh; test dashboard cập nhật theo markup mới.

## Files

- `features/dashboard/api/dashboard-api.ts` — thêm `getPeriodPreview` (GET `/billing-periods/:id/preview`, dùng `reviewSchema` từ billing barrel).
- `features/dashboard/hooks/use-dashboard.ts` — thêm hooks: students total, per-class student counts (useQueries), per-class sessions của kỳ (useQueries), preview totals.
- `features/dashboard/components/pending-attendance-alert.tsx` — restyle thành banner gộp theo prototype.
- `features/dashboard/components/dashboard-stats.tsx` — mới: hàng 4 stat cards.
- `features/dashboard/components/class-overview-cards.tsx` — mới: lưới card lớp.
- `features/dashboard/pages/dashboard-page.tsx` — bố cục mới; bỏ `PeriodStatusCard` (màn Tổng quan prototype không có — trạng thái kỳ đã nằm ở sidebar + sub của stat "Phải thu"); xoá `period-status-card.tsx` nếu không nơi khác dùng.
- `features/attendance/index.ts` — export thêm `listClassSessions`; `features/roster/index.ts` — export thêm `listStudents`.
- `features/attendance/pages/sessions-page.tsx` — đọc `?class_id=`.
- `features/dashboard/__tests__/dashboard-page.test.tsx` + `test/msw/handlers.ts` — cập nhật test, thêm factory/handler mặc định cho `/classes`, `/students`, `/classes/:id/sessions`, preview, summary.

## Verification

- `tsc -b --noEmit` sạch; eslint 0 errors (3 warnings pre-existing ở roster, không thuộc thay đổi này); `prettier --check` sạch; vitest 26 files / 121 tests pass (thêm 5 test mới: 2 error-state dashboard, 3 test `?class_id=` cho sessions-page).
- Code review (`code-reviewer` subagent): DONE_WITH_CONCERNS → đã fix cả 3 blocking: (1) prettier fail 2 file; (2) mẫu số điểm danh đếm cả buổi tương lai — giờ fetch range `[period_start, min(today, period_end)]` nên "Thiếu N"/% chỉ tính buổi đã dạy, đồng thời giảm số session row server phải generate mỗi lần vào dashboard; (3) khôi phục error-state cho 4 stat card ("Không tải được" coral) và class card ("Không tải được buổi học") kèm test. Fix thêm: banner giới hạn 3 buổi + "… và N buổi khác"; `scheduleLabel` lọc `activeSchedules` (bỏ schedule đã đóng `effective_to`); đếm lớp dùng `meta.total` thay `items.length`; sửa comment "oldest" (server trả newest-first).
- Giữ nguyên có chủ đích (đối chiếu prototype/precedent): lớp toàn buổi hủy vẫn hiện "Lớp mới" (prototype branch `!ss.length` sau khi lọc cancelled); tổng HỌC SINH gồm cả học sinh chưa ghi danh (prototype đếm `S.students.length`); MSW default handlers thêm vào danh sách global (theo tiền lệ `/sessions/pending`, `/billing-periods` đã global sẵn); `?class_id=` chỉ đọc, không write (theo quyết định bỏ search-param writes trước đó); dashboard sở hữu query key preview riêng — billing mutations không invalidate nó, chấp nhận vì stale tối đa 30s.
- Đổi contract text người dùng thấy: greeting mới "Chào buổi X, {tên}!" và banner "Có N buổi đã dạy nhưng chưa điểm danh" — đã cập nhật 6 e2e specs tương ứng (chưa chạy e2e — cần dev stack).
- Bỏ `PeriodStatusCard` (màn Tổng quan prototype không có): CTA "Chốt sổ" giờ chỉ còn ở sidebar; trạng thái kỳ nằm ở sub của stat "PHẢI THU". Nếu muốn giữ CTA trên trang chủ, cần quyết định lại với user.
