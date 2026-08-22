# Test Cases — Chốt sổ / Gửi thông báo / Thu tiền

Nguồn: phiên rà soát edge case 2026-08-13. Mục tiêu: liệt kê các test case cần
bổ sung, xếp theo độ ưu tiên. Phần cuối liệt kê nhanh các case đã có test để
tránh viết trùng.

## P0 — Phải viết trước (đụng tiền thật)

### TC-01. Double-count nợ cũ khi thu tiền sau kỳ kế tiếp đã chốt

Nghi vấn: nợ cũ tồn tại đồng thời ở phiếu kỳ cũ (outstanding) và phiếu kỳ mới
(`opening_balance`); `candidateInvoicesQuery` (`apps/api/internal/features/payments/repository.go:240`)
lấy cả hai làm ứng viên nhận tiền, chốt kỳ không settle phiếu kỳ trước.

- **Given:** Phiếu T1 nợ 500k chưa trả. Chốt T2 → phiếu T2 = opening 500k +
  phát sinh 1.000k = total_due 1.500k (nợ thật của phụ huynh: 1.500k).
- **When:** Ghi nhận payment 1.500k cho contact.
- **Then (kỳ vọng đúng):** Toàn bộ nợ về 0 — không phiếu nào còn outstanding.
- **Hành vi dự đoán theo code hiện tại (bug):** D8 pass 1 đắp 500k vào opening
  T2, pass 2 đắp 500k vào T1 + 500k vào T2 → phiếu T2 còn "nợ ma" 500k;
  collections board và reminder sai.
- Loại test: integration (`payments/integration_test.go`).
- Biến thể cần cover: trả làm 2 lần (500k rồi 1.000k); trả dư; chuỗi 3 kỳ
  (nợ carry qua 2 lần chốt).

### TC-02. Đổi contact của học sinh giữa hai kỳ → nợ tách đôi theo 2 contact

- **Given:** Học sinh nợ 500k trên phiếu T1 (contact A). Giáo viên đổi contact
  của học sinh sang B. Chốt T2 → `CarriedDebtStudents` lấy contact sống → phiếu
  T2 mang opening 500k dưới contact B; phiếu T1 vẫn outstanding dưới contact A.
- **When:** Contact B trả toàn bộ nợ.
- **Then (kỳ vọng đúng):** Nợ về 0, không còn phiếu mồ côi dưới contact A.
- **Rủi ro hiện tại:** payment của B không bao giờ chạm phiếu T1 (candidate lọc
  theo `contact_id`) → nợ vừa nhân đôi vừa mồ côi.
- Loại test: integration (billing close + payments).

### TC-03. Adjustment âm không có sàn → total_due âm

- **Given:** Phiếu issued total_due 1.000k, paid 0.
- **When:** `AddAdjustment(amount = -2.000k)`.
- **Then (kỳ vọng đúng):** Bị từ chối (400/409) — không cho `total_due` xuống
  dưới `paid_amount` (hoặc dưới 0); phần thừa đưa sang kỳ sau.
- **Hiện tại:** chỉ chặn `amount == 0` và status void/paid; DB CHECK chỉ là
  đẳng thức, không có `total_due >= 0` → phiếu âm, tin nhắn phụ huynh in
  "Tổng cộng: -1.000.000 đ".
- Biến thể: adjustment đẩy `total_due < paid_amount` trên phiếu partially_paid
  → status nhảy sang "paid" với tiền thừa không được mô hình hóa.

## P1 — Nên viết

### TC-04. Crash cứng giữa SendDM thành công và markOutcome → gửi trùng DM

- **Given:** Run zalo_personal đang gửi; process bị kill -9 ngay sau khi
  `SendDM` thành công nhưng trước khi `markOutcome` ghi DB.
- **When:** Boot reconcile → run `interrupted` → giáo viên Resume.
- **Then:** Row vẫn `queued` → gửi lại → phụ huynh nhận 2 tin. At-least-once là
  trade-off cố hữu; test này để **ghi nhận hành vi** (documented behavior) chứ
  không nhất thiết sửa. Tối thiểu: ghi rõ vào docs.

### TC-05. Reminder queue xong mới có payment → text manual bị stale

- **Given:** BulkSend purpose=reminder queue tin manual với Outstanding 500k;
  ngay sau đó phụ huynh trả đủ.
- **When:** Giáo viên copy-paste BulkText và bấm "mark sent".
- **Then:** Xác nhận hành vi mong muốn (chấp nhận stale vì link luôn render số
  sống, hay cần cảnh báo trước khi mark sent). Cần quyết định product trước
  khi viết assert.

### TC-06. Message collapse đo bằng byte thay vì ký tự

- **Given:** `statements.Build` so `maxLen` bằng `len()` (byte); tiếng Việt
  UTF-8 ~1.5–2 byte/ký tự.
- **Then:** Collapse kích hoạt sớm hơn dự kiến. Vô hại về tiền; unit test nhỏ
  chốt lại ngưỡng theo rune nếu đổi.

### TC-07. Link statement sau khi trả đủ trả về 404

- **Given:** Phụ huynh trả đủ; mở lại link cũ.
- **Then hiện tại:** `Outstanding <= 0` → ErrNotFound (đúng thiết kế Q6 PRD).
  Cân nhắc UX "đã thanh toán đủ" thay vì 404 — cần quyết định product.

## Đã có test / đã cover (không viết trùng)

- Chặn chốt khi còn buổi chưa điểm danh (R4), quyết định đọc `Total` không đọc
  list bị cắt trang — `billing/close.go`, `close_test.go`.
- Chốt đồng thời 2 request serialize qua `FOR UPDATE`; phiếu rỗng bị void;
  học sinh nghỉ hẳn còn nợ vẫn sinh phiếu — `billing/integration_test.go:827`.
- Sửa điểm danh sau chốt → reconcile sinh adjustment carry kỳ sau — ADR + tests.
- 2 payment đồng thời cùng contact không overpay, không deadlock —
  `payments/integration_test.go:300,362`.
- Run Zalo: crash → interrupted → resume re-render số sống; row hết gửi được
  fail kèm lý do; phiên chết → sweep expired; breaker 3 fail liên tiếp —
  `notifications/run_*_test.go`.
- Adjustment trên phiếu void/paid → 409; void phiếu đã có tiền → bắt reverse
  trước — `billing/adjustment_test.go`, `close.go`.
- Overpayment → `UnallocatedAmount`, cap tại outstanding —
  `payments/integration_test.go:180`.

## Unresolved questions

1. TC-01 khẳng định theo đọc code, chưa chạy test thực — bước đầu tiên là chạy
   integration test để xác nhận.
2. Có quy trình vận hành ngầm "thu đủ mới chốt kỳ sau" khiến TC-01 không xảy ra
   thực tế không? PRD nói ngược lại ("giữ lại nợ nếu có").
3. TC-05, TC-07: cần quyết định product trước khi chốt expected behavior.
