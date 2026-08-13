---
title: "Prototype v2 teaching screens"
description: "Implement 4 screens from So Lop Prototype v2 (Quản lý lớp học, Hồ sơ học sinh, Lớp & học sinh, Duyệt giáo án) in apps/web, 100% on the Học Vui Mỗi Ngày design system"
status: in-progress
priority: P1
effort: "5d"
tags: [web, ui, design-system, teaching]
created: 2026-08-13
---

# Prototype v2 teaching screens

## Overview

Implement the four teaching screens of `So Lop - Prototype v2.dc.html` (Claude Design project `4a7e6c77`) in the web app:

1. **Quản lý lớp học** — per-class dashboard: stats row, sessions table with per-session giáo án/điểm/doanh thu/nhận xét, curriculum & lesson-plan view, CSV export.
2. **Hồ sơ học sinh** — student records list (điểm TB, xu hướng, vắng) + per-student detail (score bar chart, per-session marks, inline personal notes, CSV export).
3. **Lớp & học sinh** — align the existing roster students page with the v2 layout (header actions, CHỌN LỚP tabs, table columns, Ghi danh action).
4. **Duyệt giáo án** — owner-only review queue: approve / request-changes / reopen / remind flow.

Plus the sidebar nav additions the prototype defines for these screens (DẠY HỌC ordering + owner-gated TRUNG TÂM entry).

**Hard constraint:** UI changes only, inside `apps/web`. No backend (`apps/api`) changes, no new HTTP endpoints, no schema changes.

## Design sources

- Prototype: Claude Design project `4a7e6c77-0971-44fb-9766-1b6429e8b126`, file `So Lop - Prototype v2.dc.html` (screens at extract lines 216–660; JS model lines 1380–1960; nav def lines 1401–1406). Local extract: scratchpad `prototype-v2.html` (session-temporary).
- Design system: `_ds/h-c-vui-m-i-ng-y-design-system-.../` — tokens already mirrored 1:1 in `apps/web/src/styles/tokens/` (verified identical); components in `apps/web/src/components/hv/` (HvButton, HvCard, StatusPill, HvBadge, StatPill, ProgressBar, HvModal, hvToast, icons).
- DS signature rules to follow 100%: Baloo 2 display / Nunito body, chunky press shadows (`--press-mint` + `translateY` on `:active`), pill radius `999px`, card radius 20–24px + `--shadow-md`, cream/ink/mint/sky/sun/coral token palette only — no hardcoded colors outside tokens (white on filled buttons per prototype is allowed).

## Decisions (user-approved)

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | Scores, giáo án, chương trình, nhận xét live in a **client-side local teaching store** (in-memory + `localStorage`, keyed per center). Lớp / học sinh / buổi học / điểm danh / học phí use the real APIs. | Backend has no scores/lesson-plans/curriculum features; constraint forbids API work. |
| 2 | NGÀY SINH column stays in the UI but renders **"—"** (and birthday banner/🎂 affordances are hidden). | PRD R1 keeps the student field list closed (Nghị định 13/2023); no birth_date data exists. |
| 3 | **Duyệt giáo án** nav entry + route are **owner-gated** via existing `GET /centers/me` (owner narrowed by `"members" in data`). | Prototype gates it with `role==='owner'`; overrides the earlier "no nav gating" decision for this entry only, per user. |

Supporting decision: the LÃI/LỖ stat needs a per-session cost the backend does not model — keep the prototype's fixed `300.000đ` as a named constant in the teaching store (`SESSION_COST_VND`), surfaced in the table footnote exactly like the prototype. Revisit if a real cost setting ever lands. *(Open question logged below.)*

Supporting decision (Phase 1 review): the teaching store's per-center key is the **center name** for both roles — the member shape of `/centers/me` exposes no center id, and a role-independent key is required for the phase-6 review loop to work on one device. Accepted trade-off: name collisions across centers on the same device (device-local, non-authoritative data).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | All 4 screens implemented pixel-faithful to Prototype v2, using hv components + tokens only | P1 |
| 2 | Sidebar nav matches prototype: DẠY HỌC = Điểm danh, Quản lý lớp học, Hồ sơ học sinh, Lớp & học sinh; TRUNG TÂM gains owner-only Duyệt giáo án (pending dot) | P1 |
| 3 | Real data (classes, students, sessions, attendance) from existing APIs; teaching-only data (scores, plans, notes, curriculum) from the local store, persisted per center | P1 |
| 4 | No regressions: existing web test suite stays green; eslint + tsc clean; responsive (sidebar / md rail / bottom tabs) and a11y parity with the current shell | P1 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Teaching foundation — store, feature scaffold, nav](./phase-01-start.md) | Completed |
| 2 | [Phase 2: Lớp & học sinh v2 alignment](./phase-02-lop-va-hoc-sinh-v2-alignment.md) | Pending |
| 3 | [Phase 3: Quản lý lớp học — buổi học & nhận xét](./phase-03-quan-ly-lop-hoc-buoi-hoc-va-nhan-xet.md) | Pending |
| 4 | [Phase 4: Quản lý lớp học — chương trình & giáo án](./phase-04-quan-ly-lop-hoc-chuong-trinh-va-giao-an.md) | Pending |
| 5 | [Phase 5: Hồ sơ học sinh & chi tiết](./phase-05-ho-so-hoc-sinh-va-chi-tiet.md) | Pending |
| 6 | [Phase 6: Duyệt giáo án](./phase-06-duyet-giao-an.md) | Pending |
| 7 | [Phase 7: Verification & consistency](./phase-07-verification-and-consistency.md) | Pending |

Dependency shape: Phase 1 blocks everything. Phases 3→4 are sequential (same page, two view tabs). Phases 2, 3+4, 5, 6 are otherwise independent of each other; Phase 6 reads plan state written by Phase 4's store shape (defined in Phase 1). Phase 7 last.

## Success Criteria

- [ ] Nav: DẠY HỌC shows Điểm danh, Quản lý lớp học, Hồ sơ học sinh, Lớp & học sinh in that order; TRUNG TÂM shows Duyệt giáo án (with pending-count dot) only for owners.
- [ ] `/classbook` renders class tabs, 5 stat cards, both view tabs, session detail panel (nhận xét / giáo án / điểm), and CSV export.
- [ ] `/records` + `/records/:studentId` render list, trends, bar chart, inline notes, CSV exports; NGÀY SINH shows "—".
- [ ] `/students` matches v2 layout without breaking existing roster flows/tests.
- [ ] `/lesson-plans` (owner only) supports duyệt / yêu cầu sửa (comment required) / mở lại / nhắc giáo viên; teacher side sees redo note + status chip on the classbook screen.
- [ ] Teaching store persists per center across reloads; switching center never leaks another center's data.
- [ ] Full web suite green, eslint 0 errors, tsc clean; no non-token colors introduced.

## Open questions

- Per-session cost for LÃI/LỖ is a UI constant (300.000đ) until product decides whether it becomes a center setting — flagged to user, not blocking.

## Validation Log

### Session 1 — 2026-08-13
**Trigger:** User selected "Validate plan" at post-plan handoff.
**Questions asked:** 3

### Verification Results
- **Tier:** Full (7 phases)
- **Claims checked:** 21 (file paths, exports, API signatures, schema fields, nav/pending-dot mechanism, msw strict mode, owner narrowing, route paths)
- **Verified:** 21 | **Failed:** 0 | **Unverified:** 0
- Notable confirmations beyond the plan's claims:
  - `enrollmentSchema` exposes `unit_price` (int đồng), `started_on`, `ended_on` (`apps/web/src/features/roster/schemas/roster-schemas.ts:262-272`) — revenue and monthly headcount are computable from real data.
  - `HvModal` is built on radix Dialog (focus trap + Escape built-in) but defaults to `sm:max-w-md` (~448px); the 580–600px prototype editors need a className width override.
  - Pending-dot mechanism confirmed: `usePendingSessions()` + `pending` flag on nav entries (`dashboard-layout.tsx:48-59`).

#### Questions & Answers

1. **[Assumptions]** localStorage teaching data is device-local — the plan-review loop (teacher submits → owner approves) only works within one browser; no cross-device sync. How to handle?
   - Options: Chấp nhận + ghi chú trong UI (Recommended) | Chấp nhận im lặng | Dừng, cần backend trước
   - **Answer:** Chấp nhận im lặng
   - **Rationale:** UI stays 100% prototype-faithful with no extra caption. The device-local limitation is a consciously accepted product trade-off for this UI-first delivery; no storage-location note is added to any screen.

2. **[Architecture]** Enrollment exposes real `unit_price` — how should LÃI/LỖ and DOANH THU be computed?
   - Options: Dùng unit_price thật (Recommended) | Ẩn stat LÃI/LỖ | Hiện "—"
   - **Answer:** Dùng unit_price thật
   - **Rationale:** Session revenue = Σ `unit_price` of present students − `SESSION_COST_VND`. Real data replaces the plan's degrade-to-"—" contingency, which is removed.

3. **[Scope]** BUỔI T7 column source on Lớp & học sinh?
   - Options: Fetch sessions của lớp đang chọn (Recommended) | Luôn hiện "—" | Bỏ cột
   - **Answer:** Fetch sessions của lớp đang chọn
   - **Rationale:** One `listClassSessions(classId, current month)` query for the selected class (query key shared with classbook → cache reuse, no N+1). Per-student count = sessions in the month falling within the student's enrollment window — derived without per-session roster fan-out.

#### Confirmed Decisions
- Device-local teaching data: accepted silently, no UI caption — 100% prototype fidelity preferred.
- Revenue: real `enrollment.unit_price`; only the 300.000đ cost side remains a constant.
- BUỔI T7: single current-month sessions query per selected class; enrollment-window counting.

#### Impact on Phases
- Phase 2: BUỔI T7 requirement/risk rewritten to the single-query approach.
- Phase 3: fee-source risk removed; revenue formula pinned to `unit_price`.
- Phase 4: monthly headcount pinned to verified `started_on`/`ended_on`; HvModal width-override note added.

### Whole-Plan Consistency Sweep
- Files reread: plan.md, phase-01 … phase-07 (all)
- Decision deltas checked: 3 (silent device-local acceptance; unit_price revenue; BUỔI T7 single query)
- Reconciled stale references: 5 (phase-02 requirement/architecture/risk/success-criterion; phase-03 stat formula + fee risk; phase-04 headcount dates)
- Unresolved contradictions: 0

<!-- slug: prototype-v2-teaching-screens -->
