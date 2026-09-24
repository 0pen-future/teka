package audit

import (
	"testing"

	"teka/apps/api/internal/shared/routespec"
)

// The grading feature's mutating routes must stay mapped: the session-scores
// row is the ONLY evidence of who entered a component score (the owner may
// write on any teacher's behalf), so a missing entry would silently degrade the
// trail to "METHOD route".
func TestGradingRoutesAreRegistered(t *testing.T) {
	cases := []struct {
		method, route, action, entity, idParam string
	}{
		{"POST", "/api/v1/score-sets", "score_set.create", "score_set", ""},
		{"PUT", "/api/v1/score-sets/:id", "score_set.update", "score_set", "id"},
		{"DELETE", "/api/v1/score-sets/:id", "score_set.delete", "score_set", "id"},
		{"POST", "/api/v1/classes/:id/score-set", "class.score_set.assign", "class", "id"},
		{"DELETE", "/api/v1/classes/:id/score-set", "class.score_set.clear", "class", "id"},
		{"PUT", "/api/v1/sessions/:id/scores", "session.scores.update", "session", "id"},
	}
	for _, c := range cases {
		spec, ok := LookupAction(c.method, c.route)
		if !ok {
			t.Errorf("%s %s is not registered", c.method, c.route)
			continue
		}
		if spec.Action != c.action || spec.EntityType != c.entity || spec.IDParam != c.idParam {
			t.Errorf("%s %s mapped to %+v, want action=%q entity=%q idParam=%q",
				c.method, c.route, spec, c.action, c.entity, c.idParam)
		}
	}
}

// actionSnapshot pins the actions table as it stood before LookupAction was
// rebuilt to delegate to the shared route manifest. Every entry here must
// keep resolving to the exact same ActionSpec; a change here must be
// justified by an intentional decision to rename or re-scope an audit
// action, not by a refactor accidentally dropping or renaming one.
var actionSnapshot = []struct {
	method, route string
	spec          ActionSpec
}{
	{"DELETE", "/api/v1/score-sets/:id", ActionSpec{Action: "score_set.delete", EntityType: "score_set", IDParam: "id"}},
	{"POST", "/api/v1/imports/roster", ActionSpec{Action: "import.roster", EntityType: "import", IDParam: ""}},
	{"POST", "/api/v1/billing-periods/:id/notifications/run/resume", ActionSpec{Action: "notification.run.resume", EntityType: "billing_period", IDParam: "id"}},
	{"DELETE", "/api/v1/contacts/:id/zalo-mapping", ActionSpec{Action: "contact.zalo_mapping.clear", EntityType: "contact", IDParam: "id"}},
	{"DELETE", "/api/v1/classes/:id", ActionSpec{Action: "class.delete", EntityType: "class", IDParam: "id"}},
	{"POST", "/api/v1/billing-periods/:id/draft", ActionSpec{Action: "billing.period.draft", EntityType: "billing_period", IDParam: "id"}},
	{"POST", "/api/v1/invoices/:id/void", ActionSpec{Action: "billing.invoice.void", EntityType: "invoice", IDParam: "id"}},
	{"POST", "/api/v1/payments", ActionSpec{Action: "payment.create", EntityType: "payment", IDParam: ""}},
	{"POST", "/api/v1/billing-periods/:id/statements/generate", ActionSpec{Action: "statement.generate", EntityType: "billing_period", IDParam: "id"}},
	{"POST", "/api/v1/me/zalo/link/start", ActionSpec{Action: "zalo.link.start", EntityType: "zalo_account", IDParam: ""}},
	{"POST", "/api/v1/auth/reset-password", ActionSpec{Action: "auth.password_reset", EntityType: "user", IDParam: ""}},
	{"POST", "/api/v1/students", ActionSpec{Action: "student.create", EntityType: "student", IDParam: ""}},
	{"POST", "/api/v1/sessions/:id/attendance", ActionSpec{Action: "attendance.confirm", EntityType: "session", IDParam: "id"}},
	{"POST", "/api/v1/classes/:id/lesson-plans/:index/submit", ActionSpec{Action: "lesson_plan.submit", EntityType: "class", IDParam: "id"}},
	{"POST", "/api/v1/classes/:id/lesson-plans/:index/request-redo", ActionSpec{Action: "lesson_plan.request_redo", EntityType: "class", IDParam: "id"}},
	{"POST", "/api/v1/contacts", ActionSpec{Action: "contact.create", EntityType: "contact", IDParam: ""}},
	{"DELETE", "/api/v1/enrollments/:id", ActionSpec{Action: "enrollment.delete", EntityType: "enrollment", IDParam: "id"}},
	{"POST", "/api/v1/classes/:id/lesson-plans/:index/reopen", ActionSpec{Action: "lesson_plan.reopen", EntityType: "class", IDParam: "id"}},
	{"POST", "/api/v1/score-sets", ActionSpec{Action: "score_set.create", EntityType: "score_set", IDParam: ""}},
	{"POST", "/api/v1/statements/:id/revoke", ActionSpec{Action: "statement.revoke", EntityType: "statement", IDParam: "id"}},
	{"POST", "/api/v1/notifications/mark-sent", ActionSpec{Action: "notification.mark_sent", EntityType: "notification", IDParam: ""}},
	{"PATCH", "/api/v1/centers/me", ActionSpec{Action: "center.rename", EntityType: "center", IDParam: ""}},
	{"DELETE", "/api/v1/centers/me/members/:teacherId", ActionSpec{Action: "center.member.remove", EntityType: "teacher", IDParam: "teacherId"}},
	{"POST", "/api/v1/classes/:id/schedules", ActionSpec{Action: "class.schedule.create", EntityType: "class", IDParam: "id"}},
	{"DELETE", "/api/v1/classes/:id/schedules/:scheduleID", ActionSpec{Action: "class.schedule.delete", EntityType: "schedule", IDParam: "scheduleID"}},
	{"POST", "/api/v1/sessions/:id/cancel", ActionSpec{Action: "session.cancel", EntityType: "session", IDParam: "id"}},
	{"PUT", "/api/v1/classes/:id/lesson-plans/:index", ActionSpec{Action: "lesson_plan.save", EntityType: "class", IDParam: "id"}},
	{"POST", "/api/v1/payments/:id/allocations/auto", ActionSpec{Action: "payment.allocate_auto", EntityType: "payment", IDParam: "id"}},
	{"POST", "/api/v1/me/zalo/friends/match", ActionSpec{Action: "zalo.friends.match", EntityType: "zalo_account", IDParam: ""}},
	{"DELETE", "/api/v1/contacts/:id", ActionSpec{Action: "contact.delete", EntityType: "contact", IDParam: "id"}},
	{"PUT", "/api/v1/contacts/:id/zalo-mapping", ActionSpec{Action: "contact.zalo_mapping.set", EntityType: "contact", IDParam: "id"}},
	{"PUT", "/api/v1/classes/:id/schedules/:scheduleID", ActionSpec{Action: "class.schedule.update", EntityType: "schedule", IDParam: "scheduleID"}},
	{"POST", "/api/v1/classes/:id/sessions", ActionSpec{Action: "session.create", EntityType: "class", IDParam: "id"}},
	{"DELETE", "/api/v1/sessions/:id", ActionSpec{Action: "session.delete", EntityType: "session", IDParam: "id"}},
	{"PUT", "/api/v1/sessions/:id/note", ActionSpec{Action: "session.note.update", EntityType: "session", IDParam: "id"}},
	{"POST", "/api/v1/me/zalo/friends/request", ActionSpec{Action: "zalo.friend_request", EntityType: "zalo_account", IDParam: ""}},
	{"PUT", "/api/v1/contacts/:id", ActionSpec{Action: "contact.update", EntityType: "contact", IDParam: "id"}},
	{"POST", "/api/v1/billing-periods/:id/close", ActionSpec{Action: "billing.period.close", EntityType: "billing_period", IDParam: "id"}},
	{"POST", "/api/v1/billing-periods/:id/notifications/bulk", ActionSpec{Action: "notification.bulk_send", EntityType: "billing_period", IDParam: "id"}},
	{"PUT", "/api/v1/me", ActionSpec{Action: "teacher.profile.update", EntityType: "teacher", IDParam: ""}},
	{"PUT", "/api/v1/centers/me/roles/:roleId/permissions", ActionSpec{Action: "center.role.permissions_update", EntityType: "center_role", IDParam: "roleId"}},
	{"PUT", "/api/v1/centers/me/members/:teacherId/role", ActionSpec{Action: "center.member.role_update", EntityType: "teacher", IDParam: "teacherId"}},
	{"PUT", "/api/v1/students/:id", ActionSpec{Action: "student.update", EntityType: "student", IDParam: "id"}},
	{"PUT", "/api/v1/classes/:id/curriculum", ActionSpec{Action: "curriculum.update", EntityType: "class", IDParam: "id"}},
	{"POST", "/api/v1/billing-periods", ActionSpec{Action: "billing.period.create", EntityType: "billing_period", IDParam: ""}},
	{"POST", "/api/v1/auth/forgot-password", ActionSpec{Action: "auth.password_reset_request", EntityType: "user", IDParam: ""}},
	{"PUT", "/api/v1/centers/me/members/:teacherId/overrides", ActionSpec{Action: "center.member.overrides_update", EntityType: "teacher", IDParam: "teacherId"}},
	{"PUT", "/api/v1/classes/:id", ActionSpec{Action: "class.update", EntityType: "class", IDParam: "id"}},
	{"DELETE", "/api/v1/classes/:id/score-set", ActionSpec{Action: "class.score_set.clear", EntityType: "class", IDParam: "id"}},
	{"DELETE", "/api/v1/me/zalo", ActionSpec{Action: "zalo.unlink", EntityType: "zalo_account", IDParam: ""}},
	{"PUT", "/api/v1/classes/:id/teacher", ActionSpec{Action: "class.teacher.reassign", EntityType: "class", IDParam: "id"}},
	{"DELETE", "/api/v1/classes/:id/staff/:staffId", ActionSpec{Action: "class.staff.remove", EntityType: "class_staff", IDParam: "staffId"}},
	{"POST", "/api/v1/classes/:id/invitations", ActionSpec{Action: "class_invitation.send", EntityType: "class", IDParam: "id"}},
	{"POST", "/api/v1/class-invitations/:id/accept", ActionSpec{Action: "class_invitation.accept", EntityType: "class_invitation", IDParam: "id"}},
	{"POST", "/api/v1/class-invitations/:id/decline", ActionSpec{Action: "class_invitation.decline", EntityType: "class_invitation", IDParam: "id"}},
	{"POST", "/api/v1/class-invitations/:id/cancel", ActionSpec{Action: "class_invitation.cancel", EntityType: "class_invitation", IDParam: "id"}},
	{"POST", "/api/v1/class-invitations/:id/remind", ActionSpec{Action: "class_invitation.remind", EntityType: "class_invitation", IDParam: "id"}},
	{"POST", "/api/v1/class-invitations/:id/confirm", ActionSpec{Action: "class_invitation.confirm", EntityType: "class_invitation", IDParam: "id"}},
	{"POST", "/api/v1/enrollments/:id/end", ActionSpec{Action: "enrollment.end", EntityType: "enrollment", IDParam: "id"}},
	{"POST", "/api/v1/sessions/:id/uncancel", ActionSpec{Action: "session.uncancel", EntityType: "session", IDParam: "id"}},
	{"POST", "/api/v1/sessions/:id/hold", ActionSpec{Action: "session.hold", EntityType: "session", IDParam: "id"}},
	{"POST", "/api/v1/classes/:id/lesson-plans/:index/approve", ActionSpec{Action: "lesson_plan.approve", EntityType: "class", IDParam: "id"}},
	{"PUT", "/api/v1/sessions/:id/marks", ActionSpec{Action: "session.marks.update", EntityType: "session", IDParam: "id"}},
	{"PUT", "/api/v1/payments/:id/allocations", ActionSpec{Action: "payment.reallocate", EntityType: "payment", IDParam: "id"}},
	{"DELETE", "/api/v1/students/:id", ActionSpec{Action: "student.delete", EntityType: "student", IDParam: "id"}},
	{"POST", "/api/v1/classes", ActionSpec{Action: "class.create", EntityType: "class", IDParam: ""}},
	{"PUT", "/api/v1/score-sets/:id", ActionSpec{Action: "score_set.update", EntityType: "score_set", IDParam: "id"}},
	{"POST", "/api/v1/classes/:id/score-set", ActionSpec{Action: "class.score_set.assign", EntityType: "class", IDParam: "id"}},
	{"PUT", "/api/v1/sessions/:id/scores", ActionSpec{Action: "session.scores.update", EntityType: "session", IDParam: "id"}},
	{"POST", "/api/v1/invoices/:id/adjustments", ActionSpec{Action: "billing.adjustment.create", EntityType: "invoice", IDParam: "id"}},
	{"POST", "/api/v1/payments/:id/reverse", ActionSpec{Action: "payment.reverse", EntityType: "payment", IDParam: "id"}},
	{"POST", "/api/v1/centers/me/invitations", ActionSpec{Action: "invitation.create", EntityType: "invitation", IDParam: ""}},
	{"DELETE", "/api/v1/centers/me/invitations/:id", ActionSpec{Action: "invitation.revoke", EntityType: "invitation", IDParam: "id"}},
	{"POST", "/api/v1/classes/:id/staff", ActionSpec{Action: "class.staff.assign", EntityType: "class", IDParam: "id"}},
	{"POST", "/api/v1/classes/:id/archive", ActionSpec{Action: "class.archive", EntityType: "class", IDParam: "id"}},
	{"POST", "/api/v1/task-columns", ActionSpec{Action: "task_column.create", EntityType: "task_column", IDParam: ""}},
	{"PATCH", "/api/v1/task-columns/:id", ActionSpec{Action: "task_column.update", EntityType: "task_column", IDParam: "id"}},
	{"PUT", "/api/v1/task-columns/order", ActionSpec{Action: "task_column.reorder", EntityType: "task_column", IDParam: ""}},
	{"DELETE", "/api/v1/task-columns/:id", ActionSpec{Action: "task_column.delete", EntityType: "task_column", IDParam: "id"}},
	{"POST", "/api/v1/tasks", ActionSpec{Action: "task.create", EntityType: "task", IDParam: ""}},
	{"PATCH", "/api/v1/tasks/:id", ActionSpec{Action: "task.update", EntityType: "task", IDParam: "id"}},
	{"POST", "/api/v1/tasks/:id/move", ActionSpec{Action: "task.move", EntityType: "task", IDParam: "id"}},
	{"DELETE", "/api/v1/tasks/:id", ActionSpec{Action: "task.delete", EntityType: "task", IDParam: "id"}},
	{"POST", "/api/v1/tasks/:id/restore", ActionSpec{Action: "task.restore", EntityType: "task", IDParam: "id"}},
	{"POST", "/api/v1/library/templates", ActionSpec{Action: "program_template.create", EntityType: "program_template", IDParam: ""}},
	{"PUT", "/api/v1/library/templates/:id", ActionSpec{Action: "program_template.update", EntityType: "program_template", IDParam: "id"}},
	{"DELETE", "/api/v1/library/templates/:id", ActionSpec{Action: "program_template.delete", EntityType: "program_template", IDParam: "id"}},
	{"POST", "/api/v1/library/templates/:id/versions", ActionSpec{Action: "template_version.create", EntityType: "program_template", IDParam: "id"}},
	{"POST", "/api/v1/library/versions/:vid/publish", ActionSpec{Action: "template_version.publish", EntityType: "template_version", IDParam: "vid"}},
	{"POST", "/api/v1/library/versions/:vid/archive", ActionSpec{Action: "template_version.archive", EntityType: "template_version", IDParam: "vid"}},
	{"POST", "/api/v1/library/versions/:vid/lessons", ActionSpec{Action: "template_lesson.create", EntityType: "template_version", IDParam: "vid"}},
	{"PUT", "/api/v1/library/versions/:vid/lessons/order", ActionSpec{Action: "template_lesson.reorder", EntityType: "template_version", IDParam: "vid"}},
	{"DELETE", "/api/v1/library/versions/:vid/lessons", ActionSpec{Action: "template_version.clear_lessons", EntityType: "template_version", IDParam: "vid"}},
	{"POST", "/api/v1/library/versions/:vid/exercise-groups", ActionSpec{Action: "template_exercise_group.create", EntityType: "template_exercise_group", IDParam: ""}},
	{"DELETE", "/api/v1/library/versions/:vid/exercise-groups/:gid", ActionSpec{Action: "template_exercise_group.delete", EntityType: "template_exercise_group", IDParam: "gid"}},
	{"PUT", "/api/v1/library/lessons/:lid", ActionSpec{Action: "template_lesson.update", EntityType: "template_lesson", IDParam: "lid"}},
	{"DELETE", "/api/v1/library/lessons/:lid", ActionSpec{Action: "template_lesson.delete", EntityType: "template_lesson", IDParam: "lid"}},
	{"POST", "/api/v1/library/lessons/:lid/duplicate", ActionSpec{Action: "template_lesson.duplicate", EntityType: "template_lesson", IDParam: "lid"}},
	{"PUT", "/api/v1/library/versions/:vid/log-fields", ActionSpec{Action: "template_version.set_log_fields", EntityType: "template_version", IDParam: "vid"}},
	{"PUT", "/api/v1/library/versions/:vid/score-set", ActionSpec{Action: "template_version.set_score_set", EntityType: "template_version", IDParam: "vid"}},
	{"PUT", "/api/v1/library/lessons/:lid/materials", ActionSpec{Action: "template_lesson.set_materials", EntityType: "template_lesson", IDParam: "lid"}},
	{"PUT", "/api/v1/library/lessons/:lid/exercises", ActionSpec{Action: "template_lesson.set_exercises", EntityType: "template_lesson", IDParam: "lid"}},
	{"PATCH", "/api/v1/library/lessons/:lid/prep", ActionSpec{Action: "template_lesson.prep", EntityType: "template_lesson", IDParam: "lid"}},
	{"PATCH", "/api/v1/library/lessons/:lid/assignment", ActionSpec{Action: "template_lesson.assign", EntityType: "template_lesson", IDParam: "lid"}},
	{"POST", "/api/v1/library/materials", ActionSpec{Action: "library_material.create", EntityType: "library_material", IDParam: ""}},
	{"PUT", "/api/v1/library/materials/:id", ActionSpec{Action: "library_material.update", EntityType: "library_material", IDParam: "id"}},
	{"DELETE", "/api/v1/library/materials/:id", ActionSpec{Action: "library_material.delete", EntityType: "library_material", IDParam: "id"}},
	{"PATCH", "/api/v1/library/materials/:id/status", ActionSpec{Action: "library_material.set_status", EntityType: "library_material", IDParam: "id"}},
	{"POST", "/api/v1/library/exercises", ActionSpec{Action: "library_exercise.create", EntityType: "library_exercise", IDParam: ""}},
	{"PUT", "/api/v1/library/exercises/:id", ActionSpec{Action: "library_exercise.update", EntityType: "library_exercise", IDParam: "id"}},
	{"DELETE", "/api/v1/library/exercises/:id", ActionSpec{Action: "library_exercise.delete", EntityType: "library_exercise", IDParam: "id"}},
	{"PATCH", "/api/v1/library/exercises/:id/status", ActionSpec{Action: "library_exercise.set_status", EntityType: "library_exercise", IDParam: "id"}},
	{"POST", "/api/v1/courses", ActionSpec{Action: "course.create", EntityType: "course", IDParam: ""}},
	{"PUT", "/api/v1/courses/:id", ActionSpec{Action: "course.update", EntityType: "course", IDParam: "id"}},
	{"DELETE", "/api/v1/courses/:id", ActionSpec{Action: "course.delete", EntityType: "course", IDParam: "id"}},
	{"POST", "/api/v1/courses/:id/archive", ActionSpec{Action: "course.archive", EntityType: "course", IDParam: "id"}},
	{"PUT", "/api/v1/courses/:id/tuition-packs", ActionSpec{Action: "course.set_tuition_packs", EntityType: "course", IDParam: "id"}},
	{"POST", "/api/v1/paths", ActionSpec{Action: "learning_path.create", EntityType: "learning_path", IDParam: ""}},
	{"PUT", "/api/v1/paths/:id", ActionSpec{Action: "learning_path.update", EntityType: "learning_path", IDParam: "id"}},
	{"DELETE", "/api/v1/paths/:id", ActionSpec{Action: "learning_path.delete", EntityType: "learning_path", IDParam: "id"}},
	{"POST", "/api/v1/paths/:id/stages", ActionSpec{Action: "path_stage.create", EntityType: "learning_path", IDParam: "id"}},
	{"PUT", "/api/v1/paths/:id/stages/order", ActionSpec{Action: "path_stage.reorder", EntityType: "learning_path", IDParam: "id"}},
	{"PUT", "/api/v1/paths/:id/stages/:sid", ActionSpec{Action: "path_stage.update", EntityType: "path_stage", IDParam: "sid"}},
	{"DELETE", "/api/v1/paths/:id/stages/:sid", ActionSpec{Action: "path_stage.delete", EntityType: "path_stage", IDParam: "sid"}},
	{"PUT", "/api/v1/paths/:id/stages/:sid/courses", ActionSpec{Action: "path_stage.set_courses", EntityType: "path_stage", IDParam: "sid"}},
	{"PUT", "/api/v1/classes/:id/program", ActionSpec{Action: "class_program.apply", EntityType: "class", IDParam: "id"}},
	{"DELETE", "/api/v1/classes/:id/program", ActionSpec{Action: "class_program.remove", EntityType: "class", IDParam: "id"}},
	{"POST", "/api/v1/classes/:id/messages", ActionSpec{Action: "class_message.post", EntityType: "class", IDParam: "id"}},
	{"DELETE", "/api/v1/classes/:id/messages/:mid", ActionSpec{Action: "class_message.delete", EntityType: "class_message", IDParam: "mid"}},
}

// TestActionSnapshotUnchanged proves LookupAction still resolves every
// mutating route to the exact ActionSpec it resolved to before LookupAction
// was rebuilt to delegate to the shared route manifest, and that no route
// with an Action was added or dropped in the process. Coverage runs both
// ways: every snapshot entry must resolve, and every manifest route with a
// non-empty Audit.Action must appear in the snapshot.
func TestActionSnapshotUnchanged(t *testing.T) {
	want := make(map[string]ActionSpec, len(actionSnapshot))
	for _, c := range actionSnapshot {
		id := c.method + " " + c.route
		want[id] = c.spec
		spec, ok := LookupAction(c.method, c.route)
		if !ok {
			t.Errorf("%s: not registered, snapshot wants %+v", id, c.spec)
			continue
		}
		if spec != c.spec {
			t.Errorf("%s = %+v, snapshot wants %+v", id, spec, c.spec)
		}
	}
	for _, s := range routespec.Specs {
		if s.Audit.Action == "" {
			continue
		}
		id := s.Method + " " + s.Path
		if _, ok := want[id]; !ok {
			t.Errorf("%s: has Audit.Action %q but is missing from the snapshot", id, s.Audit.Action)
		}
	}
}
