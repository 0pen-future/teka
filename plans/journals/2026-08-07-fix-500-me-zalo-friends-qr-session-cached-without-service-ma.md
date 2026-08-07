---
title: "Fix 500 /me/zalo/friends: QR session cached without service map"
date: 2026-08-07
summary: "persistLink now proves credentials with a cookie login and caches that session, not the bare QR session"
---

# Fix 500 /me/zalo/friends: QR session cached without service map

## What happened

Production logged `GET /api/v1/me/zalo/friends` → 500 `zalo_personal: no profile service URL` for a freshly QR-linked teacher. Root cause in `apps/api/internal/features/zalo/service.go`: `persistLink` cached the QR-handshake session, but only `protocol.LoginWithCredentials` populates `sess.LoginInfo` (Zalo's service map with the chat/profile URLs). `LoginQR` never does — its own comment says the caller must validate credentials via a fresh cookie login, which was never implemented. Every service-map consumer (`FetchFriends`, `SendMessage`) failed until the cache was evicted and `sessionFor` relogged in.

## Decision

`persistLink` now runs the cookie login on a fresh session before storing anything: a rejected login fails the link (`LinkStateError`, nothing persisted, nothing cached) instead of storing a dead credential blob, and the cached session is the one the cookie login produced. The UID Zalo reports at credential login wins over the QR session's. Regression is locked by asserting `protocol.ServiceURL(cachedSess, "profile")` is non-empty in the link test.

Reviewer caught one blocking collateral: `integration_test.go` (build tag `integration`, run by `make test-api` in CI) had no `Relogin` stub, so the new validation would have called Zalo for real from CI. Stubbed. Contract comments on `OnLinked`/`persistTimeout` updated to admit the hook now includes a network login.

Deferred (contract decisions, noted for later): relogin and the DB write share the 10s `persistTimeout` (a slow login can starve the write); transient transport errors during relogin fail the whole link the same as rejected credentials; `Unlink` can now block up to ~10s when racing a just-completed scan.

## Next steps

Rebuilt `teka-api:local` and redeployed the homelab stack; `/readyz` 200. The restart also cleared the already-linked teacher's poisoned cache. Working tree on master remains uncommitted (user commits themselves).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
