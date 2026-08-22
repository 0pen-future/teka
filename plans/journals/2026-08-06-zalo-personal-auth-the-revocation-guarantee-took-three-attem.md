---
title: "Zalo personal auth: the revocation guarantee took three attempts"
date: 2026-08-06
summary: Five phases delivered TDD; the consent-revocation guarantee needed three fixes across two review rounds before it held
---

# Zalo personal auth: the revocation guarantee took three attempts

## What happened

Delivered the five-phase Zalo personal-account link plan in TDD mode: a reverse-engineered protocol port, AES-GCM credential storage, an in-process session manager with QR link flow and health probe, the HTTP API, and the profile-page connect card. The interesting part was not the feature. It was one acceptance criterion — after `DELETE /me/zalo` returns, no usable credential may survive — which took three attempts and two adversarial review rounds to actually hold.

**Round one (review finding H-2).** A scan landing between `Cancel` and `repo.Delete` re-`Upsert`s the row. The persist deliberately runs on `context.WithoutCancel`, so a slow database cannot lose a link a teacher completed. The review suggested re-checking attempt ownership before the write. I rejected that as insufficient — the check and the write are not atomic — and instead made `Cancel` wait on the attempt's `done` channel.

**Round two (finding F1).** The rejection was wrong, and the wait made things worse. `Cancel` waited only on the record still in `active`. An attempt superseded by reopening the QR modal is cancelled and dropped from the map without a wait, so its persist could still land after the delete — and `repository.go` clears `deleted_at` with `OnConflict{UpdateAll}`, so it revived the row with live credentials. Worse than the original bug: `update` no-ops for a superseded record, so no state anywhere reflected it. The profile card would read "chưa kết nối" over a row holding a working account session.

The fix needed both mechanisms, not either one. `LinkManager` now tracks a `live` set of every attempt whose goroutine is still running, not just the one it names, so `Cancel` and `Close` wait on superseded and swept records too; and `run` re-checks `isCurrent` before calling `OnLinked`, so an abandoned scan is never stored. The gate covers everything the manager has let go of. The wait covers the instant between passing the gate and the write landing — which is exactly the non-atomicity I had cited as the reason to skip the gate.

**Round three (finding M-1).** With the row safe, a second reviewer found the same guarantee broken one layer over. `sessionFor` evicts, reads the row, re-logs-in over the network for seconds, then caches. Deleting Teka's row revokes nothing on Zalo's side, so a health check that started before an unlink still succeeds after it and caches a working session for a teacher who revoked consent — and nothing ever drops it, because the probe only sweeps accounts still linked. `SessionCache` now counts evictions per teacher; `sessionFor` reads the count before it starts and stores through `PutUnlessEvicted`, which refuses if the count moved.

## Decision

Fixed M-1 rather than deferring it, against the review's own "latent, non-blocking" calibration. It has no reachable consumer today. But the feature is entirely uncommitted, so "pre-existing" meant "from an earlier phase of this same unshipped work", and deferring would have meant shipping a known revocation hole on the assumption a future change remembers it.

Kept `DELETE /me/zalo` idempotent at 204 per the accepted plan; the delivered 404 was an implementation deviation with three tests pinning it, not a recorded decision. The third test only surfaced once the integration suite actually ran — which needed the Docker socket mounted into the toolchain container. Without that, the tests fail while provisioning Postgres, which reads like an environment error rather than a result.

## What this cost, and what it teaches

Every one of these three bugs was invisible to a passing test suite and to my own reading. All three were found by adversarial review with a concrete interleaving attached, and all three were confirmed by writing the failing test first and watching it fail against the current code. Two of my own fixes passed on first run — which, for a race, proves nothing; reverting to check the test actually fails is what caught the false pass.

The pattern behind all three: an operation that deliberately survives cancellation (the persist, the login) is exactly the operation a revocation has to outrank, and every layer holding derived state — the map, the row, the cache — needs its own fence. Fixing one layer moves the bug rather than closing it.

## Next steps

- Not committed; 43 paths left in the working tree at the user's request.
- Health probe still has no durable audit trail when it expires an account — open from the first review, unaffected by any of this.
- `API_STATEMENTS_TOKEN_KEY` has the same fatal-in-production, missing-from-docs gap that `API_ZALO_CRED_KEY` had. Out of scope here, worth filing.
- Sending messages, contact-to-friend mapping, and paced bulk runs remain the next milestone. The first consumer of `sessionFor` is what makes the cache fence load-bearing rather than precautionary.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
