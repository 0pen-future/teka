---
phase: 2
title: "Credential encryption and storage"
status: completed
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: Credential encryption and storage

## Overview

Give the API a way to store Zalo session credentials that is safe against a
stolen DB row. Two pieces: a small reusable AES-GCM envelope helper
(`internal/shared/crypto`) keyed by a KEK from the environment, and a
`zalo_accounts` table + repository holding one encrypted-credentials row per
teacher. Nothing here touches the network or the protocol package beyond
serializing `protocol.Credentials` to JSON as the plaintext.

## Requirements

- Functional:
  - `crypto.Seal(plaintext []byte) ([]byte, error)` / `crypto.Open(ciphertext []byte) ([]byte, error)`
    using AES-256-GCM with a random 12-byte nonce prepended to the ciphertext.
  - KEK loaded from `API_ZALO_CRED_KEY` (hex or base64, decoded to ≥32 bytes),
    reusing the exact decode+validate pattern as `StatementsConfig.TokenKey`.
  - `zalo_accounts` row: encrypted credentials, Zalo UID, display name, status,
    linked/verified timestamps, and an auditable consent record
    (`consent_at`, `consent_version`); unique per teacher.
  - A linked row **cannot exist without** a consent version — `consent_version`
    is `NOT NULL` and `Upsert` requires it (the value flows from `link/start`,
    Phase 4). (Validation session 1 — chose to persist consent, not just gate UI.)
  - Repository: `Upsert`, `GetByTeacher`, `Delete` (soft), `UpdateStatus`.
- Non-functional:
  - Production startup **fails** if `API_ZALO_CRED_KEY` is missing/short (like
    the statements key), because rotating it orphans every linked account —
    must be deliberate. Non-production falls back to a random per-process key
    with only a fingerprint logged.
  - `encrypted_credentials` is `BYTEA`; the plaintext is never a column.
  - No credential bytes in any log — the crypto helper never logs; the repo
    never logs row contents.

## Architecture

**Envelope format** (`shared/crypto`): `nonce(12) || AES-GCM(plaintext)`. The
KEK is process-wide, injected once at construction — a `Cipher` struct holds the
`cipher.AEAD`, constructed from config in the container. This mirrors how
`StatementsConfig` resolves its key in `validateStatements`, so the ops story is
identical to a key teachers already manage.

> Note: goclaw's `protocol/crypto.go` has a `DecodeAESGCM` with a **non-standard
> 16-byte nonce** for *Zalo's* wire format. That is unrelated — do not reuse it.
> Teka's at-rest encryption is standard 12-byte-nonce AES-GCM via
> `crypto/cipher.NewGCM`, entirely our own.

**Table** (`zalo_accounts`) — new migration `000004_zalo_accounts`:

```sql
CREATE TABLE zalo_accounts (
    teacher_id             UUID PRIMARY KEY REFERENCES teachers(id) ON DELETE CASCADE,
    encrypted_credentials  BYTEA        NOT NULL,
    zalo_uid               VARCHAR(50),
    display_name           VARCHAR(100),
    status                 VARCHAR(20)  NOT NULL DEFAULT 'linked'
                               CHECK (status IN ('linked','expired')),
    consent_version        VARCHAR(20)  NOT NULL,
    consent_at             TIMESTAMPTZ  NOT NULL DEFAULT now(),
    linked_at              TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_verified_at       TIMESTAMPTZ,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at             TIMESTAMPTZ
);
```

`teacher_id` as PK enforces one account per teacher (confirmed as a permanent v1
constraint in validation session 1). `consent_version` is `NOT NULL` so a linked
row is always backed by a recorded consent acknowledgement; `consent_at` stamps
when. No index on secrets. Down migration drops the table.

**Config:** add `ZaloConfig{ CredKeyRaw string; CredKey []byte }` to `Config`,
validated in a new `validateZalo()` called from `validate()`. Follow
`decodeTokenKey`/`validateStatements` verbatim in shape.

## Related Code Files

- Create: `apps/api/internal/shared/crypto/crypto.go`
- Create: `apps/api/internal/shared/crypto/crypto_test.go`
- Create: `apps/api/migrations/000004_zalo_accounts.up.sql`
- Create: `apps/api/migrations/000004_zalo_accounts.down.sql`
- Create: `apps/api/internal/features/zalo/model.go` (`Account` gorm model, `TableName`, status consts)
- Create: `apps/api/internal/features/zalo/repository.go`
- Modify: `apps/api/internal/config/config.go` (add `ZaloConfig`, `validateZalo`)
- Modify: `apps/api/internal/config/config_test.go` (key decode + prod-required cases)
- Modify: `.env.example` (document `API_ZALO_CRED_KEY`)

## Implementation Steps

1. Write `shared/crypto`: a `Cipher` built from a `[]byte` key; `Seal`/`Open`
   with prepended random nonce; error (not panic) on short key or GCM open
   failure. Table-test round-trip, tamper-detection (flip a byte → `Open` errors),
   and wrong-key rejection.
2. Add `ZaloConfig` + `validateZalo` to config, reusing `decodeTokenKey`. Add
   config tests: prod missing key → error; dev missing key → random + warn.
3. Write migration `000004` up/down. Confirm it applies against the dev DB
   (`migrations_test.go` already runs up/down — extend if it enumerates tables).
4. Write `model.go` (`Account` gorm model incl. `ConsentVersion`/`ConsentAt`,
   `TableName`, status consts) + `repository.go` (`Upsert` via
   `clause.OnConflict{Columns: teacher_id, UpdateAll}` — carries the consent
   version, `GetByTeacher`, soft `Delete`, `UpdateStatus`). Repo takes `*gorm.DB`,
   matches sibling repos.
5. `go test ./internal/shared/crypto/... ./internal/config/... ./internal/features/zalo/... ./migrations/...`.

## Success Criteria

- [x] `Seal`→`Open` round-trips; a tampered byte makes `Open` error; a wrong key
      errors. Verified: `internal/shared/secrets/secrets_test.go` —
      `TestSealOpenRoundTrip`, `TestOpenRejectsTamperedCiphertext`,
      `TestOpenRejectsWrongKey`, `TestOpenRejectsUndersizedInput`.
- [x] Production `Load()` errors without `API_ZALO_CRED_KEY`; dev logs only a
      fingerprint. Verified: `internal/config/config.go:252-269` (`validateZalo`);
      `internal/config/config_test.go` table cases "missing zalo credential key
      in production" and "short zalo credential key in production", plus
      `TestZaloCredKeyDevFallback` for the dev fallback (fingerprint-only log at
      `config.go:268`).
- [x] Migration 000004 applies and rolls back cleanly. Verified:
      `migrations/000004_zalo_accounts.{up,down}.sql` present;
      `migrations/migrations_test.go` exercises `zalo_accounts` in its up/down
      cycle, run against a real Postgres instance via
      `go test -tags=integration ./...` (clean, `go vet -tags=integration
      ./...` clean too).
- [x] `Upsert` twice for one teacher leaves exactly one row (PK conflict
      updates). Verified: `repository.go:59-86` uses
      `clause.OnConflict{Columns: teacher_id, UpdateAll: true}`;
      `TestUpsertTwiceKeepsExactlyOneRow` (`integration_test.go:93`), confirmed
      passing against real Postgres, not a stub.
- [x] `consent_version` is `NOT NULL`; an `Upsert` without a consent version fails
      (a linked row always carries an acknowledged consent version +
      `consent_at`). Verified: migration column is `NOT NULL`; `repository.go:59-62`
      returns `ErrConsentVersionRequired` on a blank version;
      `TestUpsertRejectsMissingConsentVersion` (`integration_test.go:117`),
      confirmed passing against real Postgres.
- [x] No plaintext credential path exists: `encrypted_credentials` is the only
      credential column and it is `BYTEA`. Verified: `migration 000004` schema
      and `model.go:35-54` (`Account` struct has no plaintext credential field,
      only `EncryptedCredentials []byte`, documented as sealed-only); the whole
      `internal/features/zalo` package, including `integration_test.go`, passes
      `go test -tags=integration ./...` against real Postgres.

## Execution Notes

- The envelope package landed at `internal/shared/secrets`, not
  `internal/shared/crypto`: `golangci-lint`'s revive `var-naming` rejects a
  package whose name shadows a standard library package, and a consumer would
  have had to alias either it or `crypto/rand`. API is otherwise as designed —
  `secrets.New(key) (*secrets.Cipher, error)`, `Seal`, `Open`.
- `Cipher` derives its AES-256 key as SHA-256 of the configured bytes rather
  than requiring exactly 32: `decodeTokenKey` can return any length, and
  hashing accepts every documented encoding without truncating a longer key.
- `Upsert` fills `Status`, `ConsentAt`, and `LinkedAt` when zero. GORM writes
  every mapped field, so the column defaults in the migration would never fire
  — a zero `time.Time` would be stored as year 1, not `now()`.
- `API_ZALO_CRED_KEY` also had to be wired into `docker-compose.yml` and both
  services in `docker-compose.prod.yml`. The prod `migrate` service runs config
  validation under `API_ENV=production`, so without it the deploy would fail
  before migrating.

## Risk Assessment

- **KEK loss = permanent unlink for all teachers.** Mitigation: prod-required +
  fingerprint logging so a wrong/rotated key is caught at boot, and document in
  `.env.example` that rotating it forces every teacher to re-scan (same wording
  as the statements key).
- **Encryption footgun (nonce reuse):** Mitigation: random nonce per `Seal` from
  `crypto/rand`, never a counter; unit-tested that two seals of the same
  plaintext differ.
- **Migration ordering:** 000003 is the latest; use 000004. Verify no parallel
  plan claims that number (cross-plan scan: none found).
