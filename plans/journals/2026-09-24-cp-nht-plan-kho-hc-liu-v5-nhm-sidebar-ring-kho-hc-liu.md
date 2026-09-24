---
title: "Cập nhật plan Kho học liệu v5: nhóm sidebar riêng KHO HỌC LIỆU"
date: 2026-09-24
summary: "Thêm D13 (nhóm sidebar riêng, 4 mục, tab hub thành route con), claim 13-16, Q7; Phase 3 +0.5d, Phase 6 thêm docs"
---

# Cập nhật plan Kho học liệu v5: nhóm sidebar riêng KHO HỌC LIỆU

## What happened
User yêu cầu cập nhật `plans/260924-0448-kho-hoc-lieu-v5` để sidebar có nhóm riêng tên "KHO HỌC LIỆU" thay vì một mục
"Kho học liệu" trong nhóm "Giảng dạy". Scout xác nhận: nhóm khai báo tĩnh trong `useNavGroups` (`dashboard-layout.tsx`),
header render uppercase bằng CSS, active-state (`useNavActive`) chỉ so `pathname` theo tiền tố dài nhất và không đọc query;
hub hiện chọn tab qua `?tab=` (tests + e2e đều dùng). Script nav của design v5 bị cắt ở 256 KiB nên không lấy được danh
sách mục từ design.

## Decision
D13: nhóm `{ header: "Kho học liệu" }` sau "Giảng dạy", 4 mục Chương trình mẫu `/library` · Ngân hàng nội dung
`/library/materials` · Ngân hàng bài tập `/library/exercises` · Chuẩn bị tài liệu `/prep` (perm `library.read`); tab hub
đổi từ `?tab=` sang route con để active-state đúng mà không thêm logic query, `?tab=` cũ redirect. "Danh mục khóa học" /
"Lộ trình học" ở lại "Giảng dạy". Phương án gọn hơn (2 mục, giữ `?tab=`) ghi ở Q7 để user chọn khi cook. Không đổi API,
migration hay permission key. Phase 3 1.5d → 2d, tổng 11d; Phase 6 thêm docs `frontend-guidelines.md` và
`api-guidelines.md:435`.

## Next steps
`/ak:cook plans/260924-0448-kho-hoc-lieu-v5/plan.md` — xác nhận Q7 (4 mục + route con, hay 2 mục giữ `?tab=`) trước khi
làm Phase 3.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
