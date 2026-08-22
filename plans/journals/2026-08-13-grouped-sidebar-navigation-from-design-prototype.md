---
title: Grouped sidebar navigation from design prototype
date: 2026-08-13
summary: "Applied Claude Design prototype's grouped menu to dashboard layout; renamed Trung tâm entry; added center card"
---

# Grouped sidebar navigation from design prototype

## What happened

Applied the `So Lop - Prototype.dc.html` Claude Design prototype's sidebar menu to the web app shell (`apps/web/src/layouts/dashboard-layout.tsx`), commit `4ad5518`.

- Nav model refactored from flat `useNavEntries()` to grouped `useNavGroups()`: Tổng quan ungrouped; **Dạy học** (Điểm danh, Lớp & học sinh, Phụ huynh); **Học phí** (Chốt sổ, Gửi thông báo, Thu tiền); **Trung tâm** (Cài đặt trung tâm → `/center`, renamed from "Trung tâm").
- New `CenterCard` under the logo: center name + caller role from `GET /centers/me` (owner narrowed via `"members" in data`), disc initial strips the "Trung tâm " prefix per prototype (`Bình Minh → B`). Height-stable loading state to avoid nav CLS.
- md rail renders grouped icon stacks; sidebar/rail groups exposed as `role="group"` + `aria-label`. Bottom tab bar unchanged except overflow label rename.
- Test infra: default owner-shaped `GET /centers/me` msw handler (suite runs `onUnhandledRequest: "error"`); layout tests extended to assert group ownership, center card, renamed entry.

## Decision

- Per user: no role gating on nav (backend scopes billing per-teacher, members keep Học phí); keep Phụ huynh under Dạy học despite absence in prototype; single "Cài đặt trung tâm" entry instead of prototype's two TRUNG TÂM items (no page split — YAGNI).
- Card role label "Giáo viên" (prototype wording) intentionally differs from `/center` roster badge "Thành viên" (different context).

## Verification

253/253 web tests pass, eslint 0 errors (4 pre-existing warnings), tsc clean. Code-reviewer subagent findings (wrong disc initial via `nameInitial`, stale OVERFLOW_LABELS comment, weak grouping test, CLS, a11y groups) all fixed before commit.

## Next steps

None required. Possible follow-ups: entry `id` field to decouple `OVERFLOW_LABELS` from display labels; dedupe the three `aria-label="Main"` nav landmarks.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
