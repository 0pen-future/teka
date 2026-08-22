# Phase 5: Operator CLI — Bootstrap and Recovery

## Executed Phase
- Phase: phase-05-operator-cli-bootstrap-and-recovery
- Plan: plans/260812-0904-invite-only-onboarding/
- Status: DONE_WITH_CONCERNS (own scope fully green; unrelated seeds bug blocks the full `make test-api` gate)

## Files Modified

Created:
- `apps/api/internal/cli/create_center.go`
- `apps/api/internal/cli/create_center_test.go`
- `apps/api/internal/cli/reset_password.go`
- `apps/api/internal/cli/reset_password_test.go`
- `apps/api/internal/cli/password_prompt.go`
- `apps/api/internal/cli/password_prompt_test.go`
- `apps/api/internal/cli/bootstrap_integration_test.go`

Modified:
- `apps/api/internal/cli/root.go` — register `create-center` and `reset-password`
- `apps/api/go.mod` / `apps/api/go.sum` — added `golang.org/x/term v0.45.0`
- `apps/api/internal/app/app.go`, `apps/api/internal/app/container.go` — expose `TxManager`/`Teachers`/`Centers`/`Auth` on the shared container so CLI commands and `serve` build the same stack
- `apps/api/internal/features/teachers/{errors.go,repository.go,repository_test.go,service.go,service_test.go,integration_test.go}` — added `teachers.SetPasswordForRecovery` / `repository.SetPasswordHashForRecovery` (rewrites the password hash only, never touches account status)

## Tasks Completed

- [x] `create-center --name --owner-phone --owner-name [--generate] [--force]`: one atomic transaction (center row → owner account → membership → owner_id backfill), no ownerless-center window, duplicate-phone rolls back everything
- [x] `reset-password --phone [--generate] [--force]`: rewrites password hash, revokes all refresh tokens, leaves account status untouched (works on disabled accounts, documented in `--help`)
- [x] No `--password` flag; `golang.org/x/term.ReadPassword` non-echo double-entry prompt, or `--generate` prints a one-time `crypto/rand` password exactly once
- [x] `--force` as the sole confirmation gate (no TTY y/n prompt) — both commands run unattended in a deploy shell, no environment refusal (these are production commands by design)
- [x] Non-zero exit codes on failure via cobra's standard `RunE` error return; no secrets or password hashes ever logged/echoed
- [x] Unit tests: `generatePassword` charset/length/uniqueness, arg validation for both commands
- [x] Integration tests (testcontainers, real Postgres): fresh-DB bootstrap → login → `IsOwner=true`; duplicate-phone full rollback; reset-password revoke+relogin; reset-password on a disabled account (status unchanged, login stays blocked)

## Tests Status

- Type check / build: `go build ./...` → clean
- Lint: `golangci-lint run ./...` → 0 issues
- Unit + integration tests: `go test -tags=integration ./internal/cli/... -v` → 18/18 PASS
  - 4 create-center arg-validation tests
  - 2 reset-password arg-validation tests
  - 8 password_prompt unit tests
  - 4 integration tests against real Postgres via testcontainers: `TestBootstrapCenterFreshDBOwnerCanLogInAndIsOwner`, `TestBootstrapCenterDuplicatePhoneRollsBackEverything`, `TestResetPasswordRevokesSessionAndAllowsReloginWithNewPassword`, `TestResetPasswordWorksOnDisabledAccountWithoutReactivatingIt`
- `internal/cli` coverage: 68.2% of statements (`go tool cover -func`, reproduced across two independent runs) — above the 60% floor. Uncovered lines are `migrate.go` (pre-existing, untouched) and `root.go`'s `Execute`/`notYet` (main-only glue).
- `teachers` recovery path: `go test ./internal/features/teachers/... -run "TestSetPasswordForRecovery" -v` → 4/4 PASS
- Full `make test-api`: does **not** complete — see Issues below. A full-repo `-coverpkg=./...` run (which did reach coverage computation despite the seeds test failure) reported overall repo coverage at 73.7%.

## Issues Encountered

`seeds/seed_test.go:TestRunIsIdempotent` fails with:
```
seed: resolve center for +84901000001: sql: Scan error on column index 0, name "center_id":
converting driver.Value type string ("...") to a uint8: invalid syntax
```
Root cause (not fixed — out of Phase 5's file ownership): `seeds/seed.go` `ensureOwner` (~line 216) does
`db.Raw("SELECT center_id FROM teachers WHERE id = ?", existingID).Scan(&centerID)` where `centerID` is a bare
`uuid.UUID`; GORM's `Scan` appears to attempt struct-field mapping onto the UUID's underlying byte array rather
than a plain scalar scan. Confirmed via `git status --short seeds/` that this file was already modified and
uncommitted in the working tree before this session started (last real commit `42e356f`, current diff +83/-16),
and is not part of Phase 5's ownership (`internal/cli/*`, `internal/app/*`, `internal/features/teachers/*` for the
recovery path only). This blocks the full-repo `make test-api` pipeline from reaching its coverage-gate step,
independent of Phase 5's own health.

## Next Steps

- Whoever owns `seeds/seed.go` needs to fix the `Scan` target (scan into a `string` then `uuid.Parse`, or use
  `.Row().Scan(&centerID)`) so `make test-api` can complete end-to-end.
- No further work needed on Phase 5's own scope; `internal/cli` is fully verified independently.
