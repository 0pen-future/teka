---
phase: 1
title: "Statement Generation and Tokens"
status: completed
priority: P1
effort: "5h"
dependencies: []
---

# Phase 1: Statement Generation and Tokens

## Overview

Creates the `statements` feature package: one statement per contact per closed
period, with a tokenised link whose plaintext never touches the database.

This phase covers generation, tokens, and the teacher-facing management
endpoints (list, get, revoke). The public rendering endpoint is phase 2.

## Requirements

- R5: the unit is the **contact**, not the student. One row per
  `(contact_id, period_id)`, enforced by `uq_statements`
  (`docs/schema_design.sql:428-429`).
- Generation only for a `closed` period, and only for contacts with at least one
  non-void invoice in it.
- `token_hash BYTEA NOT NULL` and unique (`docs/schema_design.sql:413`, `:430`);
  no plaintext token column exists and none is invented.
- `total_due` snapshot at issue (`docs/schema_design.sql:415`).
- `expires_at NOT NULL` — 90 days from issue (R5).
- Generation is idempotent: running it twice does not create a second row or
  change an existing token.

## Architecture

New package `apps/api/internal/features/statements`, laid out like
`apps/api/internal/features/users`.

**Model** (`model.go`) → `statements` (`docs/schema_design.sql:406-427`). The
table has `deleted_at`, so `gorm.DeletedAt` applies. `TokenHash` is `[]byte`.

### Token derivation (D10 + D10-refinement)

```go
// token returns the URL-safe statement token derived from the statement id.
// The plaintext is never persisted; only its SHA-256 is stored, so a database
// dump alone opens no link.
func deriveToken(key []byte, statementID uuid.UUID) string {
    mac := hmac.New(sha256.New, key)
    mac.Write(statementID[:])
    return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func hashToken(token string) []byte {
    sum := sha256.Sum256([]byte(token))
    return sum[:]
}
```

- `key` is `cfg.Statements.TokenKey`, a 256-bit value from configuration,
  required at startup in production (fail fast if missing or under 32 bytes).
- `hashToken` hashes the **encoded string** as it arrives in the URL, so the
  public route does no base64 decoding and cannot fail on malformed input in a
  distinguishable way.
- 43-character token, no padding, safe in a URL with no escaping.

Why derived rather than random: the plaintext is needed again at every re-send
(reminders, R7), and only the hash is stored. A random token would force either
caching the plaintext or rotating the link and breaking the one a parent already
has. Recorded as OQ-2 in `plan.md` for lead confirmation.

### Generation flow

`POST /billing-periods/:id/statements/generate`, inside one transaction:

1. Load the period; require `status='closed'`, else
   `apperror.Conflict("period is not closed")`.
2. Select target contacts: distinct `contact_id` from `invoices` where
   `teacher_id`, `period_id` match and `status <> 'void'`. Voided invoices are
   excluded, which is what makes "a class with no sessions gets no
   notification" (PRD §5) fall out for free — plan 04 phase 3 voids those
   invoices at close.
3. For each contact, sum `total_due` across their non-void invoices in the
   period. `v_contact_balance` (`docs/schema_design.sql:459`) already computes
   exactly this; reuse it rather than re-aggregating.
4. Upsert `statements` on `(contact_id, period_id)`:
   - insert with a fresh UUIDv7 (D3), `expires_at = now() + 90d`, `total_due`
     from step 3, and `token_hash = hashToken(deriveToken(key, id))`;
   - when a row already exists and is not revoked, **leave the token alone** and
     refresh only `total_due` and `updated_at`, so links already sent keep
     working;
   - when it exists but is revoked, leave it untouched and report it as skipped.

Because the token derives from the statement id, and the id is stable, step 4's
"leave the token alone" is automatic — but write it explicitly so a future
refactor cannot rotate tokens by accident.

`uq_statements` is a partial index (`WHERE deleted_at IS NULL`), so the upsert
targets that index; a previously soft-deleted statement does not block a new
one.

### Teacher-facing endpoints

- `POST /billing-periods/:id/statements/generate` → generate/refresh, returns
  counts and the list.
- `GET /billing-periods/:id/statements` → paginated list with contact name,
  phone, `total_due`, `view_count`, `first_viewed_at`, `expires_at`,
  `revoked_at`, and the **full link URL** (recomputed per request, never stored).
- `GET /statements/:id` → one statement with its link.
- `POST /statements/:id/revoke` → sets `revoked_at`; the link stops working
  immediately. Idempotent.

The link URL is `cfg.Statements.PublicBaseURL + "/s/" + token`. It appears in
teacher-authenticated responses only.

## Related Code Files

Create:

- `apps/api/internal/features/statements/model.go`
- `apps/api/internal/features/statements/token.go` — `deriveToken`, `hashToken`
- `apps/api/internal/features/statements/token_test.go`
- `apps/api/internal/features/statements/repository.go`
- `apps/api/internal/features/statements/service.go`
- `apps/api/internal/features/statements/service_test.go`
- `apps/api/internal/features/statements/dto.go`
- `apps/api/internal/features/statements/handler.go`
- `apps/api/internal/features/statements/routes.go`
- `apps/api/internal/features/statements/integration_test.go`

Modify:

- `apps/api/internal/config/config.go` — add a `Statements` section:
  `TokenKey` (hex or base64, ≥32 bytes, required in production),
  `PublicBaseURL`
- `apps/api/internal/config/config_test.go` — cases for missing and short keys
- `apps/api/internal/server/router.go` — register the feature in
  `registerFeatures` (`apps/api/internal/server/router.go:63-73`)
- `.env.example` / deployment config templates — document the two new variables
  **[verify the exact filenames in the repo before editing; do not commit a real
  key]**

Delete: none. No migration files.

## Implementation Steps

1. Add the `Statements` config section. Validate at startup: in production a
   missing or under-32-byte `TokenKey` is a fatal error; outside production a
   development default is generated once per process and logged as insecure.
   Follow the existing validation style in
   `apps/api/internal/config/config.go`.
2. Create `token.go` with `deriveToken` and `hashToken` exactly as above.
   `token_test.go` asserts: same id and key produce the same token; different
   ids produce different tokens; a different key produces a different token; the
   token is 43 characters and matches `^[A-Za-z0-9_-]{43}$`; `hashToken` returns
   32 bytes.
3. Create `model.go` mirroring the DDL, with `gorm.DeletedAt`.
4. Create `repository.go`: `UpsertStatement`, `ListByPeriod`, `GetByID`,
   `GetByTokenHash`, `Revoke`, `ContactTotals(periodID)` reading
   `v_contact_balance`. Every method except `GetByTokenHash` takes `teacherID`;
   `GetByTokenHash` is the public path and is intentionally teacher-agnostic
   (the token is the authorisation) — comment that exception at the method.
5. Create `service.go` with `Generate`, `List`, `Get`, `Revoke`. `Generate` runs
   inside `tx.WithinTx` (`apps/api/internal/database/tx_manager.go:11`).
6. Create `dto.go`: `StatementResponse` (id, contact_id, contact_name, phone,
   total_due, url, expires_at, revoked_at, view_count, first_viewed_at,
   last_viewed_at), `GenerateResponse` (created, refreshed, skipped_revoked,
   statements). Money `int64`.
7. Create `handler.go` / `routes.go` for the four teacher endpoints, all behind
   `requireAuth`.
8. Register in `registerFeatures`.
9. Write `integration_test.go`:
   - generate against an open period → `409`, nothing written;
   - generate against a closed period with two contacts → two rows, distinct
     `token_hash`;
   - a contact whose only invoice was voided → no statement;
   - a contact with two children → exactly one statement whose `total_due`
     equals the sum of both invoices (R5);
   - generate twice → same ids, same `token_hash`, `total_due` refreshed;
   - revoke then generate → the revoked row is skipped, not resurrected;
   - `token_hash` is unique across statements (`uq_statements_token`,
     `docs/schema_design.sql:430`);
   - no response from any teacher endpoint contains a raw token for another
     teacher's statement.
10. Run `go test ./apps/api/internal/features/statements/... ./apps/api/internal/config/...`.

## Success Criteria

- [x] One statement per contact per period; a two-child family gets one row with
      the combined total (R5).
- [x] Generation refuses on an open period.
- [x] Generation is idempotent and never rotates an existing token.
- [x] The database contains no plaintext token — asserted by scanning the
      `statements` row for any column equal to the derived token.
- [x] `token_hash` is 32 bytes and unique.
- [x] Contacts with only voided invoices receive no statement.
- [x] A missing or short `TokenKey` in production mode fails startup.
- [x] Revoke is idempotent and immediately reflected.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `TokenKey` rotated in ops, silently killing every live link | Medium | High | Startup logs a fingerprint (first 8 hex of `SHA-256(key)`), never the key; documented in the config template as "rotating this invalidates all parent links" |
| Weak or absent key in production | Medium | Critical | Fatal startup validation, minimum 32 bytes; the development fallback is never reachable when the environment is production |
| Regeneration rotates tokens and breaks sent links | Medium | High | Token derives from the immutable statement id; refresh path explicitly does not touch `token_hash`; integration test asserts equality across two generations |
| Statement generated for a contact with no debt, sending a pointless message | Low | Low | Generation is period-wide by design (a zero-balance parent still gets a receipt-style summary); the *reminder* path in phase 3 filters on outstanding |
| `uq_statements` partial index missed by the upsert, causing duplicates | Medium | Medium | `ON CONFLICT` targets the partial index explicitly with its `WHERE deleted_at IS NULL` predicate; duplicate test in step 9 |
| Token appears in server logs via the URL | Medium | High | Phase 2 handles the public route's log redaction; teacher-side responses are the only place the plaintext exists here, and the logger at `apps/api/internal/middleware/logger.go` must be checked for path logging before phase 2 ships |

**Rollback.** Statements are additive and never sent by this phase (sending is
phase 3), so generated rows are inert. Back out by revoking them or deleting the
package and the config section. No data outside `statements` is touched.
