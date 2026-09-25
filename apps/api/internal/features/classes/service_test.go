package classes

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
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

// fakeMember is one directory entry ActiveMemberDirectory serves, tagged with
// the center it belongs to so lookups can be center-scoped like the real join.
type fakeMember struct {
	DirectoryMember
	centerID uuid.UUID
}

// fakeRepository is an in-memory Repository enforcing the same invariants the
// SQL layer does: center-scoped reads (owner sees the whole center, a member
// only their own rows) and soft-delete filtering.
type fakeRepository struct {
	classes         map[uuid.UUID]*fakeClass
	schedules       map[uuid.UUID]*fakeSchedule
	openEnrollments map[uuid.UUID]int64      // classID -> open enrollment count
	failCreate      error                    // forces CreateWithSchedules to fail
	failUpdate      error                    // forces Update to fail
	codeCollisions  int                      // CodeExists reports a hit this many times first
	courses         map[uuid.UUID]*CourseRef // live courses per id, with their center
	courseCenters   map[uuid.UUID]uuid.UUID
	staff           map[uuid.UUID][]uuid.UUID // classID -> active class_staff teacher ids
	members         []fakeMember              // center directory rows, for ActiveMemberDirectory
}

// addStaff registers classID's active class_staff teacher ids, for
// AvailabilityClasses' busy-teacher fold.
func (f *fakeRepository) addStaff(classID uuid.UUID, teacherIDs ...uuid.UUID) {
	f.staff[classID] = append(f.staff[classID], teacherIDs...)
}

// addMember registers an active center member so ActiveMemberDirectory lists
// it; teacherID is caller-supplied so a test can register the directory
// entry for a class's own teacher.
func (f *fakeRepository) addMember(center, teacherID uuid.UUID, name string) {
	f.members = append(f.members, fakeMember{
		DirectoryMember: DirectoryMember{TeacherID: teacherID, Name: name},
		centerID:        center,
	})
}

// addCourse registers a live course of center so FindCourse resolves it.
func (f *fakeRepository) addCourse(center uuid.UUID, code string, price int64) *CourseRef {
	ref := &CourseRef{ID: uuid.New(), Code: code, Name: "Khóa " + code, Status: "active", DefaultUnitPrice: price}
	f.courses[ref.ID] = ref
	f.courseCenters[ref.ID] = center
	return ref
}

func (f *fakeRepository) LiveClassInCenter(_ context.Context, a authctx.Anchor, classID uuid.UUID) (bool, error) {
	c, ok := f.classes[classID]
	return ok && !c.deleted && c.CenterID == a.CenterID, nil
}

// ParentCreatesCycle mirrors the real recursive walk: follow parentID's own
// chain of parents and report whether selfID turns up anywhere in it.
func (f *fakeRepository) ParentCreatesCycle(_ context.Context, a authctx.Anchor, parentID, selfID uuid.UUID) (bool, error) {
	cur := parentID
	seen := map[uuid.UUID]bool{}
	for i := 0; i < 50; i++ {
		if cur == selfID {
			return true, nil
		}
		if seen[cur] {
			return false, nil
		}
		seen[cur] = true
		c, ok := f.classes[cur]
		if !ok || c.deleted || c.CenterID != a.CenterID || c.ParentClassID == nil {
			return false, nil
		}
		cur = *c.ParentClassID
	}
	return false, nil
}

func (f *fakeRepository) FindCourse(_ context.Context, a authctx.Anchor, courseID uuid.UUID) (*CourseRef, error) {
	ref, ok := f.courses[courseID]
	if !ok || f.courseCenters[courseID] != a.CenterID {
		return nil, ErrCourseNotFound
	}
	cp := *ref
	return &cp, nil
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		classes:         map[uuid.UUID]*fakeClass{},
		schedules:       map[uuid.UUID]*fakeSchedule{},
		openEnrollments: map[uuid.UUID]int64{},
		courses:         map[uuid.UUID]*CourseRef{},
		courseCenters:   map[uuid.UUID]uuid.UUID{},
		staff:           map[uuid.UUID][]uuid.UUID{},
	}
}

// noopTx satisfies database.TxManager without a database; the fake repository
// has no partial-failure mode, so a passthrough is faithful.
type noopTx struct{}

func (noopTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// noopStaffSeeder satisfies StaffSeeder without touching class_staff; the
// unit fakes carry no staff table, so the seed is a no-op here and covered by
// the classstaff integration tests.
type noopStaffSeeder struct{}

func (noopStaffSeeder) SyncPrimaryTeacher(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

func (noopStaffSeeder) RolesByClass(context.Context, uuid.UUID, uuid.UUID, []uuid.UUID) (map[uuid.UUID][]string, error) {
	return map[uuid.UUID][]string{}, nil
}

func newTestService() (*Service, *fakeRepository) {
	repo := newFakeRepository()
	return NewService(repo, noopTx{}, noopStaffSeeder{}), repo
}

// ownerScope returns a scope for a teacher who owns their own center.
func ownerScope() authctx.Scope {
	return authctx.Scope{TeacherID: id.New(), CenterID: id.New(), IsOwner: true}
}

// memberScope returns a scope for a non-owning member of some center.
func memberScope() authctx.Scope {
	return authctx.Scope{TeacherID: id.New(), CenterID: id.New(), IsOwner: false}
}

// visibleClass mirrors the real scoped() predicate: always the center, plus
// the teacher when the caller is not an owner.
func visibleClass(c *fakeClass, sc authctx.Scope) bool {
	if c.deleted || c.CenterID != sc.CenterID {
		return false
	}
	return sc.IsOwner || c.TeacherID == sc.TeacherID
}

// visibleSchedule is visibleClass's counterpart for schedule rows.
func visibleSchedule(s *fakeSchedule, sc authctx.Scope) bool {
	if s.deleted || s.CenterID != sc.CenterID {
		return false
	}
	return sc.IsOwner || s.TeacherID == sc.TeacherID
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

func (f *fakeRepository) GetByID(_ context.Context, sc authctx.Scope, classID uuid.UUID) (*Class, error) {
	c, ok := f.classes[classID]
	if !ok || !visibleClass(c, sc) {
		return nil, ErrNotFound
	}
	out := c.Class
	out.Schedules = f.liveSchedules(classID)
	out.NextClassID = f.nextChildID(classID)
	return &out, nil
}

// nextChildID mirrors the real repository's earliest-created-live-child
// lookup (ties broken by id), so a read through GetByID reflects a
// previously linked next class the same way the SQL layer would.
func (f *fakeRepository) nextChildID(classID uuid.UUID) *uuid.UUID {
	var best *fakeClass
	for _, c := range f.classes {
		if c.deleted || c.ParentClassID == nil || *c.ParentClassID != classID {
			continue
		}
		if best == nil || c.CreatedAt.Before(best.CreatedAt) ||
			(c.CreatedAt.Equal(best.CreatedAt) && c.ID.String() < best.ID.String()) {
			best = c
		}
	}
	if best == nil {
		return nil
	}
	childID := best.ID
	return &childID
}

// The unit fakes carry no class_staff table, so the readable ports collapse
// onto the own-rows ones; the widened behavior is covered by integration
// tests.
func (f *fakeRepository) GetReadableByID(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*Class, error) {
	return f.GetByID(ctx, sc, classID)
}

func (f *fakeRepository) ListReadable(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Class, int64, error) {
	return f.List(ctx, sc, filter, p)
}

func (f *fakeRepository) GetWritableByID(ctx context.Context, sc authctx.Scope, classID uuid.UUID, _ []string) (*Class, error) {
	return f.GetByID(ctx, sc, classID)
}

func (f *fakeRepository) List(_ context.Context, sc authctx.Scope, filter ListFilter, _ pagination.Params) ([]Class, int64, error) {
	today := Today()
	var out []Class
	for _, c := range f.classes {
		if !visibleClass(c, sc) {
			continue
		}
		if filter.Status != "" && c.Status != filter.Status {
			continue
		}
		if filter.Phase != "" && PhaseOf(&c.Class, today) != filter.Phase {
			continue
		}
		if filter.Q != "" && !containsFold(c.Name, filter.Q) && !containsFold(c.Code, filter.Q) {
			continue
		}
		if filter.Tag != "" && !slices.Contains(c.Tags, filter.Tag) {
			continue
		}
		if filter.CourseID != nil && (c.CourseID == nil || *c.CourseID != *filter.CourseID) {
			continue
		}
		if filter.Recruiting && !OpenForRecruitment(&c.Class, today) {
			continue
		}
		row := c.Class
		row.Schedules = f.liveSchedules(c.ID)
		if (filter.Weekday != nil || filter.Shift != "") && !hasEffectiveSlot(row.Schedules, filter, today) {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, int64(len(out)), nil
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// hasEffectiveSlot mirrors the SQL EXISTS over class_schedules: a slot still
// effective today (open-ended or ending on/after today) on the requested
// weekday and within the requested shift band.
func hasEffectiveSlot(schedules []Schedule, filter ListFilter, today time.Time) bool {
	for _, s := range schedules {
		if s.EffectiveTo != nil && s.EffectiveTo.Before(today) {
			continue
		}
		if filter.Weekday != nil && s.Weekday != *filter.Weekday {
			continue
		}
		if filter.Shift != "" && ShiftOf(s.StartTime) != filter.Shift {
			continue
		}
		return true
	}
	return false
}

// CountReadableByPhase mirrors the SQL SUM(CASE ...) over the same phase
// predicate the list filter applies, so the two can never disagree here.
func (f *fakeRepository) CountReadableByPhase(_ context.Context, sc authctx.Scope, today time.Time, recruitingOnly bool) (ClassStatsResponse, error) {
	var stats ClassStatsResponse
	for _, c := range f.classes {
		if !visibleClass(c, sc) {
			continue
		}
		if recruitingOnly && !OpenForRecruitment(&c.Class, today) {
			continue
		}
		stats.All++
		switch PhaseOf(&c.Class, today) {
		case PhaseUpcoming:
			stats.Upcoming++
		case PhaseRunning:
			stats.Running++
		case PhaseEnded:
			stats.Ended++
		case PhaseArchived:
			stats.Archived++
		}
		if OpenForRecruitment(&c.Class, today) {
			stats.Recruiting++
		}
	}
	return stats, nil
}

// CodeExists mirrors the partial unique index: live rows only, whole center,
// the row being edited excluded. codeCollisions forces the first N probes to
// report a hit so the generate-and-retry loop can be exercised.
func (f *fakeRepository) CodeExists(_ context.Context, a authctx.Anchor, code string, exceptID uuid.UUID) (bool, error) {
	if f.codeCollisions > 0 {
		f.codeCollisions--
		return true, nil
	}
	for _, c := range f.classes {
		if !c.deleted && c.CenterID == a.CenterID && c.ID != exceptID && c.Code == code {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepository) Update(_ context.Context, class *Class) error {
	if f.failUpdate != nil {
		return f.failUpdate
	}
	stored := *class
	stored.Schedules = nil
	f.classes[class.ID] = &fakeClass{Class: stored}
	return nil
}

// LinkNextClass mirrors the real repository: unlink every other live class of
// the center currently pointing at classID, then, when childID is set, point
// it at classID — reporting ErrNotFound when childID does not resolve to a
// live class of the same center.
func (f *fakeRepository) LinkNextClass(_ context.Context, a authctx.Anchor, classID uuid.UUID, childID *uuid.UUID) error {
	for _, c := range f.classes {
		if c.deleted || c.CenterID != a.CenterID || c.ParentClassID == nil || *c.ParentClassID != classID {
			continue
		}
		if childID != nil && c.ID == *childID {
			continue
		}
		c.ParentClassID = nil
	}
	if childID == nil {
		return nil
	}
	child, ok := f.classes[*childID]
	if !ok || child.deleted || child.CenterID != a.CenterID {
		return ErrNotFound
	}
	parent := classID
	child.ParentClassID = &parent
	return nil
}

// AvailabilityClasses mirrors the real repository: every live class of the
// center (excluding excludeClassID when set) with its room, teacher, active
// schedules (effective today or later) and registered class_staff ids.
func (f *fakeRepository) AvailabilityClasses(_ context.Context, sc authctx.Scope, excludeClassID *uuid.UUID) ([]AvailabilityClassRow, error) {
	today := Today()
	var out []AvailabilityClassRow
	for _, c := range f.classes {
		if c.deleted || c.CenterID != sc.CenterID {
			continue
		}
		if excludeClassID != nil && c.ID == *excludeClassID {
			continue
		}
		var schedules []AvailabilitySchedule
		for _, s := range f.liveSchedules(c.ID) {
			if s.EffectiveTo != nil && s.EffectiveTo.Before(today) {
				continue
			}
			schedules = append(schedules, AvailabilitySchedule{
				Weekday: s.Weekday, StartTime: s.StartTime, DurationMin: s.DurationMin,
			})
		}
		out = append(out, AvailabilityClassRow{
			ClassID:   c.ID,
			Room:      c.Room,
			TeacherID: c.TeacherID,
			Schedules: schedules,
			StaffIDs:  f.staff[c.ID],
		})
	}
	return out, nil
}

// ActiveMemberDirectory mirrors the real repository: every registered member
// of the caller's center, for the availability endpoint's teacher list.
func (f *fakeRepository) ActiveMemberDirectory(_ context.Context, sc authctx.Scope) ([]DirectoryMember, error) {
	var out []DirectoryMember
	for _, m := range f.members {
		if m.centerID == sc.CenterID {
			out = append(out, m.DirectoryMember)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeRepository) Archive(_ context.Context, sc authctx.Scope, classID uuid.UUID) error {
	c, ok := f.classes[classID]
	if !ok || !visibleClass(c, sc) {
		return ErrNotFound
	}
	c.Status = StatusArchived
	return nil
}

func (f *fakeRepository) SoftDelete(_ context.Context, sc authctx.Scope, classID uuid.UUID) error {
	c, ok := f.classes[classID]
	if !ok || !visibleClass(c, sc) {
		return ErrNotFound
	}
	c.deleted = true
	return nil
}

func (f *fakeRepository) ReassignTeacher(_ context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID) error {
	c, ok := f.classes[classID]
	if !ok || !visibleClass(c, sc) {
		return ErrNotFound
	}
	c.TeacherID = newTeacherID
	return nil
}

func (f *fakeRepository) CountOpenEnrollments(_ context.Context, _ authctx.Scope, classID uuid.UUID) (int64, error) {
	return f.openEnrollments[classID], nil
}

func (f *fakeRepository) CountActiveEnrollmentsByClass(_ context.Context, _ authctx.Scope, classIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := map[uuid.UUID]int64{}
	for _, classID := range classIDs {
		if n := f.openEnrollments[classID]; n > 0 {
			out[classID] = n
		}
	}
	return out, nil
}

func (f *fakeRepository) AddSchedule(_ context.Context, s *Schedule) error {
	f.schedules[s.ID] = &fakeSchedule{Schedule: *s}
	return nil
}

func (f *fakeRepository) GetSchedule(_ context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) (*Schedule, error) {
	s, ok := f.schedules[scheduleID]
	if !ok || !visibleSchedule(s, sc) || s.ClassID != classID {
		return nil, ErrScheduleNotFound
	}
	out := s.Schedule
	return &out, nil
}

func (f *fakeRepository) UpdateSchedule(_ context.Context, s *Schedule) error {
	f.schedules[s.ID] = &fakeSchedule{Schedule: *s}
	return nil
}

func (f *fakeRepository) SoftDeleteSchedule(_ context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) error {
	s, ok := f.schedules[scheduleID]
	if !ok || !visibleSchedule(s, sc) || s.ClassID != classID {
		return ErrScheduleNotFound
	}
	s.deleted = true
	return nil
}

func (f *fakeRepository) ListEffectiveSchedules(_ context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Schedule, error) {
	var out []Schedule
	for _, s := range f.schedules {
		if !visibleSchedule(s, sc) || s.ClassID != classID {
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
	sc := memberScope()

	class, err := svc.Create(context.Background(), sc, validCreateRequest())
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
	if s.TeacherID != sc.TeacherID || s.ClassID != class.ID {
		t.Fatalf("schedule must carry the class's tenant and id, got %+v", s)
	}
	if s.CenterID != sc.CenterID {
		t.Fatalf("schedule must carry the class's center, got %+v", s)
	}
	if s.EffectiveTo != nil {
		t.Fatalf("schedule must stay open-ended, got %v", s.EffectiveTo)
	}
}

func TestCreateKeepsExplicitEffectiveFrom(t *testing.T) {
	svc, _ := newTestService()
	req := validCreateRequest()
	req.Schedules[0].EffectiveFrom = "2026-02-01"

	class, err := svc.Create(context.Background(), memberScope(), req)
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

	if _, err := svc.Create(context.Background(), memberScope(), validCreateRequest()); !errors.Is(err, boom) {
		t.Fatalf("want the repository error, got %v", err)
	}
	if len(repo.classes) != 0 {
		t.Fatalf("failed create must leave no class behind, got %d", len(repo.classes))
	}
}

// The pre-insert CodeExists probe and the partial unique index can disagree
// under a concurrent create or update of the same code; the index wins, and
// its duplicate-key error must surface as the same 409 the probe produces
// rather than as a 500.
func TestDuplicateKeyFromIndexIsCodeTakenConflict(t *testing.T) {
	svc, repo := newTestService()
	sc := memberScope()

	req := validCreateRequest()
	req.Code = strPtr("TOAN8")
	repo.failCreate = gorm.ErrDuplicatedKey
	_, err := svc.Create(context.Background(), sc, req)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 || appErr.Code != CodeClassCodeTaken || !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate key on create must be 409 %s, got %v", CodeClassCodeTaken, err)
	}

	repo.failCreate = nil
	class, err := svc.Create(context.Background(), sc, req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	repo.failUpdate = gorm.ErrDuplicatedKey
	patch := UpdateClassRequest{Name: "Toán 8", StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000), Code: strPtr("TOAN9")}
	_, err = svc.Update(context.Background(), sc, class.ID, patch)
	if !errors.As(err, &appErr) || appErr.Status != 409 || appErr.Code != CodeClassCodeTaken || appErr.Fields["code"] == "" {
		t.Fatalf("duplicate key on update must be 409 %s on the code field, got %v", CodeClassCodeTaken, err)
	}
}

func TestDeleteBlockedByOpenEnrollments(t *testing.T) {
	svc, repo := newTestService()
	sc := memberScope()
	class, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	repo.openEnrollments[class.ID] = 3

	err = svc.Delete(context.Background(), sc, class.ID)
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
	sc := memberScope()
	class, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.Delete(context.Background(), sc, class.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var appErr *apperror.AppError
	if _, err := svc.Get(context.Background(), sc, class.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("deleted class must read as not found, got %v", err)
	}
}

func TestArchiveIsIdempotent(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()
	class, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	for range 2 {
		archived, err := svc.Archive(context.Background(), sc, class.ID)
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
	owner := memberScope()
	class, err := svc.Create(context.Background(), owner, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	stranger := memberScope()
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
	sc := memberScope()
	class, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	scheduleID := class.Schedules[0].ID

	updated, err := svc.UpdateSchedule(context.Background(), sc, class.ID, scheduleID, UpdateScheduleRequest{
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
	sc := memberScope()
	class, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	added, err := svc.AddSchedule(context.Background(), sc, class.ID, ScheduleRequest{
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
	sc := memberScope()
	class, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	var appErr *apperror.AppError
	err = svc.DeleteSchedule(context.Background(), sc, class.ID, id.New())
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("unknown schedule delete must be NOT_FOUND, got %v", err)
	}
}

// An owner reads, updates, and manages a member's class — center oversight,
// not per-teacher isolation. A schedule the owner adds still inherits the
// parent class's own teacher, not the owner's.
func TestOwnerScopeSeesAndManagesMembersClass(t *testing.T) {
	svc, _ := newTestService()
	center := id.New()
	member := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	owner := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: true}

	class, err := svc.Create(context.Background(), member, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Get(context.Background(), owner, class.ID); err != nil {
		t.Fatalf("owner must read a member's class, got %v", err)
	}
	added, err := svc.AddSchedule(context.Background(), owner, class.ID, ScheduleRequest{
		Weekday: int16Ptr(0), StartTime: "08:30", DurationMin: 60,
	})
	if err != nil {
		t.Fatalf("owner must add a schedule to a member's class, got %v", err)
	}
	if added.TeacherID != member.TeacherID {
		t.Fatalf("a schedule added by the owner must inherit the parent class's own teacher, got %s", added.TeacherID)
	}
	if err := svc.Delete(context.Background(), owner, class.ID); err != nil {
		t.Fatalf("owner must delete a member's class, got %v", err)
	}
}

// A peer in the same center but not the creator, and not the owner, must not
// see the class — center scope alone is not enough, isolation still holds
// between non-owning members.
func TestPeerScopeCannotSeeAnotherMembersClass(t *testing.T) {
	svc, _ := newTestService()
	center := id.New()
	author := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	peer := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}

	class, err := svc.Create(context.Background(), author, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Get(context.Background(), peer, class.ID); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("peer must not read another member's class, got %v", err)
	}
}

// anchoredClass mirrors the real anchored() predicate: exact center and
// teacher match, no owner bypass — an Anchor carries none.
func anchoredClass(c *fakeClass, a authctx.Anchor) bool {
	return !c.deleted && c.CenterID == a.CenterID && c.TeacherID == a.TeacherID
}

// anchoredSchedule is anchoredClass's class_schedules counterpart.
func anchoredSchedule(s *fakeSchedule, a authctx.Anchor) bool {
	return !s.deleted && s.CenterID == a.CenterID && s.TeacherID == a.TeacherID
}

// FindActiveByName mirrors the SQL predicate: anchor-exact, not soft-deleted,
// exact name, and status active — an archived class must not be found.
func (f *fakeRepository) FindActiveByName(_ context.Context, a authctx.Anchor, name string) (*Class, error) {
	for _, c := range f.classes {
		if anchoredClass(c, a) && c.Name == name && c.Status == StatusActive {
			out := c.Class
			return &out, nil
		}
	}
	return nil, ErrNotFound
}

// ScheduleExists mirrors the SQL predicate, including effective_from: the same
// weekday and time may legitimately recur after a timetable change.
func (f *fakeRepository) ScheduleExists(_ context.Context, a authctx.Anchor, classID uuid.UUID, weekday int16, startTime TimeOfDay, effectiveFrom time.Time) (bool, error) {
	for _, s := range f.schedules {
		if anchoredSchedule(s, a) && s.ClassID == classID && s.Weekday == weekday &&
			s.StartTime == startTime && s.EffectiveFrom.Equal(effectiveFrom) {
			return true, nil
		}
	}
	return false, nil
}

// The bulk-import lookups below are read paths for another feature, so their
// scope handling is the thing worth pinning: an import anchored on one teacher
// must never resolve a name into another teacher's row.

func TestFindActiveByNameStaysWithinTheAnchorTeacher(t *testing.T) {
	svc, _ := newTestService()
	center := id.New()
	author := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	peer := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}

	if _, err := svc.Create(context.Background(), author, validCreateRequest()); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, found, err := svc.FindActiveByName(context.Background(), author.Self(), "Toán 8"); err != nil || !found {
		t.Fatalf("the author must find their own class, got found=%v err=%v", found, err)
	}
	if _, found, err := svc.FindActiveByName(context.Background(), peer.Self(), "Toán 8"); err != nil || found {
		t.Fatalf("a same-name class of another teacher must not be found, got found=%v err=%v", found, err)
	}
}

func TestFindActiveByNameIgnoresArchivedClasses(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()
	class, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Archive(context.Background(), sc, class.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}

	// An archived class keeps deleted_at NULL, so status is what separates it
	// from a live one. Reusing it would hang a new term's students off last
	// term's class.
	if _, found, err := svc.FindActiveByName(context.Background(), sc.Self(), "Toán 8"); err != nil || found {
		t.Fatalf("an archived class must not be reused, got found=%v err=%v", found, err)
	}
}

func TestScheduleExistsKeysOnEffectiveFrom(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()
	class, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

	exists, err := svc.ScheduleExists(context.Background(), sc.Self(), class.ID, 2, "18:00", start)
	if err != nil || !exists {
		t.Fatalf("the slot created with the class must be found, got exists=%v err=%v", exists, err)
	}
	// The same weekday and time may legitimately recur after a timetable
	// change, so a different effective_from is a different slot.
	later := start.AddDate(0, 1, 0)
	if exists, err := svc.ScheduleExists(context.Background(), sc.Self(), class.ID, 2, "18:00", later); err != nil || exists {
		t.Fatalf("a later effective_from must not match, got exists=%v err=%v", exists, err)
	}
	if exists, err := svc.ScheduleExists(context.Background(), sc.Self(), class.ID, 3, "18:00", start); err != nil || exists {
		t.Fatalf("another weekday must not match, got exists=%v err=%v", exists, err)
	}
}

// Every class is born with a display code: a blank request gets one minted
// by classcode.Generate, distinct per class, and tags never serialise as null.
func TestCreateGeneratesCodeWhenBlank(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()

	first, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	second, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	generated := regexp.MustCompile(`^L[0-9A-HJKMNP-TV-Z]{6}$`)
	if !generated.MatchString(first.Code) || !generated.MatchString(second.Code) {
		t.Fatalf("generated codes must be L + 6 Crockford chars, got %q and %q", first.Code, second.Code)
	}
	if first.Code == second.Code {
		t.Fatalf("two classes must not share a generated code: %q", first.Code)
	}
	if first.Tags == nil || len(first.Tags) != 0 {
		t.Fatalf("tags must default to an empty list, got %#v", first.Tags)
	}
}

// A generated code that collides is re-rolled, up to a bounded number of
// attempts; exhausting them surfaces as the same conflict an explicit
// duplicate does rather than an opaque failure.
func TestCreateRetriesGeneratedCodeCollisions(t *testing.T) {
	svc, repo := newTestService()
	sc := memberScope()

	repo.codeCollisions = 2
	if _, err := svc.Create(context.Background(), sc, validCreateRequest()); err != nil {
		t.Fatalf("two collisions must be absorbed by retries, got %v", err)
	}

	repo.codeCollisions = 10
	_, err := svc.Create(context.Background(), sc, validCreateRequest())
	if !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("exhausted retries must report the code as taken, got %v", err)
	}
	if repo.codeCollisions != 10-3 {
		t.Fatalf("want exactly 3 generate attempts, %d probes were left unused", repo.codeCollisions)
	}
}

// An explicit code is kept as given, is unique among the center's live
// classes, and may repeat in another center — the index is per center.
func TestCreateExplicitCodeUniquePerCenter(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()

	req := validCreateRequest()
	req.Code = strPtr("TOAN8")
	class, err := svc.Create(context.Background(), sc, req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if class.Code != "TOAN8" {
		t.Fatalf("explicit code must be stored verbatim, got %q", class.Code)
	}

	_, err = svc.Create(context.Background(), sc, req)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 || appErr.Code != CodeClassCodeTaken || !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("duplicate code in the same center must be 409 %s, got %v", CodeClassCodeTaken, err)
	}

	if _, err := svc.Create(context.Background(), memberScope(), req); err != nil {
		t.Fatalf("the same code in another center must be allowed, got %v", err)
	}

	req.Code = strPtr("toán 8")
	_, err = svc.Create(context.Background(), sc, req)
	if !errors.As(err, &appErr) || appErr.Status != 422 || appErr.Fields["code"] == "" {
		t.Fatalf("a code outside ^[A-Z0-9-]{2,20}$ must be a 422 on the code field, got %v", err)
	}
}

// Update treats the catalog fields as optional patches: a body that leaves
// code, tags, recruiting and note out keeps what was stored, while a present
// pointer replaces the field whole — including an empty tag list.
func TestUpdateMergesCatalogPointers(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()

	req := validCreateRequest()
	req.Code = strPtr("TOAN8")
	req.Tags = []string{"Toán", "Khối 8"}
	req.Note = strPtr("Phòng 201")
	class, err := svc.Create(context.Background(), sc, req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	base := UpdateClassRequest{Name: "Toán 8 nâng cao", StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(200_000)}
	patch := base
	patch.Recruiting = boolPtr(true)
	updated, err := svc.Update(context.Background(), sc, class.ID, patch)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Toán 8 nâng cao" || updated.DefaultUnitPrice != 200_000 {
		t.Fatalf("existing fields must still replace, got %+v", updated)
	}
	if !updated.Recruiting || updated.Code != "TOAN8" || !slices.Equal(updated.Tags, []string{"Toán", "Khối 8"}) ||
		updated.Note == nil || *updated.Note != "Phòng 201" {
		t.Fatalf("absent pointers must keep stored catalog fields, got %+v", updated)
	}

	patch = base
	patch.Tags = &[]string{}
	patch.Note = strPtr("")
	updated, err = svc.Update(context.Background(), sc, class.ID, patch)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(updated.Tags) != 0 || updated.Tags == nil {
		t.Fatalf("an empty tag list must clear tags (never null), got %#v", updated.Tags)
	}
	if updated.Note != nil {
		t.Fatalf("an empty note must clear the note, got %q", *updated.Note)
	}
	if !updated.Recruiting {
		t.Fatalf("recruiting must survive a patch that does not mention it")
	}

	patch = base
	patch.Code = strPtr("TOAN8")
	if _, err := svc.Update(context.Background(), sc, class.ID, patch); err != nil {
		t.Fatalf("re-sending the class's own code must not conflict with itself, got %v", err)
	}

	other := validCreateRequest()
	other.Code = strPtr("VAN8")
	if _, err := svc.Create(context.Background(), sc, other); err != nil {
		t.Fatalf("create other: %v", err)
	}
	patch = base
	patch.Code = strPtr("VAN8")
	if _, err := svc.Update(context.Background(), sc, class.ID, patch); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("taking another live class's code must conflict, got %v", err)
	}
}

// Stats counts through the caller's read scope with the same phase rule the
// list filter uses: a member sees only their own rows, the owner the center.
func TestStatsFollowReadScopeAndPhase(t *testing.T) {
	svc, repo := newTestService()
	owner := ownerScope()
	member := authctx.Scope{TeacherID: id.New(), CenterID: owner.CenterID}
	today := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

	seed := func(sc authctx.Scope, start, end string, recruiting bool) *Class {
		t.Helper()
		req := validCreateRequest()
		req.StartDate = start
		req.EndDate = end
		class, err := svc.Create(context.Background(), sc, req)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if recruiting {
			repo.classes[class.ID].Recruiting = true
		}
		return class
	}
	seed(member, "2026-01-05", "", true)  // running, recruiting
	seed(member, "2026-10-01", "", false) // upcoming
	ended := seed(owner, "2026-01-05", "2026-06-30", false)
	archived := seed(owner, "2026-01-05", "", true)
	repo.classes[archived.ID].Status = StatusArchived
	_ = ended

	got, err := svc.Stats(context.Background(), member, today, false)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	want := ClassStatsResponse{All: 2, Upcoming: 1, Running: 1, Recruiting: 1}
	if got != want {
		t.Fatalf("member stats: want %+v, got %+v", want, got)
	}

	got, err = svc.Stats(context.Background(), owner, today, false)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	// The archived class keeps its flag but is no longer open for recruitment.
	want = ClassStatsResponse{All: 4, Upcoming: 1, Running: 1, Ended: 1, Archived: 1, Recruiting: 1}
	if got != want {
		t.Fatalf("owner stats: want %+v, got %+v", want, got)
	}

	// recruitingOnly narrows every counter to the recruiting list's base set.
	got, err = svc.Stats(context.Background(), owner, today, true)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	want = ClassStatsResponse{All: 1, Running: 1, Recruiting: 1}
	if got != want {
		t.Fatalf("recruiting stats: want %+v, got %+v", want, got)
	}
}

func strPtr(v string) *string { return &v }
func boolPtr(v bool) *bool    { return &v }

// A class attached to a course copies the course's default price when the
// request leaves its own out; without a course the price stays required.
func TestCreateWithCourseCopiesDefaultPrice(t *testing.T) {
	svc, repo := newTestService()
	sc := memberScope()
	course := repo.addCourse(sc.CenterID, "TOAN-6", 180_000)

	req := validCreateRequest()
	req.DefaultUnitPrice = nil
	_, err := svc.Create(context.Background(), sc, req)
	appErr := appErrorOf(t, err, http.StatusUnprocessableEntity)
	if appErr.Fields["default_unit_price"] == "" {
		t.Fatalf("price without course must be required, got %+v", appErr.Fields)
	}

	req.CourseID = strPtr(course.ID.String())
	class, err := svc.Create(context.Background(), sc, req)
	if err != nil {
		t.Fatalf("create with course: %v", err)
	}
	if class.CourseID == nil || *class.CourseID != course.ID || class.DefaultUnitPrice != 180_000 {
		t.Fatalf("class must copy the course price, got %+v", class)
	}
	if class.Course == nil || class.Course.Code != "TOAN-6" {
		t.Fatalf("created class must carry the course ref for its response, got %+v", class.Course)
	}

	// An explicit price wins over the course default.
	req.DefaultUnitPrice = int64Ptr(120_000)
	req.Code = strPtr("TOAN6B")
	class, err = svc.Create(context.Background(), sc, req)
	if err != nil || class.DefaultUnitPrice != 120_000 {
		t.Fatalf("explicit price must win: %v %+v", err, class)
	}
}

// course_id must name a live course of the class's own center; another
// center's course or an unknown id is a 422 on the field, never a 500 and
// never a silent attach.
func TestCreateRejectsForeignOrUnknownCourse(t *testing.T) {
	svc, repo := newTestService()
	sc := memberScope()
	foreign := repo.addCourse(id.New(), "VAN-6", 1)

	for name, raw := range map[string]string{"foreign": foreign.ID.String(), "unknown": id.New().String()} {
		req := validCreateRequest()
		req.CourseID = &raw
		_, err := svc.Create(context.Background(), sc, req)
		appErr := appErrorOf(t, err, http.StatusUnprocessableEntity)
		if appErr.Fields["course_id"] == "" {
			t.Fatalf("%s: want a course_id field message, got %+v", name, appErr.Fields)
		}
	}
	if len(repo.classes) != 0 {
		t.Fatalf("a refused course must leave no class behind, got %d", len(repo.classes))
	}
}

// Update follows the patch rule: nil keeps the stored course, "" detaches,
// a uuid attaches — and the same center check applies.
func TestUpdateAttachesAndDetachesCourse(t *testing.T) {
	svc, repo := newTestService()
	sc := memberScope()
	course := repo.addCourse(sc.CenterID, "TOAN-6", 1)
	class, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	upd := UpdateClassRequest{Name: class.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000)}

	upd.CourseID = strPtr(course.ID.String())
	got, err := svc.Update(context.Background(), sc, class.ID, upd)
	if err != nil || got.CourseID == nil || *got.CourseID != course.ID || got.Course == nil {
		t.Fatalf("attach: %v %+v", err, got)
	}

	upd.CourseID = nil
	got, err = svc.Update(context.Background(), sc, class.ID, upd)
	if err != nil || got.CourseID == nil {
		t.Fatalf("nil must keep the course: %v %+v", err, got)
	}

	upd.CourseID = strPtr(id.New().String())
	_, err = svc.Update(context.Background(), sc, class.ID, upd)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["course_id"] == "" {
		t.Fatalf("unknown course must land on course_id: %+v", appErr.Fields)
	}

	upd.CourseID = strPtr("")
	got, err = svc.Update(context.Background(), sc, class.ID, upd)
	if err != nil || got.CourseID != nil || got.Course != nil {
		t.Fatalf("blank must detach: %v %+v", err, got)
	}
}

// Update follows the same patch rule for the lineage fields: nil keeps,
// "" detaches, a uuid must name a live class of the same center other than
// the class itself, else 422 on parent_class_id.
func TestUpdateSetsAndClearsParentClass(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()
	parent, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	class, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	upd := UpdateClassRequest{Name: class.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000)}

	upd.ParentClassID = strPtr(parent.ID.String())
	upd.LineageNote = strPtr("Tách từ lớp cũ")
	got, err := svc.Update(context.Background(), sc, class.ID, upd)
	if err != nil || got.ParentClassID == nil || *got.ParentClassID != parent.ID || got.LineageNote == nil || *got.LineageNote != "Tách từ lớp cũ" {
		t.Fatalf("attach parent: %v %+v", err, got)
	}

	upd.ParentClassID, upd.LineageNote = nil, nil
	got, err = svc.Update(context.Background(), sc, class.ID, upd)
	if err != nil || got.ParentClassID == nil || got.LineageNote == nil {
		t.Fatalf("nil must keep the lineage: %v %+v", err, got)
	}

	upd.ParentClassID = strPtr(class.ID.String())
	_, err = svc.Update(context.Background(), sc, class.ID, upd)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["parent_class_id"] == "" {
		t.Fatalf("a class cannot be its own parent: %+v", appErr.Fields)
	}

	upd.ParentClassID = strPtr(id.New().String())
	_, err = svc.Update(context.Background(), sc, class.ID, upd)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["parent_class_id"] == "" {
		t.Fatalf("unknown parent must land on parent_class_id: %+v", appErr.Fields)
	}

	other := ownerScope()
	theirs, err := svc.Create(context.Background(), other, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	upd.ParentClassID = strPtr(theirs.ID.String())
	_, err = svc.Update(context.Background(), sc, class.ID, upd)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["parent_class_id"] == "" {
		t.Fatalf("another center's class cannot be the parent: %+v", appErr.Fields)
	}

	upd.ParentClassID, upd.LineageNote = strPtr(""), strPtr("")
	got, err = svc.Update(context.Background(), sc, class.ID, upd)
	if err != nil || got.ParentClassID != nil || got.LineageNote != nil {
		t.Fatalf("blank must detach and clear the note: %v %+v", err, got)
	}
	resp := FromModel(got)
	if resp.ParentClassID != nil || resp.LineageNote != nil {
		t.Fatalf("response must mirror the cleared lineage: %+v", resp)
	}
}

// A parent link must not close a loop through the lineage chain: neither a
// direct swap (a<-b, then a<-b's parent set back to b) nor a longer chain
// (a<-b<-c, then a's parent set to c) may succeed.
func TestUpdateRefusesParentClassLineageCycle(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()
	classA, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	classB, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	updB := UpdateClassRequest{Name: classB.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000), ParentClassID: strPtr(classA.ID.String())}
	if _, err := svc.Update(context.Background(), sc, classB.ID, updB); err != nil {
		t.Fatalf("attach b under a: %v", err)
	}

	updA := UpdateClassRequest{Name: classA.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000), ParentClassID: strPtr(classB.ID.String())}
	_, err = svc.Update(context.Background(), sc, classA.ID, updA)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["parent_class_id"] == "" {
		t.Fatalf("a direct cycle must land on parent_class_id: %v", err)
	}

	classC, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	updC := UpdateClassRequest{Name: classC.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000), ParentClassID: strPtr(classB.ID.String())}
	if _, err := svc.Update(context.Background(), sc, classC.ID, updC); err != nil {
		t.Fatalf("attach c under b: %v", err)
	}
	updA.ParentClassID = strPtr(classC.ID.String())
	_, err = svc.Update(context.Background(), sc, classA.ID, updA)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["parent_class_id"] == "" {
		t.Fatalf("a longer cycle (a<-b<-c, a->c) must land on parent_class_id: %v", err)
	}
}

func TestArchivedCourseTakesNoNewClasses(t *testing.T) {
	svc, repo := newTestService()
	sc := memberScope()
	course := repo.addCourse(sc.CenterID, "TOAN-6", 1)
	req := validCreateRequest()
	req.CourseID = strPtr(course.ID.String())
	class, err := svc.Create(context.Background(), sc, req)
	if err != nil {
		t.Fatal(err)
	}
	course.Status = courseStatusArchived

	_, err = svc.Create(context.Background(), sc, req)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["course_id"] == "" {
		t.Fatalf("an archived course takes no new class: %+v", appErr.Fields)
	}

	// A class already on the course keeps it through an unrelated edit that
	// resends the same id, and another archived course is refused.
	upd := UpdateClassRequest{Name: "Đổi tên", StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(1), CourseID: strPtr(course.ID.String())}
	got, err := svc.Update(context.Background(), sc, class.ID, upd)
	if err != nil || got.CourseID == nil || *got.CourseID != course.ID {
		t.Fatalf("resending the stored archived course must be accepted: %v %+v", err, got)
	}
	other := repo.addCourse(sc.CenterID, "VAN-6", 1)
	other.Status = courseStatusArchived
	upd.CourseID = strPtr(other.ID.String())
	_, err = svc.Update(context.Background(), sc, class.ID, upd)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["course_id"] == "" {
		t.Fatalf("moving to an archived course must be refused: %+v", appErr.Fields)
	}
}

// appErrorOf asserts err is an AppError with status and hands it back so a
// test can inspect its field messages.
func appErrorOf(t *testing.T, err error, status int) *apperror.AppError {
	t.Helper()
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("want *apperror.AppError with status %d, got %v", status, err)
	}
	if appErr.Status != status {
		t.Fatalf("want status %d, got %d (%s: %s)", status, appErr.Status, appErr.Code, appErr.Message)
	}
	return appErr
}

// An end_date earlier than start_date is refused on both create and update;
// the same day for both, and an end_date after start_date, are both allowed.
func TestCreateAndUpdateRejectEndDateBeforeStartDate(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()

	req := validCreateRequest()
	req.EndDate = "2026-01-01"
	_, err := svc.Create(context.Background(), sc, req)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["end_date"] == "" {
		t.Fatalf("end_date before start_date must land on end_date, got %+v", appErr.Fields)
	}

	req.EndDate = "2026-01-05"
	class, err := svc.Create(context.Background(), sc, req)
	if err != nil {
		t.Fatalf("end_date equal to start_date must be allowed, got %v", err)
	}

	patch := UpdateClassRequest{
		Name: class.Name, StartDate: "2026-01-05", EndDate: "2025-12-31", DefaultUnitPrice: int64Ptr(150_000),
	}
	_, err = svc.Update(context.Background(), sc, class.ID, patch)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["end_date"] == "" {
		t.Fatalf("update with end_date before start_date must land on end_date, got %+v", appErr.Fields)
	}

	patch.EndDate = "2026-06-30"
	if _, err := svc.Update(context.Background(), sc, class.ID, patch); err != nil {
		t.Fatalf("end_date after start_date must be allowed, got %v", err)
	}
}

// A self-paced class carries no weekly timetable: creating one with
// schedules is refused, and creating one with none succeeds with an empty
// schedule list.
func TestCreateSelfPacedRejectsSchedules(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()

	req := validCreateRequest()
	req.StudyMode = StudyModeSelfPaced
	_, err := svc.Create(context.Background(), sc, req)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["schedules"] == "" {
		t.Fatalf("self_paced with schedules must land on schedules, got %+v", appErr.Fields)
	}
}

func TestCreateSelfPacedWithZeroSchedulesSucceeds(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()

	req := validCreateRequest()
	req.StudyMode = StudyModeSelfPaced
	req.Schedules = nil
	class, err := svc.Create(context.Background(), sc, req)
	if err != nil {
		t.Fatalf("self-paced create with no schedules must succeed, got %v", err)
	}
	if class.StudyMode != StudyModeSelfPaced {
		t.Fatalf("want study_mode self_paced, got %q", class.StudyMode)
	}
	if len(class.Schedules) != 0 {
		t.Fatalf("a self-paced class must carry no schedules, got %d", len(class.Schedules))
	}
}

// A scheduled class (the default when study_mode is absent) still requires
// at least one schedule, as before this feature existed.
func TestCreateScheduledRequiresSchedules(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()

	req := validCreateRequest()
	req.Schedules = nil
	_, err := svc.Create(context.Background(), sc, req)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["schedules"] == "" {
		t.Fatalf("scheduled without schedules must land on schedules, got %+v", appErr.Fields)
	}
}

// Update's room and study_mode fields follow the same patch rule as the rest
// of the DTO: nil keeps, a present value replaces (an empty room clears it),
// and an invalid study_mode is refused.
func TestUpdateRoomAndStudyModePatch(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()
	req := validCreateRequest()
	req.Room = "P101"
	class, err := svc.Create(context.Background(), sc, req)
	if err != nil {
		t.Fatal(err)
	}
	base := UpdateClassRequest{Name: class.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000)}

	got, err := svc.Update(context.Background(), sc, class.ID, base)
	if err != nil || got.Room != "P101" {
		t.Fatalf("absent room must keep the stored one, got %v %+v", err, got)
	}

	patch := base
	patch.StudyMode = strPtr(StudyModeSelfPaced)
	got, err = svc.Update(context.Background(), sc, class.ID, patch)
	if err != nil || got.StudyMode != StudyModeSelfPaced {
		t.Fatalf("study_mode must switch to self_paced, got %v %+v", err, got)
	}

	patch = base
	patch.StudyMode = strPtr("weekend")
	_, err = svc.Update(context.Background(), sc, class.ID, patch)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["study_mode"] == "" {
		t.Fatalf("an invalid study_mode must land on study_mode, got %+v", appErr.Fields)
	}

	patch = base
	patch.Room = strPtr("")
	got, err = svc.Update(context.Background(), sc, class.ID, patch)
	if err != nil || got.Room != "" {
		t.Fatalf("an empty room must clear it, got %v %+v", err, got)
	}
}

// Update's next_class_id follows the reverse-lineage patch rule: nil keeps
// every existing child, "" unlinks every live child, and a uuid links that
// live class of the same center as the single child — rejecting a self-link,
// an unknown or foreign-center class, and a link that would close a lineage
// cycle.
func TestUpdateNextClassLinkAndUnlink(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()
	parent, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.Create(context.Background(), sc, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	base := UpdateClassRequest{Name: parent.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000)}

	upd := base
	upd.NextClassID = strPtr(child.ID.String())
	got, err := svc.Update(context.Background(), sc, parent.ID, upd)
	if err != nil || got.NextClassID == nil || *got.NextClassID != child.ID {
		t.Fatalf("link: %v %+v", err, got)
	}

	upd = base
	upd.NextClassID = nil
	got, err = svc.Update(context.Background(), sc, parent.ID, upd)
	if err != nil || got.NextClassID == nil || *got.NextClassID != child.ID {
		t.Fatalf("nil must keep the existing link, got %v %+v", err, got)
	}

	upd = base
	upd.NextClassID = strPtr(parent.ID.String())
	_, err = svc.Update(context.Background(), sc, parent.ID, upd)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["next_class_id"] == "" {
		t.Fatalf("a class cannot be its own next class: %+v", appErr.Fields)
	}

	upd = base
	upd.NextClassID = strPtr(id.New().String())
	_, err = svc.Update(context.Background(), sc, parent.ID, upd)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["next_class_id"] == "" {
		t.Fatalf("unknown next class must land on next_class_id: %+v", appErr.Fields)
	}

	other := ownerScope()
	theirs, err := svc.Create(context.Background(), other, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	upd = base
	upd.NextClassID = strPtr(theirs.ID.String())
	_, err = svc.Update(context.Background(), sc, parent.ID, upd)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["next_class_id"] == "" {
		t.Fatalf("another center's class cannot be linked: %+v", appErr.Fields)
	}

	// parent's next is child (linked above); linking child's next back to
	// parent would close a two-class loop.
	childUpd := UpdateClassRequest{
		Name: child.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000),
		NextClassID: strPtr(parent.ID.String()),
	}
	_, err = svc.Update(context.Background(), sc, child.ID, childUpd)
	if appErr := appErrorOf(t, err, http.StatusUnprocessableEntity); appErr.Fields["next_class_id"] == "" {
		t.Fatalf("a lineage cycle must land on next_class_id: %+v", appErr.Fields)
	}

	upd = base
	upd.NextClassID = strPtr("")
	got, err = svc.Update(context.Background(), sc, parent.ID, upd)
	if err != nil || got.NextClassID != nil {
		t.Fatalf("blank must unlink, got %v %+v", err, got)
	}
}

// AddSchedule refuses to open a schedule on a self-paced class with the
// dedicated SELF_PACED_NO_SCHEDULE code, distinct from a generic validation
// error.
func TestAddScheduleOnSelfPacedRejected(t *testing.T) {
	svc, _ := newTestService()
	sc := memberScope()
	req := validCreateRequest()
	req.StudyMode = StudyModeSelfPaced
	req.Schedules = nil
	class, err := svc.Create(context.Background(), sc, req)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AddSchedule(context.Background(), sc, class.ID, ScheduleRequest{
		Weekday: int16Ptr(2), StartTime: "18:00", DurationMin: 90,
	})
	appErr := appErrorOf(t, err, http.StatusUnprocessableEntity)
	if appErr.Code != CodeSelfPacedNoSchedule {
		t.Fatalf("want %s, got %s", CodeSelfPacedNoSchedule, appErr.Code)
	}
}

// Availability marks a room and its class's teacher and staff busy only when
// one of their classes has an active schedule overlapping a requested slot
// on the same weekday; excludeClassID leaves one class out of that check,
// and a free room or teacher is reported alongside the busy ones.
func TestAvailabilityMarksBusyRoomsAndTeachers(t *testing.T) {
	svc, repo := newTestService()
	sc := memberScope()

	busy := validCreateRequest()
	busy.Room = "P101"
	busy.Schedules = []ScheduleRequest{{Weekday: int16Ptr(1), StartTime: "08:00", DurationMin: 90}}
	busyClass, err := svc.Create(context.Background(), sc, busy)
	if err != nil {
		t.Fatal(err)
	}
	coTeacher := id.New()
	repo.addStaff(busyClass.ID, coTeacher)

	free := validCreateRequest()
	free.Room = "P102"
	free.Schedules = []ScheduleRequest{{Weekday: int16Ptr(3), StartTime: "08:00", DurationMin: 90}}
	if _, err := svc.Create(context.Background(), sc, free); err != nil {
		t.Fatal(err)
	}

	excluded := validCreateRequest()
	excluded.Room = "P103"
	excluded.Schedules = []ScheduleRequest{{Weekday: int16Ptr(1), StartTime: "08:00", DurationMin: 90}}
	excludedClass, err := svc.Create(context.Background(), sc, excluded)
	if err != nil {
		t.Fatal(err)
	}

	repo.addMember(sc.CenterID, sc.TeacherID, "Giáo viên chính")
	repo.addMember(sc.CenterID, coTeacher, "Trợ giảng")
	freeTeacher := id.New()
	repo.addMember(sc.CenterID, freeTeacher, "Giáo viên rảnh")

	slots := []AvailabilitySlot{{Weekday: 1, StartTime: "08:30", DurationMin: 30}}
	resp, err := svc.Availability(context.Background(), sc, slots, &excludedClass.ID)
	if err != nil {
		t.Fatalf("availability: %v", err)
	}

	roomFree := map[string]bool{}
	for _, r := range resp.Rooms {
		roomFree[r.Name] = r.Free
	}
	if roomFree["P101"] {
		t.Fatalf("P101 must be busy, got %+v", resp.Rooms)
	}
	if !roomFree["P102"] {
		t.Fatalf("P102 must be free, got %+v", resp.Rooms)
	}
	if _, ok := roomFree["P103"]; ok {
		t.Fatalf("the excluded class's room must not appear as busy from it, got %+v", resp.Rooms)
	}

	teacherFree := map[uuid.UUID]bool{}
	for _, tch := range resp.Teachers {
		teacherFree[tch.TeacherID] = tch.Free
	}
	if teacherFree[sc.TeacherID] {
		t.Fatalf("the busy class's own teacher must be busy, got %+v", resp.Teachers)
	}
	if teacherFree[coTeacher] {
		t.Fatalf("the busy class's co-teacher (class_staff) must be busy, got %+v", resp.Teachers)
	}
	if !teacherFree[freeTeacher] {
		t.Fatalf("an unrelated teacher must stay free, got %+v", resp.Teachers)
	}

	// A slot on a different weekday overlaps nothing, so every room is free.
	noOverlap, err := svc.Availability(context.Background(), sc, []AvailabilitySlot{{Weekday: 5, StartTime: "08:00", DurationMin: 30}}, nil)
	if err != nil {
		t.Fatalf("availability: %v", err)
	}
	for _, r := range noOverlap.Rooms {
		if !r.Free {
			t.Fatalf("no requested slot overlaps weekday 5, every room must be free, got %+v", noOverlap.Rooms)
		}
	}
}
