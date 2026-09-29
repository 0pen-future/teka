-- Dựng lại đúng cấu trúc bảng/index/cột mà 000014 đã tạo, để binary cũ chạy
-- lại được. Danh mục bộ điểm, thành phần của bộ và liên kết truy vết
-- source_set_id đã xóa ở up thì KHÔNG được khôi phục — hai bảng sinh ra
-- rỗng và cột mới mang giá trị NULL. Cần dữ liệu thì khôi phục từ backup.
CREATE TABLE score_sets (
    id         UUID PRIMARY KEY,
    center_id  UUID         NOT NULL REFERENCES centers(id) ON DELETE CASCADE,
    name       VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX score_sets_center_name_live
    ON score_sets (center_id, lower(name)) WHERE deleted_at IS NULL;

CREATE TABLE score_set_components (
    id       UUID        PRIMARY KEY,
    set_id   UUID        NOT NULL REFERENCES score_sets(id) ON DELETE CASCADE,
    name     VARCHAR(50) NOT NULL,
    position SMALLINT    NOT NULL,
    UNIQUE (set_id, name),
    UNIQUE (set_id, position)
);

ALTER TABLE class_score_components
    ADD COLUMN source_set_id UUID REFERENCES score_sets(id) ON DELETE SET NULL;
