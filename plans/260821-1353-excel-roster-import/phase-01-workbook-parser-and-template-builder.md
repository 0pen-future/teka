---
phase: 1
title: "Workbook Parser and Template Builder"
status: completed
priority: P1
effort: "5h"
dependencies: []
---

# Phase 1: Workbook Parser and Template Builder

## Overview

New `internal/features/imports/` slice containing everything that touches
`.xlsx` bytes and nothing that touches the database: the column contract, the
template writer, the sheet parser, and the cell coercion rules. Pure functions,
fully unit-testable, no Docker.

## Key Insights

- **Read every cell as a string.** `excelize.GetCellValue` returns the *display*
  string. Typed getters re-interpret values against the workbook's locale,
  which is how `01/09/2025` becomes September 1st on one machine and January
  9th on another, and how a phone in a *Number*-formatted cell arrives as
  `912345678`. All coercion happens in our own `coerce.go`.
- **Use the streaming `Rows()` iterator, not `GetRows`.** `GetRows`
  decompresses the worksheet and returns the entire sheet as `[][]string`
  *before* any row count is available, so a row cap applied afterwards protects
  nothing. Open with
  `excelize.OpenReader(r, excelize.Options{UnzipSizeLimit: 32 << 20, UnzipXMLSizeLimit: 8 << 20})`
  and abort at row `MaxRowsPerSheet+1`. A 1.9 MB workbook of repeated markup
  decompresses to hundreds of MB otherwise, and the byte cap in Phase 2 does
  not see it.
- Rows are **ragged** — trailing empty cells are dropped, so `row[7]` panics on
  a row where the operator left the last column blank. Every read goes through
  a bounds-checked `cell(row, idx)` helper returning `""`.
- **The services validate nothing.** Every constraint on the four
  `CreateRequest` DTOs is a gin `binding` tag executed at bind time; the
  imports service calls those services directly and gets none of it. Two
  concrete consequences this package must absorb:
  - `classes/service.go:52` dereferences `*req.DefaultUnitPrice`, `:97`
    dereferences `*sr.Weekday`. The parser must always populate them.
  - Length caps are otherwise enforced only by Postgres `VARCHAR`, i.e. as a
    `22001` mid-transaction that rolls the whole import back with no line
    number — the opposite of this feature's purpose.
- **Names must be trimmed and NFC-normalized.** Postgres `=` on `VARCHAR` is a
  byte comparison. macOS Excel writes NFD (`Toa´n`), the web UI writes NFC
  (`Toán`); unnormalized they are two different classes and every idempotency
  key in Phase 3 misses. Nothing in this repo normalizes today
  (`golang.org/x/text` is an indirect dependency only).
- Money and dates are rejected, never guessed: `150.000`, `150,000`, `150k`,
  `1.5e5` are all errors. Only `^\d+$` after trimming spaces and NBSP (` `,
  which Excel inserts in some locales).
- `validation.NormalizePhone` (`validation.go:55-60`) is a two-line prefix
  swap: `0…` → `+84…`, everything else passes through **unchanged**. It does
  not canonicalize `84912345678` or `+840912345678` and it does not validate.
  The parser must reject anything that is not `0…` or `+84…` before calling it.

## Requirements

- Functional: `BuildTemplate() ([]byte, error)` produces a 2-sheet workbook
  with header row, one example row per sheet, all data columns pre-formatted as
  Text (`@` number format), and a frozen header pane.
  `ParseWorkbook(b []byte) (*ParsedWorkbook, []RowError)` returns raw, coerced,
  **unresolved** rows plus every row-level error found. Parsing never stops at
  the first error — the operator must see all of them at once.
- Non-functional: no database, no network, no `authctx`. `MaxRowsPerSheet =
  500` (a stand-in — see plan open question 2), enforced by the streaming
  iterator.

## Architecture

```
internal/features/imports/
  columns.go    # sheet + column names, header rows, weekday map, length caps
  template.go   # BuildTemplate() []byte
  parser.go     # ParseWorkbook([]byte) (*ParsedWorkbook, []RowError)
  coerce.go     # parseVNDate, parseMoney, parseWeekday, parseHHMM, parseDuration, cleanPhone, cleanName
  errors.go     # RowError + error codes
```

`columns.go` is the only place a column name or a length cap appears.
`template.go` writes headers from it and `parser.go` verifies against it, so
the two cannot drift — which is why no template↔parser round-trip test is
needed.

```go
// columns.go
const (
    SheetClasses    = "Lop"
    SheetStudents   = "HocSinh"
    MaxRowsPerSheet = 500
)
// Caps mirror the binding tags on the four CreateRequest DTOs, which mirror
// the VARCHAR widths. Pinned by a test against those tags.
const (
    maxClassName   = 100  // classes/dto.go:31
    maxFullName    = 100  // students/dto.go:14, contacts/dto.go:11
    maxDisplayNote = 50   // students/dto.go:16
)
var classHeaders = []string{
    "Tên lớp", "SĐT giáo viên", "Ngày khai giảng (dd/mm/yyyy)",
    "Đơn giá/buổi (đồng)", "Thứ (2-7 hoặc CN)", "Giờ bắt đầu (HH:MM)",
    "Thời lượng (phút)", "Ngày kết thúc (dd/mm/yyyy)",
}
var studentHeaders = []string{
    "Họ tên học sinh", "Họ tên phụ huynh", "SĐT phụ huynh",
    "Tên lớp", "SĐT giáo viên", "Ngày nhập học (dd/mm/yyyy)", "Ghi chú phân biệt",
}
```

`Đơn giá riêng` is **not** a column (see the plan's *Scope cuts*).
`SĐT giáo viên` is optional on both sheets; empty means "the importing owner".

```go
// parser.go — every field is coerced but not yet resolved to a uuid.
// Line is the 1-based worksheet row, for error messages.
type ClassRow struct {
    Line         int
    Name         string      // trimmed, NFC
    TeacherPhone string      // E.164, "" when blank
    StartDate    time.Time
    UnitPrice    int64
    Weekday      int16
    StartTime    string      // "HH:MM"
    DurationMin  int16       // defaults to 90
    EndDate      *time.Time
}
type StudentRow struct {
    Line         int
    StudentName  string      // trimmed, NFC
    ContactName  string      // trimmed, NFC
    ContactPhone string      // E.164
    ClassName    string      // trimmed, NFC
    TeacherPhone string      // E.164, "" = owner
    StartedOn    *time.Time
    DisplayNote  string      // "" means unset; becomes NULL in storage
}
type ParsedWorkbook struct {
    Classes  []ClassRow
    Students []StudentRow
}
```

```go
// errors.go — one shape for every failure the operator can fix in Excel.
type RowError struct {
    Sheet   string `json:"sheet"`
    Line    int    `json:"line"`             // matches Excel's row number
    Column  string `json:"column,omitempty"` // header text, so the UI can say "cột X"
    Code    string `json:"code"`
    Message string `json:"message"`          // Vietnamese, operator-facing
}
```

Codes owned by this phase: `MISSING_REQUIRED`, `BAD_FORMAT`, `TOO_LONG`.
The resolution codes live in Phase 2.

Weekday: `"CN"` → `0`, `"2".."7"` → `1..6`; anything else is a row error.
The table lives in `columns.go` next to the headers.

The example row is simply the second row of each sheet, skipped by index.
The first draft used a `#`-prefix escape convention; a positional skip needs no
in-band channel and no parser rule.

## Related Code Files

- Create: `apps/api/internal/features/imports/{columns,template,parser,coerce,errors}.go`
- Create: `apps/api/internal/features/imports/{template_test,parser_test,coerce_test}.go`
- Modify: `apps/api/go.mod`, `go.sum` (`go get github.com/xuri/excelize/v2`,
  BSD-3-Clause), `golang.org/x/text` promoted from indirect for NFC

## Implementation Steps (TDD)

### Tests Before
1. `coerce_test.go` — table-driven, one case per rejection rule:
   `150.000`/`150,000`/`150k`/`-1`/`1.5` rejected; `150000`, `" 150000 "`,
   NBSP-padded accepted; `31/02/2025` rejected; `1/9/2025` accepted;
   `2025-09-01` rejected (wrong format, not silently accepted); `CN`→0, `2`→1,
   `8` rejected; `18:00` accepted, `18:0`/`25:00` rejected; blank duration →
   90; `84912345678` and `+840912…` rejected, `0912345678` and `+84912345678`
   both → `+84912345678`; **NFD `Toa´n` and NFC `Toán` both coerce to the same
   bytes**; a 101-char name → `TOO_LONG`; a 51-char note → `TOO_LONG`.
2. `columns_test.go` — the three length caps equal the `binding:"max=…"` values
   on `classes/dto.go`, `students/dto.go`, `contacts/dto.go`. This is the pin
   that fails if a DTO cap changes.
3. `parser_test.go` — build fixtures **in the test** with excelize (no binary
   fixtures in git):
   - happy path: the 3+3 example rows parse to 3 `ClassRow` + 3 `StudentRow`
   - the example row (row 2) is skipped
   - missing sheet / renamed header / reordered header → whole-file error
   - ragged row (blank trailing cells) does not panic
   - 501 rows → `MaxRowsPerSheet` error, and the iterator stops rather than
     materializing the sheet
   - a highly-compressed oversized sheet trips `UnzipXMLSizeLimit` and returns
     a whole-file error, not an OOM
   - **all** row errors are returned, not just the first
   - every `ClassRow` has a non-zero `Weekday` and `UnitPrice` field set, so
     the pointer fields Phase 3 builds can never be nil
4. `template_test.go` — `BuildTemplate()` output opens, has both sheets, and
   its header rows equal `columns.go`.

### Refactor
5. Implement `columns.go` → `coerce.go` → `errors.go` → `parser.go` →
   `template.go`.

### Regression Gate
```sh
cd apps/api && go test -short ./internal/features/imports/ && make lint-api
```

## Todo

- [ ] `go get github.com/xuri/excelize/v2`; promote `golang.org/x/text`
- [ ] columns/coerce/errors/parser/template
- [ ] Unit tests green, including the DTO-cap pin and the NFC cases

## Success Criteria

- [ ] Every coercion rejection rule has a test
- [ ] Length caps are pinned against the DTO binding tags
- [ ] NFD and NFC spellings of the same name coerce identically
- [ ] The row cap and the unzip limits are enforced by the iterator, provably
      before the sheet is materialized
- [ ] Package has zero imports from `database`, `authctx`, or any other feature

## Risk Assessment

- **Excel locale mangling** is the main correctness risk and the reason for
  string-only reads plus Text formatting in the generated template. The
  template is the mitigation: an operator who starts from our file cannot hit
  the `mm/dd/yyyy` trap.
- **excelize is a new dependency** (BSD-3-Clause) and it is a zip + XML parser
  reachable from an authenticated HTTP route. Confine it to this package — pin
  the version, and nothing outside `imports/` imports it.
- **Normalization is easy to under-do.** NFC alone is not enough if the caps
  are applied before trimming; order is trim → NFC → cap.

## Security Considerations

Nothing here reads a scope or writes a row, so the surfaces are resource
exhaustion (row cap + unzip limits) and parser input handling. Do not log cell
contents on error — the sheets carry parent phone numbers and children's names.

## Next Steps

Phase 2 consumes `ParsedWorkbook` and resolves names/phones to ids.
