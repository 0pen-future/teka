package students

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// EnrollmentEnder is the consumer-defined contract the enrollments feature
// implements: close every open enrollment for the student, effective on the
// given date. Declared here so students does not import enrollments, matching
// the consumer-interface pattern auth uses over teachers.
type EnrollmentEnder interface {
	EndOpenEnrollments(ctx context.Context, teacherID, studentID uuid.UUID, on time.Time) error
}

// Service owns student business rules: the same-teacher contact check and the
// anonymise-don't-erase delete.
type Service struct {
	repo  Repository
	ender EnrollmentEnder
	tx    database.TxManager
}

// NewService builds the students service.
func NewService(repo Repository, ender EnrollmentEnder, tx database.TxManager) *Service {
	return &Service{repo: repo, ender: ender, tx: tx}
}

// Create inserts a student after checking the contact is this teacher's. The
// composite FK would refuse a foreign contact anyway; the check exists to turn
// that refusal into a clean 422 instead of a 500.
func (s *Service) Create(ctx context.Context, teacherID uuid.UUID, req CreateRequest) (*Row, error) {
	if err := s.checkContact(ctx, teacherID, req.ContactID); err != nil {
		return nil, err
	}
	student := &Student{
		ID:          id.New(),
		TeacherID:   teacherID,
		ContactID:   req.ContactID,
		FullName:    req.FullName,
		DisplayNote: notePtr(req.DisplayNote),
	}
	if err := s.repo.Create(ctx, student); err != nil {
		return nil, translate(err)
	}
	return s.repo.GetByID(ctx, teacherID, student.ID)
}

// Get returns one student with its contact details.
func (s *Service) Get(ctx context.Context, teacherID, studentID uuid.UUID) (*Row, error) {
	row, err := s.repo.GetByID(ctx, teacherID, studentID)
	if err != nil {
		return nil, translate(err)
	}
	return row, nil
}

// List returns a page of students with contact details.
func (s *Service) List(ctx context.Context, teacherID uuid.UUID, filter ListFilter, p pagination.Params) ([]Row, int64, error) {
	return s.repo.List(ctx, teacherID, filter, p)
}

// Update edits the closed field list, re-checking contact ownership when the
// contact changes.
func (s *Service) Update(ctx context.Context, teacherID, studentID uuid.UUID, req UpdateRequest) (*Row, error) {
	row, err := s.repo.GetByID(ctx, teacherID, studentID)
	if err != nil {
		return nil, translate(err)
	}
	if req.ContactID != row.ContactID {
		if err := s.checkContact(ctx, teacherID, req.ContactID); err != nil {
			return nil, err
		}
	}
	student := row.Student
	student.ContactID = req.ContactID
	student.FullName = req.FullName
	student.DisplayNote = notePtr(req.DisplayNote)
	if err := s.repo.Update(ctx, &student); err != nil {
		return nil, translate(err)
	}
	return s.repo.GetByID(ctx, teacherID, studentID)
}

// Delete anonymises rather than erases: in one transaction it ends the
// student's open enrollments (a deleted student must stop appearing on future
// attendance sheets) and issues the scrub-and-stamp UPDATE. Historical
// attendance records are untouched — deleting them would change billable
// counts already reported to a parent.
func (s *Service) Delete(ctx context.Context, teacherID, studentID uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, teacherID, studentID); err != nil {
		return translate(err)
	}
	today := time.Now()
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.ender.EndOpenEnrollments(ctx, teacherID, studentID, today); err != nil {
			return err
		}
		return translate(s.repo.AnonymizeAndDelete(ctx, teacherID, studentID, AnonymizedName))
	})
}

// checkContact turns a foreign or missing contact into the 422 the API
// contract promises.
func (s *Service) checkContact(ctx context.Context, teacherID, contactID uuid.UUID) error {
	ok, err := s.repo.ContactExists(ctx, teacherID, contactID)
	if err != nil {
		return err
	}
	if !ok {
		return contactInvalid()
	}
	return nil
}

func contactInvalid() error {
	appErr := apperror.Invalid("validation failed",
		map[string]string{"contact_id": "must reference one of your contacts"})
	appErr.Err = ErrContactNotOwned
	return appErr
}

// translate maps domain errors onto the API error contract, keeping the domain
// error as the cause so errors.Is still works.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apperror.NotFound("student")
	case errors.Is(err, ErrContactNotOwned):
		return contactInvalid()
	default:
		return err
	}
}
