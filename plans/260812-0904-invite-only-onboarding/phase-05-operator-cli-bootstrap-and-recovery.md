---
phase: 5
title: "Operator CLI — Bootstrap and Recovery"
status: done
priority: P1
effort: "4h"
dependencies: [1, 3]
---

# Phase 5: Operator CLI — Bootstrap and Recovery

## Overview

Two Cobra subcommands on the existing `cmd/api` binary following the
`serve`/`migrate`/`seed` pattern: `create-center` (atomic — center + owner +
membership in one tx) and `reset-password`. This realizes the post-red-team
scope amendment to brainstorm decision 7 (see `plan.md`).

## Key Insights

- CLI bootstrap mirrors `internal/cli/{root,serve,migrate,seed}.go`:
  `config.Load()` → `database.Open(cfg.DB)` → feature wiring.
- Bootstrap is **atomic**: `centers.owner_id` stays NOT NULL, so a single
  `create-center` must create the center, the owner account+teacher, the owner
  membership, and set `owner_id` — all in one tx. There is no ownerless-center
  window and no separate `create-owner`. This is what let Phase 1 leave the
  schema and `is_owner` SQL untouched.
- Reuse `teachers.Service.CreateInCenter` (Phase 3) for the owner account+teacher
  rows rather than re-deriving bcrypt/insert logic in the CLI.
- Password input must never land in shell history (brainstorm unresolved q5,
  decided here): interactive non-echo prompt via `golang.org/x/term`
  (`ReadPassword`), plus `--generate` flag printing a random one-time
  password to stdout once. No `--password` flag at all. `golang.org/x/term`
  is a new dependency — `go get golang.org/x/term` and tidy.
- `reset-password` targets any account including owners (operator already
  holds DB superpowers); revokes all refresh tokens via
  `auth.Repository.RevokeAllForUser` (Phase 3) after update.

## Requirements

- Functional:
  - `api create-center --name "X" --owner-phone <p> --owner-name <n>
    [--generate]` → one tx: insert center → `teachers.CreateInCenter` for the
    owner (active, bcrypt) → open owner membership → set `centers.owner_id`.
    On success the owner can log in and `ResolveScope` yields IsOwner=true.
    Errors clearly when the phone already has an account. Center names aren't
    unique — always creates; print the new center id.
  - `api reset-password --phone <p> [--generate]` → non-echo prompt (twice,
    match) or generated value; bcrypt update; `RevokeAllForUser`; works for
    disabled accounts too without changing status (login still blocked —
    document in help text).
  - These commands are FOR production — no env refusal (unlike `seed`).
    Destructive/irreversible confirmation is a non-interactive `--force` flag
    (no TTY prompt), so the commands run unattended in a deploy shell.
- Non-functional: exit codes non-zero on failure; secrets never echoed or
  logged (respect security: never print stored hashes).

## Architecture

`internal/cli/{create_center,reset_password}.go` (Go snake_case files). Reuse
services, not raw SQL. Wire the needed services (`teachers`, `centers`, `auth`)
through the shared composition helper the server already uses — build them via
`app.Container`/`NewServices` (whatever `router.registerFeatures` constructs
from) rather than hand-duplicating wiring in each command. If no such helper is
exported yet, extract one so `serve` and the CLI share a single construction
path.

Password read helper `internal/cli/password_prompt.go`: `readPassword()`
(term.ReadPassword double-entry) and `generatePassword()` (16 chars from
`crypto/rand`, url-safe alphabet).

## Related Code Files

- Create: `apps/api/internal/cli/create_center.go`
- Create: `apps/api/internal/cli/reset_password.go`
- Create: `apps/api/internal/cli/password_prompt.go` (+ `password_prompt_test.go`)
- Modify: `apps/api/internal/cli/root.go` (register commands)
- Modify/Create: `apps/api/internal/app/*` — export the shared service
  construction helper if `serve` doesn't already expose one
- Modify: `apps/api/go.mod` / `go.sum` — add `golang.org/x/term`

## Implementation Steps (TDD)

### Tests Before
1. Unit: `generatePassword` charset/length/uniqueness; command arg
   validation (missing flags → usage error) via Cobra `Execute` harness.
2. Service-level tests (fakes) covering the atomic `create-center` path
   (center + owner via `CreateInCenter` + membership + owner_id set) and the
   reset path (bcrypt update + `RevokeAllForUser`).

### Refactor
3. Implement the two commands + prompt helper + root registration + shared
   service-construction helper.

### Tests After
4. Integration (`//go:build integration`, testcontainers): fresh DB →
   `create-center --generate` → login via auth service with generated password
   succeeds and `ResolveScope` yields IsOwner=true; `create-center` with an
   already-used phone → error, no partial rows (tx rollback);
   `reset-password --generate` for that owner → old refresh revoked, new
   password logs in.

### Regression Gate
```sh
make test-api && make lint-api && go build ./... (apps/api)
```

## Todo

- [ ] Two subcommands + prompt helper + shared wiring helper
- [ ] Atomic fresh-DB bootstrap integration test green (single tx, IsOwner)
- [ ] `golang.org/x/term` added and tidy; build clean
- [ ] Help texts document owner-recovery purpose + disabled-account caveat

## Success Criteria

- [ ] Brainstorm CLI acceptance bullet passes: bootstrap on fresh DB, owner
      logs in; reset works for any account incl. owner; exactly two onboarding
      subcommands beyond serve/migrate/seed (per scope amendment)

## Risk Assessment

- Interactive prompt untestable in CI → isolate behind `io.Reader`-style
  seam; integration tests use `--generate` path only; `--force` replaces the
  TTY confirmation so unattended runs never block.
- Atomicity: any failure mid-`create-center` must roll back the whole tx — no
  center left without an owner. Integration test asserts no partial rows on the
  duplicate-phone error.

## Security Considerations

No password flags (shell history); generated password printed exactly once
to stdout (operator's terminal, out of scope of API logs); bcrypt cost from
`teachers/service.go:18` (cost 12) via `CreateInCenter`, consistent with the
account path used everywhere else.

## Next Steps

Phase 6 rebuilds the web surfaces on the new API.
