---
title: Plan trang Học sinh hợp nhất và quy tắc SĐT theo quyền
date: 2026-09-29
summary: Lập plan 4 phase; red-team phát hiện BulkSendRow.Phone lộ SĐT thô cho người gửi theo lớp
---

# Plan trang Học sinh hợp nhất và quy tắc SĐT theo quyền

## What happened
- Lập plan `plans/260929-1038-unified-students-page/` (4 phase) từ brainstorm Option A.
- Red-team phát hiện lỗi có sẵn: `notifications/dto.go:48` `BulkSendRow.Phone` trả SĐT thô cho người gửi theo lớp (`AuthorizeClassSend`). Plan cũ bỏ sót lỗi này.
- Bản nháp đầu định làm rỗng `ZaloMappingsClass`. Làm vậy sẽ hỏng gửi `zalo_personal` theo lớp, trong khi hàm này lọc theo ghi danh chứ không qua fragment SĐT.
- Không tái dùng được endpoint collections cho công nợ theo tháng: kỳ thu tiền tính theo (giáo viên, năm, tháng), còn endpoint cũ chỉ đọc một kỳ.

## Decision
- Chỉ ai có `PhoneVisible` = `contacts.view_all` mới thấy SĐT; bỏ nhánh học vụ.
- Ghi mapping Zalo: chủ trung tâm + `reports.send`.
- Gửi theo lớp giữ Zalo ID; SĐT trong response bị che.
- Endpoint mới `GET /collections/contact-balances?year&month`, gate `billing.view_all`, tính theo `v_contact_balance` (gồm hoá đơn nháp).
- Danh sách dài dùng nút "Xem thêm" (`useInfiniteQuery`).
- Đổi thứ tự: trang (phase 3) làm trước sidebar (phase 4).

## Next steps
- `/ak:cook plans/260929-1038-unified-students-page/plan.md` trên nhánh `feat/unified-students-page`.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
