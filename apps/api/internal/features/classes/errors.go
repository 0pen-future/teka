package classes

import "errors"

var (
	// ErrNotFound covers both a missing class and another teacher's class —
	// the caller cannot tell them apart, by design.
	ErrNotFound = errors.New("class not found")
	// ErrScheduleNotFound is the schedule-level equivalent.
	ErrScheduleNotFound = errors.New("schedule not found")
	// ErrHasOpenEnrollments blocks soft-deleting a class that students are
	// still enrolled in; archiving is the suggested action instead.
	ErrHasOpenEnrollments = errors.New("class has open enrollments")
	// ErrCodeTaken means another live class in the center already carries the
	// requested code; surfaces as 409 CLASS_CODE_TAKEN.
	ErrCodeTaken = errors.New("class code already taken")
	// ErrCourseNotFound means the requested course_id is not a live course
	// of the class's center; surfaces as 422 on the course_id field.
	ErrCourseNotFound = errors.New("course not found")
)

// CodeClassCodeTaken is the wire error code for ErrCodeTaken, distinct from
// the generic CONFLICT so a form can attach the message to its code field.
const CodeClassCodeTaken = "CLASS_CODE_TAKEN"
