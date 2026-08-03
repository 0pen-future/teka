package classes

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

type fakeClass struct {
	Class
	deleted bool
}

type fakeSchedule struct {
	Schedule
	deleted bool
}

// fakeRepository is an in-memory Repository enforcing the same invariants the
// SQL layer does: tenant-scoped reads and soft-delete filtering.
type fakeRepository struct {
	classes         map[uuid.UUID]*fakeClass
	schedules       map[uuid.UUID]*fakeSchedule
	openEnrollments map[uuid.UUID]int64 // classID -> open enrollment count
	failCreate      error               // forces CreateWithSchedules to fail
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		classes:         map[uuid.UUID]*fakeClass{},
		schedules:       map[uuid.UUID]*fakeSchedule{},
		openEnrollments: map[uuid.UUID]int64{},
	}
}

// noopTx satisfies database.TxManager without a database; the fake repository
// has no partial-failure mode, so a passthrough is faithful.
type noopTx struct{}

func (noopTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func newTestService() (*Service, *fakeRepository) {
	repo := newFakeRepository()
	return NewService(repo, noopTx{}), repo
}

func (f *fakeRepository) CreateWithSchedules(_ context.Context, class *Class, schedules []Schedule) error {
	if f.failCreate != nil {
		return f.failCreate
	}
	f.classes[class.ID] = &fakeClass{Class: *class}
	for i := range schedules {
		f.schedules[schedules[i].ID] = &fakeSchedule{Schedule: schedules[i]}
	}
	return nil
}

func (f *fakeRepository) liveSchedules(classID uuid.UUID) []Schedule {
	var out []Schedule
	for _, s := range f.schedules {
		if !s.deleted && s.ClassID == classID {
			out = append(out, s.Schedule)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EffectiveFrom.Before(out[j].EffectiveFrom) })
	return out
}

func (f *fakeRepository) GetByID(_ context.Context, teacherID, classID uuid.UUID) (*Class, error) {
	c, ok := f.classes[classID]
	if !ok || c.deleted || c.TeacherID != teacherID {
		return nil, ErrNotFound
	}
	out := c.Class
	out.Schedules = f.liveSchedules(classID)
	return &out, nil
}

func (f *fakeRepository) List(_ context.Context, teacherID uuid.UUID, filter ListFilter, _ pagination.Params) ([]Class, int64, error) {
	var out []Class
	for _, c := range f.classes {
		if c.deleted || c.TeacherID != teacherID {
			continue
		}
		if filter.Status != "" && c.Status != filter.Status {
			continue
		}
		row := c.Class
		row.Schedules = f.liveSchedules(c.ID)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, int64(len(out)), nil
}

func (f *fakeRepository) Update(_ context.Context, class *Class) error {
	stored := *class
	stored.Schedules = nil
	f.classes[class.ID] = &fakeClass{Class: stored}
	return nil
}

func (f *fakeRepository) Archive(_ context.Context, teacherID, classID uuid.UUID) error {
	c, ok := f.classes[classID]
	if !ok || c.deleted || c.TeacherID != teacherID {
		return ErrNotFound
	}
	c.Status = StatusArchived
	return nil
}

func (f *fakeRepository) SoftDelete(_ context.Context, teacherID, classID uuid.UUID) error {
	c, ok := f.classes[classID]
	if !ok || c.deleted || c.TeacherID != teacherID {
		return ErrNotFound
	}
	c.deleted = true
	return nil
}

func (f *fakeRepository) CountOpenEnrollments(_ context.Context, _, classID uuid.UUID) (int64, error) {
	return f.openEnrollments[classID], nil
}

func (f *fakeRepository) AddSchedule(_ context.Context, s *Schedule) error {
	f.schedules[s.ID] = &fakeSchedule{Schedule: *s}
	return nil
}

func (f *fakeRepository) GetSchedule(_ context.Context, teacherID, classID, scheduleID uuid.UUID) (*Schedule, error) {
	s, ok := f.schedules[scheduleID]
	if !ok || s.deleted || s.TeacherID != teacherID || s.ClassID != classID {
		return nil, ErrScheduleNotFound
	}
	out := s.Schedule
	return &out, nil
}

func (f *fakeRepository) UpdateSchedule(_ context.Context, s *Schedule) error {
	f.schedules[s.ID] = &fakeSchedule{Schedule: *s}
	return nil
}

func (f *fakeRepository) SoftDeleteSchedule(_ context.Context, teacherID, classID, scheduleID uuid.UUID) error {
	s, ok := f.schedules[scheduleID]
	if !ok || s.deleted || s.TeacherID != teacherID || s.ClassID != classID {
		return ErrScheduleNotFound
	}
	s.deleted = true
	return nil
}

func (f *fakeRepository) ListEffectiveSchedules(_ context.Context, teacherID, classID uuid.UUID, from, to time.Time) ([]Schedule, error) {
	var out []Schedule
	for _, s := range f.schedules {
		if s.deleted || s.TeacherID != teacherID || s.ClassID != classID {
			continue
		}
		if s.EffectiveFrom.After(to) {
			continue
		}
		if s.EffectiveTo != nil && s.EffectiveTo.Before(from) {
			continue
		}
		out = append(out, s.Schedule)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EffectiveFrom.Before(out[j].EffectiveFrom) })
	return out, nil
}

func int16Ptr(v int16) *int16 { return &v }
func int64Ptr(v int64) *int64 { return &v }

func validCreateRequest() CreateClassRequest {
	return CreateClassRequest{
		Name:             "Toán 8",
		StartDate:        "2026-01-05",
		DefaultUnitPrice: int64Ptr(150_000),
		Schedules: []ScheduleRequest{
			{Weekday: int16Ptr(2), StartTime: "18:00", DurationMin: 90},
		},
	}
}

func TestCreateDefaultsScheduleEffectiveFrom(t *testing.T) {
	svc, _ := newTestService()
	teacherID := id.New()

	class, err := svc.Create(context.Background(), teacherID, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if class.Status != StatusActive {
		t.Fatalf("new class must be active, got %q", class.Status)
	}
	if len(class.Schedules) != 1 {
		t.Fatalf("want 1 schedule, got %d", len(class.Schedules))
	}
	s := class.Schedules[0]
	if got := s.EffectiveFrom.Format(dateLayout); got != "2026-01-05" {
		t.Fatalf("effective_from must default to the class start date, got %s", got)
	}
	if s.TeacherID != teacherID || s.ClassID != class.ID {
		t.Fatalf("schedule must carry the class's tenant and id, got %+v", s)
	}
	if s.EffectiveTo != nil {
		t.Fatalf("schedule must stay open-ended, got %v", s.EffectiveTo)
	}
}

func TestCreateKeepsExplicitEffectiveFrom(t *testing.T) {
	svc, _ := newTestService()
	req := validCreateRequest()
	req.Schedules[0].EffectiveFrom = "2026-02-01"

	class, err := svc.Create(context.Background(), id.New(), req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := class.Schedules[0].EffectiveFrom.Format(dateLayout); got != "2026-02-01" {
		t.Fatalf("explicit effective_from must win, got %s", got)
	}
}

func TestCreatePropagatesRepositoryFailure(t *testing.T) {
	svc, repo := newTestService()
	boom := errors.New("insert failed")
	repo.failCreate = boom

	if _, err := svc.Create(context.Background(), id.New(), validCreateRequest()); !errors.Is(err, boom) {
		t.Fatalf("want the repository error, got %v", err)
	}
	if len(repo.classes) != 0 {
		t.Fatalf("failed create must leave no class behind, got %d", len(repo.classes))
	}
}

func TestDeleteBlockedByOpenEnrollments(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	class, err := svc.Create(context.Background(), teacherID, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	repo.openEnrollments[class.ID] = 3

	err = svc.Delete(context.Background(), teacherID, class.ID)
	if !errors.Is(err, ErrHasOpenEnrollments) {
		t.Fatalf("want ErrHasOpenEnrollments cause, got %v", err)
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeConflict {
		t.Fatalf("want CONFLICT, got %v", err)
	}
	if msg := appErr.Message; msg != "class has 3 open enrollment(s); archive the class instead of deleting it" {
		t.Fatalf("message must count enrollments and suggest archiving, got %q", msg)
	}
}

func TestDeleteWithoutEnrollmentsSoftDeletes(t *testing.T) {
	svc, _ := newTestService()
	teacherID := id.New()
	class, err := svc.Create(context.Background(), teacherID, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.Delete(context.Background(), teacherID, class.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var appErr *apperror.AppError
	if _, err := svc.Get(context.Background(), teacherID, class.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("deleted class must read as not found, got %v", err)
	}
}

func TestArchiveIsIdempotent(t *testing.T) {
	svc, _ := newTestService()
	teacherID := id.New()
	class, err := svc.Create(context.Background(), teacherID, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	for range 2 {
		archived, err := svc.Archive(context.Background(), teacherID, class.ID)
		if err != nil {
			t.Fatalf("archive: %v", err)
		}
		if archived.Status != StatusArchived {
			t.Fatalf("want archived, got %q", archived.Status)
		}
	}
}

func TestCrossTenantReadsAsNotFound(t *testing.T) {
	svc, _ := newTestService()
	owner := id.New()
	class, err := svc.Create(context.Background(), owner, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	stranger := id.New()
	var appErr *apperror.AppError
	if _, err := svc.Get(context.Background(), stranger, class.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be not found, got %v", err)
	}
	if err := svc.Delete(context.Background(), stranger, class.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant delete must be NOT_FOUND, got %v", err)
	}
}

func TestUpdateScheduleClosesRow(t *testing.T) {
	svc, _ := newTestService()
	teacherID := id.New()
	class, err := svc.Create(context.Background(), teacherID, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	scheduleID := class.Schedules[0].ID

	updated, err := svc.UpdateSchedule(context.Background(), teacherID, class.ID, scheduleID, UpdateScheduleRequest{
		Weekday:       int16Ptr(2),
		StartTime:     "18:00",
		DurationMin:   90,
		EffectiveFrom: "2026-01-05",
		EffectiveTo:   "2026-03-31",
	})
	if err != nil {
		t.Fatalf("update schedule: %v", err)
	}
	if updated.EffectiveTo == nil || updated.EffectiveTo.Format(dateLayout) != "2026-03-31" {
		t.Fatalf("closing must set effective_to, got %v", updated.EffectiveTo)
	}
}

func TestAddScheduleDefaultsEffectiveFromToClassStart(t *testing.T) {
	svc, _ := newTestService()
	teacherID := id.New()
	class, err := svc.Create(context.Background(), teacherID, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	added, err := svc.AddSchedule(context.Background(), teacherID, class.ID, ScheduleRequest{
		Weekday: int16Ptr(0), StartTime: "08:30", DurationMin: 60,
	})
	if err != nil {
		t.Fatalf("add schedule: %v", err)
	}
	if got := added.EffectiveFrom.Format(dateLayout); got != "2026-01-05" {
		t.Fatalf("effective_from must default to the class start date, got %s", got)
	}
	if added.Weekday != 0 {
		t.Fatalf("weekday 0 (Sunday) must survive, got %d", added.Weekday)
	}
}

func TestScheduleOpsOnUnknownScheduleAreNotFound(t *testing.T) {
	svc, _ := newTestService()
	teacherID := id.New()
	class, err := svc.Create(context.Background(), teacherID, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	var appErr *apperror.AppError
	err = svc.DeleteSchedule(context.Background(), teacherID, class.ID, id.New())
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("unknown schedule delete must be NOT_FOUND, got %v", err)
	}
}
