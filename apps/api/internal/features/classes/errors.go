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
)
