---
phase: 4
title: "Empty import guard"
status: done
priority: P2
effort: "2h"
dependencies: []
---

# Phase 4: Empty import guard

## Overview

Stop the "successfully imported nothing" failure mode from
`plans/reports/debug-260821-2255-import-empty-workbook.md`: a workbook that
parses to zero data rows must be rejected loudly instead of returning a
success report with all-zero counts. Independent of Phases 1–3.

## Requirements

- Functional:
  - API: in `imports.Service`, after parse+resolve produce a plan with zero
    classes AND zero students → return 422 with a new error code and message
    "file không có dòng dữ liệu nào — nhập từ dòng 3 trở đi". Applies to both
    dry-run (check) and commit, before any transaction is opened.
  - Web: belt-and-braces — if a report ever arrives with every count zero,
    `import-report-summary.tsx` renders a warning card instead of the
    "Đã nhập xong" success header (covers old clients/API drift).
  - Import-page copy: note that row 2 (example row) is ignored and data must
    start at row 3, and that "SĐT giáo viên" may be left blank (class goes to
    the owner).
- Non-functional: keep the positional row-2 skip unchanged (content-matching
  the example row was considered and rejected — see the debug report's
  trade-off note); whole-file atomicity semantics untouched.

## Architecture

Guard lives in the service layer (not the parser): the parser legitimately
returns empty slices; only the service knows the request is an import with
nothing to do. Reuse the existing typed-error path that produces 422 row/file
errors (same shape the UI already renders), adding one file-level error code
(e.g. `CodeEmptyFile`) in the imports error catalog.

## Related Code Files

- Modify: `apps/api/internal/features/imports/service.go` (guard before tx;
  both check and commit paths)
- Modify: `apps/api/internal/features/imports/errors.go` (or wherever import
  error codes live — scout first) + swagger annotations if the response shape
  gains a code
- Modify: `apps/api/internal/features/imports/service_test.go` (empty workbook
  → 422 on check and on commit; nothing written)
- Modify: `apps/web/src/features/roster/components/import-report-summary.tsx`
  (all-zero-counts warning state + test)
- Modify: `apps/web/src/features/roster/pages/roster-import-page.tsx` (page
  copy: data from row 3, teacher phone optional)

## Implementation Steps

1. Scout the imports error catalog for the code/message pattern; add the
   empty-file code.
2. Add the zero-plan guard in `Service` for both dry-run and commit; unit
   tests with an empty workbook fixture (header + example row only).
3. Web: all-zero warning state in `import-report-summary.tsx` + copy tweaks
   on the import page; update component tests.
4. `go build ./... && go test ./...` (apps/api), `tsc -b --noEmit` + web
   suite.

## Success Criteria

- [ ] Uploading a blank template (header + example row only) returns 422 with
      the Vietnamese guidance message on both "Kiểm tra file" and "Nhập dữ liệu".
- [ ] No transaction is opened for an empty plan (no lock taken, nothing written).
- [ ] UI never shows the success header for an all-zero report.
- [ ] Import page copy mentions row-3 start and optional teacher phone.
- [ ] API + web suites green.

## Risk Assessment

- False positive: a legitimate workbook that only *reuses* existing entities
  still has data rows, so it produces non-empty plan entries (reused counts) —
  the guard keys off parsed rows, not created counts; assert this in tests.
- Error-shape drift: reuse the existing 422 envelope so the import page's
  current error rendering works without new UI plumbing.
