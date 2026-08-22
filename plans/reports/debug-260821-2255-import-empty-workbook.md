# Debug Report — "Nhập dữ liệu" writes nothing (roster import)

**Symptom:** Owner clicks "Nhập dữ liệu" on `/students/import`, gets the success screen, but no
classes/students appear. Environment: local homelab prod stack (API `teka-api:local` @ `0dca1af`,
DB `teka-db:5432/teka`).

## Executive Summary

The commit request succeeded (HTTP 200, `committed: true`) but the uploaded workbook contributed
**zero data rows**, and the import pipeline has **no guard against an empty plan** — so the server
"successfully imported nothing" and the UI rendered "Đã nhập xong" for a no-op. The parser skips
**row 2 of every sheet unconditionally by position** (reserved for the template's example row), so a
file whose data starts on row 2, or a blank template with only header + example, both parse to zero
rows and sail through check *and* commit cleanly.

## Evidence

1. **API logs (2026-08-21, 15:20–15:23 UTC):** template GET at 15:20:00 (200), then seven
   `POST /api/v1/imports/roster`, **all 200** — six at 6–9 ms, one at 92 ms (15:22:55, the real
   commit: only a non-dry-run pays the WAL-fsync on commit). No 4xx ever → no row errors were
   returned; every uploaded workbook resolved cleanly.
2. **DB is the one the API writes** (`API_DATABASE_URL` host/db verified: `teka-db:5432/teka`,
   credentials redacted). Queries there show **0 rows created on 2026-08-21** in `classes`,
   `students`, `contacts`, `enrollments`; latest `created_at` everywhere is 2026-08-13.
3. **"Everything already existed" ruled out:** classes named `Toán 9A` / `Văn 8` and the student
   names from the candidate workbook exist **nowhere** in the DB — so the report cannot have said
   "reused"; the plan was empty.
4. **Timeline:** first POST came **33 seconds** after the template download — not enough time to
   fill in data. The upload was almost certainly the just-downloaded blank template (header row +
   example row only), or a file with data typed into row 2.
5. **Code path** (all confirmed by reading source):
   - `parser.go:199-203` — `case exampleRow: continue`: row 2 skipped by position, no content check.
     A blank template therefore yields `[]rawRow{}` for both sheets; data typed on row 2 is
     silently swallowed.
   - No empty-plan guard exists in `parser.go`, `resolve.go`, `apply.go`, or `service.go` — an
     all-empty workbook is a "clean" file.
   - `service.go:186` — `rep.Committed = !dryRun` regardless of whether anything was written; the
     commit of an empty plan returns 200 with every entity `{created: 0, reused: 0}`.
   - `import-report-summary.tsx` — header shows "Đã nhập xong / Dữ liệu đã được ghi vào hệ thống"
     purely off `committed`, even when all ten numbers are zero.

## Root Cause

Uploaded workbook parsed to **0 data rows** (row-2 positional skip + data not starting at row 3),
and the pipeline treats an empty plan as a valid import: dry-run reports "File hợp lệ", commit
returns success. The system did exactly what it was told — import nothing — and told the operator
it succeeded.

## Contributing factor found on disk

`mau-nhap-du-lieu-trung-tam.xlsx` (repo root, dated 2026-08-17, predates the feature) is **not** a
valid import file even though its `Lop`/`HocSinh` headers match: its row 2 duplicates the template
example (skipped), and its data rows use the example teacher phone `0912345678`, which is not a
center member — uploading it would 422 on teacher-phone resolution. The all-200 log rules it out as
the file actually uploaded.

## Recommendations (not applied — awaiting go-ahead)

1. **Server guard (primary fix):** in `Service.Import`, reject a plan with zero classes *and* zero
   students with a 422 (`"file không có dòng dữ liệu nào — nhập từ dòng 3 trở đi"`). This fixes
   check and commit at once and matches the whole-file-atomicity philosophy.
2. **UI guard (belt-and-braces):** if every count is zero, render a warning card instead of the
   success header.
3. **Operator guidance now:** fill data starting at **row 3** (keep header + example rows); leave
   "SĐT giáo viên" blank to assign the owner, or use a real member's phone.
4. Optional docs tweak: the page copy "Giữ nguyên tên sheet và dòng tiêu đề" should also say the
   example row (row 2) is ignored and data must start at row 3.

## Addendum (22:42) — "số điện thoại này không thuộc trung tâm của bạn"

After moving data to row 3+, the user hit `CodeTeacherNotInCenter`. Investigated; **no code change
needed** — teacher phone is already optional by design:

- `resolve.go` `resolveTeacher`: blank cell → class assigned to the importing owner (documented in
  the function comment; also `parser.go` field docs). The error fires only for a *non-blank* phone
  absent from the member directory.
- Normalization is consistent both sides (workbook → E.164 via `parsePhone`; directory keys E.164,
  covered by `TestMemberIDsByPhoneKeysOnStorageForm`), so a real member's phone does match.
- DB check: the template example phone `0912345678` belongs to **no** account (0/6) — the user's
  file carried the example phone into its data rows, which is exactly the input this error exists
  to reject.

Resolution for operator: leave "SĐT giáo viên" blank (class goes to the owner) or enter the phone
of a teacher already added as a center member. Possible UX follow-up (optional): mention in the
import-page copy or template header that the column may be left blank.

## Addendum 2 (22:45) — request: blank teacher phone should mean "no teacher yet"

User wants blank = class has *no* teacher; owner assigns one later. Scoped the change:

- `classes.teacher_id` is **NOT NULL** (live schema verified) and is the row's tenant/authz anchor.
  `TeacherID` threads through ~20 features: session generation, attendance, billing runs,
  statements, zalo notifications, teaching, collections. Schedules carry their own copy. A NULL
  teacher breaks the anchor model: no session generation scope, no attendance owner, no billing
  attribution — "nullable teacher_id" is a schema + cross-feature redesign, not an import tweak.
- The real gap is different: **teacher reassignment does not exist**. `UpdateClassRequest`
  (`classes/dto.go:40`) edits only name/dates/price; no endpoint or UI changes a class's teacher.
  So even today, a class imported under the owner cannot be handed to a teacher later.
- Current import semantic (blank → owner) is the deliberate NOT-NULL-compatible proxy for
  "unassigned": the owner is a teacher too and is the natural custodian until handoff.

Recommendation: keep `teacher_id` NOT NULL and blank→owner; build **teacher reassignment**
(API: extend class update or dedicated sub-resource; UI: class settings). Reassignment must decide
what moves with the class (future sessions/schedules yes, past attendance no). This delivers the
user's operational goal at a fraction of the blast radius. Feature work → route through
brainstorm/plan, not a debug fix. Awaiting user decision.

## Trade-off note on the positional skip

Skipping row 2 by content-match (only when it equals the example values) would rescue data typed on
row 2, but couples the parser to the template's literal example strings and breaks silently when the
example changes. Keeping the positional skip + adding the empty-plan guard (rec 1) is the smaller,
safer change: the failure becomes loud instead of silent, which is all that is needed.
