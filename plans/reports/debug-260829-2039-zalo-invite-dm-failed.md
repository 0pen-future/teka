# Debug: "Gửi Zalo thất bại" khi mời 0389044962 vào center

Date: 2026-08-29 | Env: production (teka-api-1) | Status: fix applied locally 2026-08-29 (not yet deployed)

## Executive summary

Invite create succeeds (201, link OK) but `dm_status: "failed"` because the
best-effort Zalo DM dies at the phone→UID lookup step. Root cause (high
confidence): `seedServiceMapCookies` never seeds session cookies for the
**friend** service hosts, so every `FindUser` / `SendFriendRequest` call hits
Zalo unauthenticated and errors. A secondary defect hides it: `attemptDM`
swallows the error without logging.

## Evidence

- UI label maps `dm_status=failed` → "Gửi Zalo thất bại"
  (`apps/web/src/features/invitation/components/copy-link-dialog.tsx:21`).
- `attemptDM` returns `failed` only when `LookupPhone`/`SendDM` error with
  something other than `ErrNotLinked`
  (`apps/api/internal/features/invitations/service.go:212-229`). Error is
  discarded — nothing logged.
- Prod log 2026-08-29 13:37:31 & 13:38:25: `POST /centers/me/invitations` 201 in
  116ms/111ms, **no** `zalo: matched phones…` INFO line (fires on every
  MatchFriends success → MatchFriends errored), **no** sessionFor warn/error
  (all its failure paths log → session came from cache fine). So failure is in
  `findUser` → `protocol.FindUser`, fast and silent.
- Working vs broken splits exactly along service hosts:
  - `ListFriends` → service **profile** → worked 13:33:32 (200, 373ms).
  - `FindUser`/`SendFriendRequest` → service **friend** → zero successes in the
    container's entire log (`matched phones` count = 0, `friend request sent`
    count = 0).
- `seedServiceMapCookies` (`apps/api/internal/features/zalo/protocol/auth.go:124-153`)
  seeds Chat, Group, File, Profile, GroupPoll — **omits `sm.Friend`**. Cookies
  are set per-host (`BuildCookieJar` → `jar.SetCookies(u, …)`, config.go:189),
  and the function exists precisely because Go's cookiejar does not propagate
  across Zalo subdomains. Friend-service host (distinct subdomain, e.g.
  `tt-friend-wpa.chat.zalo.me`) therefore carries no `zpw_sek` cookie.
- Unauthenticated call outcome: redirect-to-login/HTML → `readJSON` decode
  error (matches: no `rejected as not logged in` warn, no auto-expire — so it
  was NOT an error_code=-3 envelope), returned as generic error → `failed`.

## Ruled out

- Owner not linked / session expired → would be `skipped` or logged warns; none.
- Phone form: `+84389044962` (storage form) normalizes to `0389044962`, matches
  `vnMobilePattern`, sent as `84389044962` — valid.
- First link attempt 13:33:08 failed (`login failed (empty response)`) but the
  13:33:15 retry succeeded; friends fetch after it worked.

## Fix (applied 2026-08-29, uncommitted)

1. `auth.go` `seedServiceMapCookies`: added `allURLs = append(allURLs, sm.Friend...)`.
   Regression test `TestSeedServiceMapCookies_CoversEveryServiceHost` (auth_test.go)
   fails without the fix, passes with it.
2. Observability: `invitations.attemptDM` now `slog.Warn`s lookup/send failures
   (teacher_id + error, no phone/PII). Password-reset seam already logged
   (`auth/service.go:426-432`) — no change needed there.
3. Verify: relink not needed (seeding happens on every `LoginWithCredentials`),
   but cached sessions are stale → after deploy, session cache rebuilds on next
   restore; re-run invite for 0389044962 and expect `sent`. Owner confirmed
   (2026-08-29) the phone IS already a Zalo friend, so `skipped` after the fix
   would itself be a bug (lookup resolving but friendship labeling failing).

## Unresolved questions

- Cannot 100% confirm the friend-host response shape (no live-credential test;
  prod DB query blocked by permissions). If after the cookie fix lookup still
  fails, next suspect is `ZpwServiceMapV3.Friend` empty in the login payload
  ("no friend service URL" is the other silent instant-fail path in FindUser).
