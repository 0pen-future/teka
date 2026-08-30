package enrollments

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// Service owns enrollment business rules: the price copy at creation, the
// one-open-enrollment invariant, and the end-don't-edit mutation model.
type Service struct {
	repo Repository
	bus  events.Bus
}

// NewService builds the enrollments service. bus may be nil for callers that
// do not observe events (integration tests of other features).
func NewService(repo Repository, bus events.Bus) *Service {
	return &Service{repo: repo, bus: bus}
}

// Create enrolls a student, copying unit_price from the class's current
// default. The two lookups exist to produce clean 422s and to read the price;
// the composite FKs are what actually prevent cross-center stitching, and
// uq_enrollments_active — not a pre-check — is what refuses a duplicate open
// enrollment. The enrollment is always stamped as the caller's own, so the
// CLASS check runs with owner rights stripped: a row created against a
// member's class would carry the owner's anchor while living in the member's
// roster — invisible to the member's own attendance and billing, and
// unrepeatable for them under uq_enrollments_active. The STUDENT check is
// center-scoped instead: students anchor to the owner, so requiring the
// caller's own teacher_id would refuse every legitimate reference.
func (s *Service) Create(ctx context.Context, sc authctx.Scope, req CreateRequest) (*Row, error) {
	ownScope := authctx.Scope{TeacherID: sc.TeacherID, CenterID: sc.CenterID}
	price, err := s.repo.ClassDefaultPrice(ctx, ownScope, req.ClassID)
	if errors.Is(err, ErrClassNotFound) {
		return nil, refInvalid("class_id", "must reference one of your classes", err)
	}
	if err != nil {
		return nil, err
	}
	ok, err := s.repo.StudentExists(ctx, ownScope, req.StudentID)
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
		TeacherID: sc.TeacherID,
		CenterID:  sc.CenterID,
		StudentID: req.StudentID,
		ClassID:   req.ClassID,
		StartedOn: startedOn,
		UnitPrice: price,
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return nil, translate(err)
	}
	if s.bus != nil {
		// Enrolling widens what the creating teacher can read and feeds the
		// next billing close, so every successful create leaves an event for
		// the audit trail.
		s.bus.Publish(StudentEnrolled{
			OccurredAt:   time.Now().UTC(),
			CenterID:     sc.CenterID,
			ActorID:      sc.TeacherID,
			EnrollmentID: e.ID,
			ClassID:      e.ClassID,
			StudentID:    e.StudentID,
		})
	}
	return s.repo.GetByID(ctx, sc, e.ID)
}

// pickerLimit caps the enrollable-student picker: it is an autocomplete for
// one name, not a roster export.
const pickerLimit = 20

// EnrollableStudents finds live center students by name for the enrollment
// picker. Owner-side scopes may use any class's picker; a member needs the
// class's ACTIVE giao_vien stint — other assignments grant class visibility
// (403), and no assignment means the class does not exist for the caller
// (404), mirroring classstaff's read gate. Queries under two runes answer an
// empty list so the picker cannot be walked as a center-wide name dump.
func (s *Service) EnrollableStudents(ctx context.Context, sc authctx.Scope, classID uuid.UUID, q string, limit int) ([]PickerStudent, error) {
	inCenter, err := s.repo.ClassInCenter(ctx, sc, classID)
	if err != nil {
		return nil, err
	}
	if !inCenter {
		return nil, apperror.NotFound("class")
	}
	if !sc.CenterWide() {
		assigned, teaching, err := s.repo.CallerClassStanding(ctx, sc, classID)
		if err != nil {
			return nil, err
		}
		if !assigned {
			return nil, apperror.NotFound("class")
		}
		if !teaching {
			return nil, apperror.Forbidden("only the class's active teacher may enroll students")
		}
	}
	if limit <= 0 || limit > pickerLimit {
		limit = pickerLimit
	}
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) < 2 {
		return []PickerStudent{}, nil
	}
	rows, err := s.repo.SearchEnrollableStudents(ctx, sc, classID, q, limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []PickerStudent{}
	}
	return rows, nil
}

// Get returns one enrollment with its display names; ended enrollments stay
// retrievable.
func (s *Service) Get(ctx context.Context, sc authctx.Scope, enrollmentID uuid.UUID) (*Row, error) {
	row, err := s.repo.GetByID(ctx, sc, enrollmentID)
	if err != nil {
		return nil, translate(err)
	}
	return row, nil
}

// List returns a page of enrollments, filterable by student, class, and
// open/ended state.
func (s *Service) List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Row, int64, error) {
	return s.repo.List(ctx, sc, filter, p)
}

// End closes an enrollment — "nghỉ hẳn giữa chu kỳ", the only mutation V1
// allows. Ending twice returns 409 so a double-submit cannot silently move
// the departure date.
func (s *Service) End(ctx context.Context, sc authctx.Scope, enrollmentID uuid.UUID, req EndRequest) (*Row, error) {
	// Resolve through the write gate before any state answer: only a caller
	// who may manage the roster learns 409/422 state. A readable row the
	// caller cannot manage is an honest 403; an unreadable id stays 404.
	roles := authctx.StaffRolesFor(authctx.CapEnrollmentWrite)
	row, err := s.repo.GetWritableByID(ctx, sc, enrollmentID, roles)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if _, rerr := s.repo.GetByID(ctx, sc, enrollmentID); rerr == nil {
				return nil, apperror.Forbidden("your role on this class does not allow this action")
			} else if !errors.Is(rerr, ErrNotFound) {
				return nil, translate(rerr)
			}
		}
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

	if err := s.repo.End(ctx, sc, roles, enrollmentID, endedOn); err != nil {
		return nil, translate(err)
	}
	return s.repo.GetByID(ctx, sc, enrollmentID)
}

// Delete soft-deletes an enrollment created by mistake; leaving is End, not
// Delete.
func (s *Service) Delete(ctx context.Context, sc authctx.Scope, enrollmentID uuid.UUID) error {
	roles := authctx.StaffRolesFor(authctx.CapEnrollmentWrite)
	err := s.repo.SoftDelete(ctx, sc, roles, enrollmentID)
	if errors.Is(err, ErrNotFound) {
		// The write gate refused; a caller who can at least read the row gets
		// an honest 403 instead of a 404 that lies about existence.
		if _, rerr := s.repo.GetByID(ctx, sc, enrollmentID); rerr == nil {
			return apperror.Forbidden("your role on this class does not allow this action")
		}
	}
	return translate(err)
}

// ActiveOn exposes the attendance-sheet query plan 03 consumes: enrollments
// open on the given date, inclusive at both boundaries.
func (s *Service) ActiveOn(ctx context.Context, sc authctx.Scope, classID uuid.UUID, on time.Time) ([]Enrollment, error) {
	return s.repo.ActiveOn(ctx, sc, classID, on)
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

// FindByStudentAndClass returns this student's enrollment in this class, open
// or already ended, so a bulk caller can tell "never enrolled" from "left".
// uq_enrollments_active only covers open rows, so an ended enrollment is
// invisible to the database constraint and re-creating one would backdate a
// departed student onto every session since the class began.
func (s *Service) FindByStudentAndClass(ctx context.Context, sc authctx.Scope, studentID, classID uuid.UUID) (*Enrollment, bool, error) {
	e, err := s.repo.FindByStudentAndClass(ctx, sc, studentID, classID)
	if errors.Is(err, ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return e, true, nil
}
