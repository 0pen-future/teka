# Brainstorm: Học vụ gửi báo cáo theo lớp (học phí + nhận xét/điểm từng buổi)

Date: 2026-09-02. Status: contract ACCEPTED by user (4 decisions below). Next: `/ak:plan`.

## Request

"As hoc_vu, send report to contactor and owner by each class: (1) bill/invoice
of student to contactor; (2) nhận xét từng buổi học và điểm số thành phần."

## Evidence (verified in repo)

| Area | State | Source |
|------|-------|--------|
| Role `hoc_vu` | Class staff role, one capability `statement.send` | `apps/api/internal/shared/authctx/class_staff.go:83` |
| Bill/invoice per class (API) | DONE: class-scoped statement + per-class run, hoc_vu sends from own Zalo, preview/run/resume, parallel per class | migration `000017`, `notifications/service.go:120-415`, `docs/api-guidelines.md:165-173` |
| Bill/invoice per class (web) | GONE: `ClassSendPeriodsDialog` + nút "Gửi báo cáo" deleted in `d3e6b28` (roster → owner-only). `/notifications/:periodId?class_id=` page still works; `/reports` nav only for center-wide `reports.send` | `apps/web/e2e/class-staff-write.spec.ts:129-133`, `dashboard-layout.tsx:131` |
| Nhận xét/điểm data | `session_notes` (class-wide/buổi), `session_marks` (score 0–10 + `personal_note` per HS/buổi), `student_scores` (điểm thành phần per HS/buổi/component). hoc_vu reads, cannot write | migrations `000009`, `000014`; `teaching/routes.go`, `grading/routes.go` |
| Nhận xét in messages | NONE. Secretary plan D8 (2026-08-29) explicitly excluded remarks from message content — this request reverses that for the hoc_vu channel | `plans/260829-1020-secretary-report-sender/plan.md` D8 |
| Ledger coupling | `notifications.statement_id NOT NULL`, `notification_runs.billing_period_id NOT NULL`, `purpose IN ('statements','reminder')`, message text NOT persisted | `000001`, `000005`, `notifications/model.go` |
| Run engine | `RunManager` decoupled from period: needs notification id, run id, `RunGrant{ClassID}` probe via `ClassSendAllowed(classID)` | `notifications/run_manager.go:54-100` |
| Zalo | `zalo.Service.SendDM(teacherID, toUID, text)`; contact mapping open to assigned hoc_vu (class-limited); pacing + `MaxRunSize` + `MaxMessageLen` config | `zalo/service.go:270`, `notifications/service.go:459` |
| Web hub | "Quản lý lớp học" = `/classbook` (ClassbookPage, tabs "Buổi học & nhận xét" / "Chương trình & giáo án"), perm `classes.list`, visible to GV + hoc_vu; `my_staff_roles` on class DTO | `teaching/pages/classbook-page.tsx`, `classes/dto.go:84` |

## User decisions (2026-09-02)

| # | Question | Decision |
|---|----------|----------|
| U1 | Cadence for nhận xét + điểm | **Gửi riêng sau mỗi buổi học** (not monthly digest) |
| U2 | What owner receives | **Nothing new** — owner reads in-app (ledger, audit, classbook) |
| U3 | Parent-facing fields | **Per-student only**: `session_marks.personal_note`, `session_marks.score`, `student_scores` (components). `session_notes` (class-wide) stays internal |
| U4 | Web entry point | **"Quản lý lớp học" page for GV and học vụ** — the existing `/classbook` becomes the shared class hub; send actions live there |

Assumption on U4 (state in plan, not re-ask): send buttons render only for
roles holding the send capability (hoc_vu + owner). Giáo viên sees the page
for remarks/scores input but no send action; granting GV send rights is a
one-line capability-map change the owner can request later.

## Contract

**Outcome.** On "Quản lý lớp học", an assigned học vụ (or owner) can, per
class: (a) send the class's học phí statement copies for a billing period to
each contact (restored entry to the existing class-scoped send), and (b) after
any held session, send each contact one Zalo DM containing their child's
per-session score, component scores and personal remark. Sends are paced from
the học vụ's own Zalo, attributed to them, visible in an in-app ledger.

**Constraints.**
- Reuse the existing send engine (`RunManager`, senders, pacing, `MaxRunSize`,
  `MaxMessageLen`, reconciler, audit registry); no second run loop.
- Authorization via the code-owned capability map: new
  `CapSessionReportSend = "session_report.send"` → `{hoc_vu}`; owner passes
  every gate; GV/trợ giảng → 403, outsider → 404 (repo convention).
- Tenancy invariants of `000007`/`000009`: composite FKs by center; no
  cross-center pointer.
- Contacts reached only through existing Zalo mapping; unmapped → manual
  copy-paste fallback bucket (existing pattern).
- Phone privacy rules unchanged (`PhoneVisibleVia*`).
- Zero behavior change for family statements, secretary center-wide sends,
  owner.

**Non-goals.**
- `session_notes` (class-wide remark) to parents; DM/summary to owner;
  teacher (`giao_vien`) send rights; ZNS/SMS channels; public token link for
  remarks; học vụ editing remarks/scores; monthly remarks digest inside the
  statement link (may come later as a separate decision).

**Acceptance criteria.**
1. Học vụ on `/classbook` sees "Gửi báo cáo học phí" per class → picks period
   → lands on `/notifications/:periodId?class_id=` and completes a class run
   (restored e2e journey in `class-staff-write.spec.ts`).
2. Học vụ selects a held session → "Gửi nhận xét buổi này" → preview buckets
   (mapped / unmapped / chưa có dữ liệu / đã gửi) → confirm → paced DMs from
   own Zalo; ledger rows carry `session_id`, sender = học vụ.
3. Message per contact lists each child in the class for that session:
   điểm buổi, điểm thành phần (name: score, in `position` order), nhận xét
   riêng. No `session_notes` text ever appears (unit test on builder).
4. Students with no mark/score/remark, or marked absent (U7), are skipped and
   counted in their own preview buckets; a contact already sent for that
   session is skipped unless resend is requested, and a resend supersedes the
   previous row instead of deleting it (U5). A session without confirmed
   attendance cannot be sent (U6).
5. Two học vụ on different classes send in parallel; same session twice → 409.
6. Denials: GV/trợ giảng POST → 403; foreign class → 404; ended stint → 403;
   mid-run revoke stops the run (existing `ClassSendAllowed` probe).
7. Owner sees the session ledger on the same panel + audit rows for
   bulk/resume; nothing is DM'd to owner.
8. `make test-api`, `make test-web`, e2e green; docs (`api-guidelines.md`
   capability list + class-scoped section) updated.

## Approaches compared (nhận xét per buổi)

| | A. Digest in statement link | B. Generalize `notifications` ledger with a session anchor (**chosen**) | C. New `session_reports` tables + own run loop |
|-|-|-|-|
| Cadence | Monthly | Per session | Per session |
| Schema | None on notifications | Nullable `statement_id`, add `session_id`, `contact_id`, `message_text`; runs: nullable `billing_period_id`, add `session_id`; one-of CHECKs; purpose `session_report` | 2 new tables |
| Engine reuse | Full | Full (`RunStore` is id-based; `RunGrant.ClassID` probe fits) | Duplicate `RunStore` impl, reconciler, mark-sent, ledger |
| Risk | Rejected by U1 | Polymorphic row: every period-keyed query must not accidentally include session rows (explicit `purpose`/anchor filters; period ledger index stays `WHERE statement_id IS NOT NULL`) | Most code, two ledgers to teach the UI |
| Spam exposure | Lowest | Higher (per session) — mitigated by pacing, `MaxRunSize`, skip-empty, skip-sent | Same as B |

B is the smallest design that satisfies U1 without a second engine. The
"nullable FK + CHECK one-of" anchor is the same shape `000017` used for
`class_id`, so the pattern is already precedent in this schema.

## Chosen design (B) — sketch for the plan

**Migration `000022_session_reports`.**
- `notifications`: `statement_id` DROP NOT NULL; add `session_id UUID`,
  `contact_id UUID`, `message_text TEXT`; composite FKs `(session_id, center_id)
  → class_sessions`, `(contact_id, center_id) → contacts`; CHECK
  `(statement_id IS NOT NULL) <> (session_id IS NOT NULL)`; CHECK contact_id
  required when session_id set; add `superseded_at TIMESTAMPTZ`; purpose CHECK
  += `'session_report'`; partial unique `(session_id, contact_id) WHERE
  purpose='session_report' AND superseded_at IS NULL AND deleted_at IS NULL`
  (U5: resend stamps `superseded_at` on the live row, never deletes).
- `notification_runs`: `billing_period_id` DROP NOT NULL; add `session_id`;
  CHECK one-of; partial unique active run `(session_id) WHERE
  status='running' AND session_id IS NOT NULL`. Existing partial indexes keep
  their `class_id`/period predicates, unchanged.
- Why persist `message_text` here and not for statements: statements re-render
  deterministically from the invoice snapshot; remarks have no snapshot and
  are editable after send, so the ledger must hold what the parent actually
  received.

**Backend.**
- `authctx`: `CapSessionReportSend` → `{StaffRoleHocVu}` (+ table test).
- New read port in `teaching`/`grading` for "session report rows": one query
  joining active enrollments of the session's class × `session_marks` ×
  `student_scores` (+ component names/positions), grouped by contact. Only
  students with ≥1 datum. Consider a small `sessionreport` package under
  `notifications` (pure builder, like `statements.Build`) so it is unit-testable
  without DB.
- `notifications.Service`: extract the shared "queue rows + run" core from
  `BulkSend` (target list → per-contact text → rows → run) and feed it from
  two sources (statement targets vs session targets). Strategy-shaped: the
  source decides targets and text; the core owns channel, pacing, run, audit.
- Endpoints (mirroring the period shape, service-gated → `PolicyService` in
  `route_policy.go`, plus audit registry entries):
  `GET /sessions/:id/reports/preview`, `POST /sessions/:id/reports/bulk`,
  `GET /sessions/:id/reports`, `GET /sessions/:id/reports/run`,
  `POST /sessions/:id/reports/run/resume`.
- Gate: session → class → `classscope.WriteExists` with
  `StaffRolesFor(CapSessionReportSend)`; owner bypass; `RunGrant{ClassID}`.
- Message (Vietnamese, ≤ `MaxMessageLen`; truncate remark text with "…",
  never drop scores):
  ```
  Chào anh/chị {contact},
  Buổi {dd/mm} lớp {class}:
  - {student}: điểm buổi 8.5 · Listening 7 · Speaking 8 · Nhận xét: {personal_note}
  ```

**Web (`/classbook` = Quản lý lớp học, GV + học vụ).**
- Header action "Gửi báo cáo học phí" (hoc_vu/owner via `my_staff_roles` ∋
  `hoc_vu` || `isOwner`): restore `ClassSendPeriodsDialog` from git history →
  `/notifications/:periodId?class_id=`.
- `SessionDetailPanel`: "Gửi nhận xét buổi này" → preview dialog (4 buckets +
  rendered texts, copy-paste bundle for unmapped) → run progress (reuse
  the notifications page run components) → ledger tab per session with
  "đã sửa sau khi gửi" hint when `session_marks.updated_at > sent_at`.
- `SessionsTable`: sent indicator per session (n/m contacts).
- Rename/clarify the per-student remark label to signal it is parent-facing
  (e.g. "Nhận xét gửi phụ huynh") since U3 externalizes `personal_note`.

## Risks

- **Zalo spam heuristics**: per-session cadence multiplies volume (5 lớp × 3
  buổi/tuần × 20 PH ≈ 300 DM/tuần from one account). Mitigation: existing
  pacing, `MaxRunSize`, skip-empty/skip-sent, befriend-first UX already built.
  Operational, not testable in e2e.
- **Polymorphic ledger regressions**: any query that assumed
  `statement_id NOT NULL` (period ledger, collections, statement revoke
  cascade) — plan must grep every `notifications` reader and add explicit
  anchor filters; migration parity test (`backfill_parity_test.go` pattern).
- **Privacy scope creep**: `personal_note` was written under a "private" label
  in schema comments; now parent-facing. UI label change + one-line notice in
  the remark editor mitigates surprise for teachers.
- **Stale text after edit**: persisted `message_text` + "đã sửa" hint +
  explicit resend path. No auto-resend.

## Follow-up decisions (user, 2026-09-02)

| # | Question | Decision | Consequence for the plan |
|---|----------|----------|--------------------------|
| U5 | Resend semantics | **Keep history**: N rows per (session, contact); a resend stamps `superseded_at` on the previous live row instead of deleting it | Partial unique on live rows only: `(session_id, contact_id) WHERE purpose='session_report' AND superseded_at IS NULL AND deleted_at IS NULL`; ledger shows the chain, latest first |
| U6 | Require confirmed attendance | **Yes**: session must be `held` with `attendance_confirmed_at` set | Preview/bulk return 409-style "chưa xác nhận điểm danh" otherwise; same rule that makes a session billable |
| U7 | Absent student with data | **Do not send**: attendance status absent → skipped even if a score/remark exists | Target query joins `attendance_records` and keeps only present/late statuses; absent counted in a separate preview bucket ("vắng") so the học vụ sees why |

No unresolved questions remain for the contract.
