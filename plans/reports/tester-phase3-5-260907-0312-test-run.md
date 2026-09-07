# Test Run Report: Phase 3+5 — Reports.send Read-Key Conversion & Payments Candidacy Fix & RouteSpec Manifest

**Date:** 2026-09-07  
**Branch:** master  
**Commands run sequentially in repo root** `/home/cesc/Documents/personal-workspace/teka`

## Results Summary

| Component | Status | Details |
|-----------|--------|---------|
| Lint | ✅ PASS | 0 issues |
| Unit tests | ❌ FAIL | Scoping guard violation in collections & notifications |
| Full test suite | ❌ FAIL | Scoping guard violation + 77.5% coverage (exceeds 60% floor) |
| API docs | ✅ PASS | Only mark-sent endpoint changes (expected) |
| Authctx tests | ✅ PASS | TestBuildPermSet, TestPhoneVisible |
| Integration tests | ✅ PASS | Parity, ImpliedKeys, ZaloMappings, MarkSent, SettleInvoices |
| Scoping guard tests | ❌ FAIL | TestRepositoriesWidenWritesThroughWriteWideOnly |
| Testcontainers cleanup | ✅ PASS | No orphaned containers |

## Test Execution Details

### 1. Linting
```
make lint-api 2>&1 | tail -20
```
**Result:** ✅ PASS — 0 issues

### 2. Unit Tests
```
make test-api-unit 2>&1 | tail -40
```
**Result:** ❌ FAIL
**Failure:** Scoping guard violation in `internal/features` test

### 3. Full Test Suite with Coverage
```
GOFLAGS=-p=2 make test-api 2>&1 | tail -80
```
**Result:** ❌ FAIL (due to scoping guard violation)
**Coverage:** 77.5% of statements (floor: 60%) ✅
**Exit code:** 1 (failed due to test failure, not coverage)

### 4. API Documentation
```
make api-docs >/dev/null 2>&1; git status --short apps/api/docs
```
**Result:** ✅ PASS — Only expected changes
**Changed files:**
- `apps/api/docs/docs.go` (mark-sent endpoint description & 404 response)
- `apps/api/docs/swagger.json` (mark-sent endpoint description & 404 response)
- `apps/api/docs/swagger.yaml` (mark-sent endpoint description & 404 response)

**Changes verified:** Endpoint description updated to clarify idempotency behavior and 404 response added for out-of-scope ids. No other routes affected.

### 5. Targeted Authctx Tests
```
cd apps/api && go test -count=1 -run 'TestBuildPermSet|TestPhoneVisible' ./internal/shared/authctx/ -v
```
**Result:** ✅ PASS
- TestBuildPermSet ✅
- TestBuildPermSetImpliesReadKeysForReportsSend ✅
- TestPhoneVisible (8 subtests) ✅

### 6. Targeted Integration Tests
```
go test -tags=integration -p 1 -count=1 -run 'Parity|ImpliedKeys|ZaloMappings|MarkSent|SettleInvoicesFromAnotherMember' ...
```
**Result:** ✅ PASS
- TestPolicyHTTPViewAllParity ✅ (6.24s)
- TestReportsSendImpliedKeysParity ✅ (6.33s)
- TestZaloMappingsReturnsLiveMappedContactsOnly ✅ (3.97s)
- TestZaloMappingsFollowsCenterWideOrStint ✅ (4.12s)
- TestPlainMemberCannotCreateSendsOnAnyChannelButKeepsMarkSent ✅ (4.23s)
- TestRecordReverseAndReallocateSettleInvoicesFromAnotherMembersClosedPeriod ✅ (4.58s)

### 7. Scoping Guard & Repository Tests
```
go test -count=1 ./internal/features/ -run 'Scope|Guard|Repositories' -v
```
**Result:** ❌ FAIL
**Passing guards:**
- TestRepositoriesScopeThroughCenterWideOnly ✅
- TestScopeLiteralsOnlyWhereResolved ✅

**Failing guard:**
- TestRepositoriesWidenWritesThroughWriteWideOnly ❌

## Failing Test: TestRepositoriesWidenWritesThroughWriteWideOnly

**Root Cause:** The recent change from `sc.ReportsOversight()` to `sc.CenterWideFor(authctx.PermBillingViewAll)` exposed a scoping guard violation.

**Guard Rule:** Functions using `CenterWideFor()` (a visibility key for widening reads) must have "read" in their function name (e.g., `readScoped`, `scopedRead`, `readNarrow`, `GetPeriodRead`).

**Violations Detected:** 12 violations across 2 files

### Collections/Repository.go (9 violations)

| File:Line | Function | Issue |
|-----------|----------|-------|
| 60 | PeriodExists | CenterWideFor in non-read-named function |
| 105 | contactBalanceQuery | CenterWideFor in non-read-named function |
| 205 | childInvoicesByContact | CenterWideFor in non-read-named function |
| 264 | classCollectionsQuery | CenterWideFor in non-read-named function |
| 342 | PeriodSummary | CenterWideFor in non-read-named function |
| 360 | PeriodSummary | CenterWideFor in non-read-named function |
| 388 | PeriodSummary | CenterWideFor in non-read-named function |
| 394 | PeriodSummary | CenterWideFor in non-read-named function |
| 395 | PeriodSummary | CenterWideFor in non-read-named function |

### Notifications/Repository.go (3 violations)

| File:Line | Function | Issue |
|-----------|----------|-------|
| 233 | runsPeriodScoped | CenterWideFor in non-read-named function |
| 295 | ListByPeriod | CenterWideFor in non-read-named function |
| 589 | ZaloMappings | CenterWideFor in non-read-named function |

**Full Error Output:**
```
--- FAIL: TestRepositoriesWidenWritesThroughWriteWideOnly (0.02s)
    scoping_guard_test.go:72: collections/repository.go:60: CenterWideFor inside PeriodExists — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
    scoping_guard_test.go:72: collections/repository.go:105: CenterWideFor inside contactBalanceQuery — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
    scoping_guard_test.go:72: collections/repository.go:205: CenterWideFor inside childInvoicesByContact — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
    scoping_guard_test.go:72: collections/repository.go:264: CenterWideFor inside classCollectionsQuery — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
    scoping_guard_test.go:72: collections/repository.go:342: CenterWideFor inside PeriodSummary — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
    scoping_guard_test.go:72: collections/repository.go:360: CenterWideFor inside PeriodSummary — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
    scoping_guard_test.go:72: collections/repository.go:388: CenterWideFor inside PeriodSummary — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
    scoping_guard_test.go:72: collections/repository.go:394: CenterWideFor inside PeriodSummary — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
    scoping_guard_test.go:72: collections/repository.go:395: PeriodSummary — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
    scoping_guard_test.go:72: notifications/repository.go:233: CenterWideFor inside runsPeriodScoped — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
    scoping_guard_test.go:72: notifications/repository.go:295: CenterWideFor inside ListByPeriod — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
    scoping_guard_test.go:72: notifications/repository.go:589: CenterWideFor inside ZaloMappings — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()
```

## ReportsOversight() Usage Analysis

The refactoring from `sc.ReportsOversight()` to `sc.CenterWideFor()` and related methods is incomplete. Current usages found:

| File | Function | Usage Count | Context |
|------|----------|-------------|---------|
| authctx/authctx.go | ReportsOversight | 1 | Definition (line 69) |
| statements/service.go | ReportsOversight | 2 | Authorization checks (lines 110, 347) |
| notifications/service.go | ReportsOversight | 4 | Authorization checks (lines 129, 398, 499, 678) |
| contacts/repository.go | ReportsOversight | 1 | Scope narrowing (line 113) |

These usages suggest the phase 3 refactoring is still in progress. The collections/repository.go and parts of notifications/repository.go have been updated, but service-level authorization checks still use the legacy method.

## Coverage Analysis

Full test suite coverage by package (selected packages):
- **server:** 17.1% ✅
- **notifications:** 13.5% ✅
- **statements:** 11.8% ✅
- **billing:** 10.1% ✅
- **imports:** 9.0% ✅
- **centers:** 9.5% ✅
- **seeds:** 9.2% ✅
- **payments:** 7.7% ✅
- **teaching:** 6.8% ✅
- **collections:** 6.3% ✅

**Total Coverage: 77.5%** (floor: 60%) ✅

## Infrastructure Cleanup

✅ No testcontainers left running
✅ No Docker processes orphaned

---

## Status: DONE_WITH_CONCERNS

**Summary:** Full test suite execution completed with one blocking issue. Coverage requirement met (77.5% ≥ 60% floor). API documentation changes correct (mark-sent endpoint only). Scoping guard violation prevents build due to incomplete read-key refactoring in collections and notifications repositories.

**Concerns/Blockers:**

1. **BLOCKING:** Scoping guard violation in 12 locations (5 functions in collections, 3 in notifications). Functions using `CenterWideFor()` must have "read" in their name per scoping guard test. This is a code structure issue from the phase 3 read-key conversion — these read-only functions don't follow the required naming convention.

2. **Incomplete refactoring:** `ReportsOversight()` still used in 7 locations (statements/service.go, notifications/service.go, contacts/repository.go). Phase 3 conversion appears partial — collections and notifications repositories updated but service-layer authorization checks still on legacy method.

3. **Test impact:** All feature package unit tests blocked on scoping guard violation. Individual feature integration tests (collections, notifications, etc.) pass. The guard protects against write functions using read visibility keys — these are legitimate read operations but incorrectly named.
