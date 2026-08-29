---
title: "Zalo invite DM failed: seedServiceMapCookies omitted the friend service host"
date: 2026-08-29
summary: "Invite dm_status=failed in prod; friend-service host never got session cookies seeded, so FindUser/SendFriendRequest ran unauthenticated"
---

# Zalo invite DM failed: seedServiceMapCookies omitted the friend service host

## What happened

Prod invite for 0389044962 returned 201 but `dm_status: "failed"` ("Gửi Zalo
thất bại"). Debug report `plans/reports/debug-260829-2039-zalo-invite-dm-failed.md`
pinned the cause: `seedServiceMapCookies`
(`apps/api/internal/features/zalo/protocol/auth.go`) seeded session cookies for
Chat/Group/File/Profile/GroupPoll hosts but omitted `sm.Friend`. Stored
credentials are host-only cookies (Domain stripped), and Go's cookiejar does
not propagate them across subdomains — so every `FindUser`/`SendFriendRequest`
call hit `tt-friend-wpa.chat.zalo.me` without `zpw_sek` and failed. A second
defect hid it: `invitations.attemptDM` swallowed the error without logging.

## Decision

- One-line root-cause fix: append `sm.Friend` to the seeded URL list. Seed list
  now covers 100% of `ZpwServiceMapV3` fields and the `ServiceURL` switch.
- Observability: `attemptDM` now `slog.Warn`s lookup/send failures (teacher_id +
  error, no phone/link — link carries the plaintext invite token), matching the
  existing `auth.attemptResetDM` precedent.
- Regression test `TestSeedServiceMapCookies_CoversEveryServiceHost`: proven to
  fail without the fix; asserts the `zpw_sek` cookie by name on every host and
  fails via `reflect.NumField` if a 7th service field appears without coverage.
- Verified: build, vet, blast-radius packages (zalo/protocol, zalo, invitations,
  auth) and full `make test-api-unit` green. Code review: 9/10, no critical.

## Next steps

- Commit + deploy; cached prod sessions are stale until reseeded via
  `LoginWithCredentials` on restart (rolling restart clears them).
- Re-run invite for 0389044962: expect `sent`; `skipped` would itself be a bug
  (owner confirmed the phone IS a Zalo friend).
- Watch first post-deploy auto-map run — friend-service calls go live for real.
- `mau-nhap-du-lieu-trung-tam.xlsx` sits untracked at repo root and is not
  gitignored; do not `git add -A` (may hold real personal data).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
