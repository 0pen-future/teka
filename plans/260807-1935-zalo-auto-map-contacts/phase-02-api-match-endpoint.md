---
phase: 2
title: "API match and friend-request endpoints"
status: completed
priority: P1
effort: "1-1.5d"
dependencies: [1]
---

# Phase 2: API match and friend-request endpoints

## Overview

Expose the lookup as `POST /api/v1/me/zalo/friends/match`: the caller sends
phones, the service resolves them against Zalo in paced chunks, intersects with
the friend list, and returns labeled suggestions. Nothing is persisted — the
confirm write stays on the existing contacts zalo-mapping endpoint. A second
endpoint, `POST /api/v1/me/zalo/friends/request`, sends exactly one friend
request per call.

<!-- Updated: Validation Session 1 - friend-request endpoint added -->

## Requirements

- Functional: request `{"phones": ["..."]}` (1–200 entries) → response rows
  `{phone, matched, user_id, display_name, zalo_name, avatar, is_friend}`;
  unresolved phones come back `matched: false`.
- Functional: phone normalization — trim, strip spaces/dots, `+84…`/`84…` → `0…`
  — one tested helper; response rows echo the phone exactly as the caller sent
  it so the web can join rows back to contacts without re-normalizing.
- Functional: `POST /me/zalo/friends/request` takes `{"user_id": "...",
  "message": "..."}` (message optional, defaulted to a short Vietnamese
  greeting) and sends exactly one request per call — the contract itself
  prevents bulk sending; no batch variant exists.
- Functional: not-linked and expired-session cases answer exactly like
  `GET /me/zalo/friends` does today (same error codes, same relogin path) —
  for both new endpoints.
- Non-functional: chunked lookups (30 phones/call) with 1–3s jitter between
  chunks; 200-phone cap keeps worst case ~7 chunks ≈ 21s, inside the server's
  30s write timeout.
- Non-functional: log chunk counts and durations only — never phone numbers;
  no credential material in any response.

## Architecture

**Boundary decision:** the endpoint lives in the `zalo` feature and takes raw
phones, not contact IDs. The zalo feature keeps zero knowledge of contacts; the
web joins phone→contact client-side (it already holds the contact list). This
avoids a cross-feature repo dependency for no gain — the alternative (server
joins contacts) would couple `zalo` to the contacts repository and duplicate a
join the client gets for free.

Service flow (`MatchFriends(ctx, teacherID, phones)`):

1. `sessionFor` (existing: cache hit or relogin from stored credentials).
2. Normalize phones; drop empties; keep an original→normalized index.
3. For each chunk of 30: `protocol.FindUser`; pace with jitter between chunks
   (not after the last one).
4. One `protocol.FetchFriends`; build a UID set; label each found row
   `is_friend`.
5. Return rows in request order.

Testability seams, following the `ServiceOptions` precedent (`Login`, `Relogin`,
`Friends` already exist):

- `FindUser` func field — handler/service tests inject fakes.
- `SendFriendRequest` func field — same pattern, wraps
  `protocol.SendFriendRequest`.
- `Pace func(ctx context.Context)` field — nil default sleeps rand 1–3s;
  tests inject a no-op (no real-timer tests, per the project's earlier call).

`SendRequest(ctx, teacherID, userID, message)` on the service is a thin
wrapper: `sessionFor` → one `protocol.SendFriendRequest` call. No pacing needed
— the one-per-call contract plus an explicit click per person is the rate
limit.

Concurrency: no server-side in-flight guard this iteration — the FE disables
the button while pending, and a double-click costs only duplicate lookups, not
writes. Recorded as a risk, not built (YAGNI).

## Related Code Files

- Modify: `apps/api/internal/features/zalo/service.go` — `MatchFriends`,
  `SendRequest`, `ServiceOptions.FindUser`, `ServiceOptions.SendFriendRequest`,
  `ServiceOptions.Pace`, phone-normalize helper.
- Modify: `apps/api/internal/features/zalo/handler.go` + `routes.go` —
  `POST /me/zalo/friends/match`, `POST /me/zalo/friends/request`, swag
  annotations.
- Modify: `apps/api/internal/features/zalo/dto.go` (or the file holding
  `FriendResponse`) — request/response DTOs; response types must offer no place
  for credentials (extend `TestResponseTypesHaveNoCredentialFields`).
- Modify: `apps/api/internal/features/zalo/service_test.go`,
  `handler_test.go` — TDD tests; extend
  `TestNoResponseCarriesCredentialMaterial` to hit the new route.
- Modify: `apps/api/docs/*` via `make api-docs` (generated).

## Implementation Steps (TDD)

1. Red (service): normalization table test; chunking test (61 phones → 3
   `FindUser` calls, 2 paces); intersection test (found UID in/out of friend
   list → `is_friend` true/false); not-linked and expired paths reuse existing
   error expectations; request-order preservation.
2. Red (handler): 400 on empty/oversized phone list; happy path envelope;
   auth required; canary test extended to both new routes.
3. Red (request endpoint): 400 on missing user_id; happy path calls the
   injected `SendFriendRequest` once with the given UID and message; protocol
   error surfaces as the standard error envelope.
4. Green: implement service + handler + routes + DTOs.
5. `make api-docs`; run zalo package, `-race`, and `-tags=integration` suites
   in docker; `go build ./...`.
6. **Live PoC (manual, before phase 3):** with the already-linked production
   account, call the deployed endpoint with 2–3 known parent phones (curl,
   redacted output). Confirms: endpoint liveness, VN phone format assumption
   (`0…` local), and real-world resolve rate. If the format assumption fails,
   adjust the normalize helper — the seam is one function.
   Review 260807 flagged two extra checks: (a) try both `0xx` and `84xx` for the
   same number — upstream zca-js normalizes `0xx`→`84xx` for `vi`, so `0xx` may
   silently resolve nothing; (b) record the wire shape when no phone matches
   (`{}` vs `[]` vs error code) — `FindUser` currently hard-fails on a `[]`
   data field and must be hardened only if that shape is real.

## Success Criteria

- [x] `POST /me/zalo/friends/match` returns labeled rows, paced and capped,
      with parity error handling with `GET /me/zalo/friends`.
- [x] `POST /me/zalo/friends/request` sends exactly one request per call with
      the same error parity; no batch variant exists.
- [x] Canary + response-shape credential tests cover both new routes.
- [ ] Live PoC resolved at least one real phone and settled the format question.
      (Deferred to pre-ship by user decision at the phase 2 gate; the server-side
      normalize helper is the single seam to adjust if the assumption is wrong.)
- [x] All API suites green (unit, handler, `-race`, `-tags=integration`).

Review 260807 (session 2) outcome: the dead-session branch on the friend-list
leg was dead code because `protocol.FetchFriends` returned a plain error —
fixed by returning `*APIError{Op:"friends", Code}` and routing `ListFriends` and
the match's friends leg through the shared expiry helper (so `GET
/me/zalo/friends` now answers 409, not 500, when the session dies mid-fetch).
Also added: per-element VN-mobile filtering before anything travels to Zalo,
dedupe-once-label-all behavior with tests, and skipping the friend-list fetch
when no phone resolved. Deliberately not built: a service-side deadline for the
match call (trade-off surfaced at the phase gate) and request rate limiting for
`/friends/request` (the one-per-call contract plus FE button state remains the
rate limit this iteration).

## Risk Assessment

- **Anti-abuse**: even chunked, lookups are burst-like traffic. Mitigation:
  jittered pacing, 200 cap, on-demand only (no background sync). If Zalo still
  balks (observed via PoC/logs), drop chunk size before adding infrastructure.
- **Low resolve rate**: parents with phone-discovery disabled return nothing.
  The PoC measures the real rate before the UI phase invests further; the
  manual picker remains the universal fallback.
- **Double-submit races**: acceptable duplicate lookups; revisit only if logs
  show it happening in practice.
