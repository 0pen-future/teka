# Báo cáo Phase 4 — Owner Dashboard API (TDD)

Ngày: 2026-08-12 · Nhánh: master · Trạng thái: hoàn thành, đã qua review + fix

## Kết quả

5 endpoint GET dưới `/api/v1/centers/dashboard/*`, chỉ owner truy cập, không endpoint nào ghi DB:

| Endpoint | Nội dung |
|---|---|
| `GET /teachers` | Roster + số lớp active, số học sinh active per teacher |
| `GET /overview?month=YYYY-MM` | KPI per lớp gộp theo teacher: buổi đã diễn ra, sĩ số TB, tỉ lệ có mặt, retention, doanh thu ước tính + đã lên hoá đơn |
| `GET /teachers/:id/classes?status=` | Drill-down lớp của một teacher (kể cả teacher đã rời center) |
| `GET /teachers/:id/classes/:id/sessions?from&to` | Buổi trong khoảng ngày + thống kê điểm danh, qua `ListRangeReadOnly` — không generate row |
| `GET /sessions/:id` | Chi tiết buổi: bảng điểm danh đầy đủ + hai số doanh thu |

Thành phần chính: `centers/dashboard.go` (service riêng, consumer interface hẹp `ClassReader`/`SessionReader`/`AttendanceReader`), 7 method repository SQL viết tay (mỗi bảng JOIN đều lọc `center_id` + `deleted_at IS NULL`), `sessions.ListRangeReadOnly` mới (không đổi chữ ký `ListRange` cũ), handler + routes + swagger.

## TDD & kiểm thử

- Red trước (chỉ lỗi `undefined: centers.Dashboard`), sau đó implement → green; mọi số liệu khớp tính tay trên fixture có bẫy: row soft-deleted từng bảng, dữ liệu center khác, invoice void, adjustment không nguồn.
- Tester: DONE, coverage >95%; 3/4 gap đã vá (drill-down trên member còn sống, chi tiết buổi của teacher đã bị remove, assert COUNT no-write); gap 4 (HTTP e2e) được reviewer nâng thành finding và đã bổ sung (bên dưới).
- Toàn bộ suite tích hợp (17 package + migrations + seeds) xanh; gofmt/build/vet sạch.

## Review & xử lý

Reviewer trả **request-changes**; toàn bộ finding đã xử lý:

1. **ORDER BY thiếu `c.teacher_id`** (Medium): hai teacher trùng tên (hoặc nhiều teacher đã xoá → tên rỗng) làm Overview tách một teacher thành nhiều nhóm. → Thêm `c.teacher_id` vào ORDER BY.
2. **`SessionInvoiced` đếm trùng** (Medium): khi sửa điểm danh sau chốt sổ, reconciler ghi adjustment gắn `source_session_id` VÀ record live mới cũng được cộng → trùng. Bằng chứng quyết định: chỉ reconciler gán `SourceSessionID` (`billing/adjustment.go`), adjustment thủ công không bao giờ có — nghĩa là mọi adjustment có nguồn buổi đều là delta mà hiệu ứng đã nằm trong record live. → **Bỏ hẳn vế adjustment ở tầng buổi**: `invoiced_revenue` buổi = tổng `unit_price` các record billable live có enrollment nằm trên line không-void của kỳ đã chốt. Sổ tầng lớp (`InvoicedByClass`) giữ nguyên lines + adjustments. Test pin lại: adjustment nguồn buổi không được cộng lần hai.
3. **Thiếu test tầng HTTP** (Medium): thêm `TestDashboardRoutesEndToEnd` — 401 không token, owner 200, member 403 trên cả 5 route (kể cả khi query sai), uuid rác → 403 đồng nhất, owner với param sai → 422. Đồng thời đảo **authz trước validation** trong `teacherClasses`/`classSessions` để member không học được gì về shape param.
4. **Low đã sửa**: `ErrNotFound` từ `SessionInvoiced` map về 403 đồng nhất thay vì 500.
5. **Low ghi nhận, không sửa** (có lý do):
   - Whitelist sort trùng với `classes.listSorts` (unexported): giữ trùng lặp 3 dòng thay vì export nội bộ của classes — YAGNI.
   - Tháng mặc định dùng giờ server (UTC) thay vì múi giờ VN: theo pattern hiện có toàn repo; client luôn có thể truyền `month` tường minh.
   - Record live trên enrollment đã soft-delete tính vào sĩ số nhưng không vào doanh thu: đúng ngữ nghĩa hiện tại của attendance.
   - Khoảng `from/to` của sessions không giới hạn độ dài: chấp nhận cho V1, dashboard là API nội bộ owner.

## Ngữ nghĩa hai số doanh thu (chốt trong phase)

- `estimated_revenue`: có ngay sau điểm danh xác nhận — con số vận hành.
- `invoiced_revenue`: null cho tới khi kỳ của teacher đó chốt; tầng lớp = lines + adjustments có nguồn buổi (sổ sách); tầng buổi = phần line-backed theo trạng thái record hiện tại (đã bao gồm hiệu ứng của mọi delta sau chốt, không đếm trùng). Trường hợp hiếm enrollment chưa từng có line nhưng có delta sau chốt sẽ thiếu phần delta ở tầng buổi — chấp nhận, tầng lớp vẫn đúng.

## File thay đổi

- `apps/api/internal/features/centers/{dashboard.go (mới), repository.go, handler.go, routes.go, dto.go, integration_test.go}`
- `apps/api/internal/features/sessions/{service.go, integration_test.go}` — `ListRangeReadOnly` + test no-insert
- `apps/api/internal/server/router.go` — mount dashboard sau classes/sessions/attendance
- `apps/api/docs/*` — swagger 5 route mới

## Báo cáo liên quan

- Tester: `plans/reports/tester-phase4-260812-0704-dashboard-api-validation.md`
