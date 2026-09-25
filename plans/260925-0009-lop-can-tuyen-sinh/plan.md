---
title: "Lớp cần tuyển sinh theo So Lop v5 (API + web)"
status: completed
created: 2026-09-25
branch: feat/giang-day-menu
---

# Lớp cần tuyển sinh

## Outcome
The "Giảng dạy" nav gains "Lớp cần tuyển sinh" between "Danh sách lớp học" and
"Lời mời nhận lớp". It matches the v5 prototype `screen 'classrecruit'`:
- It reuses the class table, titled "Lớp cần tuyển sinh".
- Subtitle: "Các lớp đang bật “Cần tuyển sinh” — chưa đủ sĩ số hoặc sắp khai giảng."
- Base set: classes with the recruiting flag that are not cancelled or ended.
- Status chips: all + each status, with counts inside that base set.
- Empty label: "Không có lớp nào cần tuyển sinh."
- Row actions:
  - **Sửa** opens the edit wizard in place.
  - **Mở** opens the class detail, whose back link returns to this list.

## Design source
- `handoff/04-logic-component.js` (`isRecruit` branch of the LIST block).
- The nav entry in the `GIẢNG DẠY` group (icon `users`).
- The table template is the class list's (`So Lop - Prototype v5.dc.html`).

## API contract
- "Open for recruitment" = `recruiting` AND status ≠ archived AND (`end_date` is
  null or ≥ today). This is the prototype's "not da_huy / da_ket_thuc" on the
  repo's phase model. `OpenForRecruitment` is the Go rule; `RecruitingPredicate`
  is its SQL form.
- `GET /classes?recruiting=true` narrows the list to that set. `false` or absent
  means no filter; any other value gives 422 on `recruiting`.
- `GET /classes/stats?recruiting=true` narrows every counter to that set.
  Validation is the same as the list.
- `ClassStatsResponse.recruiting` now counts the open set, not the bare flag.
  The "Cần tuyển sinh" chip count therefore equals the rows that chip lists.
- No migration: `classes.recruiting` and `idx_classes_center_recruiting`
  already exist (000025).

## Web
- The `/classes/recruiting` route renders `RecruitingClassListPage`, which is
  `ClassListPage variant="recruiting"`. The route is static and ranks above `classes/:id`.
- The main list's "Cần tuyển sinh" chip now filters server-side. The old client
  filter over one page is gone.
- The `useClassStats(params)` key is `classesKeys.stats(params)`, still under
  `classesKeys.all`, so class writes invalidate it.
- `ClassDetailHeader` takes its back link from router state `{ from }`. Only
  `/classes/recruiting` is honoured; anything else falls back to `/classes`.

## Acceptance
- [x] API unit, HTTP and integration tests cover the list filter, the stats
  narrowing and 422s.
- [x] Web tests cover the recruiting page (header, chips, empty copy, Sửa, Mở and
  its back link), the nav order and active state, and the server-side chip filter.
- [x] Typecheck, lint (no new warnings), prettier and the full vitest run pass.
- [x] The serial integration run (`-p 1`) passes the coverage floor: 50 packages, 78.7% against a 60% floor.
- [x] Deployed on 2026-09-25 at 00:28 with the production compose (`-p teka`, prod + homelab,
  `.env.production`):
  - Images are `teka-{api,web}:5d4f3a2-dirty-260925-0028`; the rollback images are `pre-260925-0028`.
  - `migrate` was a no-op, so no dump was needed; `/readyz` returns 200.
  - `/classes/recruiting` returns 200 and the bundle contains the menu.
