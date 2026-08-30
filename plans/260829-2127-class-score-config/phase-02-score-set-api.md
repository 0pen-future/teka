---
phase: 2
title: "Score-set API (owner CRUD + apply snapshot)"
status: done
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: Score-set API (owner CRUD + apply snapshot)

## Overview

Feature mới `apps/api/internal/features/grading/` — phần template: owner CRUD
bộ điểm và apply/clear snapshot cho lớp. Layout chuẩn feature: `model.go`,
`dto.go`, `repository.go`, `service.go`, `handler.go`, `routes.go` + test
cùng package.

## Requirements

- Functional:
  - `GET /score-sets` — list bộ + components (sorted position).
  - `POST /score-sets` — tạo (name + components[]).
  - `PUT /score-sets/:id` — đổi tên + **replace** toàn bộ components.
  - `DELETE /score-sets/:id` — soft delete.
  - `POST /classes/:id/score-set` body `{set_id}` — replace snapshot vào
    `class_score_components`.
  - `DELETE /classes/:id/score-set` — gỡ snapshot (trường hợp gán nhầm).
  - `GET /classes/:id/score-components` — đọc snapshot của lớp (dùng chung
    cho phase 3/5; teacher của lớp + reader center-wide).
- Non-functional: response envelope + apperror theo `docs/api-guidelines.md`;
  swag annotations (`make api-docs`).

## Architecture

- **Gate:** mọi route `/score-sets` và apply/clear = owner-only qua
  `sc.IsOwner` trong **service** (pattern teaching review-writes). Không
  perm key mới. Repo tuyệt đối không đụng `IsOwner`/`Has(` (guard test).
- **Validation:** tên bộ 1..100; components 1..10 phần tử, tên 1..50, không
  trùng trong bộ (case-insensitive trim), position = index sau normalize.
  Trùng tên bộ trong center → 409 map từ unique index. Apply một set đã
  soft-delete → 404.
- **Audit:** audit là route-based (`audit/action.go` map route → action);
  route mới KHÔNG tự được capture. Đăng ký action cho POST/PUT/DELETE
  `/score-sets`, apply/clear `/classes/:id/score-set` + test capture theo
  pattern `capture_integration_test.go`.
- **Apply = transaction:** xóa `class_score_components` hiện có của lớp,
  insert bản copy từ `score_set_components` (giữ name/position, gắn
  `source_set_id`). Guard: `EXISTS student_scores WHERE class_id` → 409
  `apperror` message rõ ("lớp đã có điểm, không thể đổi bộ điểm"). Clear
  dùng cùng guard.
- **Tenancy:** mọi query anchor `center_id = sc.CenterID`; lớp phải thuộc
  center (resolve class theo pattern `teaching.resolveClass`).
- **Routes:** `grading.RegisterRoutes(rg, h, requireAuth, resolveScope)` —
  group `/score-sets` riêng + join `/classes/:id` như teaching. Đăng ký
  trong `internal/app/container.go`.

## Related Code Files

- Create: `apps/api/internal/features/grading/{model,dto,repository,service,handler,routes}.go`
- Create: `apps/api/internal/features/grading/service_test.go` (unit) +
  `grading_integration_test.go` (theo pattern feature khác)
- Modify: `apps/api/internal/app/container.go` (wire feature)
- Generated: `apps/api/docs/*` qua `make api-docs` (không sửa tay)

## Implementation Steps

1. Soi 1 feature nhỏ gần nhất (vd `audit` hoặc `imports`) để khớp skeleton
   DI + integration test bootstrap trong `internal/testutil/`.
2. `model.go`: ScoreSet, ScoreSetComponent, ClassScoreComponent (+
   `TableName()` pin như classes/teaching).
3. Repo: CRUD set (preload components), replace-components, apply/clear
   snapshot (tx), `ClassHasScores(classID)`; scope đọc qua `sc.CenterWide()`
   khi cần rộng.
4. Service: gate owner, validate, orchestrate tx; handler + dto + routes +
   swag.
5. Đăng ký các route mới vào registry `audit/action.go` + integration test
   capture.
6. Tests: unit validate/gate; integration: CRUD roundtrip, member 403,
   apply snapshot rồi sửa bộ gốc → lớp không đổi (AC3), apply khi có điểm
   → 409 (seed 1 dòng student_scores), apply set đã xóa → 404.
7. `make test-api-unit` → `make test-api`.

## Success Criteria

- [ ] Owner CRUD hoạt động; member gọi bất kỳ route owner-only → 403.
- [ ] Tên thành phần trùng trong bộ → 422/400 theo chuẩn validation repo.
- [ ] Apply là snapshot: sửa/xóa bộ gốc không đổi `class_score_components`.
- [ ] Apply/clear khi lớp có điểm → 409 message tiếng Việt rõ ràng.
- [ ] Route mới có mặt trong `audit/action.go` và được capture (test).
- [ ] `scoping_guard_test.go` + toàn bộ `make test-api` pass.

## Risk Assessment

- Quên wire container → route 404 âm thầm; integration test qua HTTP bắt
  được.
- Replace components của bộ gốc dùng hard delete: an toàn vì snapshot đã
  copy giá trị, không tham chiếu id component gốc.
