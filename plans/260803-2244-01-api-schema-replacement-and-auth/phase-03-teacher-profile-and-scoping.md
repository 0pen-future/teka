---
phase: 3
title: "Teacher Profile and Scoping Utilities"
status: pending
priority: P1
effort: "4h"
dependencies: [2]
---

# Phase 3: Teacher Profile and Scoping Utilities

## Overview

Finish the identity slice: expose the teacher's own profile
(`GET /api/v1/me`, `PUT /api/v1/me`), delete the dead `users` feature, and
publish the shared helpers every later plan depends on — the teacher-scoping
accessor, the repository-level tenancy convention (D4), and a soft-delete read
contract that plans 02–06 follow verbatim.

Nothing here is large, but it is the last chance to establish the tenancy
pattern *before* five plans copy it. A repository written without the
`teacher_id` filter is a cross-tenant data leak, and the composite FKs in the
schema do not prevent reads. This phase writes the pattern down as code, not as
advice.

## Requirements

- `GET /api/v1/me` returns the authenticated teacher's `id`, `phone`,
  `full_name`, `timezone`, `status`, `created_at`.
- `PUT /api/v1/me` updates `full_name` and `timezone` only. Phone changes are
  out of scope (they would move the login identifier and need re-verification).
- `internal/features/users/` is deleted.
- A documented, testable scoping helper exists and is used by the teacher
  repository, so plans 02–06 have a working reference implementation.
- Swagger regenerated; every remaining operation reflects the new schema.

## Architecture

**Endpoint placement.** `/me` lives on the `teachers` feature, not on `auth`.
Auth owns credentials and sessions; the profile is business data. The existing
`GET /auth/me` (`apps/api/internal/features/auth/routes.go:16`) is removed and
replaced by `GET /api/v1/me` — one canonical place, no duplicate.

**The scoping helper.** `authctx.TeacherID(c)` (added in phase 2) is the only
sanctioned way a handler learns which tenant it is serving. Handlers pass that
id explicitly down to the service and repository as a parameter; it is never
read from a request body, query string, or path segment. Restating why: a
`teacher_id` accepted from the client is an authorization bypass, and it looks
completely ordinary in a diff.

Repository convention, applied to every table with `teacher_id`:

```go
// Every read is scoped to one teacher and skips soft-deleted rows.
// Composite FKs stop cross-teacher writes; only this filter stops
// cross-teacher reads.
func (r *gormRepository) scoped(ctx context.Context, teacherID uuid.UUID) *gorm.DB {
    return database.FromContext(ctx, r.db).Where("teacher_id = ?", teacherID)
}
```

`deleted_at IS NULL` comes free from GORM's `gorm.DeletedAt` field type, but
only on model-based queries — raw SQL and `Table()` queries must add it by
hand. Schema note (j) makes the same point: "mọi truy vấn đọc PHẢI có
`deleted_at IS NULL`".

**Why not RLS now.** Schema note (m) recommends Postgres row-level security
with `current_setting('app.teacher_id')`. It is the stronger guarantee because
it survives a forgotten `WHERE`. It is deferred because it needs a
connection-level session variable set per request, which interacts with GORM's
pooled connections and the transaction manager in ways that deserve their own
phase. Recorded as a pre-launch hardening item, tracked in this plan's Open
Questions.

**Data flow — profile update**

```
PUT /me {full_name, timezone}
  -> requireAuth middleware -> authctx.Principal
  -> handler: teacherID, ok := authctx.TeacherID(c); !ok -> 401
  -> service.UpdateProfile(ctx, teacherID, req)
       -> repo.GetByID(ctx, teacherID)   (deleted_at IS NULL)
       -> apply full_name / timezone
       -> repo.Update
  -> 200 TeacherResponse
```

Timezone is validated against `time.LoadLocation` rather than a hardcoded list
— the column is `VARCHAR(64)` holding an IANA name and the stdlib already owns
that vocabulary.

## Related Code Files

**Create**

- `apps/api/internal/features/teachers/dto.go` — `TeacherResponse`,
  `UpdateProfileRequest`, `FromModel`
- `apps/api/internal/features/teachers/handler.go` — `me`, `updateMe` with
  swagger annotations
- `apps/api/internal/features/teachers/routes.go` — `RegisterRoutes` mounting
  `GET/PUT /me` behind `requireAuth`
- `apps/api/internal/features/teachers/handler_test.go`
- `apps/api/internal/features/teachers/service_test.go`
- `apps/api/internal/features/teachers/integration_test.go`

**Modify**

- `apps/api/internal/features/teachers/service.go` — add `UpdateProfile`
- `apps/api/internal/features/teachers/repository.go` — add the `scoped` helper
  and its package-level doc comment
- `apps/api/internal/features/auth/routes.go` — drop `GET /auth/me` (line 16)
- `apps/api/internal/features/auth/service.go` — drop `Me` (lines 176-178)
- `apps/api/internal/features/auth/handler.go` — drop the `me` handler
- `apps/api/internal/server/router.go` — mount `teachers.RegisterRoutes`
- `apps/api/internal/shared/authctx/authctx.go` — doc comment on `TeacherID`
  explaining the "never from the request" rule
- `docs/` — note the tenancy convention wherever the repo documents backend
  conventions; do not invent a new file if an existing one owns this topic

**Delete**

- `apps/api/internal/features/users/` (all 11 files)

## Implementation Steps

1. Add `dto.go`: `TeacherResponse{ID, Phone, FullName, Timezone, Status,
   CreatedAt}` and `FromModel(acct *Account, t *Teacher) TeacherResponse`.
   `UpdateProfileRequest{FullName string \`binding:"required,min=1,max=100"\`,
   Timezone string \`binding:"required,max=64"\`}`.
2. Add `Service.UpdateProfile(ctx, teacherID uuid.UUID, req
   UpdateProfileRequest) (*TeacherResponse, error)`. Validate the timezone with
   `time.LoadLocation`; return `apperror.Invalid` with a field map on failure,
   matching the convention in `internal/shared/apperror/apperror.go:55`.
3. Add the `scoped` helper to `repository.go` with the doc comment shown under
   **Architecture**. Use it in `GetByID`. It is deliberately small — its job is
   to be the thing plan 02 copies.
4. Add `handler.go` with `me` and `updateMe`. Both start by calling
   `authctx.TeacherID(c)` and return `apperror.Unauthorized` when it reports
   false. Annotate for swag following the style already in
   `apps/api/internal/features/auth/handler.go`.
5. Add `routes.go`:
   `func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth gin.HandlerFunc)`
   mounting `rg.GET("/me", requireAuth, h.me)` and
   `rg.PUT("/me", requireAuth, h.updateMe)`.
6. Remove `GET /auth/me` from `auth/routes.go`, the `me` handler from
   `auth/handler.go`, and `Service.Me` from `auth/service.go:176-178`. Update
   the `RegisterRoutes` doc comment at `auth/routes.go:7-9`, which currently
   claims "only me sits behind requireAuth" — after this change no auth route
   needs the access token.
7. Mount the teachers routes in `internal/server/router.go` inside
   `registerFeatures`.
8. Delete `apps/api/internal/features/users/`. Run `go build ./...` to confirm
   nothing still imports it.
9. Write the tests:
   - `service_test.go`: profile update happy path, invalid timezone rejected,
     unknown teacher → not found.
   - `handler_test.go`: `GET /me` without a token → 401; with a token → 200 and
     the correct body shape.
   - `integration_test.go`: two teachers exist; teacher A's `GET /me` never
     returns teacher B's data; a soft-deleted teacher's token yields 401.
10. Run `make api-docs`, then inspect `apps/api/docs/docs.go` for leftover
    references to email, `admin`, or `/users`.
11. Run `make test-api` and `make lint-api`.

## Success Criteria

- [x] `GET /api/v1/me` with a valid token returns that teacher's profile
- [x] `GET /api/v1/me` without a token returns 401
- [x] `PUT /api/v1/me` updates `full_name` and `timezone`; any other field in
      the body is ignored, not persisted
- [x] An invalid IANA timezone returns 422 with a field-level message
- [x] Teacher A cannot observe teacher B's profile through any endpoint
- [x] A token belonging to a soft-deleted or disabled teacher returns 401
- [x] `apps/api/internal/features/users/` no longer exists and nothing imports it
- [x] `GET /auth/me` returns 404 (route removed, single canonical `/me`)
- [x] `make api-docs` output contains no `admin` role, no email fields, no
      `/users` paths
- [x] `make test-api` and `make lint-api` both pass

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Later plans copy the repository pattern but drop the `teacher_id` filter | High | High — cross-tenant read leak | `scoped()` exists as the copy target and carries the reasoning in its comment; every roster/session plan lists a two-teacher isolation test as an explicit success criterion |
| Frontend still calls `GET /auth/me` and breaks | Medium | Low | The web app is plan 07 and is written against the regenerated spec; noted here so plan 07 does not inherit a stale assumption |
| `PUT /me` mass-assigns `status` or `role` from the request body | Low | High — privilege escalation | `UpdateProfileRequest` carries only two fields; the service maps them explicitly rather than binding onto the model |
| RLS deferral forgotten and never revisited | Medium | Medium | Recorded in the plan's Open Questions as a pre-launch item, not an idea |
| Deleting `features/users` removes a test helper something else needed | Low | Low | `go build ./...` and `make test-api` run before the delete is committed |
