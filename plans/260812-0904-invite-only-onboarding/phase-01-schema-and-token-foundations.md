---
phase: 1
title: "Schema and Token Foundations"
status: done
priority: P1
effort: "4h"
dependencies: []
---

# Phase 1: Schema and Token Foundations

## Overview

Migration 000008 (invitations + password_reset_tokens tables), shared token
helper package, and new config knobs. Pure infrastructure — no endpoint
behavior changes yet. `centers.owner_id` is deliberately left NOT NULL (see
Key Insights); bootstrap stays atomic in Phase 5.

## Key Insights

- `refresh_tokens` (migration 000002) is the template: opaque 256-bit token,
  sha256-hex `token_hash` UNIQUE, expiry + revocation timestamps.
- `auth.HashToken` + `NewRefreshToken` live in
  `apps/api/internal/features/auth/tokens.go`; invitations must not import the
  auth feature → promote helpers to `internal/shared/token`.
- `centers.owner_id` is `NOT NULL` + `uq_centers_owner` + DEFERRABLE FK
  (migration 000007) and **stays that way**. `is_owner` is resolved in raw SQL
  as `(c.owner_id = t.id)` (`centers/repository.go:161,205,364`); a nullable
  owner_id would force a NULL→bool scan rewrite and a lossy down-migration.
  Bootstrap therefore creates center + owner in **one** atomic CLI tx (Phase 5),
  so no ownerless-center state ever exists — the schema needs no owner_id change.
- Reuse `cfg.Statements.PublicBaseURL` (`config.go:69`, `STATEMENTS_PUBLIC_BASE_URL`)
  for invite/reset links. It is already set in every deployed env; a second
  localhost-defaulting key would silently emit `http://localhost:5173` links in
  prod if forgotten.

## Requirements

- Functional: new tables queryable; token helpers produce (plaintext, hash)
  pairs; config exposes invite TTL 72h, reset TTL 48h, reset cooldown 15m.
  Public link base is the existing `Statements.PublicBaseURL` — no new key.
- Non-functional: migration reversible (`down` drops the two tables); no
  behavior change to existing suites; `centers` schema untouched.

## Architecture

Migration `000008_invitations_and_reset_tokens`:

```sql
CREATE TABLE invitations (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  center_id  UUID NOT NULL REFERENCES centers(id),
  phone      VARCHAR(20) NOT NULL,            -- E.164, normalized
  token_hash VARCHAR(64) NOT NULL UNIQUE,     -- sha256 hex
  status     VARCHAR(16) NOT NULL DEFAULT 'pending'
             CHECK (status IN ('pending','accepted','revoked')),
  expires_at  TIMESTAMPTZ NOT NULL,
  accepted_at TIMESTAMPTZ, revoked_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_invitations_pending_phone
  ON invitations (center_id, phone) WHERE status = 'pending';

CREATE TABLE password_reset_tokens (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES user_accounts(id),
  token_hash VARCHAR(64) NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at    TIMESTAMPTZ, superseded_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_password_reset_tokens_user ON password_reset_tokens (user_id);
-- At most one live token per account (enforces cooldown/supersede as an
-- invariant, not just service logic; closes the concurrent-create race).
CREATE UNIQUE INDEX uq_password_reset_active ON password_reset_tokens (user_id)
  WHERE used_at IS NULL AND superseded_at IS NULL;
```

No `centers` alteration — `owner_id` stays NOT NULL (see Key Insights).

`internal/shared/token/token.go`: `New() (plaintext, hash string, err error)`
(256-bit random, base64url plaintext) and `Hash(plaintext) string` (sha256
hex). `features/auth/tokens.go` delegates to it; public auth API unchanged.

`internal/config`: new `OnboardingConfig` block — `InviteTTL`
(`API_INVITE_TTL`, default `72h`), `ResetTTL` (`API_RESET_TTL`, `48h`),
`ResetCooldown` (`API_RESET_COOLDOWN`, `15m`). No new base-URL key: invite/reset
links reuse `cfg.Statements.PublicBaseURL`.

## Related Code Files

- Create: `apps/api/migrations/000008_invitations_and_reset_tokens.up.sql` / `.down.sql`
- Create: `apps/api/internal/shared/token/token.go` + `token_test.go`
- Modify: `apps/api/internal/features/auth/tokens.go` (delegate to shared/token)
- Modify: `apps/api/internal/config/config.go` (+ `config_test.go`)

`centers/model.go` is **not** touched — `owner_id` stays NOT NULL, so the
`OwnerID uuid.UUID` field and every `(c.owner_id = t.id)` comparison site stay
as-is.

## Implementation Steps (TDD)

### Tests Before
1. `token_test.go`: uniqueness, plaintext↔hash determinism, hash length 64.
2. `config_test.go`: defaults for the three new keys; TTL parse failures fatal.
3. Integration test (new `invitations` slice comes Phase 2 — put schema probes
   in `centers/integration_test.go`): duplicate pending invite for same
   (center, phone) rejected; second pending for *different* center allowed;
   `uq_password_reset_active` rejects a second live reset token for one user
   while allowing one after the first is used/superseded.

### Refactor
4. Write migration up/down; run `api migrate up` locally.
5. Extract `shared/token`; rewire `auth/tokens.go`.
6. Add `OnboardingConfig`; wire into `config.Load` validation.

### Tests After
7. None new beyond step 1–3 (infrastructure phase).

### Regression Gate
```sh
make test-api        # full pyramid incl. integration (migration correctness)
make lint-api
```

## Todo

- [ ] Migration 000008 up/down + integration probes green
- [ ] `shared/token` extracted, auth suite untouched-green
- [ ] Config keys + tests (three keys; no base-URL key)
- [ ] Partial unique index on `password_reset_tokens` verified

## Success Criteria

- [ ] `make test-api` green including new schema probes
- [ ] `api migrate down && api migrate up` round-trips cleanly

## Risk Assessment

- Low — additive schema only, no change to existing `centers`/`teachers`
  columns or the `is_owner` SQL. The two new tables reference existing PKs;
  `down` drops them cleanly.
- Partial unique index depends on `used_at`/`superseded_at` being set on
  supersede/consume — Phase 4 service must set them in the same tx or the
  index will reject a legitimate re-request; integration probe (step 3) guards
  this contract before Phase 4 relies on it.

## Security Considerations

Tokens stored only as sha256 hashes; plaintext exists in memory + the
returned link only. No PII beyond phone (already stored elsewhere).

## Next Steps

Phase 2 builds the invitations feature slice on these tables.
