---
title: "Phase 4: Owner Read Api"
status: done
priority: P1
effort: "1d"
dependencies: [3]
---

# Phase 4: Owner Read Api

## Overview

`GET /api/v1/audit-logs`: owner-only, filter actor/action/time-range, keyset
pagination. Hoàn thiện `features/audit` với service/handler/routes/dto.

## Requirements

- [x] Non-owner (member) → 403; chưa auth → 401
- [x] Owner chỉ thấy log `center_id` = scope của mình (auth events
      center_id NULL: chỉ hiện khi `actor_user_id` là member hiện tại của
      center — JOIN `center_members` với `left_at IS NULL`; login-fail không
      actor thì owner không thấy ở V1, ghi nhận limitation trong docs —
      user-confirmed, Validation Session 1)
- [x] Query params: `actor_id`, `action` (prefix match, vd `billing.`),
      `from`, `to` (RFC3339), `cursor`, `limit` (default 50, max 100)
- [x] Keyset: sort `occurred_at DESC, id DESC`; cursor = base64
      `occurred_at|id` của dòng cuối; response kèm `next_cursor` (rỗng khi hết)
- [x] Swagger annotations theo style handler hiện có

## Architecture

- `service.go`: `List(ctx, sc authctx.Scope, q ListQuery) ([]AuditLog, string, error)`
  — check `sc.IsOwner` → `apperror.Forbidden` (khớp pattern owner-check ở
  repo/service layer như `classes/repository.go:73`; đối chiếu helper apperror
  hiện có lúc implement).
- `repository.go` thêm `List`: WHERE center-visibility + filters + keyset
  `(occurred_at, id) < (cursor_at, cursor_id)` (row-value comparison — dùng
  `WHERE (occurred_at, id) < (?, ?)`, Postgres hỗ trợ, index
  `(center_id, occurred_at DESC, id DESC)` phase 2 cover).
- Center-visibility của auth events: `center_id IS NULL AND actor_user_id IN
  (SELECT ... membership của center)` — đối chiếu bảng membership thực tế
  (`centers` feature) lúc implement; UNION hoặc OR đều được, chọn theo EXPLAIN
  đơn giản nhất.
- `routes.go`: `RegisterRoutes(v1, h, requireAuth, resolveScope)` — mount
  `GET /audit-logs`, theo đúng pattern feature khác trong
  `server/router.go` (`registerFeatures` thêm audit sau centers).
- `dto.go`: `AuditLogResponse` — expose: id, occurred_at, actor_user_id,
  actor_name (join teachers để hiển thị — nếu join đắt, trả id và để web map
  từ member list; **quyết: trả actor_name qua LEFT JOIN teachers**, một query),
  action, method, path, entity_type, entity_id, status_code, ip, metadata.
  Không expose user_agent đầy đủ ở list (giữ trong detail? V1: expose luôn,
  đơn giản).

## Related Code Files

- Create: `apps/api/internal/features/audit/service.go`
- Create: `apps/api/internal/features/audit/handler.go`
- Create: `apps/api/internal/features/audit/routes.go`
- Create: `apps/api/internal/features/audit/dto.go`
- Modify: `apps/api/internal/features/audit/repository.go` (List + cursor)
- Modify: `apps/api/internal/server/router.go` (mount audit routes)
- Create: `apps/api/internal/features/audit/service_test.go`
- Modify: `apps/api/internal/features/audit/integration_test.go`

## Implementation Steps (TDD)

1. **Test trước** — `service_test.go`: non-owner → Forbidden; limit clamp
   (0→50, 999→100); cursor encode/decode roundtrip + cursor rác → error 4xx.
2. **Test trước** — integration (testutil fixtures 2 centers):
   - owner center A không thấy log center B
   - member center A → 403
   - filter actor/action-prefix/from-to
   - keyset: seed 120 dòng → page 1 (50) + cursor → page 2 → page 3 (20),
     không trùng không sót, thứ tự DESC ổn định khi occurred_at trùng nhau
   - auth event của member center A hiển thị cho owner A, không cho owner B
3. Implement repository.List → service → handler/dto/routes → mount router.
4. `make api-docs` (swagger regen) nếu Makefile có target; go test toàn bộ.

## Todo

- [x] service_test.go + integration cases (đỏ)
- [x] repository.List keyset + visibility (xanh)
- [x] service/handler/dto/routes + router mount
- [x] Swagger annotations + regen

## Success Criteria

- [x] Acceptance 4 brainstorm chứng minh bằng integration test
- [x] EXPLAIN dùng index (ghi nhanh kết quả vào PR/report, không cần
      benchmark formal)

## Risk Assessment

- Row-value comparison với DESC cần `(occurred_at, id) < (?, ?)` đúng chiều —
  test page-boundary khóa lại.
- actor_name join khi teacher bị xóa → LEFT JOIN, name rỗng, FE hiển thị
  "(đã xóa)".
- Action prefix filter dùng LIKE 'x%' — sanitize input (escape %_).

## Ghi chú sau review (260826)

- Review DONE_WITH_CONCERNS → chi tiết + fixes tại
  `plans/reports/review-260826-phase-04-owner-read-api.md`.
- User quyết (H2): auth rows (center NULL) hiện theo **membership window**
  `[joined_at, left_at)` — không theo membership hiện tại. Lịch sử login
  không theo giáo viên sang center mới, và không biến mất khỏi center cũ
  khi họ rời.
- H1: `Repository.List` dùng UNION ALL 2 leg keyset-limited (center leg /
  membership leg qua LATERAL) rồi mới LEFT JOIN teachers. EXPLAIN SQL thật
  (TestListQueryPlanUsesIndexes): Merge Append, cả 2 leg Index Scan
  (idx_audit_logs_center_time / idx_audit_logs_actor), keyset trong Index
  Cond, không Seq Scan trên audit_logs → chi phí O(limit)/trang.
- Cho Phase 5: `from`/`to` đều inclusive; window ngược trả 200 rỗng (không
  400) — FE đừng hiển thị thành "không có hoạt động".
