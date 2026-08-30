# Dry-run collision queries — migration 000016_owner_data_anchor (Phase 3, step 4)

**Trạng thái: CHỜ USER CHẠY.** Sandbox của phiên cook bị chặn truy cập
container prod (`teka-db`) kể cả read-only, nên số liệu chưa được ghi. Hai
query dưới đây **chỉ SELECT** — an toàn chạy trên prod bất kỳ lúc nào.

Mục đích: đếm số nhóm contact trùng mà bước MERGE của migration 000016 sẽ
gộp. Nếu số lớn bất thường, chốt lại survivor-rule (created_at sớm nhất,
tie-break nhiều students hơn) với user trước khi migrate.

## Cách chạy

```sh
docker exec teka-db psql -U postgres -d <tên-db> -f - <<'SQL'
-- Collision 1: cùng center, cùng phone, nhiều contact sống
SELECT count(*) AS phone_collision_groups,
       COALESCE(sum(n - 1), 0) AS contacts_to_merge
FROM (
    SELECT center_id, phone, count(*) AS n
    FROM contacts
    WHERE deleted_at IS NULL
    GROUP BY center_id, phone
    HAVING count(*) > 1
) g;

-- Collision 2: cùng center, cùng zalo_user_id, nhiều contact sống
SELECT count(*) AS zalo_collision_groups,
       COALESCE(sum(n - 1), 0) AS mappings_to_null
FROM (
    SELECT center_id, zalo_user_id, count(*) AS n
    FROM contacts
    WHERE deleted_at IS NULL AND zalo_user_id IS NOT NULL
    GROUP BY center_id, zalo_user_id
    HAVING count(*) > 1
) g;

-- Collision 3: sau khi gộp nhóm contact trùng phone, có bao nhiêu kỳ mà
-- NHÓM gộp (survivor + các loser) giữ nhiều hơn 1 statement sống — đây là
-- số statement migration sẽ xoá mềm (giữ bản của survivor, rồi bản sớm nhất).
SELECT count(*) AS statement_collision_periods,
       COALESCE(sum(n - 1), 0) AS statements_to_soft_delete
FROM (
    SELECT g.survivor_group, st.period_id, count(*) AS n
    FROM statements st
    JOIN (
        SELECT id,
               first_value(id) OVER (
                   PARTITION BY center_id, phone
                   ORDER BY created_at ASC, id ASC
               ) AS survivor_group
        FROM contacts
        WHERE deleted_at IS NULL
    ) g ON g.id = st.contact_id
    WHERE st.deleted_at IS NULL
    GROUP BY g.survivor_group, st.period_id
    HAVING count(*) > 1
) c;
SQL
```

(Collision 3 xấp xỉ survivor-rule của migration — tie-break đủ cho việc đếm;
số dòng đúng là điều cần biết trước khi migrate, không phải danh tính survivor.)

(Tên db lấy từ `.env.production`; hoặc gõ `! docker exec teka-db psql -U postgres -l`
trong phiên Claude Code để liệt kê.)

## Kết quả (điền sau khi chạy)

| Query | Groups | Rows bị ảnh hưởng |
|---|---|---|
| (center_id, phone) | _chưa chạy_ | _chưa chạy_ |
| (center_id, zalo_user_id) | _chưa chạy_ | _chưa chạy_ |
| statement trùng kỳ trong nhóm gộp | _chưa chạy_ | _chưa chạy_ |
