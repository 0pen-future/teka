package audit

// ActionSpec names a mutating route for humans: a stable dotted action, the
// entity type it touches, and which route parameter carries the entity id.
type ActionSpec struct {
	Action     string
	EntityType string
	// IDParam is the route parameter holding the entity id ("" when the
	// route addresses no single entity, e.g. collection creates).
	IDParam string
}

// actions maps "METHOD route-template" (gin FullPath form) to its spec.
// Login and logout are deliberately absent: they are audited through
// service-published events, not through the request middleware. The password
// reset routes ARE mapped — no service event covers them, and a password
// change must never escape the trail even with an unauthenticated actor.
//
// Convention: when adding a mutating route, add its entry here. A missing
// entry is not lost — the subscriber falls back to "METHOD route" — it is
// just less readable in the audit page.
var actions = map[string]ActionSpec{
	// auth (password reset only; the actor is unauthenticated so rows carry
	// a NULL actor — the request context is the evidence)
	"POST /api/v1/auth/forgot-password": {Action: "auth.password_reset_request", EntityType: "user"},
	"POST /api/v1/auth/reset-password":  {Action: "auth.password_reset", EntityType: "user"},

	// centers
	"PATCH /api/v1/centers/me":                     {Action: "center.rename", EntityType: "center"},
	"DELETE /api/v1/centers/me/members/:teacherId": {Action: "center.member.remove", EntityType: "teacher", IDParam: "teacherId"},
	// Grant and revoke are separate routes precisely so these two actions stay
	// distinguishable — the middleware stores no request body.
	"POST /api/v1/centers/me/members/:teacherId/send-reports":   {Action: "center.member.send_reports_grant", EntityType: "teacher", IDParam: "teacherId"},
	"DELETE /api/v1/centers/me/members/:teacherId/send-reports": {Action: "center.member.send_reports_revoke", EntityType: "teacher", IDParam: "teacherId"},
	// The permission-management routes share their action names with the
	// centers service events: this row is the HTTP evidence (status code, IP —
	// including rejected attempts), while the service event row carries the
	// committed before/after diff the middleware cannot see.
	"PUT /api/v1/centers/me/roles/:roleId/permissions":    {Action: "center.role.permissions_update", EntityType: "center_role", IDParam: "roleId"},
	"PUT /api/v1/centers/me/members/:teacherId/role":      {Action: "center.member.role_update", EntityType: "teacher", IDParam: "teacherId"},
	"PUT /api/v1/centers/me/members/:teacherId/overrides": {Action: "center.member.overrides_update", EntityType: "teacher", IDParam: "teacherId"},

	// invitations (owner-managed). The public preview/accept routes are
	// deliberately absent: the middleware skips principal-less requests, and
	// a successful accept is audited through the invitations.MemberJoined
	// service event, which is the only place that knows the center and the
	// account that joined.
	"POST /api/v1/centers/me/invitations":       {Action: "invitation.create", EntityType: "invitation"},
	"DELETE /api/v1/centers/me/invitations/:id": {Action: "invitation.revoke", EntityType: "invitation", IDParam: "id"},

	// teachers
	"PUT /api/v1/me": {Action: "teacher.profile.update", EntityType: "teacher"},

	// contacts
	"POST /api/v1/contacts":                    {Action: "contact.create", EntityType: "contact"},
	"PUT /api/v1/contacts/:id":                 {Action: "contact.update", EntityType: "contact", IDParam: "id"},
	"DELETE /api/v1/contacts/:id":              {Action: "contact.delete", EntityType: "contact", IDParam: "id"},
	"PUT /api/v1/contacts/:id/zalo-mapping":    {Action: "contact.zalo_mapping.set", EntityType: "contact", IDParam: "id"},
	"DELETE /api/v1/contacts/:id/zalo-mapping": {Action: "contact.zalo_mapping.clear", EntityType: "contact", IDParam: "id"},

	// students
	"POST /api/v1/students":       {Action: "student.create", EntityType: "student"},
	"PUT /api/v1/students/:id":    {Action: "student.update", EntityType: "student", IDParam: "id"},
	"DELETE /api/v1/students/:id": {Action: "student.delete", EntityType: "student", IDParam: "id"},

	// classes + schedules + teacher handoff
	"POST /api/v1/classes":                             {Action: "class.create", EntityType: "class"},
	"PUT /api/v1/classes/:id":                          {Action: "class.update", EntityType: "class", IDParam: "id"},
	"POST /api/v1/classes/:id/archive":                 {Action: "class.archive", EntityType: "class", IDParam: "id"},
	"DELETE /api/v1/classes/:id":                       {Action: "class.delete", EntityType: "class", IDParam: "id"},
	"POST /api/v1/classes/:id/schedules":               {Action: "class.schedule.create", EntityType: "class", IDParam: "id"},
	"PUT /api/v1/classes/:id/schedules/:scheduleID":    {Action: "class.schedule.update", EntityType: "schedule", IDParam: "scheduleID"},
	"DELETE /api/v1/classes/:id/schedules/:scheduleID": {Action: "class.schedule.delete", EntityType: "schedule", IDParam: "scheduleID"},
	"PUT /api/v1/classes/:id/teacher":                  {Action: "class.teacher.reassign", EntityType: "class", IDParam: "id"},

	// enrollments
	"POST /api/v1/enrollments":         {Action: "enrollment.create", EntityType: "enrollment"},
	"POST /api/v1/enrollments/:id/end": {Action: "enrollment.end", EntityType: "enrollment", IDParam: "id"},
	"DELETE /api/v1/enrollments/:id":   {Action: "enrollment.delete", EntityType: "enrollment", IDParam: "id"},

	// sessions (ad-hoc create nests under the class)
	"POST /api/v1/classes/:id/sessions":    {Action: "session.create", EntityType: "class", IDParam: "id"},
	"DELETE /api/v1/sessions/:id":          {Action: "session.delete", EntityType: "session", IDParam: "id"},
	"POST /api/v1/sessions/:id/cancel":     {Action: "session.cancel", EntityType: "session", IDParam: "id"},
	"POST /api/v1/sessions/:id/uncancel":   {Action: "session.uncancel", EntityType: "session", IDParam: "id"},
	"POST /api/v1/sessions/:id/hold":       {Action: "session.hold", EntityType: "session", IDParam: "id"},
	"POST /api/v1/sessions/:id/attendance": {Action: "attendance.confirm", EntityType: "session", IDParam: "id"},

	// teaching (curriculum, lesson plans, notes, marks)
	"PUT /api/v1/classes/:id/curriculum":                        {Action: "curriculum.update", EntityType: "class", IDParam: "id"},
	"PUT /api/v1/classes/:id/lesson-plans/:index":               {Action: "lesson_plan.save", EntityType: "class", IDParam: "id"},
	"POST /api/v1/classes/:id/lesson-plans/:index/submit":       {Action: "lesson_plan.submit", EntityType: "class", IDParam: "id"},
	"POST /api/v1/classes/:id/lesson-plans/:index/approve":      {Action: "lesson_plan.approve", EntityType: "class", IDParam: "id"},
	"POST /api/v1/classes/:id/lesson-plans/:index/request-redo": {Action: "lesson_plan.request_redo", EntityType: "class", IDParam: "id"},
	"POST /api/v1/classes/:id/lesson-plans/:index/reopen":       {Action: "lesson_plan.reopen", EntityType: "class", IDParam: "id"},
	"PUT /api/v1/sessions/:id/note":                             {Action: "session.note.update", EntityType: "session", IDParam: "id"},
	"PUT /api/v1/sessions/:id/marks":                            {Action: "session.marks.update", EntityType: "session", IDParam: "id"},

	// grading (score sets, class snapshot, component scores). The session
	// scores row is the trail for who entered a component score — the owner may
	// write on any teacher's behalf, so this row is the only evidence of that.
	"POST /api/v1/score-sets":              {Action: "score_set.create", EntityType: "score_set"},
	"PUT /api/v1/score-sets/:id":           {Action: "score_set.update", EntityType: "score_set", IDParam: "id"},
	"DELETE /api/v1/score-sets/:id":        {Action: "score_set.delete", EntityType: "score_set", IDParam: "id"},
	"POST /api/v1/classes/:id/score-set":   {Action: "class.score_set.assign", EntityType: "class", IDParam: "id"},
	"DELETE /api/v1/classes/:id/score-set": {Action: "class.score_set.clear", EntityType: "class", IDParam: "id"},
	"PUT /api/v1/sessions/:id/scores":      {Action: "session.scores.update", EntityType: "session", IDParam: "id"},

	// billing periods, invoices, adjustments
	"POST /api/v1/billing-periods":           {Action: "billing.period.create", EntityType: "billing_period"},
	"POST /api/v1/billing-periods/:id/draft": {Action: "billing.period.draft", EntityType: "billing_period", IDParam: "id"},
	"POST /api/v1/billing-periods/:id/close": {Action: "billing.period.close", EntityType: "billing_period", IDParam: "id"},
	"POST /api/v1/invoices/:id/void":         {Action: "billing.invoice.void", EntityType: "invoice", IDParam: "id"},
	"POST /api/v1/invoices/:id/adjustments":  {Action: "billing.adjustment.create", EntityType: "invoice", IDParam: "id"},

	// payments
	"POST /api/v1/payments":                      {Action: "payment.create", EntityType: "payment"},
	"PUT /api/v1/payments/:id/allocations":       {Action: "payment.reallocate", EntityType: "payment", IDParam: "id"},
	"POST /api/v1/payments/:id/allocations/auto": {Action: "payment.allocate_auto", EntityType: "payment", IDParam: "id"},
	"POST /api/v1/payments/:id/reverse":          {Action: "payment.reverse", EntityType: "payment", IDParam: "id"},

	// statements
	"POST /api/v1/billing-periods/:id/statements/generate": {Action: "statement.generate", EntityType: "billing_period", IDParam: "id"},
	"POST /api/v1/statements/:id/revoke":                   {Action: "statement.revoke", EntityType: "statement", IDParam: "id"},

	// notifications
	"POST /api/v1/billing-periods/:id/notifications/bulk":       {Action: "notification.bulk_send", EntityType: "billing_period", IDParam: "id"},
	"POST /api/v1/billing-periods/:id/notifications/run/resume": {Action: "notification.run.resume", EntityType: "billing_period", IDParam: "id"},
	"POST /api/v1/notifications/mark-sent":                      {Action: "notification.mark_sent", EntityType: "notification"},

	// imports
	"POST /api/v1/imports/roster": {Action: "import.roster", EntityType: "import"},

	// zalo linking
	"DELETE /api/v1/me/zalo":               {Action: "zalo.unlink", EntityType: "zalo_account"},
	"POST /api/v1/me/zalo/friends/match":   {Action: "zalo.friends.match", EntityType: "zalo_account"},
	"POST /api/v1/me/zalo/friends/request": {Action: "zalo.friend_request", EntityType: "zalo_account"},
	"POST /api/v1/me/zalo/link/start":      {Action: "zalo.link.start", EntityType: "zalo_account"},
}

// LookupAction resolves a mutating route to its spec; ok is false for routes
// not in the table, and callers then fall back to "METHOD route".
func LookupAction(method, route string) (ActionSpec, bool) {
	spec, ok := actions[method+" "+route]
	return spec, ok
}
