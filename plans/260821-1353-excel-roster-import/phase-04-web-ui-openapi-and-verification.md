---
phase: 4
title: "Web UI, OpenAPI and Verification"
status: completed
priority: P2
effort: "7h"
dependencies: [3]
---

# Phase 4: Web UI, OpenAPI and Verification

## Overview

An owner-only import page inside the existing `roster` feature — download
template, pick file, check, commit — plus the spec regeneration, the docs the
change actually invalidates, and the full gate.

## Key Insights — frontend

Three facts the first draft asserted that are **false**, each of which would
have surfaced only mid-implementation:

- **`ApiError` drops `details`.** The backend does send row errors through
  `response.ErrWithDetails` (`response.go:76-90`), but the web normalizer reads
  only `code`, `message`, `fields` — `ErrorBody` has no `details` field and
  `toApiError` never looks for one (`lib/api/errors.ts:11-29,31-35,52-73`).
  Nothing in `apps/web/src` consumes the envelope's `details` today. So the
  error table has **no data source** until `ApiError`/`toApiError` gain
  `details?: unknown` and the MSW `fail()` builder (`src/test/msw/handlers.ts`)
  can emit it. That is a change to a file every feature depends on — list it,
  budget it, and note it for existing `ApiError` consumers.
- **`useCenter()` gives a member no `is_owner`.** `GET /centers/me` is
  role-shaped: the owner body is `{center: {id, name, is_owner}, members: […]}`,
  the member body is `{center_name}` — and `centerMeSchema` is a
  `z.union` whose own comment says *"the two response bodies share no
  discriminant field, so callers narrow on `\"members\" in data\"`"*
  (`center/schemas/center-schemas.ts:38-49`; `centers/service.go:120-125`).
  Line 8 of that schema file is `is_owner` on a **roster row**, not the
  caller's role, so `member-list.tsx:26,34` is not the precedent it looks like.
  A fixture shaped `{center: {…, is_owner: false}}` matches **neither** union
  branch, so the query errors and a "member sees forbidden state" test would go
  green while rendering the error state. Narrow on `"members" in data`, or add
  an explicit `useIsOwner()` to the center feature's public surface.
- **There is no `/roster` URL prefix.** Roster routes mount bare under the
  protected layout: `contacts`, `contacts/:id`, `students`, `students/:id`,
  `classes/:id/settings` (`features/roster/routes.tsx:10-30`;
  `app/router.tsx:36-46`). The page goes at **`students/import`**, and it needs
  a navigation entry or it is unreachable.

Two that are true and matter:

- Uploads and the template download need axios options nothing in this app uses
  yet: `FormData` (let the browser set the multipart boundary — do **not**
  hand-set `Content-Type`) and `responseType: "blob"`. Both go through
  `apiClient` (`lib/api/client.ts:12`) so the bearer and single-flight refresh
  still apply. With `responseType: "blob"` a failed request puts a `Blob` in
  `err.response.data`, so `extractErrorBody`'s `"error" in data` check fails
  and every template 403 becomes `UNKNOWN_ERROR` — the download path must
  re-read the blob as text on failure.
- `apiClient` sets `timeout: 10_000` (`client.ts:15`). The import will exceed
  it. Both calls need an explicit per-request timeout, reconciled with the
  server's `WriteTimeout: 30s` and the cap measured in Phase 3. Getting this
  wrong is not cosmetic: axios aborts, the handler keeps committing, and the
  owner has no way to tell whether the data landed.

## Requirements

- Functional: `students/import` page — "Tải file mẫu", a `.xlsx` file picker, a
  "Kiểm tra" action (`dry_run=true`) showing either the planned counts or a
  scrollable error table (`Sheet · Dòng · Cột · Lỗi`), and a "Nhập dữ liệu"
  action enabled only after a clean check. Success shows created/reused per
  entity and links to the class list. A nav entry.
- Functional: swag annotations on both handlers; `make api-docs` clean; docs
  updated.
- Non-functional: the commit button re-disables while in flight; changing the
  file clears any previous report.

## Architecture — frontend

```
src/features/roster/
  api/imports-api.ts            # downloadTemplate(), importRoster(file, dryRun)
  hooks/use-roster-import.ts    # useImportRoster() mutation
  schemas/import-schemas.ts     # zod: ImportReport, ImportRowError
  components/import-error-table.tsx
  components/import-report-summary.tsx
  pages/roster-import-page.tsx
  routes.tsx                    # + students/import
```

```ts
// api/imports-api.ts
const IMPORT_TIMEOUT_MS = 60_000;   // overrides apiClient's 10s default

export async function downloadTemplate(): Promise<Blob> {
  const res = await apiClient.get("/imports/roster/template", { responseType: "blob" });
  return res.data;                  // raw blob — deliberately not envelope-unwrapped
}

export function importRoster(file: File, dryRun: boolean) {
  const form = new FormData();
  form.append("file", file);
  form.append("dry_run", String(dryRun));
  // No explicit Content-Type: the browser must set the multipart boundary.
  return apiClient.post("/imports/roster", form, { timeout: IMPORT_TIMEOUT_MS });
}
```

The picked `File` stays in component state for the whole flow, because the
commit re-sends the same bytes. Changing the file clears the report — a stale
"hợp lệ" badge next to a newly picked file is the one dangerous UI bug here.
The template download revokes its object URL after the click.

## Architecture — docs

| File | Change |
|---|---|
| `docs/api-guidelines.md` | Multipart rule: cap the body with `http.MaxBytesReader` **before** `c.FormFile`; validate content by opening the file, never by the client filename; binary responses sit outside the envelope alongside the health probes |
| `docs-vi/codebase-summary.md` | Endpoint count 94 → 96 (`codebase-summary.md:74`) and the new `imports` feature row |
| `docs-vi/system-architecture.md` | The import path in the tenancy section: phone → `MemberIDsByPhone(sc)` → anchor `Scope{IsOwner:false}` → existing `Create`; the FK guard as a **cross-center** backstop only |
| `docs-vi/project-roadmap.md` | Move the feature into "đã ship"; carry the plan's open questions forward |
| `task/import_student_and_class.md` | Resolve plan open question 3 — delete, or reduce to a pointer at the generated template |

No `adr.md` entry and no `CLAUDE.md` line. The first draft budgeted both for a
`CreateFor` seam that no longer exists;
`docs/api-guidelines.md:96-97` already states that owners may write on behalf
of any teacher in their center, so there is no deviation to record.

Swag annotations (`imports/handler.go`):

```go
// @Summary      Tải file Excel mẫu để import lớp và học sinh
// @Tags         imports
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Success      200  {file}  binary
// @Failure      401,403  {object}  response.ErrorBody
// @Security     BearerAuth
// @Router       /imports/roster/template [get]
```

The POST route is `@Accept multipart/form-data` with
`@Param file formData file true "…"`, `@Param dry_run formData bool false "…"`,
`@Success 200 {object} response.Body{data=imports.Report}`,
`@Failure 400,401,403,409,422`.

## Related Code Files

- Create: `apps/web/src/features/roster/api/imports-api.ts`
- Create: `apps/web/src/features/roster/hooks/use-roster-import.ts`
- Create: `apps/web/src/features/roster/schemas/import-schemas.ts`
- Create: `apps/web/src/features/roster/components/{import-error-table,import-report-summary}.tsx`
- Create: `apps/web/src/features/roster/pages/roster-import-page.tsx`
- Create: `apps/web/src/features/roster/__tests__/roster-import-page.test.tsx`
- **Modify: `apps/web/src/lib/api/errors.ts`** (add `details`) — shared by every
  feature
- **Modify: `apps/web/src/test/msw/handlers.ts`** (`fail()` emits `details`)
- Modify: `apps/web/src/features/roster/{routes.tsx,index.ts}`,
  `apps/web/src/features/roster/__tests__/roster-handlers.ts` (exists), and the
  nav component
- Modify: `apps/api/internal/features/imports/handler.go` (annotations),
  `apps/api/docs/` (regenerated)
- Modify: `docs/api-guidelines.md`, `docs-vi/{codebase-summary,system-architecture,project-roadmap}.md`
- Resolve: `task/import_student_and_class.md`

## Implementation Steps (TDD)

### Tests Before
1. `errors.test.ts` (or the existing suite): `toApiError` surfaces `details`
   from an envelope that carries it, and still works on one that does not.
2. `roster-import-page.test.tsx` with MSW (`onUnhandledRequest: "error"`),
   `renderWithProviders`, `signInAs`:
   - a member sees the forbidden state — fixture is the **real** member body
     `{center_name: "…"}`, and the component narrows on `"members" in data`
   - loading / error / empty / data states of the check step
   - a `422` renders every row error with sheet, line and column
   - a clean check enables the commit button; commit shows created/reused
   - picking a new file after a check clears the previous report
   - the commit button is disabled while in flight
3. `import-schemas` parse tests for the report and row-error shapes.

### Refactor
4. `errors.ts` + MSW `fail()` first (shared), then api → schemas → hooks →
   components → page → route → nav entry.
5. Annotate the handlers; `make api-docs`; commit `apps/api/docs/`.
6. Update the four docs files; resolve `task/import_student_and_class.md`.

### Regression Gate
```sh
make lint && make test-api && make test-web && make api-docs   # api-docs must leave no diff
```

No Playwright spec. Generating `.xlsx` fixtures from Node needs an xlsx writer
that is not in `apps/web/package.json`, and the contract is already covered by
the handler tests plus Phase 3's integration round trip. Revisit if the page
grows beyond one button.

## Todo

- [x] `ApiError.details` + MSW `fail()` (shared, done first, gate re-run before the page work)
- [x] Import page with the four states and the two-step flow
- [x] Real member-body fixture, narrowing on `"members" in data`
- [x] `students/import` route + an owner-only entry point (see Outcome 1)
- [x] Swag annotations + regenerated spec committed
- [x] Four docs updated; `task/` file left alone per plan decision 10

## Success Criteria

- [x] Owner completes download → check → commit without leaving the page
- [x] Error table shows sheet, line, column and a Vietnamese message per row
- [x] Member sees the forbidden state, driven by the real member response shape
- [x] `make api-docs` produces no diff on a second run
- [x] Full gate green — `make lint` 0 errors, `make test-api` 74.3% (floor 60%),
      `make test-web` 279/279, `npm run build` clean
- [x] No deep imports across features (only `@/features/center` index)

## Outcome

Four things the plan did not predict, all recorded in `adr.md`:

1. **The nav entry became a page entry.** The sidebar already holds 8 items and
   a 9th would show for members, who always get a 403 from this endpoint. The
   link lives on `students-page.tsx`, gated on `"members" in useCenter().data`.
2. **Blob error decoding had to go in the interceptor, not the api module.** By
   the time `imports-api.ts` sees a failure the interceptor has already turned
   it into an `ApiError`, and `toApiError` is synchronous so it cannot await a
   blob read. The interceptor's error handler is the only place that still has
   `err.response.data` *and* may await. Every future blob endpoint gets the fix
   for free.
3. **`GET /centers/me` needed a default MSW handler.** Once `students-page`
   calls `useCenter()`, `onUnhandledRequest: "error"` reddens every test that
   renders it. The default is the owner body; the member case overrides.
4. **`make lint-web` was already red at HEAD** — 5 files unformatted before this
   phase started. Formatted; format-only diffs.

The swag annotations were already on both handlers from Phase 2, so step 5 was
a regeneration and an idempotency check rather than new work.

Not run: the page has never been driven in a real browser — no Playwright
browser binary is installed on this machine. The plan cut the e2e spec for this
feature deliberately; coverage is 16 vitest cases through the real router, real
axios and real interceptors, plus Phase 3's live HTTP matrix against the API.

## Risk Assessment

- **`errors.ts` is shared by every feature.** Adding an optional field is
  additive, but it is the one change here that can break unrelated tests — do
  it first, on its own, with the gate run before the page work starts.
- **Stale report after a file change** invites committing a file that was never
  checked. Covered by a test.
- **Hand-set multipart `Content-Type`** breaks the boundary and yields a
  confusing `400`. Explicitly not set.
- **Blob error handling** — a failed template download carries a `Blob`, not
  JSON; without the re-read every failure collapses to `UNKNOWN_ERROR`.
- **Timeout mismatch** between axios, `WriteTimeout` and the measured cap is
  the difference between "the import failed" and "the import silently
  succeeded after the UI gave up".

## Security Considerations

The page is a convenience gate; the API's owner check is the real one, so the
member-state test is not the authorization test. The picked file stays in
memory only — never in `localStorage` or a store — since it carries parent
phone numbers and children's names.

## Next Steps

Feature complete. Plan open questions 1 (one parent → two contact rows) and 3
(the `task/` file) remain; open question 2 is answered by Phase 3's
measurement.
