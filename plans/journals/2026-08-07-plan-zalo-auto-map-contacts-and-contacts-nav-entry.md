---
title: Plan Zalo auto-map contacts and contacts nav entry
date: 2026-08-07
summary: "3-phase plan: port zcago FindUser, paced /me/zalo/friends/match endpoint, contacts nav + review-confirm auto-map UI"
---

# Plan Zalo auto-map contacts and contacts nav entry

## What happened

Consultation ("could auto map phone number of contactor to zalo friend") established that offline matching is impossible — `FetchFriends` returns no phone numbers — so the only path is Zalo's reverse lookup. Verified upstream `zcago/api/find_user.go`: batch `GET {friend-service}/api/friend/profile/multiget` with AES-encrypted phones, response keyed by phone. Our `ZpwServiceMapV3` never parsed the `friend` service; that gap is phase 1.

Created `plans/260807-1935-zalo-auto-map-contacts/` (3 phases, ~3d):

1. **Protocol FindUser port** — add `Friend` service map field + `ServiceURL` case, port `FindUser` beside `FetchFriends` with golden tests; fix `doc.go` scope drift.
2. **API match endpoint** — `POST /me/zalo/friends/match` (1–200 phones, chunks of 30 with 1–3s jitter, friend-list intersection, request-order rows). Boundary decision: zalo feature takes raw phones, web joins client-side — no cross-feature repo coupling. Ends with a live PoC against the linked production account to settle the VN phone format question before any UI work.
3. **Web menu + auto-map UI** — "Phụ huynh" nav entry to /contacts in all three nav variants; "Tự động ghép Zalo" review dialog (matched default-checked, not-friend/not-found display-only), confirm writes via existing `PUT /contacts/{id}/zalo-mapping`.

## Decision

Suggestion engine, not silent auto-write: server only returns labeled candidates; nothing persists until the teacher confirms. A wrong mapping sends one family's debt to another — one review click is cheap insurance. No friend-request sending this iteration; manual picker stays as fallback.

## Next steps

Validate or red-team the plan, then `/ak:cook` phase 1. Open questions ride on phase 2's PoC (phone format) and real-use feedback (7-tab bottom bar).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
