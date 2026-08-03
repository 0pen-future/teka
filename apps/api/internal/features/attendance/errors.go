package attendance

import "errors"

var (
	// ErrSessionNotFound covers both a missing session and another
	// teacher's — the caller cannot tell them apart, by design.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionCancelled refuses to confirm attendance on a cancelled
	// session. The schema's CHECK constraint (status <> 'cancelled' OR
	// attendance_confirmed_at IS NULL) backs this up structurally; PRD edge
	// case: "Buổi bị hủy do giáo viên → không tính tiền cho ai."
	ErrSessionCancelled = errors.New("session is cancelled")
	// ErrStudentNotEnrolled means an absent_student_id is not on the roster
	// active on the session date. Silently ignoring it would mean a typo
	// leaves a student billed for a session they missed, so it is rejected
	// rather than dropped.
	ErrStudentNotEnrolled = errors.New("student not enrolled for this session")
)
