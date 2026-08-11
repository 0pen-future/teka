package enrollments

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// Service owns enrollment business rules: the price copy at creation, the
// one-open-enrollment invariant, and the end-don't-edit mutation model.
type Service struct {
	repo Repository
}

// NewService builds the enrollments service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create enrolls a student, copying unit_price from the class's current
// default. The two lookups exist to produce clean 422s and to read the price;
// the composite FKs are what actually prevent cross-teacher stitching, and
// uq_enrollments_active — not a pre-check — is what refuses a duplicate open
// enrollment.
func (s *Service) Create(ctx context.Context, teacherID uuid.UUID, req CreateRequest) (*Row, error) {
	price, err := s.repo.ClassDefaultPrice(ctx, teacherID, req.ClassID)
	if errors.Is(err, ErrClassNotFound) {
		return nil, refInvalid("class_id", "must reference one of your classes", err)
	}
	if err != nil {
		return nil, err
	}
	ok, err := s.repo.StudentExists(ctx, teacherID, req.StudentID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, refInvalid("student_id", "must reference one of your students", ErrStudentNotFound)
	}

	startedOn := today()
	if req.StartedOn != "" {
		startedOn, err = parseDate("started_on", req.StartedOn)
		if err != nil {
			return nil, err
		}
	}

	e := &Enrollment{
		ID:        id.New(),
		TeacherID: teacherID,
		StudentID: req.StudentID,
		ClassID:   req.ClassID,
		StartedOn: startedOn,
		UnitPrice: price,
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return nil, translate(err)
	}
	return s.repo.GetByID(ctx, teacherID, e.ID)
}

// Get returns one enrollment with its display names; ended enrollments stay
// retrievable.
func (s *Service) Get(ctx context.Context, teacherID, enrollmentID uuid.UUID) (*Row, error) {
	row, err := s.repo.GetByID(ctx, teacherID, enrollmentID)
	if err != nil {
		return nil, translate(err)
	}
	return row, nil
}

// List returns a page of enrollments, filterable by student, class, and
// open/ended state.
func (s *Service) List(ctx context.Context, teacherID uuid.UUID, filter ListFilter, p pagination.Params) ([]Row, int64, error) {
	return s.repo.List(ctx, teacherID, filter, p)
}

// End closes an enrollment — "nghỉ hẳn giữa chu kỳ", the only mutation V1
// allows. Ending twice returns 409 so a double-submit cannot silently move
// the departure date.
func (s *Service) End(ctx context.Context, teacherID, enrollmentID uuid.UUID, req EndRequest) (*Row, error) {
	row, err := s.repo.GetByID(ctx, teacherID, enrollmentID)
	if err != nil {
		return nil, translate(err)
	}
	if row.EndedOn != nil {
		return nil, translate(ErrAlreadyEnded)
	}

	endedOn := today()
	if req.EndedOn != "" {
		endedOn, err = parseDate("ended_on", req.EndedOn)
		if err != nil {
			return nil, err
		}
	}
	if endedOn.Before(row.StartedOn) {
		return nil, apperror.Invalid("validation failed",
			map[string]string{"ended_on": "must not be before started_on"})
	}

	if err := s.repo.End(ctx, teacherID, enrollmentID, endedOn); err != nil {
		return nil, translate(err)
	}
	return s.repo.GetByID(ctx, teacherID, enrollmentID)
}

// Delete soft-deletes an enrollment created by mistake; leaving is End, not
// Delete.
func (s *Service) Delete(ctx context.Context, teacherID, enrollmentID uuid.UUID) error {
	return translate(s.repo.SoftDelete(ctx, teacherID, enrollmentID))
}

// ActiveOn exposes the attendance-sheet query plan 03 consumes: enrollments
// open on the given date, inclusive at both boundaries.
func (s *Service) ActiveOn(ctx context.Context, teacherID, classID uuid.UUID, on time.Time) ([]Enrollment, error) {
	return s.repo.ActiveOn(ctx, teacherID, classID, on)
}

// EndOpenEnrollments satisfies students.EnrollmentEnder: the students feature
// calls it inside the delete transaction while anonymising a student.
func (s *Service) EndOpenEnrollments(ctx context.Context, sc authctx.Scope, studentID uuid.UUID, on time.Time) error {
	return s.repo.EndOpenEnrollments(ctx, sc, studentID, on)
}

// today returns the current date at UTC midnight, matching how request dates
// parse — DATE columns carry no zone.
func today() time.Time {
	y, m, d := time.Now().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// refInvalid is the 422 for a reference that is not the teacher's, keeping
// the domain error as the cause so errors.Is still works.
func refInvalid(field, message string, cause error) error {
	appErr := apperror.Invalid("validation failed", map[string]string{field: message})
	appErr.Err = cause
	return appErr
}

// translate maps domain errors onto the API error contract, keeping the
// domain error as the cause so errors.Is still works.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apperror.NotFound("enrollment")
	case errors.Is(err, ErrAlreadyEnrolled):
		appErr := apperror.Conflict("student is already enrolled in this class")
		appErr.Err = ErrAlreadyEnrolled
		return appErr
	case errors.Is(err, ErrAlreadyEnded):
		appErr := apperror.Conflict("enrollment is already ended")
		appErr.Err = ErrAlreadyEnded
		return appErr
	default:
		return err
	}
}
