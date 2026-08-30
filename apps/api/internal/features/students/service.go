package students

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

// EnrollmentEnder is the consumer-defined contract the enrollments feature
// implements: close every open enrollment for the student, effective on the
// given date. Declared here so students does not import enrollments, matching
// the consumer-interface pattern auth uses over teachers.
type EnrollmentEnder interface {
	EndOpenEnrollments(ctx context.Context, sc authctx.Scope, studentID uuid.UUID, on time.Time) error
}

// Service owns student business rules: the same-center contact check and the
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

// Create inserts a student. Owner-only: students anchor to the center's
// owner, so no member — teacher or otherwise — creates them. The contact
// check turns the composite FK's refusal of a foreign contact into a clean
// 422; it asks only that the contact belongs to the center, since every
// contact anchors to the owner too.
func (s *Service) Create(ctx context.Context, sc authctx.Scope, req CreateRequest) (*Row, error) {
	if !sc.IsOwner {
		return nil, apperror.Forbidden("chỉ chủ trung tâm quản lý hồ sơ học sinh")
	}
	if err := s.checkContact(ctx, sc, req.ContactID); err != nil {
		return nil, err
	}
	student := &Student{
		ID:          id.New(),
		TeacherID:   sc.TeacherID,
		CenterID:    sc.CenterID,
		ContactID:   req.ContactID,
		FullName:    req.FullName,
		DisplayNote: notePtr(req.DisplayNote),
	}
	if err := s.repo.Create(ctx, student); err != nil {
		return nil, translate(err)
	}
	row, err := s.repo.GetByID(ctx, sc, student.ID)
	return maskPhone(sc, row), err
}

// Get returns one student with its contact details.
func (s *Service) Get(ctx context.Context, sc authctx.Scope, studentID uuid.UUID) (*Row, error) {
	row, err := s.repo.GetByID(ctx, sc, studentID)
	if err != nil {
		return nil, translate(err)
	}
	return maskPhone(sc, row), nil
}

// List returns a page of students with contact details.
func (s *Service) List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Row, int64, error) {
	rows, total, err := s.repo.List(ctx, sc, filter, p)
	for i := range rows {
		maskPhone(sc, &rows[i])
	}
	return rows, total, err
}

// maskPhone enforces the one phone rule at the service boundary: the repo's
// phone_visible column carries the row grant (active hoc_vu on a class with an
// active enrollment); the owner/oversight bypass lives in Scope.PhoneVisible.
// Masked means nil — the wire form is JSON null, never an empty string.
func maskPhone(sc authctx.Scope, row *Row) *Row {
	if row != nil && !sc.PhoneVisible(row.PhoneVisible) {
		row.ContactPhone = nil
	}
	return row
}

// Update edits the closed field list, re-checking the contact when it
// changes. Owner-only, like Create — the widened GetByID read cannot leak
// writability because the gate fires before the fetch.
func (s *Service) Update(ctx context.Context, sc authctx.Scope, studentID uuid.UUID, req UpdateRequest) (*Row, error) {
	if !sc.IsOwner {
		return nil, apperror.Forbidden("chỉ chủ trung tâm quản lý hồ sơ học sinh")
	}
	row, err := s.repo.GetByID(ctx, sc, studentID)
	if err != nil {
		return nil, translate(err)
	}
	if req.ContactID != row.ContactID {
		if err := s.checkContact(ctx, sc, req.ContactID); err != nil {
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
	updated, err := s.repo.GetByID(ctx, sc, studentID)
	return maskPhone(sc, updated), err
}

// Delete anonymises rather than erases: in one transaction it ends the
// student's open enrollments (a deleted student must stop appearing on future
// attendance sheets) and issues the scrub-and-stamp UPDATE. Historical
// attendance records are untouched — deleting them would change billable
// counts already reported to a parent.
func (s *Service) Delete(ctx context.Context, sc authctx.Scope, studentID uuid.UUID) error {
	if !sc.IsOwner {
		return apperror.Forbidden("chỉ chủ trung tâm quản lý hồ sơ học sinh")
	}
	if _, err := s.repo.GetByID(ctx, sc, studentID); err != nil {
		return translate(err)
	}
	today := time.Now()
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.ender.EndOpenEnrollments(ctx, sc, studentID, today); err != nil {
			return err
		}
		return translate(s.repo.AnonymizeAndDelete(ctx, sc, studentID, AnonymizedName))
	})
}

// checkContact turns a foreign or missing contact into the 422 the API
// contract promises.
func (s *Service) checkContact(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) error {
	ok, err := s.repo.ContactExists(ctx, sc, contactID)
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

// FindIDByName resolves a live student by their identity within one contact:
// exact name plus the note that distinguishes same-named siblings. note is a
// pointer, and nil means "no note" rather than "any note" — display_note is
// NULL when unset, which is the common case.
func (s *Service) FindIDByName(ctx context.Context, sc authctx.Scope, contactID uuid.UUID, fullName string, note *string) (uuid.UUID, bool, error) {
	return s.repo.FindIDByName(ctx, sc, contactID, fullName, note)
}
