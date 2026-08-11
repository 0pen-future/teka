package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// fakePendingRepo is a minimal Repository fake exercising only ListPending.
// The predicate itself (status, confirmed, deleted, cancelled, from/to,
// expected student count) is proven against real Postgres in
// integration_test.go; these unit tests cover what Service.ListPending alone
// is responsible for — resolving the teacher's timezone into a cutoff,
// clamping the limit, and computing DaysOverdue.
type fakePendingRepo struct {
	rows  []PendingRow
	total int64
	err   error

	gotScope  authctx.Scope
	gotBefore time.Time
	gotFrom   *time.Time
	gotTo     *time.Time
	gotLimit  int
}

func (f *fakePendingRepo) ListPending(_ context.Context, sc authctx.Scope, before time.Time, from, to *time.Time, limit int) ([]PendingRow, int64, error) {
	f.gotScope = sc
	f.gotBefore = before
	f.gotFrom = from
	f.gotTo = to
	f.gotLimit = limit
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.rows, f.total, nil
}

// The remaining Repository methods are unused by ListPending tests.
func (f *fakePendingRepo) BulkInsertIgnoreConflicts(context.Context, []Session) error { return nil }
func (f *fakePendingRepo) Create(context.Context, *Session) error                     { return nil }
func (f *fakePendingRepo) ListByClassAndRange(context.Context, authctx.Scope, uuid.UUID, time.Time, time.Time) ([]Row, error) {
	return nil, nil
}
func (f *fakePendingRepo) GetByID(context.Context, authctx.Scope, uuid.UUID) (*Row, error) {
	return nil, ErrNotFound
}
func (f *fakePendingRepo) UpdateStatus(context.Context, authctx.Scope, uuid.UUID, string, *string) error {
	return nil
}
func (f *fakePendingRepo) SoftDelete(context.Context, authctx.Scope, uuid.UUID) error { return nil }
func (f *fakePendingRepo) MarkHeldAndConfirmed(context.Context, authctx.Scope, uuid.UUID, time.Time) error {
	return nil
}

// ListPending on fakeRepository (defined in service_test.go) exists solely
// so that type still satisfies the Repository interface for the other tests
// in this package that build a Service through newTestService; ListPending's
// own behaviour is covered against fakePendingRepo above and against real
// Postgres in integration_test.go.
func (f *fakeRepository) ListPending(_ context.Context, sc authctx.Scope, before time.Time, from, to *time.Time, limit int) ([]PendingRow, int64, error) {
	var out []PendingRow
	for _, r := range f.rows {
		if r.deleted || !visible(sc, r) {
			continue
		}
		if !r.SessionDate.Before(before) {
			continue
		}
		if from != nil && r.SessionDate.Before(*from) {
			continue
		}
		if to != nil && r.SessionDate.After(*to) {
			continue
		}
		if r.AttendanceConfirmedAt != nil {
			continue
		}
		if r.Status != StatusHeld && r.Status != StatusPlanned {
			continue
		}
		out = append(out, PendingRow{Session: r.Session, ClassName: f.row(r).ClassName})
	}
	total := int64(len(out))
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, total, nil
}

// newPendingTestService builds a Service directly (bypassing NewService, but
// legal from within package sessions) with only the dependencies
// Service.ListPending touches: the repo and the teacher timezone source.
// fixedNow pins "now" so the resolved cutoff is deterministic.
func newPendingTestService(repo *fakePendingRepo, teacherID uuid.UUID, timezone string, fixedNow time.Time) *Service {
	teachersSrc := newFakeTeacherSource()
	teachersSrc.addTeacher(teacherID, timezone)
	return &Service{repo: repo, teachers: teachersSrc, now: func() time.Time { return fixedNow }}
}

func TestListPendingCutoffUsesTeacherTimezone(t *testing.T) {
	teacherID := id.New()
	// 2026-03-10 02:00 UTC is still 2026-03-09 evening in a large negative
	// offset zone (America/Los_Angeles, UTC-8 in March before DST) — the
	// cutoff must reflect the teacher's calendar day, not UTC's.
	fixedNow := time.Date(2026, 3, 10, 2, 0, 0, 0, time.UTC)
	repo := &fakePendingRepo{}
	svc := newPendingTestService(repo, teacherID, "America/Los_Angeles", fixedNow)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}

	if _, err := svc.ListPending(context.Background(), sc, nil, nil, 0); err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	want := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	if !repo.gotBefore.Equal(want) {
		t.Fatalf("cutoff must be today in the teacher's timezone, got %v want %v", repo.gotBefore, want)
	}
}

func TestListPendingCutoffShiftsWithNonDefaultTimezone(t *testing.T) {
	teacherID := id.New()
	// A large positive offset (Pacific/Kiritimati, UTC+14) pushes the
	// teacher's calendar day a full day ahead of the default
	// Asia/Ho_Chi_Minh (UTC+7) zone for the same instant.
	fixedNow := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}

	repoDefault := &fakePendingRepo{}
	svcDefault := newPendingTestService(repoDefault, teacherID, "Asia/Ho_Chi_Minh", fixedNow)
	if _, err := svcDefault.ListPending(context.Background(), sc, nil, nil, 0); err != nil {
		t.Fatalf("ListPending (default tz): %v", err)
	}

	repoFar := &fakePendingRepo{}
	svcFar := newPendingTestService(repoFar, teacherID, "Pacific/Kiritimati", fixedNow)
	if _, err := svcFar.ListPending(context.Background(), sc, nil, nil, 0); err != nil {
		t.Fatalf("ListPending (far tz): %v", err)
	}

	if !repoFar.gotBefore.After(repoDefault.gotBefore) {
		t.Fatalf("a teacher further east must see a later cutoff for the same instant: got %v, default %v",
			repoFar.gotBefore, repoDefault.gotBefore)
	}
	wantDefault := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	wantFar := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	if !repoDefault.gotBefore.Equal(wantDefault) {
		t.Fatalf("default tz cutoff = %v, want %v", repoDefault.gotBefore, wantDefault)
	}
	if !repoFar.gotBefore.Equal(wantFar) {
		t.Fatalf("far tz cutoff = %v, want %v", repoFar.gotBefore, wantFar)
	}
}

func TestListPendingDefaultsAndCapsLimit(t *testing.T) {
	teacherID := id.New()
	fixedNow := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}

	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{"unset falls back to default", 0, defaultPendingLimit},
		{"negative falls back to default", -5, defaultPendingLimit},
		{"within range passes through", 10, 10},
		{"over the cap is clamped", 9999, maxPendingLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakePendingRepo{}
			svc := newPendingTestService(repo, teacherID, "Asia/Ho_Chi_Minh", fixedNow)
			if _, err := svc.ListPending(context.Background(), sc, nil, nil, tc.limit); err != nil {
				t.Fatalf("ListPending: %v", err)
			}
			if repo.gotLimit != tc.want {
				t.Fatalf("limit = %d, want %d", repo.gotLimit, tc.want)
			}
		})
	}
}

func TestListPendingPassesFromToThrough(t *testing.T) {
	teacherID := id.New()
	fixedNow := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakePendingRepo{}
	svc := newPendingTestService(repo, teacherID, "Asia/Ho_Chi_Minh", fixedNow)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}

	from := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
	if _, err := svc.ListPending(context.Background(), sc, &from, &to, 0); err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if repo.gotFrom == nil || !repo.gotFrom.Equal(from) {
		t.Fatalf("from must pass through unchanged, got %v", repo.gotFrom)
	}
	if repo.gotTo == nil || !repo.gotTo.Equal(to) {
		t.Fatalf("to must pass through unchanged, got %v", repo.gotTo)
	}
}

func TestListPendingMapsTotalAndDaysOverdue(t *testing.T) {
	teacherID := id.New()
	classID := id.New()
	fixedNow := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC) // -> cutoff 2026-03-10 UTC in Asia/Ho_Chi_Minh
	startTime := classes.TimeOfDay("18:00")

	session1 := Session{
		ID: id.New(), TeacherID: teacherID, ClassID: classID,
		SessionDate: time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), // 1 day overdue
		StartTime:   &startTime, Status: StatusPlanned,
	}
	session2 := Session{
		ID: id.New(), TeacherID: teacherID, ClassID: classID,
		SessionDate: time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC), // 5 days overdue
		Status:      StatusHeld,
	}
	repo := &fakePendingRepo{
		rows: []PendingRow{
			{Session: session1, ClassName: "Toán 8", ExpectedStudentCount: 4},
			{Session: session2, ClassName: "Toán 8", ExpectedStudentCount: 3},
		},
		total: 7, // more than len(rows): total must reflect the unlimited count
	}
	svc := newPendingTestService(repo, teacherID, "Asia/Ho_Chi_Minh", fixedNow)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}

	out, err := svc.ListPending(context.Background(), sc, nil, nil, 2)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if out.Total != 7 {
		t.Fatalf("Total = %d, want 7 (unlimited count, independent of returned rows)", out.Total)
	}
	if len(out.Items) != 2 {
		t.Fatalf("Items = %d, want 2", len(out.Items))
	}
	if out.Items[0].SessionID != session1.ID || out.Items[0].DaysOverdue != 1 {
		t.Fatalf("session1 DaysOverdue = %d, want 1 (row %+v)", out.Items[0].DaysOverdue, out.Items[0])
	}
	if out.Items[0].StartTime == nil || *out.Items[0].StartTime != "18:00" {
		t.Fatalf("session1 StartTime must round-trip, got %v", out.Items[0].StartTime)
	}
	if out.Items[0].ClassID != classID || out.Items[0].ClassName != "Toán 8" {
		t.Fatalf("session1 must carry class id and name, got %+v", out.Items[0])
	}
	if out.Items[0].ExpectedStudentCount != 4 {
		t.Fatalf("session1 ExpectedStudentCount = %d, want 4", out.Items[0].ExpectedStudentCount)
	}
	if out.Items[1].SessionID != session2.ID || out.Items[1].DaysOverdue != 5 {
		t.Fatalf("session2 DaysOverdue = %d, want 5 (row %+v)", out.Items[1].DaysOverdue, out.Items[1])
	}
	if out.Items[1].StartTime != nil {
		t.Fatalf("session2 has no start_time, must stay nil, got %v", *out.Items[1].StartTime)
	}
}

func TestListPendingInvalidTimezoneFallsBackToUTC(t *testing.T) {
	teacherID := id.New()
	fixedNow := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakePendingRepo{}
	svc := newPendingTestService(repo, teacherID, "not-a-real-zone", fixedNow)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}

	if _, err := svc.ListPending(context.Background(), sc, nil, nil, 0); err != nil {
		t.Fatalf("an invalid stored timezone must not fail the request, got %v", err)
	}
	want := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	if !repo.gotBefore.Equal(want) {
		t.Fatalf("cutoff with fallback UTC zone = %v, want %v", repo.gotBefore, want)
	}
}

func TestListPendingMissingTeacherPropagatesNotFound(t *testing.T) {
	teacherID := id.New()
	fixedNow := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakePendingRepo{}
	// No addTeacher call: the fake teacher source has nothing for teacherID.
	teachersSrc := newFakeTeacherSource()
	svc := &Service{repo: repo, teachers: teachersSrc, now: func() time.Time { return fixedNow }}
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}

	_, err := svc.ListPending(context.Background(), sc, nil, nil, 0)
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("missing teacher must surface as 404, got %v", err)
	}
}

func TestListPendingRepositoryErrorIsInternal(t *testing.T) {
	teacherID := id.New()
	fixedNow := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakePendingRepo{err: errors.New("boom")}
	svc := newPendingTestService(repo, teacherID, "Asia/Ho_Chi_Minh", fixedNow)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}

	_, err := svc.ListPending(context.Background(), sc, nil, nil, 0)
	if apperror.From(err).Code != apperror.CodeInternal {
		t.Fatalf("repository error must surface as internal, got %v", err)
	}
}
