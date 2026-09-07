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

// create is the shared core behind Create and CreateAnchored: it inserts the
// enrollment anchored on a, publishes StudentEnrolled with actor as the
// acting party — the two differ for the roster import, where a member drives
// writes onto a class's own teacher — and reads the row back through an
// anchored lookup, since actor and a can name different teachers and the
// own-rows/WriteWide filter behind GetByID cannot be relied on to see it.
//
// The two pre-checks exist to produce clean 422s and to read the price; the
// composite FKs are what actually prevent cross-center stitching, and
// uq_enrollments_active — not a pre-check — is what refuses a duplicate open
// enrollment. The CLASS check runs against a with no owner bypass (an Anchor
// carries none): a row written against a class a does not itself own would
// carry a foreign anchor while living outside that class's own roster —
// invisible to its real teacher's attendance and billing, and unrepeatable
// under uq_enrollments_active. The STUDENT check is center-scoped instead:
// students anchor to the owner, so requiring a's own teacher_id would refuse
// every legitimate reference.
func (s *Service) create(ctx context.Context, actor authctx.Scope, a authctx.Anchor, req CreateRequest) (*Row, error) {
	price, err := s.repo.ClassDefaultPrice(ctx, a, req.ClassID)
	if errors.Is(err, ErrClassNotFound) {
		return nil, refInvalid("class_id", "must reference one of your classes", err)
	}
	if err != nil {
		return nil, err
	}
	ok, err := s.repo.StudentExists(ctx, actor, req.StudentID)
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
		TeacherID: a.TeacherID,
		CenterID:  a.CenterID,
		StudentID: req.StudentID,
		ClassID:   req.ClassID,
		StartedOn: startedOn,
		UnitPrice: price,
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return nil, translate(err)
	}
	if s.bus != nil {
		// Enrolling widens what the row's teacher can read and feeds the next
		// billing close, so every successful create leaves an event for the
		// audit trail. ActorID is who acted, not who the row is anchored to.
		s.bus.Publish(StudentEnrolled{
			OccurredAt:   time.Now().UTC(),
			CenterID:     a.CenterID,
			ActorID:      actor.TeacherID,
			EnrollmentID: e.ID,
			ClassID:      e.ClassID,
			StudentID:    e.StudentID,
		})
	}
	return s.repo.GetByIDAnchored(ctx, a, e.ID)
}

// Create enrolls a student under the caller's own anchor: actor and anchor
// name the same party, so this is Create's pre-migration behavior exactly.
func (s *Service) Create(ctx context.Context, sc authctx.Scope, req CreateRequest) (*Row, error) {
	return s.create(ctx, sc, sc.Self(), req)
}

// CreateAnchored enrolls a student on a proven anchor that may differ from
// the caller: the roster import writes each enrollment onto its class's own
// teacher (a) while the importing member or owner (actor) is who the audit
// event attributes the action to. a carries no owner authority of its own —
// the caller resolved it from the class row itself, not from an OwnerAnchor.
func (s *Service) CreateAnchored(ctx context.Context, actor authctx.Scope, a authctx.Anchor, req CreateRequest) (*Row, error) {
	return s.create(ctx, actor, a, req)
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
	if !sc.CenterWideFor(authctx.PermEnrollmentsViewAll) {
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

// ActiveOnClass is ActiveOn's center-scoped sibling: billing.EnrollmentSource
// calls it from post-close reconciliation so a confirming caller with no
// stint or ownership on the session's class (a teaching assistant, most
// commonly) still resolves that class's real roster instead of an empty one.
func (s *Service) ActiveOnClass(ctx context.Context, sc authctx.Scope, classID uuid.UUID, on time.Time) ([]Enrollment, error) {
	return s.repo.ActiveOnClass(ctx, sc, classID, on)
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

// foundOrNot is the shared tail behind FindByStudentAndClass and its anchored
// sibling: ErrNotFound becomes found=false, any other error propagates.
func foundOrNot(e *Enrollment, err error) (*Enrollment, bool, error) {
	if errors.Is(err, ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return e, true, nil
}

// FindByStudentAndClass returns this student's enrollment in this class, open
// or already ended, so a bulk caller can tell "never enrolled" from "left".
// uq_enrollments_active only covers open rows, so an ended enrollment is
// invisible to the database constraint and re-creating one would backdate a
// departed student onto every session since the class began. Scoped to the
// caller's own rows (WriteWide-aware) — the direct-write path's own gate.
func (s *Service) FindByStudentAndClass(ctx context.Context, sc authctx.Scope, studentID, classID uuid.UUID) (*Enrollment, bool, error) {
	return foundOrNot(s.repo.FindByStudentAndClass(ctx, sc, studentID, classID))
}

// FindByStudentAndClassAnchored is FindByStudentAndClass's unconditional
// sibling for the roster import: a is the class's own teacher anchor, already
// resolved by the caller, not the importing caller's own rows.
func (s *Service) FindByStudentAndClassAnchored(ctx context.Context, a authctx.Anchor, studentID, classID uuid.UUID) (*Enrollment, bool, error) {
	return foundOrNot(s.repo.FindByStudentAndClassAnchored(ctx, a, studentID, classID))
}
