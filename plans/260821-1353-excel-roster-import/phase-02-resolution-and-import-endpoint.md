---
phase: 2
title: "Resolution and Import Endpoint"
status: completed
priority: P1
effort: "6h"
dependencies: [1]
---

# Phase 2: Resolution and Import Endpoint

# Overview

Turn a `ParsedWorkbook` into a fully resolved, cross-checked import plan, and
expose it behind one owner-only endpoint plus a template download. This phase
delivers the `dry_run=true` path end to end; Phase 3 adds the writes behind the
same route.

## Key Insights

- **This is the API's first multipart surface.** `grep -rn "multipart\|FormFile"
  internal/` returns nothing. Get the limit ordering right: wrap the body in
  `http.MaxBytesReader` **before** calling `c.FormFile`, otherwise Gin buffers
  the whole upload to disk before any cap applies.
- **Never trust the filename.** `multipart.FileHeader.Filename` is
  attacker-controlled. Content is validated by opening the workbook and finding
  both named sheets; an extension check is UX, not a trust boundary. Never echo
  the filename into a response header.
- The owner gate is a **service-level** check matching `requireOwner`
  (`invitations/service.go:117-123`). It runs **before** parsing so the
  endpoint is not a parser oracle for members. There is no `RequireOwner`
  middleware in this codebase; do not add one for a single feature.
- The template download must **not** go through `response.OK` — it is a binary
  stream, like the health probes are outside the envelope. Use
  `c.Data(200, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", b)`
  with a fixed `Content-Disposition` filename.
- Resolution is a two-pass job: Sheet 1 first (it defines the classes Sheet 2
  refers to), then Sheet 2 against the resolved class map. A `HocSinh` row
  naming a class absent from `Lop` is a row error — the import never reaches
  into pre-existing classes by name, because class names are not unique per
  center and picking one would be a guess.
- `centers.Service` is a **process-lifetime singleton** shared by every request
  through `middleware.ResolveScope` (`router.go:100`). `MemberIDsByPhone` must
  stay a query-per-call; a per-center map cached on that struct would be a
  cross-tenant leak with a very quiet diff.

## Requirements

- Functional:
  - `GET /imports/roster/template` → the `.xlsx` from `BuildTemplate()`.
  - `POST /imports/roster` → `multipart/form-data` with `file` and `dry_run`;
    parses, resolves, cross-checks. With `dry_run=true` it returns a `Report`
    with `committed:false` and writes nothing.
  - Owner-only on both routes.
  - `centers.Service.MemberIDsByPhone(ctx, sc) (map[string]uuid.UUID, error)`.
- Non-functional: 2 MB request cap; no cell content in logs.

## Architecture

```
internal/features/imports/
  service.go    # Import(ctx, sc, b []byte, dryRun bool) (*Report, error)
  resolve.go    # resolution + cross-sheet checks
  dto.go        # Report, ReportEntity, JSON shapes
  handler.go    # multipart bind → service → envelope
  routes.go
```

```go
// centers/service.go — the new directory method.

// MemberIDsByPhone returns the caller's center's phone → teacher_id directory,
// keyed by the E.164 storage form. The scope parameter is the authorization
// check itself: there is no way to ask for another center's directory, and no
// separate "is this teacher mine?" check exists that could be forgotten.
// Removed teachers are absent by construction (ListMembers joins user_accounts
// on status = active).
//
// Callers must not report the difference between "not a member here" and "no
// such account" — that distinction is an account-enumeration oracle.
//
// This runs on a process-lifetime singleton that also backs ResolveScope on
// every authenticated request: it must stay a query per call. Never memoize.
func (s *Service) MemberIDsByPhone(ctx context.Context, sc authctx.Scope) (map[string]uuid.UUID, error) {
    rows, err := s.repo.ListMembers(ctx, sc.CenterID)
    if err != nil {
        return nil, err
    }
    out := make(map[string]uuid.UUID, len(rows))
    for _, r := range rows {
        p := validation.NormalizePhone(r.Phone)
        if prev, dup := out[p]; dup && prev != r.ID {
            // uq_users_phone should make this unreachable; if it happens the
            // anchor would be silently non-deterministic, so fail loudly.
            return nil, apperror.Internal(fmt.Errorf("center %s has two members with phone %s", sc.CenterID, p))
        }
        out[p] = r.ID
    }
    return out, nil
}
```

One query regardless of how many phones the workbook names; a center's member
count is small by construction (`uq_center_members_active` — one live
membership per teacher). The signature takes `authctx.Scope`, **not a raw
`centerID`** — every other member-data method on this service does
(`Me:116`, `Rename:141`, `RemoveMember:164`), and a raw-uuid signature would
let any caller dump any center's phone directory.

Consumer-declared interface (imports declares it; `*centers.Service`
satisfies it, wired in `server.registerFeatures` after line 127):

```go
// service.go
type MemberDirectory interface {
    MemberIDsByPhone(ctx context.Context, sc authctx.Scope) (map[string]uuid.UUID, error)
}
```

Resolution steps, in order, each accumulating into one error list:

1. **Owner gate.** Non-owner → `403` immediately, before parsing.
2. **Teacher resolution.** One `MemberIDsByPhone` call, then map each row's
   phone. An empty cell resolves to `sc.TeacherID` (the owner). An unresolved
   phone is a row error on **every** line that used it — the operator fixes one
   cell and re-uploads once.
3. **Class grouping.** Group `Lop` rows by `(teacherID, name)`. Within a group,
   `start_date`, `unit_price` and `end_date` must be identical across rows — a
   mismatch is a row error naming the first differing line, because the group
   is one class and the file is claiming two different truths about it. Each
   row in the group contributes one schedule.
4. **Duplicate schedule check.** Two rows in a group with the same
   `(weekday, start_time)` → row error.
5. **Sheet 2 → class.** Look up `(resolved teacherID, class name)` in the group
   map; miss → row error.
6. **Enrollment date.** `started_on` blank → the class's `start_date`.
   `started_on < class.start_date` → row error: `started_on` means "on the
   roster from", and `ActiveOn` filters `started_on <= session_date`
   (`enrollments/repository.go:182-193`), so an earlier date would silently
   bill sessions the student never attended. Early registration is recorded as
   the class start date, so no legitimate case is lost.
7. **Twin ambiguity.** Two `HocSinh` rows identical on
   `(contact phone, student name, class)` with the same or empty
   `display_note` → row error naming both lines.
8. **Contact name conflict.** The same phone appearing with two different
   parent names under the same teacher → row error; the phone is the identity
   key and the file is ambiguous about who owns it.

Error catalogue owned by this phase (Phase 1 owns `MISSING_REQUIRED`,
`BAD_FORMAT`, `TOO_LONG`; Phase 3 owns the three existence codes):

| Code | Meaning |
|---|---|
| `TEACHER_NOT_IN_CENTER` | phone does not resolve to a member of the caller's center |
| `CLASS_NOT_IN_FILE` | `HocSinh` names a class absent from `Lop` |
| `CLASS_FIELD_MISMATCH` | rows of one class group disagree on a class-level field |
| `DUPLICATE_SCHEDULE` | same weekday+time twice in one class group |
| `AMBIGUOUS_STUDENT` | indistinguishable duplicate student rows |
| `CONTACT_NAME_CONFLICT` | one phone, two parent names, same teacher |
| `ENROLL_BEFORE_CLASS_START` | `started_on` earlier than the class start date |

`TEACHER_NOT_IN_CENTER`'s message stays center-relative — *"số điện thoại này
không thuộc trung tâm của bạn"*. It must **never** report that the phone exists
elsewhere; that is an account-enumeration oracle and it is the exact wording a
future "better error message" change will reach for.

Report shape:

```go
type ReportEntity struct {
    Created int `json:"created"`
    Reused  int `json:"reused"`
}
type Report struct {
    Committed   bool         `json:"committed"`
    Classes     ReportEntity `json:"classes"`
    Schedules   ReportEntity `json:"schedules"`
    Contacts    ReportEntity `json:"contacts"`
    Students    ReportEntity `json:"students"`
    Enrollments ReportEntity `json:"enrollments"`
}
```

The `Reused` counts are only meaningful once Phase 3's existence lookups exist.
Until then `dry_run` returns `Created` totals and zeroed `Reused`; Phase 3
wires the lookups into **both** paths so the dry run and the commit report the
same split. A dry run that always says "tạo mới" would train the operator to
ignore the one number that tells them a re-import was a no-op.

Errors never travel in `Report` — they go out as
`response.ErrWithDetails(c, apperror.Invalid("file có dòng không hợp lệ", nil),
gin.H{"errors": rowErrors})`.

Handler skeleton:

```go
func (h *Handler) importRoster(c *gin.Context) {
    sc, ok := h.scope(c)
    if !ok { return }
    b, dryRun, appErr := readUpload(c)   // MaxBytesReader → FormFile → io.ReadAll
    if appErr != nil { response.Err(c, appErr); return }
    rep, err := h.svc.Import(c.Request.Context(), sc, b, dryRun)
    if err != nil { response.Err(c, err); return }
    response.OK(c, http.StatusOK, rep)
}
```

`readUpload` caps the body at 2 MB, requires the `file` field, parses
`dry_run`, and maps an oversize body to
`apperror.BadRequest("file vượt quá 2 MB")`.

Rate limit: `middleware.RateLimit` keyed on the caller's teacher id on the POST
route. Today `RateLimit` is used at exactly four sites, all unauthenticated
(`router.go:110-111,198-199`); nothing throttles an authenticated owner, and
this endpoint is the most expensive one in the product.

## Related Code Files

- Create: `apps/api/internal/features/imports/{service,resolve,dto,handler,routes}.go`
- Create: `apps/api/internal/features/imports/{service_test,resolve_test,handler_test}.go`
- Modify: `apps/api/internal/features/centers/service.go` (+ its `service_test.go`)
- Modify: `apps/api/internal/server/router.go` — construct the imports service
  **after line 127** (where `contactsSvc`, `classesSvc`, `enrollmentsSvc`,
  `studentsSvc`, `txMgr` and `centersSvc` are all already in scope). One router
  edit for both this phase and Phase 3.

## Implementation Steps (TDD)

### Tests Before
1. `centers/service_test.go`: `MemberIDsByPhone` keys by the E.164 storage form
   and a `0…` workbook phone finds it after `NormalizePhone`; a phone from
   another center is absent; a disabled member is absent; two members sharing a
   normalized phone returns an error rather than a silent last-wins.
2. `resolve_test.go` (pure, fake directory): each of the seven codes has a
   case; the happy path resolves the example workbook to 2 class groups /
   3 schedules / 3 contacts / 3 students / 3 enrollments; an empty teacher
   phone resolves to the owner; one bad phone used on four lines produces four
   errors.
3. `service_test.go`: non-owner → `Forbidden` before any parsing (assert the
   parser was not called).
4. `handler_test.go` (real router slice): `403` for a member on both routes;
   `401` without a token; template route returns the xlsx content type and a
   non-empty body **outside** the envelope; a 3 MB upload is rejected; a
   non-workbook upload is rejected on content, not extension; a valid file with
   `dry_run=true` returns `200` `committed:false`; an invalid file returns
   `422` with `details.errors` as a non-empty array.

### Refactor
5. Implement `MemberIDsByPhone`, then `resolve.go` → `service.go` → `dto.go` →
   `handler.go` → `routes.go`; wire in `router.go`.

### Regression Gate
```sh
cd apps/api && go test -short ./internal/features/imports/ ./internal/features/centers/ && make test-api && make lint-api
```

## Todo

- [ ] `centers.MemberIDsByPhone` (scope-typed, no memoization)
- [ ] `readUpload` with the cap applied before `FormFile` and content-based
      type validation
- [ ] Seven-code resolution with full error accumulation
- [ ] Owner gate before parse; per-teacher rate limit
- [ ] Router wiring

## Success Criteria

- [ ] `dry_run` on the example workbook returns the expected counts, zero writes
- [ ] Every error code has a test and a Vietnamese operator-facing message
- [ ] `TEACHER_NOT_IN_CENTER` message is center-relative (asserted in test)
- [ ] Member gets `403` on both routes, before parsing
- [ ] A 3 MB body and a non-workbook body are both rejected

## Risk Assessment

- **Cap ordering.** `c.FormFile` before `MaxBytesReader` buffers the whole
  upload first; the cap then protects nothing. The 3 MB handler test is the
  guard.
- **Signature drift on `MemberIDsByPhone`.** If a later refactor swaps
  `authctx.Scope` for a raw `centerID` "for reuse", the authorization check
  disappears silently. The doc comment says why; a reviewer should treat that
  change as a security change.
- **Class-name collision with pre-existing data** is deliberately not resolved
  here — Sheet 1 is the only source of classes for Sheet 2. Phase 3 decides
  what happens when a Sheet-1 class already exists.

## Security Considerations

Owner gate before parse. Content-based file validation, never the
client-supplied filename, and the filename is never echoed back. No cell
contents in logs. The resolution error surface is center-relative to avoid
account enumeration. Body cap + row cap + per-teacher rate limit bound the work
an authenticated owner can trigger — the pool is 25 connections shared across
every tenant (`config.go:47`), so an unthrottled expensive endpoint is a
cross-tenant availability risk, not just a slow request.

## Next Steps

Phase 3 turns the same resolution into writes behind `dry_run=false`.
