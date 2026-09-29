---
title: Modal Sửa lộ trình học theo prototype v5
date: 2026-09-29
summary: "PathDialog theo v5, thêm Xoá xếp chồng; vitest courses 60/60, typecheck/lint/prettier sạch"
---

# Modal Sửa lộ trình học theo prototype v5

# Modal Sửa lộ trình học theo prototype v5

## Chuyện gì đã xảy ra
- Dựng lại modal "Sửa lộ trình học" từ `So Lop - Prototype v5.dc.html` qua DesignSync; HTML chính bị cắt ở 256KB nên logic lấy từ `handoff/04-logic-component.js` (`libForm('path')`, `TITLES.path`, `libAsk`).
- `PathDialog`: tiêu đề "Tạo/Sửa lộ trình học", mô tả v5, thứ tự Tên → Trạng thái → Mã → Mô tả, nút "Lưu thay đổi"/"Tạo mới"; khi sửa có nút "Xoá" mở confirm xếp chồng (huỷ confirm quay lại form).
- Tách `PathDeleteConfirm` dùng chung cho modal và trang chi tiết lộ trình; trang danh sách `/paths` lần đầu xoá được lộ trình.
- Cập nhật unit test (thêm sửa tên/trạng thái, xoá qua form, không có Xoá khi tạo) và e2e `paths.spec.ts`.

## Quyết định
- Giữ trường Mã và Mô tả dù prototype chỉ nói "Tên và trạng thái": API bắt buộc mã, và modal là nơi duy nhất sửa chúng.
- Không thêm chặn "lộ trình còn khoá học" của prototype: khoá học chỉ là gợi ý của chặng, vẫn nằm trong danh mục.

## Việc tiếp theo
- Chưa chạy e2e `paths.spec.ts` trên stack `teka-e2e`.
- 23 test lỗi có sẵn trên HEAD (roster `_buffer`, library/tasks `focus`) vẫn còn, đã xác nhận bằng stash.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
