package server

import "teka/apps/api/internal/shared/authctx"

// PolicyKind classifies how a route is authorized. Every registered route
// carries exactly one intentional classification; the manifest test fails
// closed on any route added without one.
type PolicyKind string

const (
	// PolicyPublic routes need no session: none exists yet (login, password
	// reset) or the route is infrastructure (health, swagger).
	PolicyPublic PolicyKind = "public"
	// PolicyPublicToken routes are unauthenticated but gated on an
	// unguessable token in the URL or body (public statements, invitations).
	PolicyPublicToken PolicyKind = "public_token"
	// PolicySelf routes serve an authenticated caller acting on their own
	// account/center membership; no center permission involved.
	PolicySelf PolicyKind = "self"
	// PolicyOwnerOnly routes are hard-gated on ownership, never grantable —
	// one grant away from escalation otherwise.
	PolicyOwnerOnly PolicyKind = "owner_only"
	// PolicyPermission routes are gated on one grantable catalog key.
	// Tenant scope, object visibility, and class-capability checks remain
	// independent layers behind the permission.
	PolicyPermission PolicyKind = "permission"
)

// RoutePolicy is one route's frozen authorization classification. Method and
// Path match gin's registration exactly. Key is set only for
// PolicyPermission.
type RoutePolicy struct {
	Method string
	Path   string
	Kind   PolicyKind
	Key    string
}

func perm(method, path, key string) RoutePolicy {
	return RoutePolicy{Method: method, Path: path, Kind: PolicyPermission, Key: key}
}

func classified(method, path string, kind PolicyKind) RoutePolicy {
	return RoutePolicy{Method: method, Path: path, Kind: kind}
}

// routePolicies is the phase-1 inventory frozen as data: the single manifest
// the coverage test compares bidirectionally against engine.Routes(). The
// permission keys name the target policy of each route; enforcement moves
// onto them wave by wave during the endpoint cutover.
var routePolicies = []RoutePolicy{
	// Infrastructure and unauthenticated entry points.
	classified("GET", "/healthz", PolicyPublic),
	classified("GET", "/readyz", PolicyPublic),
	classified("GET", "/swagger/*any", PolicyPublic),
	classified("POST", "/api/v1/auth/login", PolicyPublic),
	classified("POST", "/api/v1/auth/refresh", PolicyPublic),
	classified("POST", "/api/v1/auth/logout", PolicyPublic),
	classified("POST", "/api/v1/auth/forgot-password", PolicyPublic),
	classified("POST", "/api/v1/auth/reset-password", PolicyPublic),
	classified("POST", "/api/v1/invitations/preview", PolicyPublicToken),
	classified("POST", "/api/v1/invitations/accept", PolicyPublicToken),
	classified("GET", "/public/statements/:token", PolicyPublicToken),
	classified("GET", "/public/statements/:token/qr.png", PolicyPublicToken),

	// Authenticated self: own profile, own Zalo link, own center membership.
	classified("GET", "/api/v1/me", PolicySelf),
	classified("PUT", "/api/v1/me", PolicySelf),
	classified("GET", "/api/v1/me/zalo", PolicySelf),
	classified("DELETE", "/api/v1/me/zalo", PolicySelf),
	classified("GET", "/api/v1/me/zalo/friends", PolicySelf),
	classified("POST", "/api/v1/me/zalo/friends/match", PolicySelf),
	classified("POST", "/api/v1/me/zalo/friends/request", PolicySelf),
	classified("POST", "/api/v1/me/zalo/link/start", PolicySelf),
	classified("GET", "/api/v1/me/zalo/link/status", PolicySelf),
	classified("GET", "/api/v1/centers/me", PolicySelf),

	// Owner-only hard gates: permission administration, the legacy
	// send-reports toggle, staffing/handoff, sensitive review writes, and
	// score-set configuration.
	classified("GET", "/api/v1/centers/me/permissions", PolicyOwnerOnly),
	classified("PUT", "/api/v1/centers/me/roles/:roleId/permissions", PolicyOwnerOnly),
	classified("PUT", "/api/v1/centers/me/members/:teacherId/role", PolicyOwnerOnly),
	classified("PUT", "/api/v1/centers/me/members/:teacherId/overrides", PolicyOwnerOnly),
	classified("POST", "/api/v1/centers/me/members/:teacherId/send-reports", PolicyOwnerOnly),
	classified("DELETE", "/api/v1/centers/me/members/:teacherId/send-reports", PolicyOwnerOnly),
	classified("POST", "/api/v1/classes/:id/staff", PolicyOwnerOnly),
	classified("DELETE", "/api/v1/classes/:id/staff/:staffId", PolicyOwnerOnly),
	classified("PUT", "/api/v1/classes/:id/teacher", PolicyOwnerOnly),
	classified("POST", "/api/v1/classes/:id/lesson-plans/:index/approve", PolicyOwnerOnly),
	classified("POST", "/api/v1/classes/:id/lesson-plans/:index/request-redo", PolicyOwnerOnly),
	classified("POST", "/api/v1/classes/:id/lesson-plans/:index/reopen", PolicyOwnerOnly),
	classified("GET", "/api/v1/score-sets", PolicyOwnerOnly),
	classified("POST", "/api/v1/score-sets", PolicyOwnerOnly),
	classified("PUT", "/api/v1/score-sets/:id", PolicyOwnerOnly),
	classified("DELETE", "/api/v1/score-sets/:id", PolicyOwnerOnly),
	classified("POST", "/api/v1/classes/:id/score-set", PolicyOwnerOnly),
	classified("DELETE", "/api/v1/classes/:id/score-set", PolicyOwnerOnly),

	// Center administration.
	perm("PATCH", "/api/v1/centers/me", authctx.PermCenterManage),
	perm("DELETE", "/api/v1/centers/me/members/:teacherId", authctx.PermMembersManage),
	perm("POST", "/api/v1/centers/me/invitations", authctx.PermInvitationsManage),
	perm("GET", "/api/v1/centers/me/invitations", authctx.PermInvitationsManage),
	perm("DELETE", "/api/v1/centers/me/invitations/:id", authctx.PermInvitationsManage),
	perm("GET", "/api/v1/audit-logs", authctx.PermAuditRead),
	perm("GET", "/api/v1/imports/roster/template", authctx.PermImportsRun),
	perm("POST", "/api/v1/imports/roster", authctx.PermImportsRun),

	// Dashboard: multi-resource aggregate behind one explicit key.
	perm("GET", "/api/v1/centers/dashboard/overview", authctx.PermDashboardView),
	perm("GET", "/api/v1/centers/dashboard/teachers", authctx.PermDashboardView),
	perm("GET", "/api/v1/centers/dashboard/teachers/:teacherId/classes", authctx.PermDashboardView),
	perm("GET", "/api/v1/centers/dashboard/teachers/:teacherId/classes/:classId/sessions", authctx.PermDashboardView),
	perm("GET", "/api/v1/centers/dashboard/sessions/:sessionId", authctx.PermDashboardView),

	// Classes and schedules.
	perm("POST", "/api/v1/classes", authctx.PermClassesCreate),
	perm("GET", "/api/v1/classes", authctx.PermClassesList),
	perm("GET", "/api/v1/classes/:id", authctx.PermClassesRead),
	perm("GET", "/api/v1/classes/:id/staff", authctx.PermClassesRead),
	perm("PUT", "/api/v1/classes/:id", authctx.PermClassesEdit),
	perm("DELETE", "/api/v1/classes/:id", authctx.PermClassesDelete),
	perm("POST", "/api/v1/classes/:id/archive", authctx.PermClassesArchive),
	perm("POST", "/api/v1/classes/:id/schedules", authctx.PermSchedulesCreate),
	perm("PUT", "/api/v1/classes/:id/schedules/:scheduleID", authctx.PermSchedulesEdit),
	perm("DELETE", "/api/v1/classes/:id/schedules/:scheduleID", authctx.PermSchedulesDelete),

	// Contacts.
	perm("POST", "/api/v1/contacts", authctx.PermContactsCreate),
	perm("GET", "/api/v1/contacts", authctx.PermContactsList),
	perm("GET", "/api/v1/contacts/:id", authctx.PermContactsRead),
	perm("PUT", "/api/v1/contacts/:id", authctx.PermContactsEdit),
	perm("DELETE", "/api/v1/contacts/:id", authctx.PermContactsDelete),
	perm("PUT", "/api/v1/contacts/:id/zalo-mapping", authctx.PermContactsLinkZalo),
	perm("DELETE", "/api/v1/contacts/:id/zalo-mapping", authctx.PermContactsLinkZalo),

	// Students.
	perm("POST", "/api/v1/students", authctx.PermStudentsCreate),
	perm("GET", "/api/v1/students", authctx.PermStudentsList),
	perm("GET", "/api/v1/students/:id", authctx.PermStudentsRead),
	perm("PUT", "/api/v1/students/:id", authctx.PermStudentsEdit),
	perm("DELETE", "/api/v1/students/:id", authctx.PermStudentsDelete),

	// Enrollments. The picker exists only to create — it rides the create
	// key by the phase-1 irregular-mapping decision.
	perm("POST", "/api/v1/enrollments", authctx.PermEnrollmentsCreate),
	perm("GET", "/api/v1/classes/:id/enrollable-students", authctx.PermEnrollmentsCreate),
	perm("GET", "/api/v1/enrollments", authctx.PermEnrollmentsList),
	perm("GET", "/api/v1/enrollments/:id", authctx.PermEnrollmentsRead),
	perm("DELETE", "/api/v1/enrollments/:id", authctx.PermEnrollmentsDelete),
	perm("POST", "/api/v1/enrollments/:id/end", authctx.PermEnrollmentsEnd),

	// Sessions. /sessions/pending is a single-resource aggregate on the
	// list key.
	perm("POST", "/api/v1/classes/:id/sessions", authctx.PermSessionsCreate),
	perm("GET", "/api/v1/classes/:id/sessions", authctx.PermSessionsList),
	perm("GET", "/api/v1/sessions/pending", authctx.PermSessionsList),
	perm("GET", "/api/v1/sessions/:id", authctx.PermSessionsRead),
	perm("DELETE", "/api/v1/sessions/:id", authctx.PermSessionsDelete),
	perm("POST", "/api/v1/sessions/:id/cancel", authctx.PermSessionsLifecycle),
	perm("POST", "/api/v1/sessions/:id/uncancel", authctx.PermSessionsLifecycle),
	perm("POST", "/api/v1/sessions/:id/hold", authctx.PermSessionsLifecycle),

	// Attendance.
	perm("GET", "/api/v1/sessions/:id/attendance", authctx.PermAttendanceRead),
	perm("POST", "/api/v1/sessions/:id/attendance", authctx.PermAttendanceConfirm),

	// Scores.
	perm("GET", "/api/v1/sessions/:id/scores", authctx.PermScoresRead),
	perm("GET", "/api/v1/classes/:id/score-components", authctx.PermScoresRead),
	perm("PUT", "/api/v1/sessions/:id/scores", authctx.PermScoresEdit),

	// Teaching: curriculum, lesson plans, marks, remarks.
	perm("GET", "/api/v1/classes/:id/curriculum", authctx.PermTeachingRead),
	perm("GET", "/api/v1/classes/:id/lesson-plans", authctx.PermTeachingRead),
	perm("GET", "/api/v1/classes/:id/marks", authctx.PermTeachingRead),
	perm("PUT", "/api/v1/classes/:id/curriculum", authctx.PermTeachingEdit),
	perm("PUT", "/api/v1/classes/:id/lesson-plans/:index", authctx.PermTeachingEdit),
	perm("POST", "/api/v1/classes/:id/lesson-plans/:index/submit", authctx.PermTeachingEdit),
	perm("PUT", "/api/v1/sessions/:id/note", authctx.PermTeachingEdit),
	perm("PUT", "/api/v1/sessions/:id/marks", authctx.PermTeachingEdit),
	perm("GET", "/api/v1/teaching/review-queue", authctx.PermTeachingReviewQueue),

	// Billing.
	perm("POST", "/api/v1/billing-periods", authctx.PermBillingCreate),
	perm("GET", "/api/v1/billing-periods", authctx.PermBillingList),
	perm("GET", "/api/v1/billing-periods/:id", authctx.PermBillingRead),
	perm("GET", "/api/v1/billing-periods/:id/preview", authctx.PermBillingRead),
	perm("GET", "/api/v1/billing-periods/:id/collections", authctx.PermBillingRead),
	perm("GET", "/api/v1/billing-periods/:id/collections/summary", authctx.PermBillingRead),
	perm("GET", "/api/v1/invoices/:id/adjustments", authctx.PermBillingRead),
	perm("POST", "/api/v1/billing-periods/:id/draft", authctx.PermBillingDraft),
	perm("POST", "/api/v1/billing-periods/:id/close", authctx.PermBillingClose),
	perm("POST", "/api/v1/invoices/:id/void", authctx.PermBillingVoidInvoice),
	perm("POST", "/api/v1/invoices/:id/adjustments", authctx.PermBillingAdjustInvoice),

	// Payments.
	perm("POST", "/api/v1/payments", authctx.PermPaymentsCreate),
	perm("GET", "/api/v1/payments", authctx.PermPaymentsList),
	perm("GET", "/api/v1/payments/:id", authctx.PermPaymentsRead),
	perm("PUT", "/api/v1/payments/:id/allocations", authctx.PermPaymentsAllocate),
	perm("POST", "/api/v1/payments/:id/allocations/auto", authctx.PermPaymentsAllocate),
	perm("POST", "/api/v1/payments/:id/reverse", authctx.PermPaymentsReverse),

	// Statements.
	perm("GET", "/api/v1/billing-periods/:id/statements", authctx.PermStatementsList),
	perm("GET", "/api/v1/statements/:id", authctx.PermStatementsRead),
	perm("POST", "/api/v1/billing-periods/:id/statements/generate", authctx.PermStatementsGenerate),
	perm("POST", "/api/v1/statements/:id/revoke", authctx.PermStatementsRevoke),

	// Reports: frozen legacy oversight axis (ReportsOversight OR class
	// hoc_vu authorization stays inside the service).
	perm("POST", "/api/v1/billing-periods/:id/notifications/bulk", authctx.PermReportsSend),
	perm("GET", "/api/v1/billing-periods/:id/notifications", authctx.PermReportsSend),
	perm("GET", "/api/v1/billing-periods/:id/notifications/preview", authctx.PermReportsSend),
	perm("GET", "/api/v1/billing-periods/:id/notifications/run", authctx.PermReportsSend),
	perm("POST", "/api/v1/billing-periods/:id/notifications/run/resume", authctx.PermReportsSend),
	perm("POST", "/api/v1/notifications/mark-sent", authctx.PermNotificationsMarkSent),
}
