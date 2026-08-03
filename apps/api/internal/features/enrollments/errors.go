package enrollments

import "errors"

var (
	// ErrNotFound covers both a missing enrollment and another teacher's —
	// the caller cannot tell them apart, by design.
	ErrNotFound = errors.New("enrollment not found")
	// ErrAlreadyEnrolled is the uq_enrollments_active unique violation: the
	// student already has an open enrollment in this class.
	ErrAlreadyEnrolled = errors.New("student is already enrolled in this class")
	// ErrAlreadyEnded refuses to move a departure date on double-submit.
	ErrAlreadyEnded = errors.New("enrollment is already ended")
	// ErrClassNotFound means the class id is not one of this teacher's.
	ErrClassNotFound = errors.New("class not found")
	// ErrStudentNotFound means the student id is not one of this teacher's.
	ErrStudentNotFound = errors.New("student not found")
)
