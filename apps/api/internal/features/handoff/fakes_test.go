package handoff

import (
	"context"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

// fakeClasses stands in for the classes service. It holds one class and records
// the reassign, so a test can assert what moved and — via the recorded scope —
// that the move ran inside the transaction, not before the lock.
type fakeClasses struct {
	class      *classes.Class
	getErr     error
	reassigned []reassignCall
	reassErr   error
}

type reassignCall struct {
	classID   uuid.UUID
	teacherID uuid.UUID
}

func (f *fakeClasses) Get(_ context.Context, _ authctx.Scope, classID uuid.UUID) (*classes.Class, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.class, nil
}

func (f *fakeClasses) ReassignTeacher(_ context.Context, _ authctx.Scope, classID, newTeacherID uuid.UUID) error {
	f.reassigned = append(f.reassigned, reassignCall{classID, newTeacherID})
	return f.reassErr
}

// fakeSessions stands in for the sessions service, reporting a fixed count of
// moved planned sessions and recording each call. gotOldTeacher captures the
// timezone-anchor teacher so a test can assert the pre-handoff teacher is
// threaded through, not the new one.
type fakeSessions struct {
	moved         int64
	movedErr      error
	reassigned    []reassignCall
	gotOldTeacher uuid.UUID
}

func (f *fakeSessions) ReassignPlanned(_ context.Context, _ authctx.Scope, classID, oldTeacherID, newTeacherID uuid.UUID) (int64, error) {
	f.gotOldTeacher = oldTeacherID
	f.reassigned = append(f.reassigned, reassignCall{classID, newTeacherID})
	return f.moved, f.movedErr
}

// fakeMembers answers the active-member check with whatever the test set, and
// records every teacher it was asked about so a test can assert the no-op path
// skips the check entirely.
type fakeMembers struct {
	active  bool
	err     error
	checked []uuid.UUID
}

func (f *fakeMembers) IsActiveMember(_ context.Context, _ authctx.Scope, teacherID uuid.UUID) (bool, error) {
	f.checked = append(f.checked, teacherID)
	return f.active, f.err
}

// fakeLocker answers TryLockCenter with whatever the test set and records the
// centers it was keyed on.
type fakeLocker struct {
	locked   bool
	lockErr  error
	timeouts int
	centers  []uuid.UUID
}

func (l *fakeLocker) TryLockCenter(_ context.Context, centerID uuid.UUID) (bool, error) {
	l.centers = append(l.centers, centerID)
	return l.locked, l.lockErr
}

func (l *fakeLocker) SetStatementTimeout(_ context.Context) error {
	l.timeouts++
	return nil
}

// rollbackTxManager reproduces the one transaction property these tests depend
// on: the fn runs, and its error propagates unwrapped.
type rollbackTxManager struct{}

func (rollbackTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// handoffFixture wires the service over the fakes with an owner caller whose
// center already holds the class being handed off.
type handoffFixture struct {
	svc        *Service
	classes    *fakeClasses
	sessions   *fakeSessions
	members    *fakeMembers
	locker     *fakeLocker
	scope      authctx.Scope
	classID    uuid.UUID
	oldTeacher uuid.UUID
	newTeacher uuid.UUID
}

func newHandoffFixture() *handoffFixture {
	centerID := uuid.New()
	classID := uuid.New()
	oldTeacher := uuid.New()
	newTeacher := uuid.New()
	fc := &fakeClasses{class: &classes.Class{ID: classID, TeacherID: oldTeacher, CenterID: centerID}}
	fs := &fakeSessions{moved: 3}
	fm := &fakeMembers{active: true}
	fl := &fakeLocker{locked: true}
	svc := NewService(fc, fs, fm, fl, rollbackTxManager{})
	return &handoffFixture{
		svc:        svc,
		classes:    fc,
		sessions:   fs,
		members:    fm,
		locker:     fl,
		scope:      authctx.Scope{TeacherID: oldTeacher, CenterID: centerID, IsOwner: true},
		classID:    classID,
		oldTeacher: oldTeacher,
		newTeacher: newTeacher,
	}
}

func (f *handoffFixture) reassign() (*Result, error) {
	return f.svc.Reassign(context.Background(), f.scope, f.classID, f.newTeacher)
}

// status pulls the HTTP status out of an app error, for the failure-path tests.
func status(err error) int {
	return apperror.From(err).Status
}
