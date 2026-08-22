---
title: Teacher profile page and sidebar logout
date: 2026-08-06
summary: Implemented /profile page + sidebar footer logout per design prototype; PUT /me persists full_name only; review findings fixed; committed 97e626e + 54f1340
---

# Teacher profile page and sidebar logout

## What happened

Implemented the "Hồ sơ giáo viên" screen and the sidebar Đăng xuất footer from
the claude.ai/design prototype (`So Lop - Prototype.dc.html`, screen
`isProfile`), replicating the layout and design tokens 1:1 in the repo's
Tailwind v4 token system.

- New feature folder `apps/web/src/features/profile/` (routes/api/hooks/
  schemas/page/tests, lazy-loaded `/profile` inside the protected
  DashboardLayout).
- `dashboard-layout.tsx`: sidebar footer at lg (36px mint avatar with
  Vietnamese given-name initial + "Hồ sơ giáo viên" caption + Đăng xuất row),
  ProfileDisc on the md icon rail, header profile link + logout below lg.
- Backend untouched by user decision: `PUT /me` persists only
  `{full_name, timezone}`; phone renders readOnly from the login account via
  `formatPhoneLocal`; subject/bank fields are local-only and feed the live
  Zalo-footer preview.
- Additive `setUser` on the auth store; shared `nameInitial` util in
  `lib/utils/format.ts`.

## Decision

- No DB migration — unbacked prototype fields stay client-only until real
  columns land; each carries a caption "Chưa lưu trên máy chủ — tính năng đang
  phát triển" (user-approved minimal deviation from 100% prototype fidelity,
  resolving review finding H1 about a misleading save toast).
- Zalo connect and .xlsx export are toast stubs (declared non-goals).

## Verification

Code review (code-reviewer subagent) returned DONE_WITH_CONCERNS; fixed H1
(captions), M1 (`formatPhoneLocal` on the phone input), M2 (`nameInitial` unit
tests), M3 (empty-name test now asserts zero `PUT /me` calls). Vitest 128/128,
eslint 0 errors, `tsc -b` clean. Commits: 97e626e (feature), 54f1340 (plan).

## Next steps

- Run the Playwright auth spec against the running stack (two same-named
  Đăng xuất buttons now exist in the DOM; only one visible per viewport).
- When subject/bank columns land server-side, wire the fields to the API and
  drop the captions; implement Zalo OAuth and the xlsx export.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
