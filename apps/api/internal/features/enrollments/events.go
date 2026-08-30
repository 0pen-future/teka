package enrollments

import (
	"time"

	"github.com/google/uuid"
)

// StudentEnrolled records a successful enrollment for the audit trail. It is
// published after the row is committed, carrying what the request middleware
// cannot see: the exact enrollment produced and the class/student pair it
// links. It lives next to its publisher so subscribers import enrollments and
// enrollments never imports them back.
type StudentEnrolled struct {
	OccurredAt   time.Time
	CenterID     uuid.UUID
	ActorID      uuid.UUID
	EnrollmentID uuid.UUID
	ClassID      uuid.UUID
	StudentID    uuid.UUID
}

// EventName implements events.Event.
func (StudentEnrolled) EventName() string { return "enrollments.student_enrolled" }
