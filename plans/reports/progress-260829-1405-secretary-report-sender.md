# Progress — Secretary as delegated report sender

Date: 2026-08-29. Feature COMPLETE, all 5 phases done, tests green, review
findings triaged.

## Summary

`can_send_reports` membership flag lets a designated secretary read
periods/statements/debt center-wide and run statement/reminder sends from her
own Zalo, attributed to her. Plain teachers lose all send ability (every
channel, own period included) — direction change 2026-08-29; owner keeps
current behavior as fallback sender.

## Per-phase

1. Permission model & scope resolution — Done. Migration 000011, `ReportsOversight()` helper, grant/revoke routes + audit, flag reset on rejoin.
2. API read oversight & delegated send path — Done. `scopedRead` cluster, period-scoped ledger, D8 exclusivity gate on BulkSend/ResumeRun, migration 000012 one-run-per-period, preview endpoint.
3. Web owner grant/revoke UI — Done. Toggle + badge on `/center`, MSW-covered.
4. Web secretary send experience — Done. Reports nav/page, server-bucket pre-send dialog, 3 plain-member send surfaces hidden.
5. E2E, regression & docs — Done. Seed extended (secretary + 2nd teacher), `secretary-send.spec.ts`, `EnsurePeriod` bug found+fixed, docs updated.

## Verification evidence

- `golangci-lint run ./...` (apps/api): 0 issues (post H1 fix)
- `go vet` / `go build`: clean
- Billing unit + integration: green, incl. new `TestEnsurePeriodReturnsCallersOwnPeriodWhenMemberSharesTheMonth`
- Seeds integration: green (post M3 graceful-skip fix)
- Web vitest: 407 passed, 3 skipped
- `npm run typecheck` (tsc -b incl. e2e): clean
- Playwright isolated stack (`teka-e2e`): 26/26 passed
- `secretary-send.spec.ts`: green twice in a row on same reused DB (idempotency proven)

## Post-review fixes applied (review-260829-1150-secretary-report-sender-phase5.md)

- H1 lint: `service_test.go:71` unused `ctx` → `_`
- H2 docs: `docs/api-guidelines.md` read-cluster corrected to include contacts List + notification ledger
- M3 seed: `seedClosedPeriod` catches `ErrUnconfirmedSessions`, warns+skips close instead of hard-fail/irreversible-close
- M5 sort: `ListPeriodsRead` gets `id` tie-breaker after `period_start DESC`
- L9 comment: stale "open period" → "seeded closed"
- M4 (calendar flake) and M6 (`--grep D8` partial-run coupling): rejected with evidence (see below)

## Remaining known notes (accepted, not blocking)

- M4: backfill logic (seed.go:772-822) guarantees Minh always has an invoice regardless of seed run date; theoretical residual edge (day-1 + matching weekday on empty DB) not worth a fix — operational note only.
- M6: `secretary-send.spec.ts` D8 assertions depend on test-1 data; safe under `workers:1`/`fullyParallel:false` full-suite run; a lone `--grep D8` on a clean DB would fail. Documented in spec comment, not fixed (low value, no product risk).
- L7/L8/L10/L11/L12: accepted, no action (see review report).

## Plan-file consistency check (this task)

- All 5 phase files: `status: done`, all Todo/Success Criteria already ticked — no edit needed.
- plan.md: phase table all Done, success criteria (lines 123-139) already ticked. Only frontmatter `status` was stale (`in-progress`) — corrected to `done`. No `ak plan` CLI available in this session (no Bash tool) to apply via the tracked contract; frontmatter edited directly per this task's explicit acceptance criteria.

## Unresolved questions

None.
