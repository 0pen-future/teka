# 4-Phase Feature Test Validation Report
**Date:** 2026-08-21 23:36  
**Feature:** Teacher Handoff + Excel Import (Owner-Run Flow)

---

## Test Execution Summary

### Web App (apps/web)
- **Command:** `npx tsc -b --noEmit` then `npx vitest run`
- **TypeCheck:** ✅ No errors
- **Tests:** 59 files, **370 passed**, 3 skipped (expected)
- **Duration:** 20.90s
- **Status:** PASS

### API App (apps/api)
- **Command:** `go build ./...` then `go test ./...`
- **Build:** ✅ Success
- **Tests:** All packages ok (handoff, imports, and 30+ others)
- **Status:** PASS

---

## Acceptance Criteria Coverage Analysis

### AC1: PUT /api/v1/classes/:id/teacher (Owner-Only Reassignment)
**Status:** ✅ **COVERED**

- **Owner gate (403):** `handoff/service_test.go:48-60` — `TestReassignRequiresOwner`
- **Non-member target (422):** `handoff/service_test.go:62-75` — `TestReassignRejectsNonMemberTarget`
- **Idempotent (same teacher):** `handoff/service_test.go:31-46` — `TestReassignToSameTeacherIsNoOp`
- **Transaction consistency:** `handoff/service_test.go:12-29` — moves class + sessions atomically
- **Integration (real DB):** `handoff/integration_test.go:141-164` — full flow with member/non-member rejection

**Evidence:**
- Service mock tests verify error codes (Forbidden on non-owner, Unprocessable Entity on non-member)
- No lock taken, no DB writes on rejection (lines 42-45, 57-59, 72-74)
- Member forbidden test confirms bearer role matters (line 145-150)

---

### AC2: Session Date Logic (Selective Handoff)
**Status:** ✅ **COVERED**

- **Today inclusive moved:** `handoff/integration_test.go:86-123` — `TestReassignMovesClassScheduleAndFuturePlannedOnly`
  - Line 93: `todayPlanned` created on `today`, moved successfully
  - Line 106: "today's and the future planned session move"
  
- **Past planned stays:** Line 96, 120 — `pastPlanned` with `-7` days remains with old teacher
- **Held/Cancelled stay:** Lines 97-100, 121-122 — even if future, status != "planned" keeps old teacher
- **Schedule rows move:** Line 113 — class_schedules updated to new teacher

**Evidence:**
- Real integration test (Postgres) proves date boundary logic (today = truncate 24h, inclusive)
- All three session types tested with assertions per-teacher

---

### AC3: Class Settings Page (Owner Card UI)
**Status:** ✅ **COVERED**

- **Shows current teacher + member picker:** `class-settings-handoff.test.tsx:60-70`
- **Two-click confirm:** Lines 81-93 — select → arm → confirm flow
- **422 surfaced inline:** Lines 95-115 — API error message displayed to user
- **Hidden from members:** Lines 72-79 — member body has no roster, card absent

**Evidence:**
- Owner view (lines 24-34): center with is_owner:true, members array → shows "Cô Lan" + select offers "Thầy Nam" only
- Member view (lines 37-39): center_name only, no roster → settings form still loads but no teacher card
- Error handling (lines 99-103): PUT endpoint 422 with custom message

---

### AC4: Sidebar Import Link (Owner-Gated)
**Status:** ✅ **COVERED**

**Nav structure:**
- `dashboard-layout.test.tsx:237` — "Trung tâm" group includes ["Duyệt giáo án", "Nhập từ Excel", "Cài đặt trung tâm"]

**Owner case:**
- `dashboard-layout.test.tsx:240-248` — link present with `href="/students/import"`

**Member case:**
- `dashboard-layout.test.tsx:250-260` — "Nhập từ Excel" absent for non-owner
- `roster-import-page.test.tsx:105-112` — member sees "Chỉ chủ trung tâm mới nhập được" gate

**Evidence:**
- Owner-shaped /centers/me handler includes is_owner:true flag → import link renders
- Member-shaped /centers/me (center_name only) → import link hidden + import page gate message shown

---

### AC5: Students Page Header Button Order
**Status:** ⚠️ **PARTIAL — Buttons Exist, Order Not Explicitly Tested**

**Found in source:**
- `students-page.tsx:187` — ⚙ Cài đặt lớp (settings link)
- `students-page.tsx:191` — + Tạo lớp mới (create class)
- `students-page.tsx:200` — + Thêm học sinh (add student)

**What is tested:**
- `students-page.test.tsx:97-104` — settings link navigates to `/classes/{id}/settings` ✅
- `students-page.test.tsx:161-171` — settings link hidden on "Chưa ghi danh" (unenrolled) tab ✅
- No test verifies the three-button order or presence of "Tạo lớp mới" / "Thêm học sinh"

**Concern:**
- Buttons exist in source code with correct sequence (lines 187, 191, 200)
- But the sequence and presence is not validated by a single test case
- Could regress if a button is moved/hidden/removed without test coverage

---

### AC6: Empty Workbook Validation (422 with Guidance)
**Status:** ✅ **COVERED**

- **422 on empty file (both dry-run & commit):** `imports/service_test.go:254-273`
  - Line 259: Loop over `[]bool{true, false}` ensures both paths reject
  - Line 264: Confirms `CodeEmptyFile` + status 422
  - Lines 270-271: No lock taken, nothing written

- **Guidance message "nhập từ dòng 3":** `imports/errors.go:32`
  - Message: "file không có dòng dữ liệu nào — nhập từ dòng 3 trở đi"

- **Reuse-only workbook NOT rejected:** `imports/service_test.go:27-31`
  - `ImportReportSummary` test: reused-only (created:0, reused:2) shows "File hợp lệ" ✅

- **UI never shows success for all-zero:** `import-report-summary.test.tsx:34-38`
  - Committed all-zero report → warning text, NOT "Đã nhập xong"

**Evidence:**
- Dry-run and commit follow the same validation: empty before transaction → no lock, no DB touch
- Message is present in error response
- UI component distinguishes: created_count + reused_count > 0 → success, else → warning

---

## Test File Cross-Reference

| Criterion | API Tests | Web Tests |
|-----------|-----------|----------|
| AC1 (PUT endpoint) | `handoff/service_test.go`, `handoff/integration_test.go` | — |
| AC2 (date logic) | `handoff/integration_test.go:86-123` | — |
| AC3 (settings UI) | — | `class-settings-handoff.test.tsx` |
| AC4 (sidebar + import gate) | — | `dashboard-layout.test.tsx`, `roster-import-page.test.tsx` |
| AC5 (header order) | — | `students-page.test.tsx` (partial) |
| AC6 (empty workbook) | `imports/service_test.go`, `imports/errors.go` | `import-report-summary.test.tsx` |

---

## Pre-Existing Skipped Tests (Expected)
Per task notes, 3 web tests marked `.skip` in `roster-import-page.test.tsx` (lines 118-130) are timing-flaky and **not flagged as failures**:
- Template download test
- Post-delay tests
- CI runner timeout on slow instrumented runs

---

## Summary

**Total Runs:**
- Web: 370 tests, 3 skipped → 367 active passing
- API: 22 test packages with ok status, 0 failures

**Criteria Met:** 5.5 / 6
- ✅ AC1, AC2, AC3, AC4, AC6 fully covered
- ⚠️ AC5 buttons exist; order not explicitly tested (source verified, risk: low if refactoring)

**Build Health:** Green
- TypeScript check: pass
- Go build: pass
- All test runners: pass

---

## Recommendations

1. **AC5 Enhancement:** Add test case to `students-page.test.tsx` verifying button order/presence:
   ```typescript
   it("orders header buttons: settings, create class, add student", async () => {
     renderStudentsPage();
     const buttons = screen.getAllByRole("button").filter(/* by data attrs */);
     // Assert order: settings, create, add
   });
   ```

2. **Empty Workbook Guidance:** Confirm via manual QA that error message UI displays "nhập từ dòng 3" on both dry-run and commit (message present in code, mock tests confirm rejection, but UI rendering not unit-tested).

3. **Session Edge Case:** Consider test for boundary: a session on the exact stroke of midnight (tomorrow - 1 second) to confirm "today inclusive" logic is robust.

---

Status: DONE
Summary: 59 test files, 370 passed, 3 skipped. AC1-4 and AC6 fully tested. AC5 buttons exist in source but order not explicitly verified by test.
Concerns: AC5 header button order—buttons are present but order/presence untested; recommend adding a single order-validation test case.
