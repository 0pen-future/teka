# Phase 1 Validation Report: view_all write-scope hotfix

**Date:** 2026-09-06  
**Status:** DONE  
**Coverage:** Guard, pin tests (9 packages), HTTP, unit tests, assertion evaluation

## Validation Summary

| Component | Test | Result | Notes |
|-----------|------|--------|-------|
| Guard AST | TestRepositoriesWidenWritesThroughWriteWideOnly | ✓ PASS | Probe confirmed test fails when CenterWideFor inserted in write function (students/repository.go:278 lockThing) |
| Pin tests | TestViewAllWidens* (classes, sessions, enrollments, billing, payments, statements, notifications, attendance, students) | ✓ PASS (9/9) | All pass with `-p 1 -count=1`, ~6s each |
| HTTP policy | TestPolicyHTTP (./internal/server) | ✓ PASS | Integration test for write-denial over HTTP |
| Unit tests | make test-api-unit | ✓ PASS | All packages cached or passing |

## Commands Executed

```bash
# Guard test (baseline + probe verification)
go test -count=1 -run 'TestRepositories' ./internal/features/     # baseline: ok
# [probe inserted into students/repository.go:278]
go test -count=1 -run 'TestRepositoriesWidenWritesThroughWriteWideOnly' ./internal/features/  # fails as expected
# [probe deleted, verified via git status]
go test -count=1 -run 'TestRepositories' ./internal/features/     # restored: ok

# Pin tests (all 9 packages)
for pkg in classes sessions enrollments billing payments statements notifications attendance students; do
  go test -tags integration -p 1 -count=1 -run 'TestViewAllWidens' ./internal/features/$pkg/
done
# All: ok, 5–7s each

# HTTP test
go test -tags integration -p 1 -count=1 -run 'TestPolicyHTTP' ./internal/server/  # ok

# Unit tests
make test-api-unit  # ok (cached or passing)
```

## Assertion Quality (Billing, Payments, Statements)

**Billing** (TestViewAllWidensBillingReadsNotWrites, line 1521):
- Error code + DB state: Denied Close → 404 + period still open ✓
- DB invariants: Invoice status unchanged after denied AddAdjustment/VoidInvoice ✓
- Own-rows write: member closes own period ✓

**Payments** (TestViewAllWidensPaymentReadsNotWrites, line 825):
- Error codes: Denied Reverse/AutoAllocate/Reallocate all → 404 ✓
- DB state: Payment unreversed (ReversedAt is nil) ✓
- Ledger invariant: assertLedgerInvariant call ✓
- Own-rows write: member reverses own payment ✓
- Bonus test (TestMemberRecordsPaymentForCenterContactWithoutViewAll, line 887): member without view_all records payment for center contact; payment anchors on contact owner ✓

**Statements** (TestViewAllWidensStatementReadsNotWrites, line 391):
- Error codes: Denied Generate/Revoke → 404; List also → 404 (oversight axis) ✓
- DB state: Denied Generate writes nothing (count=0) ✓
- DB field: Statement stays live (RevokedAt is nil) ✓
- Own-rows write: member generates and revokes own statements ✓

**Findings:** No weak assertions (all check error + DB state). All resource tests include owner behavior verification and own-rows write success paths. No missing cases.

## Unresolved Questions

None.

---

**Status:** DONE  
**Summary:** Guard, 9 pin tests (classes, sessions, enrollments, billing, payments, statements, notifications, attendance, students), HTTP policy, and unit tests all pass. Guard confirmed to fail on write CenterWideFor via probe; probe removed and verified deleted. Assertion quality across billing/payments/statements is comprehensive (error codes + DB state + invariants). Ready to proceed.
