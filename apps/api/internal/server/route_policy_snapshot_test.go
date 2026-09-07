package server

import "testing"

// routePolicySnapshot pins every routePolicies entry as it stood before
// routePolicies was rebuilt from the shared route manifest. If this test ever
// needs to change, the change must be justified by an intentional
// authorization decision, not by a refactor accidentally reshuffling a route's
// classification or permission key.
var routePolicySnapshot = []RoutePolicy{
	{Method: "GET", Path: "/healthz", Kind: PolicyPublic, Key: ""},
	{Method: "GET", Path: "/readyz", Kind: PolicyPublic, Key: ""},
	{Method: "GET", Path: "/swagger/*any", Kind: PolicyPublic, Key: ""},
	{Method: "POST", Path: "/api/v1/auth/login", Kind: PolicyPublic, Key: ""},
	{Method: "POST", Path: "/api/v1/auth/refresh", Kind: PolicyPublic, Key: ""},
	{Method: "POST", Path: "/api/v1/auth/logout", Kind: PolicyPublic, Key: ""},
	{Method: "POST", Path: "/api/v1/auth/forgot-password", Kind: PolicyPublic, Key: ""},
	{Method: "POST", Path: "/api/v1/auth/reset-password", Kind: PolicyPublic, Key: ""},
	{Method: "POST", Path: "/api/v1/invitations/preview", Kind: PolicyPublicToken, Key: ""},
	{Method: "POST", Path: "/api/v1/invitations/accept", Kind: PolicyPublicToken, Key: ""},
	{Method: "GET", Path: "/public/statements/:token", Kind: PolicyPublicToken, Key: ""},
	{Method: "GET", Path: "/public/statements/:token/qr.png", Kind: PolicyPublicToken, Key: ""},
	{Method: "GET", Path: "/api/v1/me", Kind: PolicySelf, Key: ""},
	{Method: "PUT", Path: "/api/v1/me", Kind: PolicySelf, Key: ""},
	{Method: "GET", Path: "/api/v1/me/zalo", Kind: PolicySelf, Key: ""},
	{Method: "DELETE", Path: "/api/v1/me/zalo", Kind: PolicySelf, Key: ""},
	{Method: "GET", Path: "/api/v1/me/zalo/friends", Kind: PolicySelf, Key: ""},
	{Method: "POST", Path: "/api/v1/me/zalo/friends/match", Kind: PolicySelf, Key: ""},
	{Method: "POST", Path: "/api/v1/me/zalo/friends/request", Kind: PolicySelf, Key: ""},
	{Method: "POST", Path: "/api/v1/me/zalo/link/start", Kind: PolicySelf, Key: ""},
	{Method: "GET", Path: "/api/v1/me/zalo/link/status", Kind: PolicySelf, Key: ""},
	{Method: "GET", Path: "/api/v1/centers/me", Kind: PolicySelf, Key: ""},
	{Method: "GET", Path: "/api/v1/centers/me/permissions", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "PUT", Path: "/api/v1/centers/me/roles/:roleId/permissions", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "PUT", Path: "/api/v1/centers/me/members/:teacherId/role", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "PUT", Path: "/api/v1/centers/me/members/:teacherId/overrides", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "POST", Path: "/api/v1/classes/:id/staff", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "DELETE", Path: "/api/v1/classes/:id/staff/:staffId", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "PUT", Path: "/api/v1/classes/:id/teacher", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "POST", Path: "/api/v1/classes/:id/lesson-plans/:index/approve", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "POST", Path: "/api/v1/classes/:id/lesson-plans/:index/request-redo", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "POST", Path: "/api/v1/classes/:id/lesson-plans/:index/reopen", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "GET", Path: "/api/v1/score-sets", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "POST", Path: "/api/v1/score-sets", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "PUT", Path: "/api/v1/score-sets/:id", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "DELETE", Path: "/api/v1/score-sets/:id", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "POST", Path: "/api/v1/classes/:id/score-set", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "DELETE", Path: "/api/v1/classes/:id/score-set", Kind: PolicyOwnerOnly, Key: ""},
	{Method: "PATCH", Path: "/api/v1/centers/me", Kind: PolicyPermission, Key: "center.manage"},
	{Method: "DELETE", Path: "/api/v1/centers/me/members/:teacherId", Kind: PolicyPermission, Key: "members.manage"},
	{Method: "POST", Path: "/api/v1/centers/me/invitations", Kind: PolicyPermission, Key: "invitations.manage"},
	{Method: "GET", Path: "/api/v1/centers/me/invitations", Kind: PolicyPermission, Key: "invitations.manage"},
	{Method: "DELETE", Path: "/api/v1/centers/me/invitations/:id", Kind: PolicyPermission, Key: "invitations.manage"},
	{Method: "GET", Path: "/api/v1/audit-logs", Kind: PolicyPermission, Key: "audit.read"},
	{Method: "GET", Path: "/api/v1/imports/roster/template", Kind: PolicyPermission, Key: "imports.run"},
	{Method: "POST", Path: "/api/v1/imports/roster", Kind: PolicyPermission, Key: "imports.run"},
	{Method: "GET", Path: "/api/v1/centers/dashboard/overview", Kind: PolicyPermission, Key: "dashboard.view"},
	{Method: "GET", Path: "/api/v1/centers/dashboard/teachers", Kind: PolicyPermission, Key: "dashboard.view"},
	{Method: "GET", Path: "/api/v1/centers/dashboard/teachers/:teacherId/classes", Kind: PolicyPermission, Key: "dashboard.view"},
	{Method: "GET", Path: "/api/v1/centers/dashboard/teachers/:teacherId/classes/:classId/sessions", Kind: PolicyPermission, Key: "dashboard.view"},
	{Method: "GET", Path: "/api/v1/centers/dashboard/sessions/:sessionId", Kind: PolicyPermission, Key: "dashboard.view"},
	{Method: "POST", Path: "/api/v1/classes", Kind: PolicyPermission, Key: "classes.create"},
	{Method: "GET", Path: "/api/v1/classes", Kind: PolicyPermission, Key: "classes.list"},
	{Method: "GET", Path: "/api/v1/classes/:id", Kind: PolicyPermission, Key: "classes.read"},
	{Method: "GET", Path: "/api/v1/classes/:id/staff", Kind: PolicyPermission, Key: "classes.read"},
	{Method: "PUT", Path: "/api/v1/classes/:id", Kind: PolicyPermission, Key: "classes.edit"},
	{Method: "DELETE", Path: "/api/v1/classes/:id", Kind: PolicyPermission, Key: "classes.delete"},
	{Method: "POST", Path: "/api/v1/classes/:id/archive", Kind: PolicyPermission, Key: "classes.archive"},
	{Method: "POST", Path: "/api/v1/classes/:id/schedules", Kind: PolicyPermission, Key: "schedules.create"},
	{Method: "PUT", Path: "/api/v1/classes/:id/schedules/:scheduleID", Kind: PolicyPermission, Key: "schedules.edit"},
	{Method: "DELETE", Path: "/api/v1/classes/:id/schedules/:scheduleID", Kind: PolicyPermission, Key: "schedules.delete"},
	{Method: "POST", Path: "/api/v1/contacts", Kind: PolicyPermission, Key: "contacts.create"},
	{Method: "GET", Path: "/api/v1/contacts", Kind: PolicyPermission, Key: "contacts.list"},
	{Method: "GET", Path: "/api/v1/contacts/:id", Kind: PolicyPermission, Key: "contacts.read"},
	{Method: "PUT", Path: "/api/v1/contacts/:id", Kind: PolicyPermission, Key: "contacts.edit"},
	{Method: "DELETE", Path: "/api/v1/contacts/:id", Kind: PolicyPermission, Key: "contacts.delete"},
	{Method: "PUT", Path: "/api/v1/contacts/:id/zalo-mapping", Kind: PolicyPermission, Key: "contacts.link_zalo"},
	{Method: "DELETE", Path: "/api/v1/contacts/:id/zalo-mapping", Kind: PolicyPermission, Key: "contacts.link_zalo"},
	{Method: "POST", Path: "/api/v1/students", Kind: PolicyPermission, Key: "students.create"},
	{Method: "GET", Path: "/api/v1/students", Kind: PolicyPermission, Key: "students.list"},
	{Method: "GET", Path: "/api/v1/students/:id", Kind: PolicyPermission, Key: "students.read"},
	{Method: "PUT", Path: "/api/v1/students/:id", Kind: PolicyPermission, Key: "students.edit"},
	{Method: "DELETE", Path: "/api/v1/students/:id", Kind: PolicyPermission, Key: "students.delete"},
	{Method: "POST", Path: "/api/v1/enrollments", Kind: PolicyPermission, Key: "enrollments.create"},
	{Method: "GET", Path: "/api/v1/classes/:id/enrollable-students", Kind: PolicyPermission, Key: "enrollments.create"},
	{Method: "GET", Path: "/api/v1/enrollments", Kind: PolicyPermission, Key: "enrollments.list"},
	{Method: "GET", Path: "/api/v1/enrollments/:id", Kind: PolicyPermission, Key: "enrollments.read"},
	{Method: "DELETE", Path: "/api/v1/enrollments/:id", Kind: PolicyPermission, Key: "enrollments.delete"},
	{Method: "POST", Path: "/api/v1/enrollments/:id/end", Kind: PolicyPermission, Key: "enrollments.end"},
	{Method: "POST", Path: "/api/v1/classes/:id/sessions", Kind: PolicyPermission, Key: "sessions.create"},
	{Method: "GET", Path: "/api/v1/classes/:id/sessions", Kind: PolicyPermission, Key: "sessions.list"},
	{Method: "GET", Path: "/api/v1/sessions/pending", Kind: PolicyPermission, Key: "sessions.list"},
	{Method: "GET", Path: "/api/v1/sessions/:id", Kind: PolicyPermission, Key: "sessions.read"},
	{Method: "DELETE", Path: "/api/v1/sessions/:id", Kind: PolicyPermission, Key: "sessions.delete"},
	{Method: "POST", Path: "/api/v1/sessions/:id/cancel", Kind: PolicyPermission, Key: "sessions.lifecycle"},
	{Method: "POST", Path: "/api/v1/sessions/:id/uncancel", Kind: PolicyPermission, Key: "sessions.lifecycle"},
	{Method: "POST", Path: "/api/v1/sessions/:id/hold", Kind: PolicyPermission, Key: "sessions.lifecycle"},
	{Method: "GET", Path: "/api/v1/sessions/:id/attendance", Kind: PolicyPermission, Key: "attendance.read"},
	{Method: "POST", Path: "/api/v1/sessions/:id/attendance", Kind: PolicyPermission, Key: "attendance.confirm"},
	{Method: "GET", Path: "/api/v1/sessions/:id/scores", Kind: PolicyPermission, Key: "scores.read"},
	{Method: "GET", Path: "/api/v1/classes/:id/score-components", Kind: PolicyPermission, Key: "scores.read"},
	{Method: "PUT", Path: "/api/v1/sessions/:id/scores", Kind: PolicyPermission, Key: "scores.edit"},
	{Method: "GET", Path: "/api/v1/classes/:id/curriculum", Kind: PolicyPermission, Key: "teaching.read"},
	{Method: "GET", Path: "/api/v1/classes/:id/lesson-plans", Kind: PolicyPermission, Key: "teaching.read"},
	{Method: "GET", Path: "/api/v1/classes/:id/marks", Kind: PolicyPermission, Key: "teaching.read"},
	{Method: "PUT", Path: "/api/v1/classes/:id/curriculum", Kind: PolicyPermission, Key: "teaching.edit"},
	{Method: "PUT", Path: "/api/v1/classes/:id/lesson-plans/:index", Kind: PolicyPermission, Key: "teaching.edit"},
	{Method: "POST", Path: "/api/v1/classes/:id/lesson-plans/:index/submit", Kind: PolicyPermission, Key: "teaching.edit"},
	{Method: "PUT", Path: "/api/v1/sessions/:id/note", Kind: PolicyPermission, Key: "teaching.edit"},
	{Method: "PUT", Path: "/api/v1/sessions/:id/marks", Kind: PolicyPermission, Key: "teaching.edit"},
	{Method: "GET", Path: "/api/v1/teaching/review-queue", Kind: PolicyPermission, Key: "teaching.review_queue"},
	{Method: "POST", Path: "/api/v1/billing-periods", Kind: PolicyPermission, Key: "billing.create"},
	{Method: "GET", Path: "/api/v1/billing-periods", Kind: PolicyPermission, Key: "billing.list"},
	{Method: "GET", Path: "/api/v1/billing-periods/:id", Kind: PolicyPermission, Key: "billing.read"},
	{Method: "GET", Path: "/api/v1/billing-periods/:id/preview", Kind: PolicyPermission, Key: "billing.read"},
	{Method: "GET", Path: "/api/v1/billing-periods/:id/collections", Kind: PolicyPermission, Key: "billing.read"},
	{Method: "GET", Path: "/api/v1/billing-periods/:id/collections/summary", Kind: PolicyPermission, Key: "billing.read"},
	{Method: "GET", Path: "/api/v1/invoices/:id/adjustments", Kind: PolicyPermission, Key: "billing.read"},
	{Method: "POST", Path: "/api/v1/billing-periods/:id/draft", Kind: PolicyPermission, Key: "billing.draft"},
	{Method: "POST", Path: "/api/v1/billing-periods/:id/close", Kind: PolicyPermission, Key: "billing.close"},
	{Method: "POST", Path: "/api/v1/invoices/:id/void", Kind: PolicyPermission, Key: "billing.void_invoice"},
	{Method: "POST", Path: "/api/v1/invoices/:id/adjustments", Kind: PolicyPermission, Key: "billing.adjust_invoice"},
	{Method: "POST", Path: "/api/v1/payments", Kind: PolicyPermission, Key: "payments.create"},
	{Method: "GET", Path: "/api/v1/payments", Kind: PolicyPermission, Key: "payments.list"},
	{Method: "GET", Path: "/api/v1/payments/:id", Kind: PolicyPermission, Key: "payments.read"},
	{Method: "PUT", Path: "/api/v1/payments/:id/allocations", Kind: PolicyPermission, Key: "payments.allocate"},
	{Method: "POST", Path: "/api/v1/payments/:id/allocations/auto", Kind: PolicyPermission, Key: "payments.allocate"},
	{Method: "POST", Path: "/api/v1/payments/:id/reverse", Kind: PolicyPermission, Key: "payments.reverse"},
	{Method: "GET", Path: "/api/v1/billing-periods/:id/statements", Kind: PolicyPermission, Key: "statements.list"},
	{Method: "GET", Path: "/api/v1/statements/:id", Kind: PolicyPermission, Key: "statements.read"},
	{Method: "POST", Path: "/api/v1/billing-periods/:id/statements/generate", Kind: PolicyPermission, Key: "statements.generate"},
	{Method: "POST", Path: "/api/v1/statements/:id/revoke", Kind: PolicyPermission, Key: "statements.revoke"},
	{Method: "POST", Path: "/api/v1/billing-periods/:id/notifications/bulk", Kind: PolicyService, Key: ""},
	{Method: "GET", Path: "/api/v1/billing-periods/:id/notifications", Kind: PolicyService, Key: ""},
	{Method: "GET", Path: "/api/v1/billing-periods/:id/notifications/preview", Kind: PolicyService, Key: ""},
	{Method: "GET", Path: "/api/v1/billing-periods/:id/notifications/run", Kind: PolicyService, Key: ""},
	{Method: "POST", Path: "/api/v1/billing-periods/:id/notifications/run/resume", Kind: PolicyService, Key: ""},
	{Method: "POST", Path: "/api/v1/notifications/mark-sent", Kind: PolicyPermission, Key: "notifications.mark_sent"},
}

// TestRoutePolicySnapshotUnchanged proves routePolicies still classifies
// every route exactly as it did before the manifest migration: same Kind,
// same permission Key, nothing added or dropped. It compares by (method,
// path) rather than position, since reordering routePolicies changes nothing
// about runtime behavior.
func TestRoutePolicySnapshotUnchanged(t *testing.T) {
	if len(routePolicies) != len(routePolicySnapshot) {
		t.Fatalf("routePolicies has %d entries, snapshot has %d", len(routePolicies), len(routePolicySnapshot))
	}
	want := make(map[string]RoutePolicy, len(routePolicySnapshot))
	for _, p := range routePolicySnapshot {
		want[p.Method+" "+p.Path] = p
	}
	seen := make(map[string]bool, len(routePolicies))
	for _, got := range routePolicies {
		id := got.Method + " " + got.Path
		if seen[id] {
			t.Errorf("%s appears more than once in routePolicies", id)
			continue
		}
		seen[id] = true
		w, ok := want[id]
		if !ok {
			t.Errorf("%s is in routePolicies but not in the snapshot", id)
			continue
		}
		if got != w {
			t.Errorf("%s = %+v, snapshot wants %+v", id, got, w)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("%s is in the snapshot but missing from routePolicies", id)
		}
	}
}
