---
title: Lập plan kéo-thả Kanban + mô tả rich text
date: 2026-09-17
summary: "Plan 260917-1515-task-dnd-rich-text: 6 phase, 2 researcher + 2 red-team; red-team web tìm 4 High khiến phase 4 không chạy được nếu viết như bản đầu"
---

# Lập plan kéo-thả Kanban + mô tả rich text

## What happened

Lập plan `plans/260917-1515-task-dnd-rich-text/` nối tiếp plan Kanban 260913. Hai researcher chốt thư viện (`@dnd-kit/core@6.3.1` + `sortable@10`, TipTap 3.31.3, bluemonday v1.0.27, DOMPurify 3.4.15). Hai red-team chạy song song trên plan + code thật + tarball thư viện.

Red-team API (1 High, 5 Medium, 8 Low): `WithinTx` chỉ READ COMMITTED không khoá nên hai move đồng thời cùng `after_task_id` trùng position; `NOT LIKE '<p>%'` trong migration bỏ sót mô tả thuần bắt đầu bằng `<p>`; `normalizeDescription` không bọc plain text nên client cũ gửi text có xuống dòng thành "nửa HTML"; chuỗi không UUID trả 400 chứ không 422; có bộ fake repo thứ hai ở `internal/features/tasks/service_test.go` cần sửa.

Red-team web (4 High, 8 Medium, 6 Low), đều đã kiểm lại trên code: `taskProps` của lib đã chứa `ref` nên `ref={mergeRefs(setNodeRef)}` bị ghi đè và `mergeRefs` là private; dnd-kit destructure `attributes` với giá trị mặc định nên `role: undefined` vẫn ra `role="button"`; quy tắc `resolveDrop` "chèn trước over" lệch 1 khi kéo xuống cùng cột (sortable dùng `arrayMove`); `PointerSensor` chiếm cử chỉ trước `TouchSensor` nên nhấn giữ trên mobile không bao giờ chạy. Ngoài ra `make e2e` không dựng stack cô lập, `src/index.css` và Popover wrapper không tồn tại, `@tiptap/extension-character-count` là shim deprecated, TipTap v3 không re-render theo transaction.

## Decision

Nhận toàn bộ phát hiện (trừ 2 Low không cần sửa). Chốt: advisory lock theo `(center_id, column_id)` trong `ListColumnPositions`; migration không guard nội dung, post-check `count(*) = 0`; server bọc plain text như migration; card truyền `setNodeRef` + `listeners` qua tham số `extra` của `getTaskProps`, không spread `attributes`; `MouseSensor` + `TouchSensor` ở mọi viewport; `resolveDrop` theo `arrayMove`; `useEditorState` cho toolbar; `@tiptap/extensions`; ô URL inline thay popover; thêm `make e2e-isolated`; tách `task-form-modal.test.tsx`.

Bài học: giả định về hành vi thư viện ("`undefined` tắt attribute", "hai sensor cùng tồn tại") phải kiểm trên bundle thật trước khi thành thiết kế; red-team đọc tarball rẻ hơn nhiều so với phát hiện lúc cook.

## Next steps

- Chạy `/ak:cook plans/260917-1515-task-dnd-rich-text` theo thứ tự phase 1/2/3 song song → 4, 5 → 6.
- Phase 4: e2e mobile là gate cho cặp sensor; nếu CDP touch vẫn không kích hoạt, kiểm tay thiết bị thật trước ship.
- Ship: backup DB prod trước `compose up` (migrate tự chạy trước api).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
