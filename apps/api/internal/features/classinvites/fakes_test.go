package classinvites

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/handoff"
	"teka/apps/api/internal/shared/authctx"
)

// fakeRepo is an in-memory Repository. It scopes every read by center id the
// way the SQL does, so a service test can prove tenant isolation without a
// database.
type fakeRepo struct {
	rows      map[uuid.UUID]*Row
	createErr error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[uuid.UUID]*Row{}} }

func (f *fakeRepo) add(inv Invitation, className, teacherName string) *Row {
	row := &Row{Invitation: inv, ClassName: className, TeacherName: teacherName, InvitedByName: "Chủ"}
	f.rows[inv.ID] = row
	return row
}

func (f *fakeRepo) Create(_ context.Context, inv *Invitation) error {
	if f.createErr != nil {
		return f.createErr
	}
	for _, r := range f.rows {
		if r.ClassID == inv.ClassID && r.TeacherID == inv.TeacherID && r.Status == StatusPending {
			return gorm.ErrDuplicatedKey
		}
	}
	inv.ID = uuid.New()
	inv.Status = StatusPending
	f.rows[inv.ID] = &Row{Invitation: *inv, ClassName: "Lớp", TeacherName: "GV", InvitedByName: "Chủ"}
	return nil
}

func (f *fakeRepo) Get(_ context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error) {
	r, ok := f.rows[id]
	if !ok || r.CenterID != sc.CenterID {
		return nil, ErrNotFound
	}
	copied := *r
	return &copied, nil
}

func (f *fakeRepo) List(_ context.Context, sc authctx.Scope, filter Filter) ([]Row, error) {
	var out []Row
	for _, r := range f.rows {
		if r.CenterID != sc.CenterID {
			continue
		}
		if filter.Status != "" && r.Status != filter.Status {
			continue
		}
		if filter.ClassID != nil && r.ClassID != *filter.ClassID {
			continue
		}
		if filter.TeacherID != nil && r.TeacherID != *filter.TeacherID {
			continue
		}
		out = append(out, *r)
	}
	return out, nil
}

func (f *fakeRepo) HasPending(_ context.Context, sc authctx.Scope, classID, teacherID uuid.UUID) (bool, error) {
	for _, r := range f.rows {
		if r.CenterID == sc.CenterID && r.ClassID == classID && r.TeacherID == teacherID && r.Status == StatusPending {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) Transition(_ context.Context, sc authctx.Scope, id uuid.UUID, from []string, to string) error {
	r, ok := f.rows[id]
	if !ok || r.CenterID != sc.CenterID {
		return ErrNotFound
	}
	for _, s := range from {
		if r.Status == s {
			r.Status = to
			return nil
		}
	}
	return ErrNotFound
}

func (f *fakeRepo) Remind(_ context.Context, sc authctx.Scope, id uuid.UUID) error {
	r, ok := f.rows[id]
	if !ok || r.CenterID != sc.CenterID || r.Status != StatusPending {
		return ErrNotFound
	}
	now := nowUTC()
	r.RemindedAt = &now
	return nil
}

func (f *fakeRepo) CancelOpenForMember(_ context.Context, centerID, teacherID uuid.UUID) (int64, error) {
	var n int64
	for _, r := range f.rows {
		if r.CenterID == centerID && r.TeacherID == teacherID && (r.Status == StatusPending || r.Status == StatusAccepted) {
			r.Status = StatusCancelled
			n++
		}
	}
	return n, nil
}

// fakeClasses answers Get for one class under the owner's center only.
type fakeClasses struct {
	class *classes.Class
}

func (f *fakeClasses) Get(_ context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error) {
	if f.class == nil || f.class.ID != classID || f.class.CenterID != sc.CenterID {
		return nil, classes.ErrNotFound
	}
	return f.class, nil
}

// fakeMembers marks the listed teachers as active members.
type fakeMembers struct {
	active  map[uuid.UUID]bool
	checked []uuid.UUID
}

func (f *fakeMembers) IsActiveMember(_ context.Context, _ authctx.Scope, teacherID uuid.UUID) (bool, error) {
	f.checked = append(f.checked, teacherID)
	return f.active[teacherID], nil
}

// fakeStaffRoles reports active role keys per class.
type fakeStaffRoles struct {
	roles map[uuid.UUID][]string
}

func (f *fakeStaffRoles) RolesByClass(_ context.Context, _, _ uuid.UUID, _ []uuid.UUID) (map[uuid.UUID][]string, error) {
	return f.roles, nil
}

// fakeAssigner records classstaff assigns and handoff reassigns so a test can
// prove confirm routes each role to the right write.
type fakeAssigner struct {
	assigns    []classstaff.AssignRequest
	reassigns  []uuid.UUID
	assignErr  error
	reassErr   error
	movedCount int64
}

func (f *fakeAssigner) Assign(_ context.Context, _ authctx.Scope, _ uuid.UUID, req classstaff.AssignRequest) (*classstaff.StaffResponse, error) {
	if f.assignErr != nil {
		return nil, f.assignErr
	}
	f.assigns = append(f.assigns, req)
	return &classstaff.StaffResponse{TeacherID: req.TeacherID, RoleKey: req.RoleKey}, nil
}

func (f *fakeAssigner) Reassign(_ context.Context, _ authctx.Scope, classID, newTeacherID uuid.UUID) (*handoff.Result, error) {
	if f.reassErr != nil {
		return nil, f.reassErr
	}
	f.reassigns = append(f.reassigns, newTeacherID)
	return &handoff.Result{ClassID: classID, TeacherID: newTeacherID, MovedPlannedSessions: f.movedCount}, nil
}

// fakeTx runs fn inline and counts calls, so a test can assert confirm wraps
// the invitation update and the stint write in one transaction.
type fakeTx struct {
	calls int
}

func (f *fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	f.calls++
	return fn(ctx)
}

var errBoom = errors.New("boom")
