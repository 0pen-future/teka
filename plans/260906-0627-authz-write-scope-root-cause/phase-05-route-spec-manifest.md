---
phase: 5
title: "Route spec manifest"
status: completed
priority: P2
effort: "2d"
dependencies: []
---

# Phase 5: Route spec manifest

## Overview

Hợp nhất ba bảng thuộc tính route — `server.routePolicies` (policy),
`audit.actions` (action/entity), ba map bỏ qua trong
`middleware/request_events.go` — thành một manifest trong package leaf
`internal/shared/routespec`. Ba nơi tiêu thụ derive từ manifest; test hai chiều
với `engine.Routes()` giữ nguyên và thêm test "mọi route mutating có audit
source".

## Requirements

- Functional:
  - Một route = một `routespec.Spec{Method, Path, Kind, Key, Audit}` với
    `Audit{Source, Action, EntityType, IDParam}`; `Source ∈ {request, service, auth_session, anonymous, none}`.
  - `server.routePolicies` = `routespec.Policies()`; `audit.LookupAction` đọc từ
    `routespec`; `authSessionRoutes`/`serviceAuditedRoutes`/`anonymousAuditedRoutes`
    derive từ `Source`.
  - Hành vi runtime không đổi: cùng policy, cùng action, cùng tập route bị bỏ
    qua audit.
- Non-functional:
  - `routespec` chỉ import `authctx` (và stdlib) — không tạo import cycle
    (hiện `audit` → `middleware`, `server` → cả hai; authctx không import
    features/server/middleware).
  - Ba map bỏ qua audit trong `request_events.go:106/110` key theo **path
    only** (không method): `WithSource` trả map path-only và test fail-closed
    "mọi Spec cùng path phải cùng Source" để derive không đổi semantic.
  - Test fail-closed: route đăng ký mà không có Spec → đỏ; Spec không có route → đỏ;
    route mutating (POST/PUT/PATCH/DELETE) với `Source == none` → đỏ trừ khi
    nằm trong allowlist có lý do (health/swagger không mutating nên không cần).

## Architecture

```go
package routespec
type Kind string        // public, public_token, self, owner_only, permission, service (chuyển từ server.PolicyKind)
type AuditSource string // request, service, auth_session, anonymous, none
type Audit struct{ Source AuditSource; Action, EntityType, IDParam string }
type Spec struct{ Method, Path string; Kind Kind; Key string; Audit Audit }
var Specs = []Spec{ ... }           // manifest duy nhất
func Policies() []Spec              // cho server
func Lookup(method, path string) (Spec, bool)
func WithSource(s AuditSource) map[string]struct{} // cho request_events, key = path only
```

`server.PolicyKind` giữ tên/hằng để test hiện có (`route_policy_test.go`,
`route_policy_enforce_test.go`) không đổi, alias sang `routespec.Kind`.
`audit.ActionSpec` giữ shape; `LookupAction` gọi `routespec.Lookup`. Grading
test `TestGradingRoutesAreRegistered` giữ và tổng quát hoá thành "mọi route
mutating có action".

## Related Code Files

- Create: `apps/api/internal/shared/routespec/{routespec.go,routespec_test.go}`
- Modify: `apps/api/internal/server/route_policy.go` (manifest → derive), `route_policy_enforce.go` (dùng Kind alias), `route_policy_test.go` (giữ hai chiều; thêm mutating-has-audit)
- Modify: `apps/api/internal/features/audit/action.go`, `action_test.go`
- Modify: `apps/api/internal/middleware/request_events.go` (map derive), `request_events_test.go` nếu có
- Modify: `docs/api-guidelines.md` hoặc `docs/adding-permissions.md` (mục "Thêm route": một chỗ khai báo policy + audit), `apps/api/CLAUDE.md` (một dòng trỏ tới routespec)

## Implementation Steps

1. Tạo `routespec` với `Kind`, `AuditSource`, `Spec`; chuyển dữ liệu từ `routePolicies` (~100 dòng) và `audit.actions` (69 entry) và 3 map (6 entry) thành `Specs` bằng script một lần (không commit script), đối chiếu số lượng.
2. Test `routespec_test.go`: không trùng (method, path) (cần vì `audit.actions` là map còn `routePolicies` là slice); mọi Spec có Kind hợp lệ; Kind permission ⇔ Key khác rỗng; mutating ⇒ Source ≠ none; `Source=request` ⇒ `Action != ""` (đóng fallback im lặng "METHOD route" của `audit/action.go:20,142`); cùng path ⇒ cùng Source.
3. `server`: `routePolicies = routespec.Policies()`; alias `PolicyKind`; test hai chiều hiện có xanh không đổi.
4. `audit.LookupAction` → `routespec.Lookup`; `action_test.go` tổng quát.
5. `request_events.go`: ba map derive từ `WithSource`; test rằng tập route bỏ qua trước/sau giống nhau (snapshot một lần trong test).
6. Docs; `make test-api`, `make lint-api`, `make api-docs` (không đổi swag).

## Success Criteria

- [x] `grep -n 'perm(\|classified(' apps/api/internal/server/route_policy.go` không còn literal route; `audit.actions` literal đã xoá.
- [x] `TestRoutePolicyCoversEveryRegisteredRoute` xanh; thêm route mới mà thiếu Spec → đỏ; thêm Spec mutating thiếu audit → đỏ; Spec `Source=request` với Action rỗng → đỏ; hai Spec cùng path khác Source → đỏ.
- [x] `go list -deps ./internal/shared/routespec` không chứa `internal/features/*`, `internal/middleware`, `internal/server`.
- [x] Số entry `Specs` = số route `engine.Routes()`; số Spec có `Source=request` = số entry `audit.actions` cũ (đối chiếu ghi trong PR).

## Risk Assessment

- **Path string lệch giữa ba bảng cũ** (một bảng dùng `:id`, bảng khác `:sessionId`). Tín hiệu: bước hợp nhất phát hiện key không khớp. Phản ứng: lấy `engine.Routes()` làm chuẩn, sửa bảng lệch, ghi lại khác biệt trong PR.
- **Import cycle** nếu `routespec` cần hằng capability từ package feature. Tín hiệu: `go vet` cycle. Phản ứng: chỉ dùng hằng từ `authctx`; không kéo type feature vào leaf.
- **Thay đổi ngầm tập route bị bỏ qua audit** khi derive. Tín hiệu: test snapshot trước/sau đỏ. Phản ứng: sửa Spec, không sửa snapshot.

## Execution Notes

- 2026-09-07, slice `phase5-routespec-manifest` (báo cáo
  `plans/reports/phase5-routespec-manifest-260907-0232-route-spec-manifest.md`).
  TDD: snapshot test pin trước khi đổi (`server/route_policy_snapshot_test.go`
  128 entry, `audit/action_test.go` `TestActionSnapshotUnchanged` 69 entry,
  `middleware/request_events_test.go` `TestSkipMapsUnchanged` 3/1/2) → xanh
  trước và sau migration; 4 probe fail-closed đỏ đúng rồi gỡ.
- Đối chiếu ba bảng cũ với `engine.Routes()`: **0 lệch** (rủi ro path-string
  drift không xảy ra). 128 Spec = 128 route; 67 `request` + 2 `anonymous` có
  Action = 69 entry `audit.actions` cũ. `routespec` là leaf (`go list -deps`).
- Docs "Thêm route" chưa áp vào `docs/` vì file đang do slice phase 3 sửa; đoạn
  văn nằm trong báo cáo, lead gộp sau phase 3. `apps/api/CLAUDE.md` đã có dòng
  trỏ tới `routespec`.
- `make api-docs` trong bước verify sinh diff swagger của endpoint mark-sent —
  đó là annotation 404 mới của phase 3, hợp lệ, giữ.
- Review (`plans/reports/code-review-260907-phase-05-routespec.md`, 8.5/10,
  không Critical/High; reviewer tự diff ba bảng ở HEAD với manifest: 0 lệch).
  Lead đã sửa 3 Medium: (M1) `GET /api/v1/enrollments` về `none()`; (M2) thêm
  `TestSkipSourceNeverSharesPathWithAuditedRoute` — probe `GET /api/v1/students`
  = service làm test đỏ đúng rồi gỡ; (M3) export `routespec.IsMutating`, middleware
  và test dùng chung. Low: gỡ nhắc "phase-1" trong comment, sửa doc package.
  Low còn lại (Policies copy trong khi `Specs` exported, phantom test
  `TestWithSourceMatchesManifest`) giữ nguyên — không ảnh hưởng hành vi.

