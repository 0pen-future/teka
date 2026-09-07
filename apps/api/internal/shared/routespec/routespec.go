// Package routespec is the single manifest of every registered HTTP route's
// authorization policy and audit-trail behavior. It replaces three tables
// that were maintained separately — server.routePolicies (policy),
// audit.actions (audit action naming), and the three skip maps in
// middleware/request_events.go (audit-source classification) — with one Spec
// per route that all three consumers derive from. A route's policy and its
// audit behavior are declared together, so adding a route means deciding
// both in the same change.
//
// This package is a dependency leaf: it imports only stdlib and authctx (for
// the grantable permission-key constants), never a feature, middleware, or
// server package, so nothing importing it can form a cycle.
package routespec

import "teka/apps/api/internal/shared/authctx"

// Kind classifies how a route is authorized. Every registered route carries
// exactly one intentional classification; the manifest test fails closed on
// any route added without one.
type Kind string

const (
	// KindPublic routes need no session: none exists yet (login, password
	// reset) or the route is infrastructure (health, swagger).
	KindPublic Kind = "public"
	// KindPublicToken routes are unauthenticated but gated on an
	// unguessable token in the URL or body (public statements, invitations).
	KindPublicToken Kind = "public_token"
	// KindSelf routes serve an authenticated caller acting on their own
	// account/center membership; no center permission involved.
	KindSelf Kind = "self"
	// KindOwnerOnly routes are hard-gated on ownership, never grantable —
	// one grant away from escalation otherwise.
	KindOwnerOnly Kind = "owner_only"
	// KindPermission routes are gated on one grantable catalog key. Tenant
	// scope, object visibility, and class-capability checks remain
	// independent layers behind the permission.
	KindPermission Kind = "permission"
	// KindService routes carry their whole authorization inside the feature
	// service, because no single catalog key names the allowed callers: the
	// notifications routes admit reports oversight, an active class-send
	// stint, or (for reads) the period's own teacher. The policy layer only
	// guarantees an authenticated live member; the service's own gates
	// decide, and fail closed.
	KindService Kind = "service"
)

// AuditSource classifies how (or whether) a route's mutation reaches the
// audit trail.
type AuditSource string

const (
	// SourceRequest routes are audited generically by the request
	// middleware: LookupAction resolves Action/EntityType/IDParam for the
	// audit row. Every mutating route with this source must carry a
	// non-empty Action — an empty one would silently fall back to the
	// less-readable "METHOD route" label.
	SourceRequest AuditSource = "request"
	// SourceService routes publish their own domain event instead of
	// letting the generic request middleware log them — the service event
	// carries richer identifiers the HTTP layer cannot see (e.g.
	// enrollments.StudentEnrolled carries the class and student ids). The
	// request middleware must skip these paths entirely to avoid a second,
	// poorer row for the same mutation.
	SourceService AuditSource = "service"
	// SourceAuthSession routes are audited by the auth service's own
	// events, never by the request middleware: login and logout would
	// double-log, and refresh is deliberate noise avoidance (token
	// rotation is not a user action).
	SourceAuthSession AuditSource = "auth_session"
	// SourceAnonymous routes are the rare unauthenticated mutations worth a
	// row even with no principal: a password change must never escape the
	// trail even though the caller has no session yet.
	SourceAnonymous AuditSource = "anonymous"
	// SourceNone routes never produce a request-audit row: either the
	// route does not mutate (most GETs), or it is a mutating route
	// deliberately excluded with a documented reason (see Specs).
	SourceNone AuditSource = "none"
)

// Audit is one route's audit-trail classification. Action, EntityType, and
// IDParam are meaningful only when Source is SourceRequest or
// SourceAnonymous — the audit subscriber's requestRow reads them to build a
// human-readable row; IDParam is the route parameter holding the entity id
// ("" when the route addresses no single entity, e.g. collection creates).
type Audit struct {
	Source     AuditSource
	Action     string
	EntityType string
	IDParam    string
}

// Spec is one registered route's frozen authorization and audit
// classification. Method and Path match gin's registration exactly. Key is
// set only for KindPermission.
type Spec struct {
	Method string
	Path   string
	Kind   Kind
	Key    string
	Audit  Audit
}

// perm builds a KindPermission Spec with the given audit classification.
func perm(method, path, key string, audit Audit) Spec {
	return Spec{Method: method, Path: path, Kind: KindPermission, Key: key, Audit: audit}
}

// classified builds a non-permission Spec with the given audit
// classification.
func classified(method, path string, kind Kind, audit Audit) Spec {
	return Spec{Method: method, Path: path, Kind: kind, Audit: audit}
}

// none is the audit classification for a route that never produces a
// request-audit row, usually because it does not mutate.
func none() Audit { return Audit{Source: SourceNone} }

// req is the audit classification for a route audited generically by the
// request middleware.
func req(action, entityType, idParam string) Audit {
	return Audit{Source: SourceRequest, Action: action, EntityType: entityType, IDParam: idParam}
}

// Specs is the single manifest: the bidirectional coverage test compares it
// against engine.Routes(), server.routePolicies derives the policy view from
// it, audit.LookupAction derives the action view from it, and
// middleware/request_events.go derives its three skip sets from it via
// WithSource.
var Specs = []Spec{
	// Infrastructure and unauthenticated entry points.
	classified("GET", "/healthz", KindPublic, none()),
	classified("GET", "/readyz", KindPublic, none()),
	classified("GET", "/swagger/*any", KindPublic, none()),
	classified("POST", "/api/v1/auth/login", KindPublic, Audit{Source: SourceAuthSession}),
	classified("POST", "/api/v1/auth/refresh", KindPublic, Audit{Source: SourceAuthSession}),
	classified("POST", "/api/v1/auth/logout", KindPublic, Audit{Source: SourceAuthSession}),
	// The password reset routes ARE audited — no service event covers
	// them, and a password change must never escape the trail even with an
	// unauthenticated actor.
	classified("POST", "/api/v1/auth/forgot-password", KindPublic,
		Audit{Source: SourceAnonymous, Action: "auth.password_reset_request", EntityType: "user"}),
	classified("POST", "/api/v1/auth/reset-password", KindPublic,
		Audit{Source: SourceAnonymous, Action: "auth.password_reset", EntityType: "user"}),
	// The public preview/accept routes never carry a session (KindPublicToken
	// means no RequireAuth ever runs ahead of them), so the request
	// middleware's authenticated-actor gate always skips them regardless of
	// this Source; accept's outcome is audited instead through the
	// invitations.MemberJoined service event, the only place that knows the
	// center and the account that joined, and preview mutates no durable
	// state. SourceNone documents that this route is deliberately excluded
	// from the request-audit path, not merely unmapped.
	classified("POST", "/api/v1/invitations/preview", KindPublicToken, none()),
	classified("POST", "/api/v1/invitations/accept", KindPublicToken, none()),
	classified("GET", "/public/statements/:token", KindPublicToken, none()),
	classified("GET", "/public/statements/:token/qr.png", KindPublicToken, none()),

	// Authenticated self: own profile, own Zalo link, own center membership.
	classified("GET", "/api/v1/me", KindSelf, none()),
	classified("PUT", "/api/v1/me", KindSelf, req("teacher.profile.update", "teacher", "")),
	classified("GET", "/api/v1/me/zalo", KindSelf, none()),
	classified("DELETE", "/api/v1/me/zalo", KindSelf, req("zalo.unlink", "zalo_account", "")),
	classified("GET", "/api/v1/me/zalo/friends", KindSelf, none()),
	classified("POST", "/api/v1/me/zalo/friends/match", KindSelf, req("zalo.friends.match", "zalo_account", "")),
	classified("POST", "/api/v1/me/zalo/friends/request", KindSelf, req("zalo.friend_request", "zalo_account", "")),
	classified("POST", "/api/v1/me/zalo/link/start", KindSelf, req("zalo.link.start", "zalo_account", "")),
	classified("GET", "/api/v1/me/zalo/link/status", KindSelf, none()),
	classified("GET", "/api/v1/centers/me", KindSelf, none()),

	// Owner-only hard gates: permission administration, staffing/handoff,
	// sensitive review writes, and score-set configuration.
	classified("GET", "/api/v1/centers/me/permissions", KindOwnerOnly, none()),
	classified("PUT", "/api/v1/centers/me/roles/:roleId/permissions", KindOwnerOnly,
		req("center.role.permissions_update", "center_role", "roleId")),
	classified("PUT", "/api/v1/centers/me/members/:teacherId/role", KindOwnerOnly,
		req("center.member.role_update", "teacher", "teacherId")),
	classified("PUT", "/api/v1/centers/me/members/:teacherId/overrides", KindOwnerOnly,
		req("center.member.overrides_update", "teacher", "teacherId")),
	classified("POST", "/api/v1/classes/:id/staff", KindOwnerOnly, req("class.staff.assign", "class", "id")),
	classified("DELETE", "/api/v1/classes/:id/staff/:staffId", KindOwnerOnly,
		req("class.staff.remove", "class_staff", "staffId")),
	classified("PUT", "/api/v1/classes/:id/teacher", KindOwnerOnly, req("class.teacher.reassign", "class", "id")),
	classified("POST", "/api/v1/classes/:id/lesson-plans/:index/approve", KindOwnerOnly,
		req("lesson_plan.approve", "class", "id")),
	classified("POST", "/api/v1/classes/:id/lesson-plans/:index/request-redo", KindOwnerOnly,
		req("lesson_plan.request_redo", "class", "id")),
	classified("POST", "/api/v1/classes/:id/lesson-plans/:index/reopen", KindOwnerOnly,
		req("lesson_plan.reopen", "class", "id")),
	classified("GET", "/api/v1/score-sets", KindOwnerOnly, none()),
	classified("POST", "/api/v1/score-sets", KindOwnerOnly, req("score_set.create", "score_set", "")),
	classified("PUT", "/api/v1/score-sets/:id", KindOwnerOnly, req("score_set.update", "score_set", "id")),
	classified("DELETE", "/api/v1/score-sets/:id", KindOwnerOnly, req("score_set.delete", "score_set", "id")),
	classified("POST", "/api/v1/classes/:id/score-set", KindOwnerOnly, req("class.score_set.assign", "class", "id")),
	classified("DELETE", "/api/v1/classes/:id/score-set", KindOwnerOnly, req("class.score_set.clear", "class", "id")),

	// Center administration.
	perm("PATCH", "/api/v1/centers/me", authctx.PermCenterManage, req("center.rename", "center", "")),
	perm("DELETE", "/api/v1/centers/me/members/:teacherId", authctx.PermMembersManage,
		req("center.member.remove", "teacher", "teacherId")),
	perm("POST", "/api/v1/centers/me/invitations", authctx.PermInvitationsManage,
		req("invitation.create", "invitation", "")),
	perm("GET", "/api/v1/centers/me/invitations", authctx.PermInvitationsManage, none()),
	perm("DELETE", "/api/v1/centers/me/invitations/:id", authctx.PermInvitationsManage,
		req("invitation.revoke", "invitation", "id")),
	perm("GET", "/api/v1/audit-logs", authctx.PermAuditRead, none()),
	perm("GET", "/api/v1/imports/roster/template", authctx.PermImportsRun, none()),
	perm("POST", "/api/v1/imports/roster", authctx.PermImportsRun, req("import.roster", "import", "")),

	// Dashboard: multi-resource aggregate behind one explicit key.
	perm("GET", "/api/v1/centers/dashboard/overview", authctx.PermDashboardView, none()),
	perm("GET", "/api/v1/centers/dashboard/teachers", authctx.PermDashboardView, none()),
	perm("GET", "/api/v1/centers/dashboard/teachers/:teacherId/classes", authctx.PermDashboardView, none()),
	perm("GET", "/api/v1/centers/dashboard/teachers/:teacherId/classes/:classId/sessions",
		authctx.PermDashboardView, none()),
	perm("GET", "/api/v1/centers/dashboard/sessions/:sessionId", authctx.PermDashboardView, none()),

	// Classes and schedules.
	perm("POST", "/api/v1/classes", authctx.PermClassesCreate, req("class.create", "class", "")),
	perm("GET", "/api/v1/classes", authctx.PermClassesList, none()),
	perm("GET", "/api/v1/classes/:id", authctx.PermClassesRead, none()),
	perm("GET", "/api/v1/classes/:id/staff", authctx.PermClassesRead, none()),
	perm("PUT", "/api/v1/classes/:id", authctx.PermClassesEdit, req("class.update", "class", "id")),
	perm("DELETE", "/api/v1/classes/:id", authctx.PermClassesDelete, req("class.delete", "class", "id")),
	perm("POST", "/api/v1/classes/:id/archive", authctx.PermClassesArchive, req("class.archive", "class", "id")),
	perm("POST", "/api/v1/classes/:id/schedules", authctx.PermSchedulesCreate,
		req("class.schedule.create", "class", "id")),
	perm("PUT", "/api/v1/classes/:id/schedules/:scheduleID", authctx.PermSchedulesEdit,
		req("class.schedule.update", "schedule", "scheduleID")),
	perm("DELETE", "/api/v1/classes/:id/schedules/:scheduleID", authctx.PermSchedulesDelete,
		req("class.schedule.delete", "schedule", "scheduleID")),

	// Contacts.
	perm("POST", "/api/v1/contacts", authctx.PermContactsCreate, req("contact.create", "contact", "")),
	perm("GET", "/api/v1/contacts", authctx.PermContactsList, none()),
	perm("GET", "/api/v1/contacts/:id", authctx.PermContactsRead, none()),
	perm("PUT", "/api/v1/contacts/:id", authctx.PermContactsEdit, req("contact.update", "contact", "id")),
	perm("DELETE", "/api/v1/contacts/:id", authctx.PermContactsDelete, req("contact.delete", "contact", "id")),
	perm("PUT", "/api/v1/contacts/:id/zalo-mapping", authctx.PermContactsLinkZalo,
		req("contact.zalo_mapping.set", "contact", "id")),
	perm("DELETE", "/api/v1/contacts/:id/zalo-mapping", authctx.PermContactsLinkZalo,
		req("contact.zalo_mapping.clear", "contact", "id")),

	// Students.
	perm("POST", "/api/v1/students", authctx.PermStudentsCreate, req("student.create", "student", "")),
	perm("GET", "/api/v1/students", authctx.PermStudentsList, none()),
	perm("GET", "/api/v1/students/:id", authctx.PermStudentsRead, none()),
	perm("PUT", "/api/v1/students/:id", authctx.PermStudentsEdit, req("student.update", "student", "id")),
	perm("DELETE", "/api/v1/students/:id", authctx.PermStudentsDelete, req("student.delete", "student", "id")),

	// Enrollments. The picker exists only to create, so it rides the create
	// key. Create is SourceService: the enrollments service publishes
	// StudentEnrolled with the class and student ids, and the subscriber
	// writes that one richer row instead of a plain request row. The list
	// route shares the path but is not a mutation, so it is simply
	// unaudited; the derived per-path skip set already covers it.
	perm("POST", "/api/v1/enrollments", authctx.PermEnrollmentsCreate, Audit{Source: SourceService}),
	perm("GET", "/api/v1/classes/:id/enrollable-students", authctx.PermEnrollmentsCreate, none()),
	perm("GET", "/api/v1/enrollments", authctx.PermEnrollmentsList, none()),
	perm("GET", "/api/v1/enrollments/:id", authctx.PermEnrollmentsRead, none()),
	perm("DELETE", "/api/v1/enrollments/:id", authctx.PermEnrollmentsDelete,
		req("enrollment.delete", "enrollment", "id")),
	perm("POST", "/api/v1/enrollments/:id/end", authctx.PermEnrollmentsEnd, req("enrollment.end", "enrollment", "id")),

	// Sessions. /sessions/pending is a single-resource aggregate on the
	// list key.
	perm("POST", "/api/v1/classes/:id/sessions", authctx.PermSessionsCreate, req("session.create", "class", "id")),
	perm("GET", "/api/v1/classes/:id/sessions", authctx.PermSessionsList, none()),
	perm("GET", "/api/v1/sessions/pending", authctx.PermSessionsList, none()),
	perm("GET", "/api/v1/sessions/:id", authctx.PermSessionsRead, none()),
	perm("DELETE", "/api/v1/sessions/:id", authctx.PermSessionsDelete, req("session.delete", "session", "id")),
	perm("POST", "/api/v1/sessions/:id/cancel", authctx.PermSessionsLifecycle, req("session.cancel", "session", "id")),
	perm("POST", "/api/v1/sessions/:id/uncancel", authctx.PermSessionsLifecycle,
		req("session.uncancel", "session", "id")),
	perm("POST", "/api/v1/sessions/:id/hold", authctx.PermSessionsLifecycle, req("session.hold", "session", "id")),

	// Attendance.
	perm("GET", "/api/v1/sessions/:id/attendance", authctx.PermAttendanceRead, none()),
	perm("POST", "/api/v1/sessions/:id/attendance", authctx.PermAttendanceConfirm,
		req("attendance.confirm", "session", "id")),

	// Scores.
	perm("GET", "/api/v1/sessions/:id/scores", authctx.PermScoresRead, none()),
	perm("GET", "/api/v1/classes/:id/score-components", authctx.PermScoresRead, none()),
	perm("PUT", "/api/v1/sessions/:id/scores", authctx.PermScoresEdit, req("session.scores.update", "session", "id")),

	// Teaching: curriculum, lesson plans, marks, remarks.
	perm("GET", "/api/v1/classes/:id/curriculum", authctx.PermTeachingRead, none()),
	perm("GET", "/api/v1/classes/:id/lesson-plans", authctx.PermTeachingRead, none()),
	perm("GET", "/api/v1/classes/:id/marks", authctx.PermTeachingRead, none()),
	perm("PUT", "/api/v1/classes/:id/curriculum", authctx.PermTeachingEdit, req("curriculum.update", "class", "id")),
	perm("PUT", "/api/v1/classes/:id/lesson-plans/:index", authctx.PermTeachingEdit,
		req("lesson_plan.save", "class", "id")),
	perm("POST", "/api/v1/classes/:id/lesson-plans/:index/submit", authctx.PermTeachingEdit,
		req("lesson_plan.submit", "class", "id")),
	perm("PUT", "/api/v1/sessions/:id/note", authctx.PermTeachingEdit, req("session.note.update", "session", "id")),
	perm("PUT", "/api/v1/sessions/:id/marks", authctx.PermTeachingEdit, req("session.marks.update", "session", "id")),
	perm("GET", "/api/v1/teaching/review-queue", authctx.PermTeachingReviewQueue, none()),

	// Billing.
	perm("POST", "/api/v1/billing-periods", authctx.PermBillingCreate,
		req("billing.period.create", "billing_period", "")),
	perm("GET", "/api/v1/billing-periods", authctx.PermBillingList, none()),
	perm("GET", "/api/v1/billing-periods/:id", authctx.PermBillingRead, none()),
	perm("GET", "/api/v1/billing-periods/:id/preview", authctx.PermBillingRead, none()),
	perm("GET", "/api/v1/billing-periods/:id/collections", authctx.PermBillingRead, none()),
	perm("GET", "/api/v1/billing-periods/:id/collections/summary", authctx.PermBillingRead, none()),
	perm("GET", "/api/v1/invoices/:id/adjustments", authctx.PermBillingRead, none()),
	perm("POST", "/api/v1/billing-periods/:id/draft", authctx.PermBillingDraft,
		req("billing.period.draft", "billing_period", "id")),
	perm("POST", "/api/v1/billing-periods/:id/close", authctx.PermBillingClose,
		req("billing.period.close", "billing_period", "id")),
	perm("POST", "/api/v1/invoices/:id/void", authctx.PermBillingVoidInvoice,
		req("billing.invoice.void", "invoice", "id")),
	perm("POST", "/api/v1/invoices/:id/adjustments", authctx.PermBillingAdjustInvoice,
		req("billing.adjustment.create", "invoice", "id")),

	// Payments.
	perm("POST", "/api/v1/payments", authctx.PermPaymentsCreate, req("payment.create", "payment", "")),
	perm("GET", "/api/v1/payments", authctx.PermPaymentsList, none()),
	perm("GET", "/api/v1/payments/:id", authctx.PermPaymentsRead, none()),
	perm("PUT", "/api/v1/payments/:id/allocations", authctx.PermPaymentsAllocate,
		req("payment.reallocate", "payment", "id")),
	perm("POST", "/api/v1/payments/:id/allocations/auto", authctx.PermPaymentsAllocate,
		req("payment.allocate_auto", "payment", "id")),
	perm("POST", "/api/v1/payments/:id/reverse", authctx.PermPaymentsReverse, req("payment.reverse", "payment", "id")),

	// Statements.
	perm("GET", "/api/v1/billing-periods/:id/statements", authctx.PermStatementsList, none()),
	perm("GET", "/api/v1/statements/:id", authctx.PermStatementsRead, none()),
	perm("POST", "/api/v1/billing-periods/:id/statements/generate", authctx.PermStatementsGenerate,
		req("statement.generate", "billing_period", "id")),
	perm("POST", "/api/v1/statements/:id/revoke", authctx.PermStatementsRevoke,
		req("statement.revoke", "statement", "id")),

	// Reports: frozen legacy oversight axis. ReportsOversight OR class
	// hoc_vu authorization stays inside the service — no one catalog key
	// covers the allowed callers (a reports.send route gate would deny the
	// class secretary's send and the period owner's own ledger read before
	// the service's gates could admit them).
	classified("POST", "/api/v1/billing-periods/:id/notifications/bulk", KindService,
		req("notification.bulk_send", "billing_period", "id")),
	classified("GET", "/api/v1/billing-periods/:id/notifications", KindService, none()),
	classified("GET", "/api/v1/billing-periods/:id/notifications/preview", KindService, none()),
	classified("GET", "/api/v1/billing-periods/:id/notifications/run", KindService, none()),
	classified("POST", "/api/v1/billing-periods/:id/notifications/run/resume", KindService,
		req("notification.run.resume", "billing_period", "id")),
	perm("POST", "/api/v1/notifications/mark-sent", authctx.PermNotificationsMarkSent,
		req("notification.mark_sent", "notification", "")),
}

// Policies returns the manifest for the server package's route-policy
// enforcement middleware. Callers must not mutate the returned slice's
// backing array in place (it aliases Specs' element values by copy, so
// mutation is actually safe per-element, but treat it as read-only for the
// same reason the rest of this package's data is: it is process-wide,
// shared, frozen configuration).
func Policies() []Spec {
	out := make([]Spec, len(Specs))
	copy(out, Specs)
	return out
}

// IsMutating reports whether an HTTP method changes state. It is the one
// definition the request-audit middleware and the manifest tests share, so
// "which routes need an audit source" cannot drift from "which requests the
// middleware publishes".
func IsMutating(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

// specIndex is built once from Specs for Lookup.
var specIndex = func() map[string]Spec {
	m := make(map[string]Spec, len(Specs))
	for _, s := range Specs {
		m[s.Method+" "+s.Path] = s
	}
	return m
}()

// Lookup resolves a route's Spec by method and gin route template (e.g.
// "/api/v1/classes/:id").
func Lookup(method, path string) (Spec, bool) {
	s, ok := specIndex[method+" "+path]
	return s, ok
}

// WithSource returns the set of paths carrying at least one Spec with audit
// source s, keyed by path only (not method): the request middleware's skip
// checks match on c.FullPath() alone, so a path carrying a skip source must
// not also carry an audited (SourceRequest) Spec, and mutating Specs sharing
// a path must agree on Source — the manifest tests enforce both, so the
// derived set means the same thing the original per-path maps meant.
func WithSource(s AuditSource) map[string]struct{} {
	out := map[string]struct{}{}
	for _, spec := range Specs {
		if spec.Audit.Source == s {
			out[spec.Path] = struct{}{}
		}
	}
	return out
}
