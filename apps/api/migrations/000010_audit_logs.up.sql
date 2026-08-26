-- =============================================================
-- 000010 — Audit log: một dòng cho mỗi mutation API và mỗi auth event
-- (login / login-fail / logout).
--
-- Append-only và KHÔNG có FK tới users/centers: bản ghi audit phải sống
-- lâu hơn actor lẫn center của nó — xoá teacher hay center không được
-- kéo theo lịch sử hành động. Tính đúng tenant khi ĐỌC được đảm bảo bằng
-- JOIN membership tại query, không phải bằng ràng buộc ở đây.
--
-- center_id / actor_user_id NULL có nghĩa: auth event chưa gắn center
-- (login/logout) và login-fail không xác định được actor. Không lưu
-- request body; metadata jsonb chỉ chứa dữ liệu đã làm sạch từ publisher
-- (vd phone đã mask).
-- =============================================================

CREATE TABLE audit_logs (
    id            UUID PRIMARY KEY,
    occurred_at   TIMESTAMPTZ NOT NULL,
    center_id     UUID        NULL,
    actor_user_id UUID        NULL,
    actor_role    TEXT        NOT NULL DEFAULT '',
    action        TEXT        NOT NULL,
    method        TEXT        NOT NULL DEFAULT '',
    path          TEXT        NOT NULL DEFAULT '',
    entity_type   TEXT        NOT NULL DEFAULT '',
    entity_id     TEXT        NOT NULL DEFAULT '',
    status_code   INT         NOT NULL DEFAULT 0,
    request_id    TEXT        NOT NULL DEFAULT '',
    ip            TEXT        NOT NULL DEFAULT '',
    user_agent    TEXT        NOT NULL DEFAULT '',
    metadata      JSONB       NOT NULL DEFAULT '{}'
);

-- Trang audit của owner đọc theo center mới nhất trước, phân trang keyset
-- (occurred_at, id) — index phủ đúng thứ tự đó.
CREATE INDEX idx_audit_logs_center_time ON audit_logs (center_id, occurred_at DESC, id DESC);

-- Lọc theo actor trong màn điều tra một người cụ thể.
CREATE INDEX idx_audit_logs_actor ON audit_logs (actor_user_id, occurred_at DESC);
