package classes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// StaffSeeder is what classes needs from the class-staff feature: maintaining
// the class_staff mirror of classes.teacher_id (the dual-write invariant:
// whoever teacher_id names must hold the class's one active giao_vien stint),
// and loading the caller's active roles for the readable ports.
// classstaff.Repository satisfies it structurally; classes must not import
// classstaff (classstaff reads classes for its existence gate).
type StaffSeeder interface {
	SyncPrimaryTeacher(ctx context.Context, classID, centerID, teacherID uuid.UUID) error
	RolesByClass(ctx context.Context, teacherID, centerID uuid.UUID, classIDs []uuid.UUID) (map[uuid.UUID][]string, error)
}

// Service owns class business rules: atomic create-with-schedules, the
// archive-vs-delete distinction, and effective-range schedule management.
type Service struct {
	repo  Repository
	tx    database.TxManager
	staff StaffSeeder
}

// NewService builds the classes service. staff is required: every class must
// be born with its teacher's giao_vien stint, or assignment-based read
// scoping would silently exclude the primary teacher.
func NewService(repo Repository, tx database.TxManager, staff StaffSeeder) *Service {
	return &Service{repo: repo, tx: tx, staff: staff}
}

// Create inserts the class and its schedule rows in one transaction — a
// failing schedule insert leaves no class row. Schedules with no
// effective_from inherit the class start date. The class is always stamped as
// the caller's own — including an owner, who creates rows as themselves,
// never on behalf of another teacher.
func (s *Service) Create(ctx context.Context, sc authctx.Scope, req CreateClassRequest) (*Class, error) {
	startDate, err := parseDate("start_date", req.StartDate)
	if err != nil {
		return nil, err
	}
	endDate, err := parseDatePtr("end_date", req.EndDate)
	if err != nil {
		return nil, err
	}

	class := &Class{
		ID:               id.New(),
		TeacherID:        sc.TeacherID,
		CenterID:         sc.CenterID,
		Name:             req.Name,
		StartDate:        startDate,
		EndDate:          endDate,
		DefaultUnitPrice: *req.DefaultUnitPrice,
		Status:           StatusActive,
	}
	schedules := make([]Schedule, len(req.Schedules))
	for i, sr := range req.Schedules {
		schedule, err := scheduleFromRequest(sc.TeacherID, sc.CenterID, class.ID, startDate, sr)
		if err != nil {
			return nil, err
		}
		schedules[i] = *schedule
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.CreateWithSchedules(ctx, class, schedules); err != nil {
			return err
		}
		// Same transaction as the class row: teacher_id and the giao_vien
		// stint must never be observable apart.
		return s.staff.SyncPrimaryTeacher(ctx, class.ID, class.CenterID, class.TeacherID)
	})
	if err != nil {
		return nil, err
	}
	class.Schedules = schedules
	return class, nil
}

// scheduleFromRequest builds a schedule row, defaulting effective_from to the
// class start date. teacherID and centerID are the row's own tenant anchors —
// for a new class these are the caller's scope, but for a schedule added to
// an existing class (AddSchedule) they must be the parent class's own
// anchors, since an owner may be adding to a member's class.
func scheduleFromRequest(teacherID, centerID, classID uuid.UUID, startDate time.Time, sr ScheduleRequest) (*Schedule, error) {
	effectiveFrom := startDate
	if sr.EffectiveFrom != "" {
		parsed, err := parseDate("effective_from", sr.EffectiveFrom)
		if err != nil {
			return nil, err
		}
		effectiveFrom = parsed
	}
	effectiveTo, err := parseDatePtr("effective_to", sr.EffectiveTo)
	if err != nil {
		return nil, err
	}
	return &Schedule{
		ID:            id.New(),
		TeacherID:     teacherID,
		CenterID:      centerID,
		ClassID:       classID,
		Weekday:       *sr.Weekday,
		StartTime:     TimeOfDay(sr.StartTime),
		DurationMin:   sr.DurationMin,
		EffectiveFrom: effectiveFrom,
		EffectiveTo:   effectiveTo,
	}, nil
}

// Get returns one class with its schedules; archived classes stay
// retrievable by id.
func (s *Service) Get(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*Class, error) {
	class, err := s.repo.GetByID(ctx, sc, classID)
	if err != nil {
		return nil, translate(err)
	}
	return class, nil
}

// List returns a page of classes. The default filter (active only) matches
// the idx_classes_teacher partial-index predicate.
func (s *Service) List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Class, int64, error) {
	return s.repo.List(ctx, sc, filter, p)
}

// GetReadable is Get's READ-port counterpart: own rows plus any class the
// caller holds a class_staff stint on (ended included — history reads). It
// also returns the caller's ACTIVE role keys on the class, for the response's
// my_staff_roles. Only the GET handlers use it; every write path keeps
// resolving through Get, the shared write gate.
func (s *Service) GetReadable(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*Class, []string, error) {
	class, err := s.repo.GetReadableByID(ctx, sc, classID)
	if err != nil {
		return nil, nil, translate(err)
	}
	roles, err := s.staff.RolesByClass(ctx, sc.TeacherID, sc.CenterID, []uuid.UUID{classID})
	if err != nil {
		return nil, nil, err
	}
	return class, roles[classID], nil
}

// GetWritable is the capability write gate: it resolves the writing roles for
// capability from the authctx map and fetches the class through the write
// port (active stint in those roles, or center-wide scope). When the write
// fetch misses, the read port disambiguates the honest answer: readable means
// the caller has SOME relationship to the class (an ended stint, a different
// role, or creator rows) but not this capability → 403; unreadable → 404, so
// outsiders cannot probe class ids.
func (s *Service) GetWritable(ctx context.Context, sc authctx.Scope, classID uuid.UUID, capability authctx.ClassCapability) (*Class, error) {
	roles := authctx.StaffRolesFor(capability)
	class, err := s.repo.GetWritableByID(ctx, sc, classID, roles)
	if err == nil {
		return class, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if _, rerr := s.repo.GetReadableByID(ctx, sc, classID); rerr == nil {
		return nil, apperror.Forbidden("your role on this class does not allow this action")
	} else if !errors.Is(rerr, ErrNotFound) {
		return nil, rerr
	}
	return nil, apperror.NotFound("class")
}

// ListReadable is List's READ-port counterpart. Roles come from one batch
// query over the page's class ids, keyed by class id — never per row.
func (s *Service) ListReadable(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Class, map[uuid.UUID][]string, int64, error) {
	rows, total, err := s.repo.ListReadable(ctx, sc, filter, p)
	if err != nil {
		return nil, nil, 0, err
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	roles, err := s.staff.RolesByClass(ctx, sc.TeacherID, sc.CenterID, ids)
	if err != nil {
		return nil, nil, 0, err
	}
	return rows, roles, total, nil
}

// Update edits the class's own fields; status and schedules have their own
// endpoints.
func (s *Service) Update(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req UpdateClassRequest) (*Class, error) {
	class, err := s.repo.GetByID(ctx, sc, classID)
	if err != nil {
		return nil, translate(err)
	}
	startDate, err := parseDate("start_date", req.StartDate)
	if err != nil {
		return nil, err
	}
	endDate, err := parseDatePtr("end_date", req.EndDate)
	if err != nil {
		return nil, err
	}
	class.Name = req.Name
	class.StartDate = startDate
	class.EndDate = endDate
	class.DefaultUnitPrice = *req.DefaultUnitPrice
	if err := s.repo.Update(ctx, class); err != nil {
		return nil, err
	}
	return class, nil
}

// Archive flips the class to archived — the normal end-of-term action, and
// idempotent.
func (s *Service) Archive(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*Class, error) {
	if err := s.repo.Archive(ctx, sc, classID); err != nil {
		return nil, translate(err)
	}
	return s.Get(ctx, sc, classID)
}

// Delete soft-deletes a class created by mistake. A class students are still
// enrolled in refuses with 409 and points at archiving instead — soft delete
// would leave live enrollments dangling on an invisible class.
func (s *Service) Delete(ctx context.Context, sc authctx.Scope, classID uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, sc, classID); err != nil {
		return translate(err)
	}
	open, err := s.repo.CountOpenEnrollments(ctx, sc, classID)
	if err != nil {
		return err
	}
	if open > 0 {
		appErr := apperror.Conflict(fmt.Sprintf(
			"class has %d open enrollment(s); archive the class instead of deleting it", open))
		appErr.Err = ErrHasOpenEnrollments
		return appErr
	}
	return translate(s.repo.SoftDelete(ctx, sc, classID))
}

// AddSchedule appends a timetable row to the class — the second half of the
// close-and-replace flow for changing a weekly slot. The new row inherits the
// parent class's own teacher and center, not the caller's scope: an owner
// adding a schedule to a member's class must not stamp themselves as its
// teacher.
func (s *Service) AddSchedule(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req ScheduleRequest) (*Schedule, error) {
	class, err := s.repo.GetByID(ctx, sc, classID)
	if err != nil {
		return nil, translate(err)
	}
	schedule, err := scheduleFromRequest(class.TeacherID, class.CenterID, classID, class.StartDate, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.AddSchedule(ctx, schedule); err != nil {
		return nil, err
	}
	return schedule, nil
}

// UpdateSchedule edits one row in place — for correcting a mistake or closing
// the row by setting effective_to. Real timetable changes should close the
// old row and add a new one.
func (s *Service) UpdateSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID, req UpdateScheduleRequest) (*Schedule, error) {
	schedule, err := s.repo.GetSchedule(ctx, sc, classID, scheduleID)
	if err != nil {
		return nil, translate(err)
	}
	effectiveFrom, err := parseDate("effective_from", req.EffectiveFrom)
	if err != nil {
		return nil, err
	}
	effectiveTo, err := parseDatePtr("effective_to", req.EffectiveTo)
	if err != nil {
		return nil, err
	}
	schedule.Weekday = *req.Weekday
	schedule.StartTime = TimeOfDay(req.StartTime)
	schedule.DurationMin = req.DurationMin
	schedule.EffectiveFrom = effectiveFrom
	schedule.EffectiveTo = effectiveTo
	if err := s.repo.UpdateSchedule(ctx, schedule); err != nil {
		return nil, err
	}
	return schedule, nil
}

// DeleteSchedule soft-deletes a timetable row.
func (s *Service) DeleteSchedule(ctx context.Context, sc authctx.Scope, classID, scheduleID uuid.UUID) error {
	return translate(s.repo.SoftDeleteSchedule(ctx, sc, classID, scheduleID))
}

// ListEffectiveSchedules exposes the session-generation contract: rows whose
// effective range intersects [from, to].
func (s *Service) ListEffectiveSchedules(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Schedule, error) {
	if _, err := s.repo.GetByID(ctx, sc, classID); err != nil {
		return nil, translate(err)
	}
	return s.repo.ListEffectiveSchedules(ctx, sc, classID, from, to)
}

// translate maps domain errors onto the API error contract, keeping the
// domain error as the cause so errors.Is still works.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apperror.NotFound("class")
	case errors.Is(err, ErrScheduleNotFound):
		return apperror.NotFound("schedule")
	default:
		return err
	}
}

// FindActiveByName resolves a live, active class by its exact name inside the
// scope. It is the create-or-reuse seam for bulk flows; classes.name carries
// no unique index, so this is a lookup and the caller decides what a hit means.
//
// An archived class is deliberately not a hit: it still has deleted_at IS NULL,
// so a name-only match would enrol this year's students into a class the
// teacher closed last term.
func (s *Service) FindActiveByName(ctx context.Context, sc authctx.Scope, name string) (*Class, bool, error) {
	class, err := s.repo.FindActiveByName(ctx, sc, name)
	if errors.Is(err, ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return class, true, nil
}

// ReassignTeacher moves the class and its schedule rows to newTeacherID within
// the scope. It is the classes-owned half of the owner-only class handoff: the
// coordinating handoff feature calls it inside a transaction that also moves
// the class's future planned sessions (owned by the sessions feature), so this
// touches only tables classes owns. ErrNotFound surfaces as a 404.
func (s *Service) ReassignTeacher(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID) error {
	return translate(s.repo.ReassignTeacher(ctx, sc, classID, newTeacherID))
}

// ScheduleExists reports whether the class already carries this exact weekly
// slot. Callers must pass the same effective_from they intend to write:
// AddSchedule defaults a blank one to the stored class's start_date, so a
// caller that checked with one date and wrote with another would append a
// duplicate slot on every run, and class_schedules has no unique index to
// stop it.
func (s *Service) ScheduleExists(ctx context.Context, sc authctx.Scope, classID uuid.UUID, weekday int16, startTime TimeOfDay, effectiveFrom time.Time) (bool, error) {
	return s.repo.ScheduleExists(ctx, sc, classID, weekday, startTime, effectiveFrom)
}
