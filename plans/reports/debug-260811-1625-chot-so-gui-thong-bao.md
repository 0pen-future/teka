# Debug: Chốt sổ hiển thị 1.360.000₫ nhưng không "Gửi thông báo" được cho phụ huynh mới

Teacher: Thược Nguyễn (+84987692969), env: production (teka-api.cauchuyenlaptrinh.com).
Kỳ: 8/2026 (`019fd553-953d-7e88-b10c-ba1a1618f588`), status `closed`, closed_at 2026-08-06T05:33Z.

## Executive summary

Không có lỗi runtime. Đây là lệch dữ liệu do thiết kế: kỳ 8/2026 đã chốt ngày 06/08
(hoá đơn đóng băng: 2 học sinh, 1.000.000₫). Ngày 11/08 giáo viên tạo thêm lớp "Toán 3C",
phụ huynh "Tùng Golang", học sinh "Quang Minh", điểm danh 6 buổi tháng 8 (360.000₫).

- Màn hình Chốt sổ (closed) đọc `GET /billing-periods/:id/preview` → **tính lại live** →
  hiển thị 3 HS / 1.360.000₫.
- "Gửi thông báo" (statements) chỉ target **contact có hoá đơn non-void trong kỳ**
  (`notifications/service.go:79`) → chỉ Hải Yến (1.000.000₫). Không bao giờ tạo được
  thông báo cho Tùng Golang.
- Generate/gửi thực tế vẫn chạy OK: ledger có 12 dòng, run zalo_personal hôm nay
  09:22Z completed (sent=1, failed=0) — đều cho Hải Yến.

## Evidence (production API, 2026-08-11)

- `GET .../preview` totals: `{student_count:3, total_charge:1360000, total_due:1360000}`;
  dòng thứ 3: Quang Minh / Tùng Golang / Toán 3C, 6 buổi × 60.000 = 360.000, `invoice_id: null`.
- `GET .../collections`: chỉ 1 contact (Hải Yến), 2 invoices (Bảo An 300k, Minh Anh 700k),
  total_due 1.000.000, đã thu đủ.
- UUIDv7 timestamps: class `019ff002…`, contact `019ff01d…`, student `019ff01f…`,
  enrollment `019ff020…` → đều tạo ~09:0x–09:2xZ ngày 11/08, **sau** closed_at 06/08.
- `GET .../notifications/run`: `{status:"completed", total:1, sent:1, failed:0}`.

## Root cause

1. **Symptom trực tiếp**: notifications purpose=statements sinh message từ hoá đơn đã chốt;
   Quang Minh không có hoá đơn (enroll + điểm danh sau khi chốt) → không có gì để gửi
   cho Tùng Golang.
2. **Nguồn nhầm lẫn UI**: `apps/web/src/features/billing/hooks/use-billing.ts` — review
   của kỳ closed đọc qua `Preview` (`billing/service.go:114` → `ComputePeriod` live),
   không đọc hoá đơn đóng băng → header hiện 1.360.000₫ trong khi sổ đã khoá ở 1.000.000₫.
3. **Product gap (mất tiền âm thầm)**: `Close` từ chối kỳ không-open (`close.go:140`),
   không có reopen. Opening balance kỳ sau chỉ carry **outstanding của hoá đơn** kỳ trước
   (`repository.go:102`), không carry buổi học chưa từng được lập hoá đơn → 360.000₫
   buổi tháng 8 của Quang Minh sẽ **không bao giờ được bill** ở bất kỳ kỳ nào.

## Workaround vận hành (cho giáo viên, ngay bây giờ)

Sang kỳ 9/2026, ở màn review (draft), thêm **điều chỉnh +360.000₫** vào hoá đơn của
Quang Minh, lý do "Học phí tháng 8 (6 buổi Toán 3C)". Adjustment cho phép trên
draft/issued/partially_paid (`adjustment.go:40`).

## Đề xuất fix code (chưa thực hiện)

1. **Closed-period review đọc số đóng băng**: khi `status=closed`, review page nên hiển thị
   từ invoices đã chốt (hoặc API preview trả figures từ invoice khi closed), kèm cảnh báo
   nếu live-recompute ≠ frozen total ("Có N buổi/học sinh phát sinh sau khi chốt").
2. **Chặn/cảnh báo ghi lùi vào kỳ đã chốt**: khi xác nhận điểm danh cho buổi thuộc kỳ closed
   (hoặc enroll có buổi rơi vào kỳ closed), cảnh báo rằng buổi này sẽ không được bill.
3. **Dài hạn**: cơ chế carry "unbilled sessions" sang kỳ sau, hoặc reopen kỳ có kiểm soát.

## Addendum 260811-1649: đánh giá hướng "reopen nhưng không gửi lặp thông báo"

User đề xuất: cho mở lại kỳ, nhưng không lặp thông báo cho phụ huynh đã nhận. Kết luận:
**hợp lý về sản phẩm, nhưng code hiện tại thiếu 3 mảnh**:

1. Chưa có endpoint reopen (Close từ chối kỳ non-open, `close.go:140-142`; không có route mở lại).
2. Re-close sẽ **409 chắc chắn**: `DraftPeriod` trả `ErrInvoiceNotDraft` ngay khi gặp
   invoice đã issued (`preview.go:177-193`) — 2 hoá đơn của Hải Yến đã issued + paid.
   Reopen phải kèm sửa close: skip invoice issued có số tiền recompute không đổi;
   **tuyệt đối không đụng invoice đã có payment** (409 rõ ràng nếu recompute lệch).
3. Notification **không có dedup**: `BulkSend` target mọi contact có invoice non-void
   (`notifications/service.go:78-81`), insert ledger mới mỗi lần. Kênh zalo_personal
   auto-send sẽ **gửi lặp cho Hải Yến** (cô ấy đã mapped — run 09:22Z sent=1).

Cách không-lặp hiện có sẵn (zero code): sau khi re-close, chọn kênh **"Zalo thủ công"**,
chỉ copy card của Tùng Golang và mark sent riêng card đó — không gì tự gửi đi.
Ghi chú: tab "Nhắc nợ" (reminder) tự skip contact đã trả đủ → sau re-close sẽ chỉ
target Tùng Golang, nhưng nội dung là nhắc nợ, không phải thông báo học phí.

Dedup đúng nghĩa cho auto-send (cần code): purpose=statements skip contact đã có
notification `status=sent` cùng kỳ khi statement figures không đổi.

## Unresolved questions

- Sản phẩm muốn hướng nào: chặn ghi lùi, carry sang kỳ sau, hay cho reopen? (quyết định PRD)
- Giáo viên chốt kỳ 8 từ ngày 06/08 (giữa tháng) — có cần UX ngăn chốt quá sớm không?
