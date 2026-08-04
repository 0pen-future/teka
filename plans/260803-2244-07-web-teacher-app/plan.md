---
title: "07 Web Teacher App"
description: "Teacher-facing mobile-friendly web app: phone login, pending-attendance dashboard, roster, one-touch attendance, billing close (chốt sổ), collections board, and manual Zalo notification hand-off."
status: completed
priority: P2
effort: "8d"
branch: main
tags: [web, react, typescript, vite, tailwind, shadcn, teacher, mobile, design-system]
created: 2026-08-03
blockedBy: [260803-2244-06-api-statements-and-notifications, 260803-2325-web-design-system-foundation]
---

# 07 Web Teacher App

## Overview

Build the teacher UI on top of the V1 API (plans 01–06) inside the existing
`apps/web` React app. The teacher is a class owner with 100–300 students who
works **on a phone right after class ends** (PRD §5, story 1). Every screen must
be usable one-handed at a 375px viewport; the only screen allowed to assume a
wider viewport is the billing review table, which still degrades to stacked
cards.

The app already ships a working feature-oriented skeleton (`apps/web/src/app`,
`layouts/`, `features/auth`, `features/users`, `components/ui`, `lib/api`). This
plan **reuses that skeleton and its conventions verbatim** — a feature owns
`api/`, `components/`, `hooks/`, `pages/`, `routes.tsx`, `schemas/`, `types/`,
`__tests__/`, and exports a narrow barrel `index.ts`
(`apps/web/src/features/users/index.ts:1`). No new architecture is introduced.

One caveat about those citations: `features/users` is a scaffolded CRUD over
`/users`, and API plan 01 phase 3 deletes that API feature
(`plans/260803-2244-01-api-schema-replacement-and-auth/phase-03-teacher-profile-and-scoping.md:32`),
so phase 1 deletes the web mirror too. Later phases still cite its files as the
structural reference for query-key factories, list pages, dialogs, and route
modules. Read those citations against the pre-deletion tree (or `git show`) —
what carries forward is the convention, not the files.

Domain vocabulary kept in Vietnamese, per PRD:

| Term | Gloss |
|---|---|
| điểm danh | attendance taking for one session |
| chốt sổ | closing a billing period — freeze numbers and issue invoices |
| buổi | one class session |
| người liên hệ (contact) | the paying parent; billing and notification unit (PRD R5) |
| nợ cũ | opening balance carried from the previous period |

UI copy is Vietnamese; identifiers, code comments, and test names stay English,
matching the current codebase.

## Design Source — 100% Adherence

The visual and interaction design is fixed by the imported Claude Design
project (`claude.ai/design/p/4a7e6c77-0971-44fb-9766-1b6429e8b126`): the
**"Học Vui Mỗi Ngày" design system, direction "Dịu Mát"**, and the six-screen
**`So Lop - Prototype.dc.html`**. Plan
`260803-2325-web-design-system-foundation` lands the tokens, fonts, and the
`components/hv` kit; this plan consumes them. Rules:

1. **Tokens only.** No raw hex, no ad-hoc shadows or radii; every color, font,
   radius, shadow, and easing routes through the DS custom properties /
   Tailwind utilities from the foundation plan.
2. **Prototype is the screen authority.** Where the prototype renders a screen,
   its layout, palette, copy tone, and micro-interactions are replicated; the
   per-phase "Design Spec" sections in this plan transcribe the exact recipes.
3. **IA follows the prototype nav** — six sections: Tổng quan `/`, Điểm danh
   `/sessions`, Lớp & học sinh `/students`, Chốt sổ `/billing/:periodId`, Gửi
   thông báo `/notifications/:periodId`, Thu tiền `/collections/:periodId`.
   Consequences reconciled against the original route plan:
   - Roster consolidates into **one screen** (`/students`) with class tabs and
     a combined table (student · contact · enrollment date · sessions ·
     badges), per the prototype's "Lớp & học sinh". Creating a class and
     adding a student are **modals** (`HvModal`), not separate pages
     (prototype `modalClass`, `modalAdd`); a new contact is created inline in
     the add-student modal.
   - `/contacts`, `/contacts/:id`, `/students/:id`, `/classes/:id` survive as
     drill-down detail routes (PRD needs them for merged-children view,
     anonymize, enrollment end) but leave the primary nav; they are reached
     from rows on the consolidated screen and are styled with the same kit.
   - The sidebar shows a persistent "KỲ HIỆN TẠI" period card at the bottom
     (mint-50), and a coral notification dot on Điểm danh when unattended
     sessions exist.
4. **Prototype demo behaviors are not product behaviors.** The prototype's
   "Gửi tất cả bằng 1 chạm" simulates sending; V1's real channel stays
   `zalo_manual` (copy + explicit "Đã gửi", phase 4). The prototype labels the
   attendance right panel "mô phỏng thao tác trên điện thoại" — on the web app
   that panel is the phone-width attendance screen itself.
5. **Anything the prototype does not show** (auth screens, detail routes,
   anonymize dialog, empty/error states) is composed from the same kit and
   tokens, matching the DS readme's tone: warm, encouraging, verb-first
   Vietnamese; parent-facing surfaces slightly more formal.

### Responsive Strategy (phone / tablet / desktop)

Three tiers on Tailwind's default breakpoints, anchored to the DS width tokens
(`--w-phone` 390, `--w-content` 720, `--w-page` 1080):

| Tier | Breakpoint | Shell | Content |
|---|---|---|---|
| Phone | `< md` (<768px) | Fixed bottom tab bar (6 items, icon over 11px label); "KỲ HIỆN TẠI" card at top of dashboard | Single column, 16px gutters, full-bleed cards |
| Tablet | `md`–`lg` (768–1023px) | **Icon rail sidebar 72px**: icons only ≥48px hit areas, labels as tooltips, coral dot preserved, condensed "KỲ HIỆN TẠI" (period number only); no bottom bar | Content centered `max-width: var(--w-content)` + padding 24px; 2-col grids |
| Desktop | `lg+` (≥1024px) | Full prototype sidebar 236px with labels and full period card | Content `max-width: var(--w-page)` 1080px, padding 28px 32px — the prototype layout verbatim; 3–4-col grids |

Per-surface consequences (each phase's Design Spec restates its own):

- Dashboard: stat cards 2-col phone/tablet → 4-col desktop; class cards 1-col
  phone → 2-col tablet → 3-col desktop.
- Attendance: two-pane (list + 400px panel) only at `lg+`; phone **and**
  tablet use the standalone panel page centered at `--w-content`.
- Billing review: stacked cards `< sm`; sticky-first-column table with
  confined horizontal scroll `sm`–`lg`; at `lg+` the full table fits without
  scrolling.
- Collections/notifications: message cards 1-col phone/tablet-portrait →
  2-col `lg+`; tables gain no columns across tiers — the same fields reflow.
- Modals: `HvModal` is a bottom sheet (full-width, rounded top) `< sm`; a
  centered panel `max-w-md` from `sm`; never wider than 480px — forms stay
  narrow even on desktop.
- Touch targets keep ≥48px at every tier; hover styles (card lift, ghost
  hover tints) activate only on `hover`-capable pointers
  (`@media (hover: hover)`), so tablet touch never sticks a hover state.
- E2E: existing 375px specs stay; add one viewport assertion per key screen at
  768×1024 and 1280×800 (no horizontal page scroll, nav variant correct).

## Scope

In scope:

- Rework the existing email-based auth feature to **phone + password** (API plan
  01 replaces the identity model; `user_accounts.phone` is the login identifier,
  `docs/schema_design.sql:47`), sourcing profile data from the canonical
  `GET /api/v1/me` and deleting the scaffolded `features/users` CRUD.
- Dashboard with the "buổi đã qua chưa điểm danh" warning (PRD R2 AC 3).
- Roster: contacts, students (closed field list), classes + weekly schedule,
  enrollments including mid-cycle join and leave.
- Attendance: session list, one-touch điểm danh, editing a past session.
- Billing: period review table, blocked-close state, per-line adjustment, close
  confirmation.
- Collections board: by-contact (default) and by-class views, unpaid filter,
  contact-level mark-paid with allocation preview and manual override.
- Send notifications: per-parent generated message, copy and bulk copy, sent
  status, one reminder per family.

Out of scope (PRD §3 non-goals and §6 P1):

- Parent-facing statement page — owned by plan 08.
- Any ZNS or API-driven Zalo send. V1 channel is `zalo_manual`
  (`docs/schema_design.sql:437`): the system renders text, the teacher pastes it.
- Assistant roles and permissions, automated debt reminders, sibling discounts,
  the "tiền đang thất thoát" board, grades/homework features.
- Client-side analytics for statement opens — tracking is server-side on the
  public statement endpoint (plan 08).
- Offline mode / PWA install.

## Phases

| # | Phase | Effort | Depends on | Status |
|---|-------|--------|------------|--------|
| 1 | [Phone auth rework, app shell, dashboard warnings](./phase-01-auth-shell-dashboard.md) | 1.5d | — | Pending |
| 2 | [Roster: contacts, students, classes, enrollments](./phase-02-roster.md) | 2d | 1 | Pending |
| 3 | [Attendance: sessions, one-touch điểm danh, edits](./phase-03-attendance.md) | 1.5d | 1 | Pending |
| 4 | [Billing close, collections, notifications](./phase-04-billing-collections-notifications.md) | 3d | 1, 2, 3 | Pending |

Phases 2 and 3 depend only on phase 1 and own disjoint feature folders
(`features/roster` vs `features/attendance`), so they can run in parallel. Their
one shared file is `apps/web/src/app/router.tsx:15`; whichever lands second
rebases its route registration.

## Key Screens → PRD Mapping

| Route | Screen | Prototype screen | PRD source |
|---|---|---|---|
| `/login` | Phone + password sign-in | — (kit-composed) | R1 identity; `user_accounts.phone` |
| `/` | Dashboard: unattended-session warning banner, stat cards, class cards | `home` | R2 AC 3; §5 story 7 |
| `/students` | Consolidated "Lớp & học sinh": class tabs, combined table, create-class + add-student modals | `students` + `modalClass` + `modalAdd` | R1 data-scope table and its ACs |
| `/contacts/:id`, `/students/:id`, `/classes/:id` | Drill-down details: children merged, anonymize, schedule, enrollments (off-nav) | — (kit-composed) | R1; R5 "the unit is the contact"; §5 story 6 |
| `/sessions` | Class pill tabs, session list, attendance panel | `attend` | R2; §5 story 8 |
| `/sessions/:id/attendance` | One-touch điểm danh (same panel, deep-linkable) | `attend` right panel | R2 AC 1 (max 3 interactions) |
| `/billing/:periodId` | Review table grouped by class, blocked state, adjustments, close footer | `close` + `modalAdjust` + `modalWarn` | R3, R4 |
| `/collections/:periodId` | Contact/class segmented board, filter chips, record payment | `pay` + `modalPay` | R7 |
| `/notifications/:periodId` | Message cards, copy, sent status | `send` | R5 layer 1, R6 |

## Acceptance Criteria

From PRD §6; each phase file restates the subset it owns.

- [x] Given a 30-student class with 2 absentees, when the teacher takes
      attendance, then at most 3 interactions are needed (R2 AC 1).
- [x] Given a session confirmed 3 days ago, when reopened, then it is still
      editable and the fee recomputes (R2 AC 2).
- [x] Given a past session with no attendance, when the teacher opens the app,
      then the warning is visible on the first screen (R2 AC 3).
- [x] Given the student create form, when it renders, then it contains no field
      outside full name, display note, contact, and enrollment start date
      (R1 AC).
- [x] Given the teacher deletes a student, when confirmed, then personal data is
      erased and only anonymized financial records remain (R1 AC).
- [x] Given a class with 10 sessions already held, when a student is added, then
      that student is billed only from the next session onwards (R1 AC).
- [x] Given unconfirmed sessions inside the period, when the teacher attempts
      chốt sổ, then close is blocked and each offending session is listed and
      linkable (R4 AC 1).
- [x] Given a closed period, when attendance inside it is edited, then the UI
      warns that the delta becomes an adjustment in the next period (R4 AC 2).
- [x] Given 150 students, when the collections board opens, then the unpaid
      group can be filtered in one interaction (R7 AC 1).
- [x] Given a contact with 2 children who underpaid, when switching to the
      by-class view, then the shortfall allocated to each child is visible
      (R7 AC 4).
- [x] Given a contact with 2 children both in debt, when sending a reminder,
      then exactly one reminder exists for that contact (R7 AC 5).
- [x] Given a closed period, when the notifications screen opens, then each
      contact has exactly one generated message containing per-child session
      counts, absences, amounts, nợ cũ, and the family grand total (R5 layer 1).

## Test Strategy

| Level | Tool | Covers |
|---|---|---|
| Unit / component | vitest + Testing Library + msw (`apps/web/src/test/msw/handlers.ts:1`) | Forms, schema validation, table rendering, allocation preview math, interaction-count assertions |
| E2E | playwright (`apps/web/playwright.config.ts:8`) against the compose stack | Login, attendance happy path, blocked close, mark-paid persistence |

`apps/web/playwright.config.ts:11` runs specs sequentially with one worker
because they mutate shared data; new specs must keep that assumption and seed
their own class and students with a unique suffix.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| API response shapes for plans 02–06 not final when web work starts | High | High | Every feature parses responses through zod (`apps/web/src/lib/api/envelope.ts:24`); contract drift fails loudly at the parse site instead of deep in a component. Each phase file records its assumed contract in one table for a cheap reconciliation pass. |
| Auth rework breaks existing auth tests | Medium | Medium | Phase 1 updates `apps/web/src/features/auth/__tests__/` and `apps/web/e2e/auth.spec.ts` in the same change; the session type moves into the auth feature as `teacherSchema`, and the five modules importing `@/features/users` are enumerated in phase 1. |
| Billing review table unusable on a phone | Medium | High | Sticky first column with horizontal scroll confined to the table region, plus a stacked card layout under `sm`. Review is the one workflow teachers do sitting down. |
| Manual copy flow silently loses "sent" state | Medium | Medium | Marking sent is an explicit server call, never a side effect of the copy button; status chips read from the server's notification status. |

## Open Questions

1. Exact API paths and payloads for plans 02–06 are assumed in each phase
   file's "Assumed API contract" table. Reconcile before coding; a mismatch
   should be a rename, not a redesign.
2. Q5 (PRD §8): when attendance changes after notifications were sent, does the
   system resend or roll the delta into the next period? Phase 3 ships the
   warning UI; the resend action stays behind that answer.
3. Q7 (PRD §8): browsing historical periods. This plan exposes only the current
   and previous period in the period selector; a full history list is deferred.
4. Does V1 keep public self-registration for teachers, or is onboarding manual?
   Phase 1 keeps `/register` with phone fields; drop it if API plan 01 removes
   the endpoint.
5. **Phone storage format is unresolved upstream.** Is a phone stored and
   accepted as E.164 (`+84912345678`) or in the local form (`0912345678`)? API
   plan 01 phase 2 currently normalizes to E.164
   (`plans/260803-2244-01-api-schema-replacement-and-auth/phase-02-phone-auth-rewrite.md:116`),
   but the decision is not final. The login form must not hardcode an answer by
   accident: phase 1 confines it to a single `normalizePhone` helper, and the
   accepted *input* forms (`0…` and `+84…`) hold either way. Confirm against the
   regenerated swagger before phase 1 merges — a mismatch means every login
   fails, and it fails as "wrong password", not as a validation error.

<!-- slug: 07-web-teacher-app -->
