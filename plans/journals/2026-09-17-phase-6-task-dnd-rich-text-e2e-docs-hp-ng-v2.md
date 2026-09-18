---
title: "Phase 6 task DnD + rich text: e2e, docs, hợp đồng v2"
date: 2026-09-17
summary: "E2E Playwright cho kéo-thả, rich text và cảm ứng trên stack cô lập; docs và hợp đồng API v2; ship prod chờ duyệt"
---

# Phase 6 task DnD + rich text: e2e, docs, hợp đồng v2

## Chuyện gì đã xảy ra

- Thêm mục tiêu make `e2e-isolated` (compose project `teka-e2e`, cổng 55173/58080/55432, seed mới, `down -v` luôn chạy). Chạy trọn vẹn: 37/37 spec pass trong 5 phút; ba suite billing/collections/statement từng đỏ trên stack dev nay xanh vì seed sạch, chứng tỏ nguyên nhân là dữ liệu dev bẩn.
- Ba spec mới: `tasks-board-dnd.spec.ts` (mouse kéo theo bước, thứ tự [1,3,2] sau reload, kéo sang cột khác), `tasks-board-rich-text.spec.ts` (toolbar Đậm/list/link rồi mở lại; payload XSS qua `page.request` về không còn script/handler), `tasks-board-mobile.spec.ts` (Pixel 7, CDP `Input.dispatchTouchEvent`: nhấn giữ 300ms thì kéo, vuốt nhanh thì cuộn, không mở dialog). Playwright thêm `projects` desktop/mobile với `testIgnore`/`testMatch` để spec mobile không chạy hai lần.
- Docs: `docs/architecture.md` (position float midpoint + renormalize trong tx dưới advisory lock; mô tả sanitize hai lớp; DnD là adapter opt-in ngoài lib kanban), `docs/frontend-guidelines.md`, README; `reports/api-contract-v2.md` mô tả `POST /tasks/:id/move` v2 và HTML subset của `description`.

## Lỗi và cách sửa

- Kéo card sang cột "Hoàn thành" rơi về cuối cột nguồn: tâm listbox đích nằm dưới viewport vì các cột kéo dài theo cột cao nhất. Thêm điểm thả `at: "top"` trong `dragTo`.
- Spec mobile so sánh cả cột nên dính task của spec chạy trước; đổi sang `topCardTitles(list, n)` vì task mới luôn lên đầu cột.
- `make test-api` chạy song song timeout do tranh chấp tài nguyên (load 17–24); chạy `-p 1` thì 40 gói ok, độ phủ 77.2%.
- Reviewer H1: `up` nằm ngoài khối `status` nên khi `up` fail stack không được dọn; đã gộp vào cùng khối. M2 ghim `POSTGRES_*` trong `E2E_COMPOSE`; M3 `scrollIntoViewIfNeeded` trước khi đo toạ độ; M4 assert cột đích chính xác; M5 spec cũ dùng helper chung. Sau sửa 5/5 spec `tasks-board*` pass.

## Quyết định

- Gate của plan là bốn spec `tasks-board*`; toàn bộ suite vẫn được chạy và ghi vào Test evidence.
- Phase 6 đóng trừ tiêu chí Prod. Bước ship (pg_dump -Fc prod trước `compose up` chạy migration 000023, deploy web, checklist sau deploy) và push master cần người dùng duyệt riêng.

## Bước tiếp theo

- Người dùng duyệt ship: backup prod DB, `compose up` prod, kiểm `count(*)` mô tả chưa bọc `<p>` = 0, deploy web, theo dõi log API 30 phút.
- Push 15 commit master sau khi được duyệt.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
