-- Gỡ ba permission key đã nghỉ hưu khỏi hai bảng phép gán. Quyền hiệu lực
-- không đổi với dữ liệu đã backfill:
--  * data.view_center_wide: migration 000018 đã materialize đủ các key
--    view_all theo từng loại dữ liệu cho mọi dòng legacy, nên dòng alias chỉ
--    còn là bản sao — catalog v3 không còn expand nó nữa.
--  * scores.view_all / teaching.view_all: chưa từng có điểm enforcement
--    (grading/teaching scope qua lớp/buổi học), grant hay deny đều no-op.
-- Ở production cả ba key đều đang có 0 dòng (đã kiểm kê trước khi chạy).
DELETE FROM center_role_permissions
WHERE permission_key IN ('data.view_center_wide', 'scores.view_all', 'teaching.view_all');

DELETE FROM center_member_permissions
WHERE permission_key IN ('data.view_center_wide', 'scores.view_all', 'teaching.view_all');
