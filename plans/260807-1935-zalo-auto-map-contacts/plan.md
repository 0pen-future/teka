---
title: "Zalo Auto-Map Contacts"
description: "Auto-suggest contact→Zalo-friend mapping by phone lookup, plus a Contacts entry in the app nav"
status: done
priority: P1
effort: "4d"
tags: [zalo, contacts, web, protocol]
created: 2026-08-07
blocks: [260811-1055-manager-class-oversight]
---

# Zalo Auto-Map Contacts

## Overview

Two deliverables in one flow:

1. **Auto-map by phone.** Port Zalo's `FindUser` lookup (batch phone → account)
   into the protocol package, expose a paced `POST /me/zalo/friends/match`
   endpoint, and give the contacts page a one-click "Tự động ghép Zalo" flow
   that suggests mappings for review. Confirmed rows are written through the
   existing `PUT /contacts/{id}/zalo-mapping`. The manual picker stays as the
   fallback for contacts the lookup cannot resolve. Found-but-not-friend rows
   get a per-person "Kết bạn" button (ported `SendFriendRequest`, one explicit
   click per person — never bulk).
2. **Contacts nav entry.** `/contacts` and its Zalo picker exist but are
   unreachable from the app shell. Add a "Phụ huynh" entry to the sidebar and
   rail; the mobile bottom bar is restructured to primary tabs plus a "Thêm"
   sheet so it never exceeds five slots.

**Design decision (from consultation 260807):** this is a *suggestion engine,
not silent auto-write*. The server only returns candidates; nothing is
persisted until the teacher confirms. A wrong mapping sends one family's debt
to another family — the cost of one review click is worth it.

Key protocol facts (verified against upstream `zcago` source, 260807):

- `FetchFriends` returns no phone numbers, so offline matching is impossible;
  the only path is Zalo's reverse lookup.
- Upstream `api/find_user.go` calls `{friend-service}/api/friend/profile/multiget`
  (GET, AES-encrypted `{"phones": [...], "avatar_size": 240, "language": ...}`)
  and **accepts a batch of phones per call** — response is a map keyed by phone
  with `uid`, `display_name`, `zalo_name`, `avatar`.
- Our `ZpwServiceMapV3` does not yet parse the `friend` service; the field and
  a `ServiceURL` case must be added.
- Lookup only finds accounts whose privacy allows phone discovery, and a found
  account is not necessarily a friend — results must be intersected with
  `FetchFriends` and labeled (`friend` / `not friend` / `not found`).

## Constraints

- Protocol package stays quarantined: no imports from the rest of Teka.
- Lookups are paced (chunked batches with jitter) — a burst of lookups is the
  classic bot pattern and risks the teacher's account.
- Phones are personal data: log counts, never numbers; responses must carry no
  credential material (extend the existing canary test).
- Friend requests are sent one at a time by an explicit teacher click per
  person — no bulk send, no auto-send. A burst of requests is the classic bot
  pattern and risks the teacher's account.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Teacher reaches /contacts from the app menu on mobile, tablet, and desktop | P1 |
| 2 | One click suggests Zalo mappings for all unmapped contacts; teacher bulk-confirms | P1 |
| 3 | Unresolvable contacts keep the existing manual picker as fallback | P1 |
| 4 | Teacher can send a friend request to a found-but-not-friend contact, one click per person | P2 |

## Phases

| # | Phase | Status | Depends on | Effort |
|---|-------|--------|------------|--------|
| 1 | [Protocol FindUser + SendFriendRequest port](./phase-01-start.md) | Done | — | 1d |
| 2 | [API match + friend-request endpoints](./phase-02-api-match-endpoint.md) | Done (PoC pre-ship) | 1 | 1–1.5d |
| 3 | [Web menu, more-sheet, and auto-map UI](./phase-03-web-menu-and-auto-map-ui.md) | Done | 2 | 1.5–2d |

## Success Criteria

- [x] "Phụ huynh" nav entry navigates to /contacts from sidebar, rail, and the mobile "Thêm" sheet; bottom bar shows at most five slots and remains usable at 360px width.
- [x] `POST /me/zalo/friends/match` returns per-phone results labeled friend / not-friend / not-found, paced in chunks, capped per request.
- [x] "Tự động ghép Zalo" on /contacts proposes mappings; confirming writes only accepted rows via the existing zalo-mapping endpoint; a summary reports mapped/skipped counts.
- [x] Each not-friend row offers a per-person "Kết bạn" action backed by `POST /me/zalo/friends/request`; no bulk or automatic sending exists anywhere in the flow.
- [x] No response carries credential material (canary test extended to the new endpoints); phone numbers never appear in logs.
- [x] All gates green: API unit + handler + `-tags=integration` tests, web vitest + lint + typecheck + build.

## Open Questions

1. Exact phone format `multiget` expects for VN numbers (`0xxx` vs `84xxx`) —
   resolved by the live PoC step in phase 2; normalization assumption is
   `+84…`/`84…` → `0…`.

## Validation Log

### Session 1 — 2026-08-07
**Trigger:** post-plan `/ak:plan validate` chosen at handoff.
**Questions asked:** 4

#### Questions & Answers

1. **[Scope]** Nhãn menu điều hướng tới /contacts nên là gì?
   - Options: Phụ huynh (Recommended) | Liên hệ | Danh bạ
   - **Answer:** Phụ huynh
   - **Rationale:** khớp ngôn ngữ giáo viên; contacts trong hệ thống là phụ huynh học sinh.
2. **[Assumptions]** Chiến lược định dạng số điện thoại khi tra cứu Zalo?
   - Options: Chuẩn hóa về 0xxx, PoC quyết (Recommended) | Gửi cả hai biến thể mỗi số | Chuẩn hóa về 84xxx
   - **Answer:** Chuẩn hóa về 0xxx, PoC quyết
   - **Rationale:** giữ số lượng tra cứu tối thiểu; sai giả định chỉ sửa một helper sau PoC phase 2.
3. **[Tradeoffs]** Bottom bar mobile sẽ tăng lên 7 tab — xử lý thế nào?
   - Options: Chấp nhận 7 tab (Recommended) | Làm "more" sheet ngay
   - **Answer:** Làm "more" sheet ngay
   - **Rationale:** đảo khuyến nghị YAGNI — bottom bar giữ tối đa 5 slot (4 tab chính + "Thêm"), các mục còn lại vào sheet. Tăng effort phase 3 lên 1.5–2d.
4. **[Scope]** Số tìm thấy tài khoản nhưng chưa kết bạn — có gửi lời mời kết bạn không?
   - Options: Chỉ hiển thị (Recommended) | Thêm nút gửi kết bạn từng người
   - **Answer:** Thêm nút gửi kết bạn từng người
   - **Rationale:** đảo constraint gốc "no friend-request sending". Chốt an toàn thay thế: chỉ gửi từng người theo click tường minh, không bulk/auto. Kéo theo port `SendFriendRequest` (phase 1) và endpoint request (phase 2).

#### Confirmed Decisions
- Nav label: "Phụ huynh" — as planned.
- Phone normalization: `+84…`/`84…` → `0…`, live PoC in phase 2 settles it — as planned.
- Bottom bar: build the "Thêm" sheet now; ≤5 slots — reversal of the deferred option.
- Friend requests: per-person explicit send this iteration — reversal of the display-only constraint.

#### Action Items
- [x] Phase 1: add `SendFriendRequest` port (`POST {friend}/api/friend/sendreq`, upstream verified 260807).
- [x] Phase 2: add `POST /me/zalo/friends/request` endpoint + `SendFriendRequest` seam.
- [x] Phase 3: bottom bar "Thêm" sheet; "Kết bạn" button on not-friend rows.
- [x] Plan overview, constraints, goals, phases table, success criteria reconciled.

#### Impact on Phases
- Phase 1: scope + effort (0.5–1d → 1d); new endpoint port and tests.
- Phase 2: scope + effort (1d → 1–1.5d); new route, DTOs, canary coverage.
- Phase 3: scope + effort (1–1.5d → 1.5–2d); nav restructure + per-row action.

### Verification Results
- **Tier:** Standard (Fact Checker + Contract Verifier)
- **Claims checked:** 27
- **Verified:** 27 | **Failed:** 0 | **Unverified:** 0
- Spot checks: `ZpwServiceMapV3` models.go:14 (no `Friend` field), `ServiceURL` cases client.go:388-396 (no `"friend"`), `FetchFriends` contacts.go:13, helpers client.go:75/326/342, doc.go:8 drift confirmed, `ServiceOptions` service.go:58 (`Login`/`Relogin`/`Friends`), `sessionFor` service.go:295, routes.go `GET /friends`, `FriendResponse` dto.go:40, canary tests handler_test.go:372/418, `make api-docs` Makefile:75, `useNavEntries` dashboard-layout.tsx:32, profile zalo api/hooks/schemas files present, `useZaloFriends` exported index.ts:4, roster `__tests__/` with `contact-zalo-mapping.test.tsx`, zalo-mapping routes contacts/routes.go:14-15, send.go POST-form + `imei` pattern for the friend-request port.

### Session 2 — 2026-08-07 (execution)

- Phases 1–3 implemented TDD (Red→Green per phase); all criteria checked except
  the phase-2 live PoC, deferred to pre-ship by user decision at the gate.
- Phase 3 code review (DONE_WITH_CONCERNS) surfaced two blockers, both fixed:
  the match call now travels in requests of 100 phones with a 30s per-request
  timeout (the server paces Zalo lookups 1–3s per 30-phone chunk, so one
  200-phone request could not finish inside the 10s client default), and the
  dialog aborts an in-flight lookup on dismiss via AbortController so reopening
  cannot stack concurrent live Zalo lookups. Also fixed: confirm loop stops on
  401 and dedupes by Zalo user id (shared-phone guardians), scan query key moved
  out from under the invalidated list key, contact-detail cache invalidated,
  page-boundary duplicate rows deduped, disabled trigger explains itself.
- Remaining before ship: live PoC (phone-format assumption, redacted numbers),
  manual visual pass of the bottom bar at 360px against the deployed web app.
- Gates at close: API suites green (phase 2); web vitest 194/194, eslint 0
  errors, tsc clean, production bundle OK.

### Whole-Plan Consistency Sweep
- Files reread: plan.md, phase-01-start.md, phase-02-api-match-endpoint.md, phase-03-web-menu-and-auto-map-ui.md
- Decision deltas checked: 2 (friend-request reversal, more-sheet reversal)
- Reconciled stale references: 8 (overview, constraints, goals, phases table, success criteria, open questions, phase 1–3 bodies)
- Unresolved contradictions: 0

<!-- slug: zalo-auto-map-contacts -->
