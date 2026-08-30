// Package classscope owns the one SQL fragment that grants class-data READ
// access through class_staff assignments, so every repository widens its reads
// with identical semantics instead of hand-rolling an EXISTS per feature.
//
// The rules the fragment encodes, and that its callers inherit:
//   - ANY stint grants read — ended ones included: a closed assignment keeps
//     read access to the class's history. Revoking a mistaken assignment is a
//     hard delete (mode=void), never a filter here.
//   - A soft-deleted class grants nothing, no matter what stints point at it.
//     Every existing readScoped carries classes.deleted_at IS NULL; dropping
//     it would widen reads onto deleted classes.
//   - Read access only. Write gates keep their own scoping (classes.Get and
//     the per-feature service checks) until the capability map exists.
//
// The functions return plain SQL with `?` placeholders; the caller binds the
// caller's teacher id and center id, in that order. The column expressions
// passed in are compile-time constants from the calling repository, never user
// input.
package classscope

// ReadExists reports whether the caller holds any class_staff stint on the
// class identified by classIDExpr (a column expression of the outer query,
// e.g. "enrollments.class_id" or "classes.id"). Bind args: teacherID,
// centerID.
func ReadExists(classIDExpr string) (sql string, argCount int) {
	return `EXISTS (
		SELECT 1 FROM class_staff cs
		JOIN classes c2 ON c2.id = cs.class_id
		  AND c2.center_id = cs.center_id
		  AND c2.deleted_at IS NULL
		WHERE cs.class_id = ` + classIDExpr + `
		  AND cs.teacher_id = ?
		  AND cs.center_id = ?)`, 2
}

// WriteExists is the write gate counterpart of ReadExists: the caller writes
// class data only through an ACTIVE stint (ended_at IS NULL — ended stints
// keep reads, never writes) whose role_key is in the bound slice. The service
// resolves that slice from the capability map (authctx.StaffRolesFor); the
// repository only binds it, so the map has one home. Bind args: teacherID,
// centerID, roles ([]string).
func WriteExists(classIDExpr string) (sql string, argCount int) {
	return `EXISTS (
		SELECT 1 FROM class_staff cs
		JOIN classes c2 ON c2.id = cs.class_id
		  AND c2.center_id = cs.center_id
		  AND c2.deleted_at IS NULL
		WHERE cs.class_id = ` + classIDExpr + `
		  AND cs.teacher_id = ?
		  AND cs.center_id = ?
		  AND cs.ended_at IS NULL
		  AND cs.role_key IN ?)`, 3
}

// ReadExistsViaEnrollment is ReadExists for tables that reach a class only
// through a student's enrollments (students, attendance.StudentNames):
// the caller reads the student when any live enrollment links them to a class
// the caller holds a stint on. studentIDExpr is a column expression of the
// outer query, e.g. "students.id". Bind args: teacherID, centerID.
func ReadExistsViaEnrollment(studentIDExpr string) (sql string, argCount int) {
	return `EXISTS (
		SELECT 1 FROM enrollments e2
		JOIN class_staff cs ON cs.class_id = e2.class_id
		  AND cs.center_id = e2.center_id
		JOIN classes c2 ON c2.id = e2.class_id
		  AND c2.center_id = e2.center_id
		  AND c2.deleted_at IS NULL
		WHERE e2.student_id = ` + studentIDExpr + `
		  AND e2.deleted_at IS NULL
		  AND cs.teacher_id = ?
		  AND cs.center_id = ?)`, 2
}

// PhoneVisibleViaStudent is the phone-privacy derived column: it reports
// whether the caller holds an ACTIVE hoc_vu stint on a live class where the
// student identified by studentIDExpr is ACTIVELY enrolled. Deliberately
// stricter than the read fragments — ended stints and ended enrollments keep
// history readable but never carry the contact's phone along. Owner and
// reports-oversight bypass happens in the service via Scope.PhoneVisible, not
// here. Bind args: teacherID, centerID.
func PhoneVisibleViaStudent(studentIDExpr string) (sql string, argCount int) {
	return `EXISTS (
		SELECT 1 FROM enrollments e3
		JOIN class_staff cs3 ON cs3.class_id = e3.class_id
		  AND cs3.center_id = e3.center_id
		  AND cs3.role_key = 'hoc_vu'
		  AND cs3.ended_at IS NULL
		JOIN classes c3 ON c3.id = e3.class_id
		  AND c3.center_id = e3.center_id
		  AND c3.deleted_at IS NULL
		WHERE e3.student_id = ` + studentIDExpr + `
		  AND e3.deleted_at IS NULL
		  AND e3.ended_on IS NULL
		  AND cs3.teacher_id = ?
		  AND cs3.center_id = ?)`, 2
}

// PhoneVisibleViaContact is PhoneVisibleViaStudent for rows that carry a
// contact id instead of a student id (contacts, statements, notifications,
// collections): the phone is visible when ANY of the contact's students
// satisfies the student rule. Bind args: teacherID, centerID.
func PhoneVisibleViaContact(contactIDExpr string) (sql string, argCount int) {
	return `EXISTS (
		SELECT 1 FROM students s3
		JOIN enrollments e3 ON e3.student_id = s3.id
		  AND e3.center_id = s3.center_id
		  AND e3.deleted_at IS NULL
		  AND e3.ended_on IS NULL
		JOIN class_staff cs3 ON cs3.class_id = e3.class_id
		  AND cs3.center_id = e3.center_id
		  AND cs3.role_key = 'hoc_vu'
		  AND cs3.ended_at IS NULL
		JOIN classes c3 ON c3.id = e3.class_id
		  AND c3.center_id = e3.center_id
		  AND c3.deleted_at IS NULL
		WHERE s3.contact_id = ` + contactIDExpr + `
		  AND s3.deleted_at IS NULL
		  AND cs3.teacher_id = ?
		  AND cs3.center_id = ?)`, 2
}
