---
title: "Zalo auto-map contacts: phases 1-3 delivered TDD"
date: 2026-08-07
summary: "FindUser/SendFriendRequest port, paced match API, web auto-map dialog + nav restructure; review caught a timeout-vs-pacing defect fixed by request splitting"
---

# Zalo auto-map contacts: phases 1-3 delivered TDD

## What happened

Executed the 3-phase plan `plans/260807-1935-zalo-auto-map-contacts` end to end with TDD (Red→Green per phase).

- **Phase 1 (protocol):** ported `FindUser` (batch phone → account via `{friend}/api/friend/profile/multiget`) and `SendFriendRequest` into the quarantined protocol package.
- **Phase 2 (API):** `POST /me/zalo/friends/match` (paced chunks of 30, 1–3s jitter, cap 200, rows labeled friend/not-friend/not-found) and `POST /me/zalo/friends/request` (one user per call — the contract is the rate limit). Canary tests extended; logs carry counts, never phone numbers.
- **Phase 3 (web):** "Phụ huynh" nav entry (sidebar/rail) + bottom-bar restructure to 4 primary tabs + "Thêm" sheet; "Tự động ghép Zalo" review dialog on /contacts (scan unmapped contacts paged, one match lookup per open, three row groups, confirm writes only checked rows, "Đã ghép N/M" summary); per-row "Kết bạn" button. No bulk friend-request control exists anywhere.

## Review findings that mattered

The phase 3 code-reviewer round 1 caught two real blockers that tests could not (msw answers instantly):

- **C1:** one 200-phone match request can never finish — the server sleeps 1–3s per 30-phone chunk, but the axios instance default timeout is 10s (and the server WriteTimeout is 30s). Fixed by splitting client-side into requests of `ZALO_MATCH_REQUEST_SIZE = 100` with a per-request 30s timeout; the cap test now pins the `[100, 100]` split.
- **C2:** the fire-once ref guard is per-mount, so close→reopen stacked concurrent live Zalo lookups from the teacher's personal account. Fixed with an AbortController threaded through the mutation into axios; every dismiss path funnels through `handleOpenChange`, which aborts. Deliberately NOT aborted in the effect cleanup — StrictMode's mount→cleanup→mount would kill the one permitted lookup.

Also fixed: confirm loop stops on 401 and dedupes by Zalo user id (siblings' guardians share phones — a second write can only 409), scan query key moved out from under `contactsKeys.lists()` so the confirm invalidation cannot replay the paged scan, detail cache invalidated, page-boundary duplicate rows deduped by id (name sort has no tiebreaker), disabled trigger explains itself inline with a /profile link (title tooltips never surface on touch). Round 2 verdict: DONE, no blockers.

## Decision

- Match is a TanStack **mutation**, not a query: every call is a live paced Zalo lookup and must never refetch on remount/focus.
- Cap 200 stays (mirrors server `zalo.MaxMatchPhones`); transport fits it by splitting requests, not by shrinking the product scope.
- Live PoC (phone-format assumption `+84…`→`0…`) deferred to pre-ship by user decision at the phase 2 gate; the server-side normalize helper is the single seam if wrong.

## Next steps

- Pre-ship: live PoC against the deployed match endpoint with 2–3 real (redacted) parent phones, both `0xx` and `84xx` forms; manual 360px pass on the bottom bar.
- Optional hardening flagged by review, untested today: abort-on-dismiss, dismissal block while saving, 401 loop break.
- User chose not to commit yet; working tree holds phases 1–3 uncommitted.

Gates at close: web vitest 194/194, eslint 0 errors, tsc clean, production bundle OK; API suites green from phase 2.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
