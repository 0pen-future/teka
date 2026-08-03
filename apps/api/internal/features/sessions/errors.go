package sessions

import "errors"

var (
	// ErrNotFound covers both a missing session and another teacher's — the
	// caller cannot tell them apart, by design.
	ErrNotFound = errors.New("session not found")
	// ErrSessionExists is the uq_class_sessions_per_day unique violation: an
	// ad-hoc session was requested for a date that already has one.
	ErrSessionExists = errors.New("a session already exists on this date")
	// ErrAttendanceConfirmed refuses to cancel or delete a session whose
	// attendance has already been confirmed.
	ErrAttendanceConfirmed = errors.New("session has confirmed attendance")
	// ErrReasonRequired refuses to cancel a session without a non-empty
	// reason — it becomes the line parents see on their statement.
	ErrReasonRequired = errors.New("cancel reason is required")
	// ErrInvalidTransition refuses a lifecycle move that would break the
	// held-implies-confirmed invariant, such as un-cancelling a session that
	// was never cancelled or holding a cancelled one.
	ErrInvalidTransition = errors.New("invalid session status transition")
)
