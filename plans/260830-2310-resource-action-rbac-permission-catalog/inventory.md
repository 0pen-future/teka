# Phase 1 freeze: route inventory, catalog keys, compatibility contract

Frozen 2026-08-31 on branch `teka/260831-0016` (base master 1b00b2f, latest
migration `000017`). Evidence: full read of all 22 `routes.go` files plus
middleware, authctx, migrations 000007/000013/000015, and service/repository
gates. Route-count completeness is enforced mechanically by the Phase 2
registry⇄`engine.Routes()` bidirectional test, which fails on any
unclassified route.

## 1. Namespace convention (frozen)

- Catalog keys are `<resource>.<action>`.
- `action` ∈ canonical CRUD verbs `create|list|read|edit|delete`, the scope
  verb `view_all`, or a named special (snake_case, never a CRUD verb).
- No catalog key may equal a class-staff capability string
  (`attendance.write`, `scores.write`, `remarks.write`, `lesson_plan.write`,
  `enrollment.write`, `sessions.write`, `statement.send`) — registry guard
  test enforces this.
- Wildcard/prefix aliases are forbidden; every alias is an explicit
  old-key → set-of-new-keys mapping.
- Legacy keys `reports.send`, `members.manage`, `center.manage`,
  `invitations.manage`, `audit.read`, `imports.run`, `dashboard.view`,
  `teaching.review_queue` keep their exact strings as canonical catalog
  entries (identity mapping, no alias rows needed).
- `data.view_center_wide` is the only decomposed key: alias 1:N to the 12
  `<resource>.view_all` keys in §3. Legacy grant → all 12 canonical grants;
  legacy deny → all 12 canonical denies; a deny of one canonical key never
  propagates back through the legacy key to the other 11.

## 2. Catalog definitions (kind=crud and specials)

Definition fields (Phase 2 struct): key, resource, action, kind
(crud|scope|special), label (vi), description (vi), risk (low|medium|high),
grantable, deprecated, order.

Default-grant policy reproducing today's access: today membership alone
authorizes every member-baseline operation (roles carry zero permission rows).
Therefore every key marked **D** below is default-granted to all three system
roles (`giao_vien`, `hoc_vu`, `tro_giang`) by the Phase 3 backfill and by the
new-center default path; members with `role_id IS NULL` (non-owner) get the
same set as member-level grants. The 8 legacy identity keys keep their
existing per-center assignments unchanged — no expansion. `view_all` keys are
granted only via `data.view_center_wide` backfill.

| Key | Kind | Risk | D | Routes covered (method path) |
|---|---|---|---|---|
| classes.create | crud | low | D | POST /classes |
| classes.list | crud | low | D | GET /classes |
| classes.read | crud | low | D | GET /classes/:id; GET /classes/:id/staff (staff roster rides class read visibility) |
| classes.edit | crud | low | D | PUT /classes/:id (AND active-stint write gate) |
| classes.delete | crud | medium | D | DELETE /classes/:id (AND write gate) |
| classes.archive | special | medium | D | POST /classes/:id/archive (AND write gate) |
| schedules.create | crud | low | D | POST /classes/:id/schedules (AND write gate) |
| schedules.edit | crud | low | D | PUT /classes/:id/schedules/:scheduleID (AND write gate) |
| schedules.delete | crud | low | D | DELETE /classes/:id/schedules/:scheduleID (AND write gate) |
| contacts.create | crud | low | D | POST /contacts |
| contacts.list | crud | low | D | GET /contacts |
| contacts.read | crud | low | D | GET /contacts/:id |
| contacts.edit | crud | low | D | PUT /contacts/:id |
| contacts.delete | crud | medium | D | DELETE /contacts/:id |
| contacts.link_zalo | special | low | D | PUT+DELETE /contacts/:id/zalo-mapping (visibility keeps frozen ReportsOversight branch) |
| students.create | crud | low | D | POST /students |
| students.list | crud | low | D | GET /students |
| students.read | crud | low | D | GET /students/:id |
| students.edit | crud | low | D | PUT /students/:id |
| students.delete | crud | high | D | DELETE /students/:id (anonymize+delete) |
| enrollments.create | crud | low | D | POST /enrollments; GET /classes/:id/enrollable-students (picker exists only to create — irregular mapping, AND CapEnrollmentWrite for members) |
| enrollments.list | crud | low | D | GET /enrollments |
| enrollments.read | crud | low | D | GET /enrollments/:id |
| enrollments.delete | crud | medium | D | DELETE /enrollments/:id |
| enrollments.end | special | medium | D | POST /enrollments/:id/end |
| sessions.create | crud | low | D | POST /classes/:id/sessions (AND CapSessionsWrite for members) |
| sessions.list | crud | low | D | GET /classes/:id/sessions; GET /sessions/pending (single-resource aggregate) |
| sessions.read | crud | low | D | GET /sessions/:id |
| sessions.delete | crud | medium | D | DELETE /sessions/:id (AND write gate) |
| sessions.lifecycle | special | medium | D | POST /sessions/:id/{cancel,uncancel,hold} (AND write gate) |
| attendance.read | crud | low | D | GET /sessions/:id/attendance |
| attendance.confirm | special | medium | D | POST /sessions/:id/attendance (AND CapAttendanceWrite for members) |
| scores.read | crud | low | D | GET /sessions/:id/scores; GET /classes/:id/score-components |
| scores.edit | crud | medium | D | PUT /sessions/:id/scores (AND CapScoresWrite for members) |
| teaching.read | crud | low | D | GET /classes/:id/{curriculum,lesson-plans,marks} (rides class read visibility) |
| teaching.edit | crud | low | D | PUT /classes/:id/curriculum; PUT /classes/:id/lesson-plans/:index; POST …/submit; PUT /sessions/:id/{note,marks} (AND write gates) |
| teaching.review_queue | special (legacy identity) | low | — | GET /teaching/review-queue |
| billing.create | crud | low | D | POST /billing-periods (EnsurePeriod, self-anchored) |
| billing.list | crud | low | D | GET /billing-periods |
| billing.read | crud | low | D | GET /billing-periods/:id; GET /billing-periods/:id/preview; GET /invoices/:id/adjustments; GET /billing-periods/:id/collections{,/summary} (financial reads, irregular mapping) |
| billing.draft | special | medium | D | POST /billing-periods/:id/draft |
| billing.close | special | high | D | POST /billing-periods/:id/close |
| billing.void_invoice | special | high | D | POST /invoices/:id/void |
| billing.adjust_invoice | special | high | D | POST /invoices/:id/adjustments |
| payments.create | crud | low | D | POST /payments |
| payments.list | crud | low | D | GET /payments |
| payments.read | crud | low | D | GET /payments/:id |
| payments.allocate | special | medium | D | PUT /payments/:id/allocations; POST /payments/:id/allocations/auto |
| payments.reverse | special | high | D | POST /payments/:id/reverse |
| statements.list | crud | low | D | GET /billing-periods/:id/statements (read visibility keeps frozen ReportsOversight branch) |
| statements.read | crud | low | D | GET /statements/:id (same frozen read branch) |
| statements.generate | special | high | D | POST /billing-periods/:id/statements/generate |
| statements.revoke | special | high | D | POST /statements/:id/revoke |
| notifications.mark_sent | special | low | D | POST /notifications/mark-sent |
| reports.send | special (legacy identity) | high | — | POST /billing-periods/:id/notifications/bulk; GET …/notifications{,/preview,/run}; POST …/run/resume — semantics FROZEN: gate stays `ReportsOversight() OR AuthorizeClassSend` (dual-write with can_send_reports continues; class hoc_vu path unchanged) |
| members.manage | special (legacy identity) | high | — | DELETE /centers/me/members/:teacherId |
| center.manage | special (legacy identity) | medium | — | PATCH /centers/me |
| invitations.manage | special (legacy identity) | medium | — | POST+GET /centers/me/invitations; DELETE /centers/me/invitations/:id |
| audit.read | special (legacy identity) | medium | — | GET /audit-logs |
| imports.run | special (legacy identity) | high | — | GET /imports/roster/template; POST /imports/roster (verified: gate is `requireImportsRun`/`Has(PermImportsRun)`, runs under server-side owner anchor) |
| dashboard.view | special (legacy identity) | medium | — | GET /centers/dashboard/* (5 routes — multi-resource aggregate, one explicit key) |

## 3. Scope keys (kind=scope) — decomposition of data.view_center_wide

Widening today happens only through `Scope.CenterWide()` (= IsOwner ∨
`data.view_center_wide`). Every branch maps to the `view_all` key of the
resource being read/written-across, including branches reached through the
shared class-readable helpers (`classscope.ReadExists*`, `resolveReadable*`),
which Phase 4 parameterizes per resource key. 12 keys, all risk=high,
grantable, never default-granted:

| Key | CenterWide() branch points today |
|---|---|
| classes.view_all | classes scoped/readScoped (List, GetReadable); classstaff readAccess |
| contacts.view_all | contacts scoped (Create/Update/Delete), CountActiveStudents, ListStudentNames |
| students.view_all | students scoped (writes), readScoped (List/Get), ContactExists |
| enrollments.view_all | enrollments scoped/readScoped/writeScoped, ClassDefaultPrice |
| sessions.view_all | sessions scoped/readScoped/writeScoped |
| attendance.view_all | attendance readScoped, TallyByEnrollment, StudentNames |
| scores.view_all | grading resolveReadableClass/resolveReadableSession center-wide reader arm |
| teaching.view_all | teaching class-visibility resolution center-wide arm |
| billing.view_all | billing scoped (ListPeriodsRead/GetPeriodRead/writes) |
| payments.view_all | payments scoped/allocationScoped, CandidateInvoices, InvoicesByIDs, ResolveContactScope |
| statements.view_all | statements scoped (Revoke), GetPeriodStatus (generate) |
| notifications.view_all | notifications MarkSent scoped |

Frozen and NOT part of this decomposition: every `ReportsOversight()` branch
(contacts scopedRead, collections, statements scopedRead/GetPeriodStatusRead,
notifications ListByPeriod/ZaloMappings, billing period reads, zalo
matchFriends) — that is the `reports.send`/`can_send_reports` axis, untouched
until its own cleanup plan.

Operation-vs-visibility invariant: a policy key answers "may call this
operation at all" (self-scoped rows), the resource's `view_all` answers "on
whose rows". Example: any member may `payments.reverse` their own-anchored
payments today; only `payments.view_all` holders (ex-`data.view_center_wide`)
reach other teachers' rows. Cutover must preserve both directions per
resource.

## 4. Owner-only, non-grantable (frozen — no catalog keys ever)

| Routes | Rationale |
|---|---|
| GET /centers/me/permissions; PUT /centers/me/roles/:roleId/permissions; PUT /centers/me/members/:teacherId/{role,overrides} | permission administration (one-hop escalation) |
| POST+DELETE /centers/me/members/:teacherId/send-reports | legacy dual-write toggle, frozen |
| POST /classes/:id/staff; DELETE /classes/:id/staff/:staffId | staffing |
| PUT /classes/:id/teacher (handoff) | staffing/handoff |
| POST /classes/:id/lesson-plans/:index/{approve,request-redo,reopen} | sensitive review writes |
| GET+POST /score-sets; PUT+DELETE /score-sets/:id; POST+DELETE /classes/:id/score-set | grading configuration — owner-only today, delegation deliberately deferred |

## 5. Exempt classifications

- **public** (no auth): POST /auth/{login,refresh,logout,forgot-password,reset-password}; POST /invitations/{preview,accept} (rate-limited, token in body).
- **public-token** (token in path): GET /public/statements/:token{,/qr.png}.
- **authenticated-self** (no center scope): GET+PUT /me; GET+DELETE /me/zalo; GET /me/zalo/friends; POST /me/zalo/friends/request; POST /me/zalo/link/start; GET /me/zalo/link/status. POST /me/zalo/friends/match keeps its frozen `ReportsOversight() OR active hoc_vu` gate (reports axis).
- **member-baseline** (any active member, deliberately un-keyed): GET /centers/me.

## 6. Compatibility mapping (old → new), grants and denies symmetric

| Legacy key | Disposition |
|---|---|
| data.view_center_wide | alias → all 12 `view_all` keys (grants AND denies expand to the full set; retained during compatibility window; deleted in Phase 8 after parity soak) |
| reports.send, members.manage, center.manage, invitations.manage, audit.read, imports.run, dashboard.view, teaching.review_queue | identity — same string stays canonical; existing role rows and member grant/deny rows remain valid untouched |
| can_send_reports column | out of scope — frozen dual-write semantics continue |

Role-default backfill: insert the §2 **D** key set for the 3 system roles per
center, idempotent/conflict-safe; also as member-level grants for any
non-owner member whose `role_id IS NULL`. Custom roles do not exist (no role
CRUD endpoint; only the 3 system roles are seeded) — the "custom role"
expansion path therefore only covers NULL-role members.

## 7. Fixture matrix (Phase 2/7 test contract)

Personas per center: owner; member role giao_vien; member role hoc_vu; member
role tro_giang; member with NULL role; member with `data.view_center_wide`
grant (→ post-backfill: 12 view_all); member with a deny of one key that the
role grants. Class-staff dimension per class-scoped resource (attendance,
scores, sessions, enrollments, teaching): active stint × ended stint × no
stint — reads allowed for any stint (active or ended), writes only for active
stint whose role holds the capability (per plan 260830-0938 assignment-based
scoping). Every route family asserts: unauthenticated 401; missing key 403;
owner ok; role-granted ok; member-grant ok; member-deny overrides role+grant;
wrong-center 404-masked; hidden object non-leak; capability denial where
applicable.

## 8. Traced flows (verification of layering)

- CRUD: GET /contacts/:id → RequireAuth → ResolveScope → handler → service →
  scopedRead (ReportsOversight read branch, frozen) — future policy
  contacts.read at route layer, visibility unchanged.
- Special: POST /payments/:id/reverse → scope resolve → service → scoped()
  CenterWide branch → row lock + reversal — future payments.reverse +
  payments.view_all split.
- Owner-only: PUT /classes/:id/teacher → service `!sc.IsOwner` 403 — stays.
- Self: GET /me → RequireAuth only, no center scope — stays exempt.
- Public-token: GET /public/statements/:token → token_hash lookup, no
  Principal — stays exempt.

## 9. Aggregate endpoints (single-key rule)

dashboard.view (5 dashboard routes), sessions.list (/sessions/pending),
billing.read (collections + summary), imports.run (template + run),
enrollments.create (enrollable-students picker). None composes multiple
`view_all` keys.

## 10. Mixed-version notes (single-host)

Old binary + new DB rows: unknown keys are dropped by `BuildPermSet` — legacy
rows still present ⇒ legacy behavior intact. New binary + old DB: aliases
resolve legacy rows ⇒ equivalent access. Deploy order (single host):
alias-capable binary first, then backfill migration, then enforcement flag,
then UI. Stale web clients: assignment writes carry catalog+assignment
versions; stale replacement → 409, never a destructive merge.
