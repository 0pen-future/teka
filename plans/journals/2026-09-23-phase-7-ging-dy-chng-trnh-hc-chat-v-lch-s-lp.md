---
title: "Phase 7 Giảng dạy: chương trình học, chat và lịch sử lớp"
date: 2026-09-23
summary: "Lớp áp dụng phiên bản chương trình mẫu (owner-only, xác nhận khi sổ đầu bài khác), chat nội bộ theo cursor, lineage cùng center, và fix sau review: phiên bản lưu trữ vẫn đọc được, trần 100 buổi, xác nhận xoá tin"
---

# Phase 7 Giảng dạy: chương trình học, chat và lịch sử lớp

## Bối cảnh
Phase 7 của plan `260923-0715-giang-day-menu` nối lớp học với kho học liệu: lớp áp dụng một phiên bản chương trình mẫu, ba tab Bài tập, Tài liệu, Chat mở ra, và lịch sử lớp (lớp gốc, ghi chú, nhật ký thay đổi).

## Đã làm
- API `0f5c174`: migration 000031 (`class_programs`, `class_messages`, lineage trên `classes`, index audit theo entity, backfill `class_messages.post`), feature `classprogram` (PUT/DELETE owner-only, 409 `CURRICULUM_DIFFERS` + `confirm`, ghi `class_curricula` qua `teaching.PutCurriculum`) và `classchat` (keyset cursor, xoá theo tác giả hoặc owner), filter `entity_type`/`entity_id` cho `audit-logs`.
- Web `952f0e5`: card Chương trình học, card Lịch sử lớp với nhật ký thay đổi, tab Chat vô hạn, tab Bài tập và Tài liệu đọc qua lớp, tab Buổi học zip với buổi mẫu.
- Sau review `ae5a75b`, `a390f2d`: `library.ReleasedVersion` để lớp vẫn đọc phiên bản đã lưu trữ, `version_status` và badge "Đã lưu trữ", trần `teaching.MaxCurriculumLessons` khi áp dụng, xác nhận xoá tin nhắn với nhãn a11y theo tin, link thư viện gate `library.read`.

## Bài học
- Envelope bỏ hẳn key `data` khi payload nil (`omitempty`), nên schema web cho tài nguyên có thể rỗng phải `.nullish()` và MSW phải trả `{ success: true }` y hệt.
- Audit bus ghi bất đồng bộ; e2e đọc nhật ký phải reload đến khi thấy dòng (`toPass`) thay vì chờ cố định.
- Hai cổng đọc khác nhau cho cùng một phiên bản: "áp dụng được" (chỉ published) khác "đang theo thì vẫn đọc được" (published hoặc archived). Gộp hai điều kiện làm lưu trữ trong thư viện làm trống lớp.

## Để lại
Review `review-phase-07-260924.md` để lại L1 (TOCTOU curriculum), L3 (chu trình lineage), L4 (change log ồn), L5 (guard `TEMPLATE_IN_USE`), L6 (đếm buổi sinh tự động), test gaps và hai câu spec cần sửa cho Phase 9. L9 (chat tự làm mới) cần user quyết định.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
