# Scout → docs-manager: Teka delivery history, current state, roadmap material

Date: 2026-08-19 · Repo: `/home/vmo/workspace/testing/teka` · Branch `master` (clean)
Sources read: `docs/prd.md`, 25 dirs under `plans/`, `plans/journals/` (24), `plans/reports/` (37), `adr.md` (57 entries), `docs/*`, git log (100 commits, 2026-08-03 → 2026-08-13).
Purpose: input for a PROJECT ROADMAP doc + a PRODUCT OVERVIEW/PDR doc. Read-only pass; no code touched.

---

## 1. Product summary (from `docs/prd.md`)

**Title:** PRD — Hệ thống quản lý lớp dạy thêm (V1) *(after-school / private tuition class management)*. Author Nguyễn Văn Thược, 2026-08-03. **Status in the doc itself: `Draft — chưa validate bằng phỏng vấn khách hàng`** (not validated by customer interviews). The PRD opens with an explicit warning: build only after ≥10 interviews with **chủ lớp** (class owners); if Phase 0 fails, throw the spec away rather than patch it. **All code below was built anyway — the validation gate was never recorded as passed.** Flag this prominently in the PDR.

### 1.1 Problem statement

Teachers running ≥3 classes, each with a different **ngày khai giảng** (start date) and students enrolling mid-cycle, must bill by **số buổi thực học của từng cá thể** (per-individual count of sessions actually attended) — not per class. This is where Excel breaks: the problem changed from "manage a list" to "compute money per individual". Cost of not solving: hours of monthly manual reconciliation, and worse — **thu thiếu, quên thu, thu nhầm** (under-collect, forget to collect, mis-collect). At 150 students, missing 3 people = 1.5–2M VND/month, usually undetected. Current alternative (hiring a **trợ giảng** / teaching assistant to keep books) costs more and still errs.

### 1.2 Goals and measurement

| # | Mục tiêu | Cách đo (measure) |
|---|---|---|
| G1 | Teacher closes a whole class's tuition in <10 min | Time from opening the **chốt sổ** (period close) screen to notifications sent |
| G2 | No student ever missed from the billing list | Count of students with sessions but no invoice = 0 |
| G3 | Shorten time-to-collect | Avg days from chốt sổ to ≥90% of period tuition collected |
| G4 | Attendance data trustworthy enough that teacher never re-checks | % of sessions **điểm danh** (attendance-taken) within 24h ≥ 90% |
| G5 | Prove willingness to pay | ≥30% of trial teachers convert to paid after 2 monthly cycles |

**North Star = G4.** PRD: *"Mọi giá trị phía sau đều là hệ quả toán học của dữ liệu điểm danh. Nếu tỉ lệ này dưới 90%, sản phẩm vô giá trị bất kể báo cáo đẹp đến đâu."*

### 1.3 Non-goals (V1)

| Không làm | Lý do |
|---|---|
| Installable parent app | Parents use it ~2×/month; adoption barrier ≫ a web link |
| Chat / class feed / social | Not the pain; head-on fight with Zalo, unwinnable |
| Configurable tuition policy | Micro-business SaaS dies of configuration; V1 supports exactly one model |
| Grades, feedback, homework, materials | Unrelated to cash flow; different product |
| Student-facing features | Student has no strong job in this problem |
| Automatic payment gateway / collect-on-behalf | Needs a payment-intermediary licence or partner; V1 shows a transfer QR, teacher confirms manually |

### 1.4 The single most important scope decision (PRD §4)

**V1 supports exactly one tuition model: trả sau, tính theo buổi thực học, chốt theo tháng dương lịch** (pay-after, billed by sessions actually attended, closed per calendar month).

Rationale: this is *the only model where Excel actually breaks*. Prepaid fixed-course classes work fine in Excel — those teachers do not hurt and will not pay. Explicitly **not** supported in V1, and their teachers are declared not-customers: prepaid by course; prepaid session packs; flat tuition independent of session count; per-student pricing inside one class.

### 1.5 Personas

| Persona | Definition | Notes |
|---|---|---|
| **Giáo viên / chủ lớp** (teacher / class owner) | Primary. Owns 100–300 students, works on a phone right after class | All P0 stories |
| **Phụ huynh** (parent, = **người liên hệ**/contact) | Secondary, read-only. Opens a link ~2×/month on mobile data | No login, no app |
| *(not in PRD)* **Owner of a trung tâm** (center) | Introduced later by the Center Tenancy plan (2026-08-11) | **PRD drift — see §5** |
| *(not in PRD)* **Operator** (CLI bootstrap/recovery) | Introduced by Invite-Only Onboarding (2026-08-12) | **PRD drift — see §5** |

### 1.6 User stories, priority order

**Teacher (high → low):** 1) điểm danh a session in <15s; 2) auto-compute per-student fee from sessions attended; 3) see instantly who paid / who owes how much; 4) send per-parent tuition notice in one action; 5) review the whole fee table before sending; 6) add a student to a running class with zero recalculation work; 7) be warned about sessions not yet điểm danh; 8) edit attendance of a past session.

**Parent:** 1) open one link, see this month's session count + amount due; 2) see every session attended/absent to trust the number; 3) transfer QR on that same screen.

### 1.7 Edge cases the PRD requires covered

Mid-cycle enrol → bill only from first attended session · mid-cycle quit → close to last session, keep any debt · teacher-cancelled session → nobody billed · class with zero sessions in period → no invoice, no notification · attendance edited after notification sent → needs a rule (see Q5).
**Multi-child family group:** two children in different classes → billed independently but **one** message and **one** payment · two children in the same class → two separate attendance rows, UI must disambiguate (same surname, easy mis-tick) · one child in two classes of the same teacher → merged into that child's one invoice · family underpays → allocation rule needed (Q8) · one child quits, sibling stays → never delete the contact or the quitter's debt history.
Not covered in V1: both mother and father receiving the notice (P1).

### 1.8 Requirements skeleton

P0: **R1** roster (closed field list: student name, enrol date, class, contact phone — the phone lives on **người liên hệ**, never on the student; **đơn giá** lives on the **enrollment**, not the class) · **R2** one-touch điểm danh (≤3 interactions for 30 students / 2 absent) · **R3** per-individual fee = attended×unit price + nợ cũ (carried debt) · **R4** chốt sổ + review screen with hard block on unconfirmed sessions · **R5** two-layer statement (summary inside the Zalo message + tokenised live detail link, unit = contact) · **R6** one-action bulk send · **R7** collections board with two views (by contact = default, by class).
P1: "tiền đang thất thoát" (money leaking) board · auto-reconciled QR reference codes · auto debt reminders after X days · excused absence + make-up in another class · assistant role (attendance only, no money) · second contact per student (1:n → n:n + primary-contact concept).
P2: read receipts · transparency export for **TT29/2024** + **TT19/2026** compliance · multi-shift scheduling with clash warnings · payment gateway with transaction fee · grades/feedback/homework.
Architectural constraint to preserve from V1: keep `buổi học` — `sự có mặt` — `khoản phải thu` — `khoản đã thu` as separate entities.

### 1.9 PRD's own Open Questions

| # | Question | Owner | Status per PRD |
|---|---|---|---|
| Q1 | How to bulk-send over Zalo? ZNS needs template approval, per-message cost, industry restrictions. Fallback = pre-render for manual send, or SMS? | Engineering | **Blocking** |
| Q2 | Nghị định 13/2023 children's-data duties; controller-vs-processor split; consent capture at enrolment | Legal | **Blocking** — narrowed, not closed |
| Q3 | What does TT19/2026 (issued 2026-03-31) change in TT29/2024? Market may shrink — or transparency becomes the sales wedge | Stakeholder | **Blocking** |
| Q4 | What share of the market actually uses pay-after-by-session? If <30%, §4 is wrong | Market research | **Blocking** |
| Q5 | Attendance edited after sending: resend, or carry the delta to next period? | Product | Non-blocking |
| Q6 | Should the parent link expire? | — | **Closed**: summary in message, detail behind token, expires on full payment or after 90 days |
| Q7 | Does V1 need previous-period fee history? | User research | Non-blocking |
| Q8 | Multi-child family underpays — allocation rule? Proposal: old debt first, then current, then earliest-started class; teacher must be able to override | Product + user research | Non-blocking |
| Q9 | Sibling discount prevalence? Data model ready (price on enrollment), build decision needs data | Market research | Non-blocking |

Success metrics (leading, 2–6 wks): điểm danh-within-24h 90%/97% · chốt sổ per class <10min/<4min · teachers completing a full cycle 60%/80% · parent link open rate 50%/75% · manual edits at review <5%/<1% of students. Lagging (2–6 mo): month-3 retention ≥50% · paid conversion ≥30% · end-of-period arrears down ≥30% · students missed from billing = 0. Phasing: 0 (10 interviews) → 0.5 (pre-sell, ≥3 commitments) → 1 (build R1–R7, run live with 3 teachers for one cycle) → 2 (P1 once G4 ≥90%).

---

## 2. Delivery timeline

Plan dirs are `<yymmdd-hhmm>-<slug>`; `26` = 2026. Status column is what the evidence supports, not only what the file claims.

| # | Date | Slug | Goal | Status | Evidence |
|---|---|---|---|---|---|
| 1 | 08-03 15:52 | `fullstack-project-provisioning` | Monorepo skeleton: Go/Gin/GORM/Postgres API + React/Vite/Tailwind/shadcn web, compose dev stack, Makefile, CI/CD, 2 reference features | **Done** (1 AC left open) | frontmatter `completed`; 8/8 phases Completed; AC "CI green on a PR" unchecked — no GitHub remote existed yet; later resolved by `525b7d5` |
| 2 | 08-03 22:44 | `01-api-schema-replacement-and-auth` | Install `docs/schema_design.sql` as migration baseline; rewrite auth to phone+password over `user_accounts`/`teachers` | **Done** | `completed`; commits `173dd4e`,`8a59256`; 12 ADR entries; `test-report-260804-0020-plan-01-auth.md` |
| 3 | 08-03 22:44 | `02-api-roster-management` | PRD R1: contacts, students (closed field list + anonymise), classes, schedules, enrollments | **Done** | `completed`; commit `2eb94dc`; journal 08-03 |
| 4 | 08-03 22:44 | `03-api-sessions-and-attendance` | PRD R2 + North Star G4: session generation, lifecycle, one-touch điểm danh, pending feed | **Done** | `completed`; all AC `[x]`; commit `dc60d25` |
| 5 | 08-03 22:44 | `04-api-billing-engine` | PRD R3+R4: periods, per-student compute, chốt sổ with hard block, immutable invoices, adjustments, void | **Done** | `completed`; commit `c8192ed`; 14 ADR entries incl. concurrent-reconcile fix |
| 6 | 08-03 22:44 | `05-api-payments-and-collections` | PRD R7: contact-level payments, auto-allocation across a family, reversal, two-axis board | **Done** | `completed`; commit `68d8ad8`; ADR "review chốt" on deadlock ordering |
| 7 | 08-03 22:44 | `06-api-statements-and-notifications` | PRD R5+R6: per-contact statements, tokenised live public link, message builder, bulk send | **Done** | `completed`; commit `3e183bd`; ADR review 0 CRITICAL/0 HIGH |
| 8 | 08-03 23:25 | `web-design-system-foundation` | "Học Vui Mỗi Ngày" design system (direction "Dịu Mát"): tokens, Baloo 2 + Nunito, chunky-press kit | **Done** | `completed`; commit `630e7db` |
| 9 | 08-03 22:44 | `07-web-teacher-app` | Teacher app: phone login, dashboard, roster, điểm danh, chốt sổ, collections, manual Zalo hand-off | **Done** | `completed`; commits `984c118`,`8b6722a`,`98843a5`; ADR 8 entries on contract drift vs assumed API |
| 10 | 08-04 | `08-web-parent-statement-page` | Public `/s/:token`: per-child breakdown, family total, QR, neutral error page | **Done** | `completed`; commit `0bdcb35`; ADR entry on real payload shape |
| 11 | 08-04 20:07 | `web-api-e2e-auth-verification` | Run the real stack, verify web↔API live, fix contract drift | **Done** | `completed`; 3/3 phases; commits `7676a5a`,`6cbf48a`,`ed26b3a` |
| 12 | 08-05 13:07 | `class-settings-page` | "Cài đặt lớp" screen (name, weekly timetable, time, đơn giá) per prototype | **Done, with deferrals** | "implemented — verified 108/108"; 3 High fixed, **M1–M6/L1–L4 deferred by user decision**; commit `9beef41` |
| 13 | 08-05 13:48 | `remove-class-mgmt-flow` | Delete the legacy class-management flow (web only) | **Done** | "implemented — verified 104/104"; commit `1161903`; user accepted losing archived-class list + date editing from the web |
| 14 | 08-05 14:35 | `design-system-screens` | Restyle "Lớp & học sinh" + "Điểm danh" to 100% prototype | **Done** | `done`; commit `ff6649c`; notes e2e red at the time (pre-existing) |
| 15 | 08-05 15:16 | `class-picker-section` | "CHỌN LỚP" picker per prototype on 2 screens | **Done** | `done`; commit `8135e8d`; also removed ad-hoc session creation (`43be3b6`) per user decision |
| 16 | 08-05 15:51 | `diem-danh-prototype-gaps` | Close 3 remaining prototype gaps on Điểm danh | **Shipped, plan never closed** | plan.md still says `in-progress`, Verification says "(điền sau khi chạy)"; but code has all three: `sessions-page.tsx:122` CHỌN LỚP, `:110` 14px subtitle, `confirm-attendance-bar.tsx:32` "ĐÃ XÁC NHẬN ✓"; commit `0c8bad3` |
| 17 | 08-06 13:06 | `tong-quan-dashboard` | Rebuild "Tổng quan" dashboard: greeting, pending banner, 4 stat cards, class grid | **Done** | `done`; commit `03aea54`; 3 blocking review findings fixed |
| 18 | 08-06 17:07 | `teacher-profile-page` | "Hồ sơ giáo viên" page + sidebar logout | **Done, partly stubbed** | `done — verified 128/128`; commit `97e626e`; **Môn dạy / bank fields have no backing — labelled "Chưa lưu trên máy chủ"** |
| 19 | 08-06 | `homelab-traefik-deployment` | Publish API+web behind existing Traefik/Cloudflare Tunnel | **Done** | `completed`; both phases `[x]`; commits `e4235e0`,`d713d06`; report `pm-260806-1052` |
| 20 | 08-06 21:12 | `zalo-personal-auth` | Link teacher's **personal** Zalo via QR; AES-GCM creds at rest; auto re-login | **Done** | `completed`; 5/5 phases; commits `adbc913`→`b3de2da`; 3 code-review rounds (`zalo-phase-04-*`) |
| 21 | 08-07 12:24 | `zalo-personal-send` | Send statements as personal DMs, paced background run, progress UI | **Done** | `completed`; "Hoàn thành 2026-08-07: cả 5 phase done"; commits `0e6d9ed`,`8c637f4` |
| 22 | 08-07 19:35 | `zalo-auto-map-contacts` | Auto-suggest contact↔Zalo-friend by phone lookup + "Phụ huynh" nav entry | **Done (phase 2 tagged "PoC pre-ship")** | `done`; all AC `[x]`; commit `4cb225b`; live PoC of the match endpoint deferred to pre-ship by user decision |
| 23 | 08-11 10:55 | `manager-class-oversight` → retitled **Center Tenancy** | Move tenant from teacher to **center**: `centers` table, re-key whole schema to `center_id`, owner full read+write, centers API + UI + owner dashboard API | **Done** | `done`, `updated: 2026-08-12`; 5/5 phases; commits `39da232`…`ce401cf`; journal 08-12; **scope pivoted twice** (see §5) |
| 24 | 08-11 15:08 | `class-schedule-slots` | Multi **khung giờ** (time slot) timetable in create + settings | **Done; plan.md stale** | plan says "pending commit" but commit `20aa0aa` (08-11) landed it |
| 25 | 08-12 09:04 | `invite-only-onboarding` | Replace self-registration with owner-issued invite links; disable-on-remove; forgot-password over Zalo DM; operator CLI bootstrap | **Done except e2e run** | `done`; 7/7 phases; commits `7496153`→`46d9735`,`40680a6`,`e40d022`,`1ef1c65`; **success criterion "full `make e2e` against seeded stack" still unchecked** |

Post-plan commits: `28b52a8` (Postgres 16 alignment + operator-run DB docs) and `0c411fb` (docker/Makefile code review), both 2026-08-13. **No commits since 2026-08-13** — repo has been idle ~6 days as of this report.

**Abandoned / superseded work** (nothing was abandoned mid-build; three direction reversals happened at the design stage):
- `brainstorm-260806-1532-zalo-bot-invoice-send.md` — official Zalo **Bot API** direction, explicitly superseded by the personal-account direction (`…-1611`).
- `manager-class-oversight` original design (delegation grants, manager read-only, no org layer) — replaced 260811-1700 by user decision with the center-tenancy model; the plan's own §"Lịch sử scope" records the reversal, and the old dir slug was kept.
- Legacy web class-management flow (`/classes`, `/classes/:id`, ScheduleEditor, ad-hoc session creation dialog) — deliberately deleted, capability kept API-side.

---

## 3. Current state — what works end-to-end today

**Backend** (`apps/api`, Go/Gin/GORM/Postgres, 16 feature packages, 8 migrations, Cobra CLI):

| Area | Shipped |
|---|---|
| Identity | Phone+password login, JWT access + rotating refresh with family reuse-revocation, `GET/PUT /me`, forgot/reset password over Zalo DM, invite-only registration (`/invitations/preview`,`/accept`), rate limiter keyed on business identity |
| Tenancy | **Center is the tenant.** `centers` table, `center_id` on every business table, composite FKs `(id, center_id)`, membership resolved from DB per request (no JWT caching → instant revoke), owner = full read+write in center, teacher = own data only |
| Roster | Contacts (unique phone per center, delete blocked while referenced), students (closed field list + anonymise), classes, `class_schedules` with multi-slot timetables, enrollments (start/end, one active per student+class) |
| Attendance | Idempotent on-demand session generation from schedules, lifecycle planned/held/cancelled(+hold/uncancel), one-touch confirm (server materialises presents), past-session edit, pending feed |
| Billing | Periods, live preview, draft invoices, chốt sổ with hard block on unconfirmed past sessions, immutable issued invoices, manual adjustments with mandatory reason, void, post-close attendance edits → next-period adjustments |
| Payments | Contact-level payment recording, auto-allocation across a family's invoices, manual reallocation, reversal-never-delete, `paid_amount`/status maintenance |
| Collections | By-contact (default) and by-class boards, summary, unpaid filter |
| Statements | One statement per contact per period, hashed token, public unauthenticated live-rendering endpoint + `qr.png` (VietQR EMVCo TLV, CRC-16/CCITT-FALSE), view tracking, neutral 404, `X-Robots-Tag`/`Cache-Control`/`Referrer-Policy` headers, token redacted from access logs |
| Notifications | Bulk send per period; channels `zalo_manual` (render + copy), `zalo_personal` (paced 3–8s background run, resume, progress polling), `zalo_zns` (stub, cannot send until PRD Q1 answered), `sms` (declared only); reminders |
| Zalo | Quarantined reverse-engineered protocol port (QR login, cookie re-login, AES-GCM creds at rest, health probe, `SendMessage`, `FetchFriends`, `MatchFriends`/find-user, `SendFriendRequest`), contact↔friend mapping, auto-map suggestion flow |
| Owner oversight | `GET /centers/me/overview`, `/teachers`, `/teachers/:id/classes`, read-only session range listing |
| Operator | `cmd/api` subcommands: `serve`, `migrate`, `seed`, `create-center` (atomic center+owner), `reset-password` |
| Docs/CI | OpenAPI regenerated in the Docker build (can never be stale), swagger drift-checked in CI |

**Web** (`apps/web`, React+TS+Vite+Tailwind+shadcn, "Học Vui Mỗi Ngày" DS, 10 feature modules, PWA manifest):

`/login` · `/forgot-password` · `/reset-password/:token` · `/invite/:token` (accept invite) · `/` Tổng quan dashboard (greeting, pending-attendance banner, 4 stat cards, class grid) · `/students` Lớp & học sinh (class picker, roster table, enrol dialog, create-class dialog) · `/students/:id` · `/contacts` Phụ huynh + `/contacts/:id` (Zalo mapping + auto-map) · `/classes/:id/settings` Cài đặt lớp (multi khung giờ) · `/sessions` + `/sessions/:id/attendance` Điểm danh · `/billing` + `/billing/:periodId` chốt sổ review · `/collections/:periodId` · `/notifications/:periodId` (channel choice, run progress) · `/profile` Hồ sơ giáo viên · `/center` Trung tâm (member roster, rename, remove, leave) · public `/s/:token` parent statement.

**Test/quality state:** `make test-api` ~70.5–70.9% coverage (gate 60%), `make test-web` 251/251, lint-api 0 issues, lint-web has known pre-existing Prettier residue on ~5 files that keeps `format:check` red on master. Playwright specs exist for auth, roster, attendance, billing, collections, statement, invite-accept, forgot-password — **but a full `make e2e` against a seeded stack has not been run since the invite-only rewrite** (deferred to the deploy stage).

**Deployment:** homelab Traefik + Cloudflare Tunnel overlay live at `teka-api.cauchuyenlaptrinh.com` / `teka-web.cauchuyenlaptrinh.com`; Postgres is operator-run (`infrastructure/postgres/docker-compose.yml`, `postgres:16-alpine`, joins external `teka_default`). Production has had at least one real teacher using it (debug report 2026-08-11 works against live production data).

---

## 4. Deferred work and known gaps

| # | Item | One-line | Documented at |
|---|---|---|---|
| D1 | **Postgres Row Level Security** | Tenancy enforced only in the repository/query layer; RLS recorded as a pre-launch hardening item, still not built after the center re-key | `plans/…-01-api-schema-replacement-and-auth/plan.md:51,94`; `plans/260811-1055-.../plan.md:34`; `docs/api-guidelines.md:101` |
| D2 | **Timezone: "hôm nay" = UTC midnight** | `today()` uses process TZ (UTC in container) while `teachers.timezone` defaults `Asia/Ho_Chi_Minh` and is read nowhere; 00:00–07:00 VN can be off by one day. Explicitly flagged as "must be settled before billing stands up" — never was | `adr.md:131` |
| D3 | **Attendance guard on enrollment delete** | `DELETE /enrollments/:id` should 409 when attendance exists; deferred from plan 02 to plan 03 and no evidence it was added | `adr.md:149` |
| D4 | **NAPAS BIN lookup for VietQR** | EMVCo field 38 sub-tag 01 uses the teacher's free-text `BankCode` instead of a real NAPAS BIN; wallets may not resolve the bank. Must be fixed before this leaves V1 | `adr.md:679` |
| D5 | **Token denylist / instant revocation** | Access tokens are valid until expiry; a denylist is only to be added if the product ever needs instant revocation | `docs/api-guidelines.md:164`; `plans/260804-2007-…/plan.md:55` |
| D6 | **OTP / phone verification** | `user_accounts.password_hash` is nullable for a future OTP-only path; V1 always writes bcrypt, no verification step | `plans/…-01-…/plan.md:49,140`; `docs/api-guidelines.md:180` |
| D7 | **Parent and student portal auth** | `role` supports `'parent'`/`'students'` in schema; V1 creates `'teachers'` only. Center tenancy re-confirmed student/parent accounts stay "schema-only" | `plans/…-01-…/plan.md`; `plans/260811-1055-…/plan.md:34` |
| D8 | **Web e2e red between plan 01 and plan 07** | Accepted deliberately: old email-based e2e specs stayed red while the API identity model changed; CI gate not disabled. *Resolved historically*, but the same pattern recurs now with invite-only (see D9) | `adr.md:77` |
| D9 | **Full `make e2e` not run for invite-only world** | Specs written, only smoke-tested against the public HTTPS edge; full seeded run deferred to deploy stage | `plans/260812-0904-…/plan.md:101`; `reports/phase7-260812-1337-…md` |
| D10 | **Unbilled sessions silently lost** | Sessions/enrollments created **after** a period closed are never billed by any period: `Close` refuses non-open periods, there is no reopen, and next period's opening balance carries only invoice outstanding — not un-invoiced sessions. Confirmed on live production data, 360,000₫ lost for one student | `reports/debug-260811-1625-chot-so-gui-thong-bao.md` |
| D11 | **Closed-period review shows live recompute, not frozen figures** | `/billing/:periodId` for a closed period reads `preview` (live), so the header can show a different total than the frozen invoices | same report, root cause #2 |
| D12 | **No notification dedup** | `BulkSend` targets every contact with a non-void invoice and inserts a fresh ledger row each call → a re-send would DM already-notified parents again | same report, addendum |
| D13 | **No period reopen** | Product has no controlled reopen; re-close would 409 on issued invoices (`preview.go:177-193`) | same report |
| D14 | **Owner dashboard UI** | Center-tenancy Phase 4 shipped API only; the owner roll-up UI is explicitly a separate future plan | `journals/2026-08-12-center-tenancy-plan-completed-end-to-end.md` |
| D15 | **Profile fields with no backend** | Môn dạy, bank name/account/holder are local-only on `/profile`, captioned "Chưa lưu trên máy chủ — tính năng đang phát triển"; `.xlsx` export is a toast stub. Bank details for the statement QR come from `API_BANK_*` env, not from the teacher's profile | `plans/260806-1707-…/plan.md`; `adr.md:793` (L5) |
| D16 | **Class settings follow-ups** | M1 name-only save silently unifies times · M2 replaced row's future `effective_to` not carried · M4 `min(1)` price blocks renaming a 0₫ class · L1 `today()` UTC vs `currentMonth()` local disagree 00:00–07:00 VN · L4 student-count stat caps at 100 | `plans/260805-1307-…/plan.md` Follow-ups |
| D17 | **Archived classes have no web surface at all** | No list, no settings, no archive action; capability is API-only. Accepted end state | `plans/260805-1348-…/plan.md` L3 |
| D18 | **Ad-hoc / make-up session creation removed from the UI** | `POST /classes/:id/sessions` still exists server-side; the web can no longer create a make-up session | `plans/260805-1516-…/plan.md` M1 |
| D19 | **Same weekday cannot have two khung giờ** | Blocked client-side; the generator materialises at most one session per class per date (`uq_class_sessions_per_day`) — real support needs a time-aware index + generator | `plans/260811-1508-…/plan.md` non-goals + AC5 |
| D20 | **Excused absence (`nghỉ có phép`) and make-up in another class** | Schema reserves `excused` and a `billable` column; V1 writes only present/absent, both billable | `plans/…-03-…/plan.md` non-goals |
| D21 | **`v_unbilled_attendance` view exists, no endpoint** | The P1 "tiền đang thất thoát" board's data source is already in the baseline schema, unexposed. Note: this is exactly the view D10's bug would surface | `plans/…-04-…/plan.md` non-goals |
| D22 | **Automatic reminders after X days; auto reconciliation from `reference_code`** | Both P1, both explicitly out of V1 | `plans/…-05-…/plan.md`, `plans/…-06-…/plan.md` non-goals |
| D23 | **Read receipts** | P2; `notifications.acknowledged_at` reserved in schema | `plans/…-06-…/plan.md` non-goals |
| D24 | **Retention / hard-delete job** | Per-student anonymise action shipped; the scheduled retention job waits on the unresolved PRD Q2 policy | `plans/…-02-…/plan.md` non-goals |
| D25 | **Rate limit + multi-center membership + cross-center data transfer** | All named non-goals of the center-tenancy plan (rate limiting later landed for auth endpoints only, via invite-only plan) | `plans/260811-1055-…/plan.md:34` |
| D26 | **Bundle-budget measurement never taken** | Parent statement route chunk `<30 KB` gzip target was never measured; build hook blocked the analyzer. "Cần đo lại `stats.html` trong CI/máy dev trước khi phát hành production" | `adr.md:1230` |
| D27 | **Repo-wide Prettier residue** | 5 pre-existing files keep `format:check` red on master | `journals/2026-08-12-center-tenancy-…md` |
| D28 | **Infra cleanups** | `scripts/wait-for.sh` orphan; Makefile `fmt` permanent placeholder; dead `dev*` guards; Dependabot doesn't watch compose images; prod compose can't tune `API_INVITE_TTL`/`RESET_TTL`/DB pool knobs; stale dev `node_modules` anon volume | `reports/code-review-260813-1034-docker-compose-makefile.md` |
| D29 | **Live PoC of Zalo `friends/match`** | Phase 2 shipped as "Done (PoC pre-ship)" — real-account verification deferred to pre-ship by user decision | `plans/260807-1935-…/plan.md:144` |
| D30 | **ADR discipline lapsed after 2026-08-04** | All 57 `adr.md` entries are dated 2026-08-04 (plans 01–08). Deviations from 08-05 onward live only in per-plan "Code review outcome" sections and journals | `adr.md` (`grep '^## '`) |

---

## 5. Open decisions (no chosen answer yet)

| # | Decision | Where it sits |
|---|---|---|
| O1 | **Concrete deployment target** — managed container platform, Kubernetes, or VPS. Stated verbatim as "Open decision" | `docs/deployment.md:5` |
| O2 | **Unbilled-after-close policy** — block back-dated writes into a closed period, carry unbilled sessions forward, or add a controlled reopen? Called a PRD-level decision | `reports/debug-260811-1625-…md` Unresolved questions |
| O3 | **Should the UX stop a teacher closing a period mid-month?** (the live incident was a period closed on the 6th) | same |
| O4 | **PRD Q1 (Zalo bulk-send mechanism)** — *de facto* answered in code (personal-account reverse-engineered protocol) but never written back into the PRD; ZNS remains a non-functional stub, and the account-ban / ToS risk acceptance lives only in a brainstorm report | `docs/prd.md` Q1 vs `reports/brainstorm-260806-1611-…md` |
| O5 | **PRD Q2 (Nghị định 13/2023 children's data)** — controller-vs-processor split and consent capture at enrolment still open; the app collects student names in production today | `docs/prd.md` Q2 |
| O6 | **PRD Q3 (TT19/2026 impact)** — full text still unread | `docs/prd.md` Q3 |
| O7 | **PRD Q4 (market share of pay-after-by-session)** — if <30%, the §4 scope decision is wrong. No interview data exists in the repo | `docs/prd.md` Q4 |
| O8 | **PRD Q5** — resend vs carry-forward after post-send attendance edits. Partially decided in code (carried adjustment `{amount, session_dates}` on the public statement), never confirmed as the product answer | `docs/prd.md` Q5; `adr.md:697` |
| O9 | **PRD Q7** — previous-period history in V1? | `docs/prd.md` Q7 |
| O10 | **PRD Q8** — family underpayment allocation rule + teacher override. Auto-allocation shipped per D8 of plan 05; the "teacher must be able to override" and "family debt vs per-child" question is unvalidated | `docs/prd.md` Q8 |
| O11 | **PRD Q9** — sibling discount: build or not? Structure ready (price on enrollment) | `docs/prd.md` Q9 |
| O12 | **Center-tenancy red-team re-run** — plan's own Open Question #1: the model changed at the root, so the earlier red-team is only partly valid; a fresh pass on Phase 1 (big-bang migration) and Phase 3 (relaxed write invariant) was recommended and there is no evidence it ran | `plans/260811-1055-…/plan.md:86` |
| O13 | **Dashboard "Chốt sổ" CTA placement** — removing `PeriodStatusCard` left the CTA only in the sidebar; flagged as needing a user decision if it should return | `plans/260806-1306-…/plan.md` |
| O14 | **PRD is stale vs the product** — three shipped concepts have no PRD entry: **trung tâm/center** as tenant, **owner** persona, and **invite-only onboarding + operator CLI**. The PRD still describes a single-teacher tenant with self-registration. This is the single biggest documentation debt for the PDR |
| O15 | **Phase 0 / 0.5 validation gate never recorded** — no interview notes, no pre-sale commitments anywhere in the repo, yet Phase 1 was built in full |

---

## 6. Roadmap material (proposal)

Tag key: **[R]** = repo-evidenced (an explicit deferral, open decision, or documented bug); **[I]** = my inference from the evidence, not stated anywhere.

### Near term — unblock what is already broken or unverified

| Item | Why now | Tag |
|---|---|---|
| Fix the unbilled-after-close loss (D10): decide O2, then implement — block/warn on back-dated writes, or carry unbilled sessions, or controlled reopen with notification dedup (D12) and issued-invoice-safe re-close (D13) | Direct violation of **G2** ("students missed from billing = 0"), proven with real money on production | [R] |
| Closed-period review reads frozen invoices, with a "N sessions arose after close" warning (D11) | Same incident; teacher currently sees a number the ledger does not hold | [R] |
| Settle the timezone standard (D2): set `TZ=Asia/Ho_Chi_Minh` or a `today(tz)` helper | Was flagged as a must-fix *before* billing; billing has shipped since | [R] |
| Run and green the full `make e2e` for the invite-only world (D9); clear Prettier residue (D27) | Only gate not proven after the largest auth change | [R] |
| VietQR NAPAS BIN (D4) | Wallets may not resolve the bank → parents fall back to manual entry, weakening R5/G3 | [R] |
| Add the attendance guard on enrollment delete (D3) | Silent data loss path | [R] |
| Infra hygiene sweep (D28) | Small, bounded, already itemised with fixes | [R] |
| Measure the parent-statement bundle (D26) before any real launch | It is a stated product requirement, never verified | [R] |

### Mid term — close the V1 product loop and the doc gap

| Item | Why | Tag |
|---|---|---|
| Rewrite the PRD to a V2 PDR covering **center/owner tenancy**, invite-only onboarding, and the operator role (O14) | Product doc no longer describes the product | [R] |
| Write Q1's real answer into the product doc: personal-Zalo channel, its ban/ToS risk acceptance, and what happens if a teacher's account is blocked | Decision made in a brainstorm, never promoted to product doc | [R] |
| Owner dashboard UI on the shipped roll-up API (D14) | Explicitly named "a separate future plan" | [R] |
| Ship the P1 **"tiền đang thất thoát"** board over `v_unbilled_attendance` (D21) | PRD calls it "đồng thời là luận điểm bán hàng mạnh nhất"; D10 makes it diagnostically valuable too | [R] |
| Automatic debt reminders after X days (D22) + auto reconciliation from `reference_code` | PRD P1; directly serves **G3** | [R] |
| Excused absence + make-up sessions (D20), and multi-slot-per-weekday support (D19) | Both reserved in schema, both blocked today | [R] |
| Assistant role (điểm danh only, no money) | PRD P1; the center-tenancy role machinery makes this cheap now | [I] |
| Second contact per student (1:n → n:n + primary contact) | PRD P1; explicitly warned to need a primary-contact concept to avoid double-counting debt | [R] |
| Sibling discount (unblock per-enrollment price editing) pending O11 | Structure already exists | [R] |
| Retention / hard-delete job (D24) once O5 lands | Legal exposure grows with production data | [R] |
| Class-settings follow-ups D16, plus a web surface for archived classes (D17) if teachers ask | Known rough edges | [R] |

### Long term — hardening, compliance, and the P2 horizon

| Item | Why | Tag |
|---|---|---|
| Postgres RLS (D1) | Named a pre-launch hardening item since day one; the center re-key widened the blast radius (cross-teacher rows are now legal inside a center, isolation lives only in `scoped()`) | [R] |
| Pick and execute the deployment target (O1); until then homelab is the de facto production | Blocks any multi-tenant scale story | [R] |
| Fresh red-team on center tenancy (O12) | Plan's own open question | [R] |
| Parent / student portal auth (D7) + OTP (D6) + token denylist (D5) | All three reserved in schema/docs; only worth building when a real portal or a real revocation need exists | [R] |
| Read receipts (D23) — PRD P2, "chỉ có giá trị sau khi thông báo học phí đã có trạng thái" | Its precondition (notification status ledger) now exists | [R] |
| TT29/2024 + TT19/2026 transparency export (PRD P2), gated on O6 | Could become the sales wedge the PRD hypothesises | [R] |
| Payment gateway / thu hộ with transaction fee (PRD P2) | Needs an intermediary licence or partner | [R] |
| Multi-shift scheduling with clash warnings (PRD P2) | Partially approached by khung-giờ slots; full version needs backend generator work | [R] |
| Multi-center membership + cross-center data transfer (D25) | Named non-goals that become real if centers grow | [R] |
| Horizontal scale of the API | The Zalo personal channel holds a per-teacher session **in process** — the brainstorm states plainly that horizontal scale would break it. Any scale-out plan must solve this first | [R] |
| **Go back and run PRD Phase 0/0.5** (O15, O7) before investing further | The PRD's own instruction; everything above rests on an unvalidated hypothesis | [R] |

---

## 7. Team / process conventions observed

| Convention | Detail |
|---|---|
| **Plan directories** | `plans/<yymmdd-hhmm>-<slug>/` with `plan.md` + `phase-NN-<name>.md` + optional `reports/`. Two generations of `plan.md`: early ones carry YAML frontmatter (`title/description/status/priority/effort/tags/created`), from 2026-08-05 the lighter ones use a plain `Status: … · Branch: … · Source: …` line. Status vocabulary is inconsistent: `completed`, `done`, `implemented`, `in-progress`. Slug is kept even when the plan's subject changes (`manager-class-oversight` now titled "Center Tenancy") |
| **ADR-on-deviation** | `adr.md` (Vietnamese) — "ghi lại các điểm thực tế không khớp với plan… ghi nhận rồi tiếp tục làm việc". 57 entries, each `## <date> — Plan NN, Phase N: <deviation>` + `**Quyết định:**`. **Discipline stopped after 2026-08-04**; later deviations live in plan "Code review outcome" sections and journals instead |
| **Journals** | `plans/journals/<YYYY-MM-DD>-<slug>.md`, frontmatter `title/date/summary`, body `## What happened / ## Decision / ## Next steps`, footer "Historical work record — not durable authority". 24 entries |
| **Reports** | `plans/reports/<kind>-<yymmdd-hhmm>-<slug>.md` where kind ∈ brainstorm / code-review / debug / implement / phase / sweep / tester / test-report / pm. 37 files. Reviews end with a `Status: DONE \| DONE_WITH_CONCERNS \| BLOCKED` + Summary + Concerns block |
| **Review gate** | A `code-reviewer` subagent pass on most plans; findings triaged H/M/L, Highs fixed, Mediums/Lows explicitly accepted **with the user's decision recorded in the plan file**. Several plans also went through a 4-reviewer "red team" (security / failure-mode / assumption-audit / scope) requiring `file:line` evidence per finding |
| **TDD** | Later plans (center tenancy, invite-only, zalo auto-map) state TDD mode with Tests-Before / Refactor / Tests-After / Regression-Gate structure per phase |
| **Conventional commits** | Enforced by commitlint via lefthook `commit-msg`; scopes seen: `api, web, zalo, notifications, billing, payments, infra, deploy, ci, e2e, plans, docs, readme, dev, test, chore, style, refactor, fix`. Escape hatch `LEFTHOOK=0` |
| **Pre-commit hooks** | lefthook, serial: `gofmt -w` (restaged), `golangci-lint run`, `prettier --write` (restaged), `eslint --fix` (restaged) |
| **CI gates** | `api-ci.yml` (test-api with coverage gate 60%, `make api-docs` + fail-if-stale OpenAPI, GHCR image publish), `web-ci.yml` (lint, `format:check`, typecheck, `test:coverage`, build+push, and a Playwright e2e job that boots the full compose stack and seeds it), `security.yml` (govulncheck, `audit-ci --high`, Trivy image scan, CodeQL SARIF upload). Dependabot on the two app Dockerfiles only |
| **Design source of truth** | Screens are built to a single Claude design prototype — project `4a7e6c77`, file `So Lop - Prototype.dc.html` — and plans cite it by screen label and line number, with an explicit "100% adherence" clause and named deviations |
| **Language split** | Product/UI copy, plans from 08-05 onward, and all of `adr.md` are Vietnamese; identifiers, code comments, test names, and the API/architecture docs are English |
| **Brand** | Renamed from **Sổ Lớp** to **Teka** on 2026-08-05 (`498e2e2`); prototype files and plan titles still say "Sổ Lớp" |

---

## Unresolved questions for the docs-manager

1. **PRD vs product divergence (O14)** — should the new PDR be written as "V1 as built" (center tenancy, owner, invite-only) with the original PRD preserved as a historical hypothesis doc, or should `docs/prd.md` be edited in place? The PRD's own framing (an unvalidated hypothesis to be discarded if Phase 0 fails) argues for preserving it.
2. **Which status is authoritative for plan 16 (`260805-1551-diem-danh-prototype-gaps`)?** File says `in-progress` with an empty Verification section; code contains all three deliverables. I recorded it as "shipped, plan never closed" — someone with history should confirm before the roadmap treats it as closed.
3. **Is the homelab deployment "production"?** The debug report treats `teka-api.cauchuyenlaptrinh.com` as production with a real teacher's data, while `docs/deployment.md` still calls the deployment target an open decision. The roadmap's framing of D1/D26/O1 depends on this answer.
4. **Was PRD Phase 0/0.5 ever done outside the repo?** Nothing in `plans/`, `docs/`, or git suggests interviews or pre-sales happened. If they did, the evidence is not here and the roadmap's "go back and validate" item should be dropped.
5. **Should ADR discipline be restarted or formally retired?** `adr.md` has been dormant since 2026-08-04 while the equivalent content moved into plan files and journals — one of the two conventions should be documented as the real one.
