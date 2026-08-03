package classes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// Service owns class business rules: atomic create-with-schedules, the
// archive-vs-delete distinction, and effective-range schedule management.
type Service struct {
	repo Repository
	tx   database.TxManager
}

// NewService builds the classes service.
func NewService(repo Repository, tx database.TxManager) *Service {
	return &Service{repo: repo, tx: tx}
}

// Create inserts the class and its schedule rows in one transaction — a
// failing schedule insert leaves no class row. Schedules with no
// effective_from inherit the class start date.
func (s *Service) Create(ctx context.Context, teacherID uuid.UUID, req CreateClassRequest) (*Class, error) {
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
		TeacherID:        teacherID,
		Name:             req.Name,
		StartDate:        startDate,
		EndDate:          endDate,
		DefaultUnitPrice: *req.DefaultUnitPrice,
		Status:           StatusActive,
	}
	schedules := make([]Schedule, len(req.Schedules))
	for i, sr := range req.Schedules {
		schedule, err := scheduleFromRequest(teacherID, class.ID, startDate, sr)
		if err != nil {
			return nil, err
		}
		schedules[i] = *schedule
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		return s.repo.CreateWithSchedules(ctx, class, schedules)
	})
	if err != nil {
		return nil, err
	}
	class.Schedules = schedules
	return class, nil
}

// scheduleFromRequest builds a schedule row, defaulting effective_from to the
// class start date.
func scheduleFromRequest(teacherID, classID uuid.UUID, startDate time.Time, sr ScheduleRequest) (*Schedule, error) {
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
func (s *Service) Get(ctx context.Context, teacherID, classID uuid.UUID) (*Class, error) {
	class, err := s.repo.GetByID(ctx, teacherID, classID)
	if err != nil {
		return nil, translate(err)
	}
	return class, nil
}

// List returns a page of classes. The default filter (active only) matches
// the idx_classes_teacher partial-index predicate.
func (s *Service) List(ctx context.Context, teacherID uuid.UUID, filter ListFilter, p pagination.Params) ([]Class, int64, error) {
	return s.repo.List(ctx, teacherID, filter, p)
}

// Update edits the class's own fields; status and schedules have their own
// endpoints.
func (s *Service) Update(ctx context.Context, teacherID, classID uuid.UUID, req UpdateClassRequest) (*Class, error) {
	class, err := s.repo.GetByID(ctx, teacherID, classID)
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
func (s *Service) Archive(ctx context.Context, teacherID, classID uuid.UUID) (*Class, error) {
	if err := s.repo.Archive(ctx, teacherID, classID); err != nil {
		return nil, translate(err)
	}
	return s.Get(ctx, teacherID, classID)
}

// Delete soft-deletes a class created by mistake. A class students are still
// enrolled in refuses with 409 and points at archiving instead — soft delete
// would leave live enrollments dangling on an invisible class.
func (s *Service) Delete(ctx context.Context, teacherID, classID uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, teacherID, classID); err != nil {
		return translate(err)
	}
	open, err := s.repo.CountOpenEnrollments(ctx, teacherID, classID)
	if err != nil {
		return err
	}
	if open > 0 {
		appErr := apperror.Conflict(fmt.Sprintf(
			"class has %d open enrollment(s); archive the class instead of deleting it", open))
		appErr.Err = ErrHasOpenEnrollments
		return appErr
	}
	return translate(s.repo.SoftDelete(ctx, teacherID, classID))
}

// AddSchedule appends a timetable row to the class — the second half of the
// close-and-replace flow for changing a weekly slot.
func (s *Service) AddSchedule(ctx context.Context, teacherID, classID uuid.UUID, req ScheduleRequest) (*Schedule, error) {
	class, err := s.repo.GetByID(ctx, teacherID, classID)
	if err != nil {
		return nil, translate(err)
	}
	schedule, err := scheduleFromRequest(teacherID, classID, class.StartDate, req)
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
func (s *Service) UpdateSchedule(ctx context.Context, teacherID, classID, scheduleID uuid.UUID, req UpdateScheduleRequest) (*Schedule, error) {
	schedule, err := s.repo.GetSchedule(ctx, teacherID, classID, scheduleID)
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
func (s *Service) DeleteSchedule(ctx context.Context, teacherID, classID, scheduleID uuid.UUID) error {
	return translate(s.repo.SoftDeleteSchedule(ctx, teacherID, classID, scheduleID))
}

// ListEffectiveSchedules exposes the session-generation contract: rows whose
// effective range intersects [from, to].
func (s *Service) ListEffectiveSchedules(ctx context.Context, teacherID, classID uuid.UUID, from, to time.Time) ([]Schedule, error) {
	if _, err := s.repo.GetByID(ctx, teacherID, classID); err != nil {
		return nil, translate(err)
	}
	return s.repo.ListEffectiveSchedules(ctx, teacherID, classID, from, to)
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
