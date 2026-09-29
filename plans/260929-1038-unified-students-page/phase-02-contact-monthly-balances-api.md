---
phase: 2
title: "API công nợ theo người liên hệ theo tháng"
status: completed
priority: P1
effort: "4h"
dependencies: [1]
---

# Phase 2: API công nợ theo người liên hệ theo tháng

## Goal

Một endpoint đọc trả số còn nợ của từng gia đình trong một tháng, cộng qua mọi
kỳ thu tiền của trung tâm trong tháng đó, chỉ cho chủ trung tâm và `billing.view_all`.

## Vì sao không tái dùng `GET /billing-periods/:id/collections?view=contact`

Quyết định brainstorm yêu cầu xác nhận việc tái dùng. Kết quả kiểm tra: **không tái dùng được một cách đúng đắn.**

- Kỳ thu tiền là theo **(giáo viên, năm, tháng)**: `billing/service.go:45-58` (`EnsurePeriod` luôn tạo kỳ của chính người gọi), `billing/repository.go:466-480` ("several teachers hold periods for the same month").
- Endpoint cũ chỉ đọc **một** kỳ (`collections/repository.go:110-117`, `vcb.period_id = ?`). Một gia đình học lớp của hai giáo viên có hai dòng nợ ở hai kỳ khác nhau.
- Tái dùng buộc web phải: liệt kê kỳ (`GET /billing-periods`, chỉ trả trang), lọc theo tháng, gọi collections cho từng kỳ và **mọi trang** (`per_page` tối đa 100), rồi cộng theo `contact_id`. Đó là fan-out N×trang và dễ sai số khi phân trang.
- Endpoint mới là một câu SQL gộp trên view có sẵn, không thêm bảng, không migration.

## Contract

`GET /api/v1/collections/contact-balances?year=2026&month=9`

- Route gate: `perm("GET", "/api/v1/collections/contact-balances", authctx.PermBillingViewAll, none())` trong `routespec.go` (owner qua `Has` superuser; `reports.send` qua implied key; `*.view_all` là key cấp được nên dùng trực tiếp làm route key).
- Path: nhóm mới `/collections` (không đặt dưới `/billing-periods` để tránh trùng segment với `/:id` và vì endpoint không thuộc một kỳ). Hiện `collections/routes.go` chỉ có nhóm `/billing-periods`; thêm nhóm thứ hai trong cùng `RegisterRoutes`.
- Ngữ nghĩa số nợ: **theo `v_contact_balance` như trang Thu tiền** (quyết định 2026-09-29) — gồm hoá đơn nháp của kỳ đang mở, chỉ loại hoá đơn huỷ. Ghi rõ trong swag description.
- Validate: `year` 2020-2100, `month` 1-12 (cùng binding tags với `EnsurePeriodRequest`).
- Response: `{ "success": true, "data": [{ "contact_id": "uuid", "outstanding": 350000 }] }` — chỉ gia đình có `outstanding <> 0`; không phân trang (bị chặn bởi số người liên hệ của một trung tâm). Tiền là số nguyên VND như các DTO collections hiện có.
- Tháng không có kỳ nào → mảng rỗng, không 404.

## Implementation Steps

1. `collections/repository.go`: `ContactOutstandingByMonth(ctx, sc, year, month int16) ([]ContactOutstanding, error)`:
   ```sql
   SELECT vcb.contact_id, SUM(vcb.outstanding) AS outstanding
   FROM v_contact_balance vcb
   JOIN billing_periods bp ON bp.id = vcb.period_id
     AND bp.center_id = vcb.center_id AND bp.deleted_at IS NULL
   WHERE vcb.center_id = ? AND bp.year = ? AND bp.month = ?
   GROUP BY vcb.contact_id
   HAVING SUM(vcb.outstanding) <> 0
   ```
   Luôn lọc `center_id` (scopelint). Kiểm tra `v_contact_balance` có cột `center_id`, `period_id`, `outstanding` (định nghĩa ở migration `000007_centers`).
2. `collections/service.go`: `ContactOutstandingByMonth(ctx, sc, year, month)` — kiểm tra lại `CenterWideFor(PermBillingViewAll)` (phòng thủ, kể cả khi route đã gate).
3. `collections/dto.go` + `handler.go`: request binding, swag annotations, handler `contactBalances`.
4. `collections/routes.go`: thêm `c := rg.Group("/collections", auth...)`, `c.GET("/contact-balances", h.contactBalances)`; `server/router.go:299` không đổi; thêm spec ở `routespec.go`.
5. `make api-docs`.
6. Tests: integration test mới `collections/contact_balances_integration_test.go`:
   - Hai giáo viên có kỳ cùng tháng, một gia đình có nợ ở cả hai → một dòng, tổng đúng.
   - Kỳ tháng khác không bị cộng; trung tâm khác không lộ.
   - Gia đình trả đủ → không có dòng.
   - Giáo viên mặc định (`billing.read` nhưng không `billing.view_all`) → 403; vai trò được cấp `billing.view_all` → 200; owner → 200.
   - `month=13` → 422.

## Todo

- [x] Repo + service + handler + route + routespec
- [x] `make api-docs`
- [x] Integration test theo danh sách trên

## Verification

- `make test-api-unit`, rồi `make test-api` (chạy riêng)
- `make lint-api`

## Risk Assessment

- `v_contact_balance` có thể tính theo `teacher_id` của kỳ; phép cộng theo `contact_id` đúng vì mỗi dòng là (kỳ, gia đình). Test hai giáo viên chốt điều này.
- Dữ liệu nợ là dữ liệu nhạy cảm: gate bằng `billing.view_all`, **không** `billing.read` (key này có trong grant mặc định của giáo viên — `000018:84`).

## Rollback

Revert commit; endpoint chỉ đọc, không có migration.
