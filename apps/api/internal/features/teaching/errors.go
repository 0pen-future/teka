package teaching

import "errors"

var (
	// ErrClassNotFound covers both a missing class and another teacher's —
	// the caller cannot tell them apart, by design (owner scope excepted).
	ErrClassNotFound = errors.New("class not found")
	// ErrPlanNotFound is a lesson plan the caller may read but that has no
	// row where a row is required (review actions on an unsaved plan resolve
	// through the state machine instead, as StatusNone).
	ErrPlanNotFound = errors.New("lesson plan not found")
	// ErrIllegalTransition is an action the review-loop state machine does
	// not allow from the plan's current status — a stale screen, not a bug,
	// so it maps to 409 and the client refetches.
	ErrIllegalTransition = errors.New("illegal lesson plan transition")
	// ErrNotClassTeacher rejects content writes from anyone but the class's
	// own teacher. The owner deliberately included: the owner's power over
	// another teacher's class is read + the three review actions, never
	// editing content (decision 2026-08-14).
	ErrNotClassTeacher = errors.New("only the class teacher can edit teaching content")
	// ErrOwnerOnly rejects review actions (approve/request-redo/reopen) and
	// the review queue for non-owner members.
	ErrOwnerOnly = errors.New("owner-only action")
	// ErrSessionNotFound covers both a missing session and another teacher's
	// — indistinguishable to the caller, same as ErrClassNotFound.
	ErrSessionNotFound = errors.New("session not found")
	// ErrNotSessionTeacher rejects note/marks writes from anyone but the
	// session's own teacher — the owner reads center-wide but never records
	// scores or notes in another teacher's class (matches the UI).
	ErrNotSessionTeacher = errors.New("only the session's teacher can record notes and marks")
)
