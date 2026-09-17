---
title: "Phase 2 kanban: mô tả task thành HTML subset đã sanitize"
date: 2026-09-17
summary: "API lưu description dạng HTML subset qua bluemonday, giới hạn 4000 ký tự text, migration 000023 bọc mô tả thuần; reviewer M1 (heuristic '<') đã sửa"
---

# Phase 2 kanban: mô tả task thành HTML subset đã sanitize

## Điều gì đã xảy ra
- Thêm `internal/features/tasks/description.go`: policy bluemonday allowlist (`p, br, strong, em, u, s, ul, ol, li, a[href http/https/mailto]`), plain text được bọc `<p>…<br>…</p>` giống migration, tài liệu toàn markup quy về `""`, text > 4000 rune → 422 `fields.description`.
- Binding DTO nới `max=20000` (đếm rune) để chặn markup thô; cap text thật nằm ở service.
- Migration `000023_task_description_html`: bọc mọi mô tả thuần (kể cả soft-deleted), escape `&` trước `<>`; down lossy có ghi chú. Test up→down→up chứng minh round-trip; test cũ `TestDownFoldsPersonalChannelIntoManual` bump 18→19 bước.
- Tester DONE; reviewer 0 Critical/High. M1 là bằng chứng mới: chuỗi `<3 me\nline two` đi nhánh HTML và mất xuống dòng → đổi heuristic sang `^<[A-Za-z!/]`.

## Quyết định
- M3 (sanitize phình ~2.6x) chấp nhận: bị chặn bởi binding 20000 rune + BodyLimit 1 MiB, ghi rõ trong doc comment thay vì thêm cap thứ hai.
- M2/L4/L6 thuộc Phase 5 (schema zod đếm text, render trong container riêng, bảng mất ranh giới ô) → ghi vào Risk của phase-05.
- Test migration dùng `m.Migrate(23)/m.Migrate(22)` để không vỡ khi có 000024.

## Bước tiếp theo
- Phase 3: helper vị trí kanban ở web.
- Phase 6: backup DB prod trước khi chạy 000023; deploy API cùng web (schema 2000 ký tự cũ sẽ chặn mô tả legacy đã bọc).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
