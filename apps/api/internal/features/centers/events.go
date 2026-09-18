package centers

import (
	"time"

	"github.com/google/uuid"
)

// The three events below record permission mutations for the audit trail with
// the before/after sets the request middleware cannot see (it stores no
// request body). Each is published strictly after its transaction commits, so
// the trail never shows a diff that was rolled back. They live next to their
// publisher so subscribers import centers and centers never imports them back.

// RolePermissionsChanged records a role's permission set being replaced.
type RolePermissionsChanged struct {
	OccurredAt time.Time
	CenterID   uuid.UUID
	ActorID    uuid.UUID
	RoleID     uuid.UUID
	RoleKey    string
	Before     []string
	After      []string
}

// EventName implements events.Event.
func (RolePermissionsChanged) EventName() string { return "centers.role_permissions_changed" }

// MemberRoleChanged records a member's role assignment changing. Role keys
// name the roles; "" means the stint held no role.
type MemberRoleChanged struct {
	OccurredAt time.Time
	CenterID   uuid.UUID
	ActorID    uuid.UUID
	TeacherID  uuid.UUID
	Before     string
	After      string
}

// EventName implements events.Event.
func (MemberRoleChanged) EventName() string { return "centers.member_role_changed" }

// MemberOverridesChanged records a member's grant/deny override lists being
// replaced.
type MemberOverridesChanged struct {
	OccurredAt   time.Time
	CenterID     uuid.UUID
	ActorID      uuid.UUID
	TeacherID    uuid.UUID
	BeforeGrants []string
	BeforeDenies []string
	AfterGrants  []string
	AfterDenies  []string
}

// EventName implements events.Event.
func (MemberOverridesChanged) EventName() string { return "centers.member_overrides_changed" }

// MemberTasksHandedOver records a departing member's task board handover:
// their assigned tasks are unassigned, and the tasks they created are
// reassigned to SuccessorID (the center owner). Published after RemoveMember's
// transaction commits, alongside the membership close it happened inside —
// the generic request-log audit path cannot describe this on its own since
// it is triggered from inside another feature's transaction, not a direct
// request to the task board.
type MemberTasksHandedOver struct {
	OccurredAt  time.Time
	CenterID    uuid.UUID
	ActorID     uuid.UUID
	MemberID    uuid.UUID
	SuccessorID uuid.UUID
	Unassigned  int
	Reassigned  int
}

// EventName implements events.Event.
func (MemberTasksHandedOver) EventName() string { return "centers.member_tasks_handed_over" }
