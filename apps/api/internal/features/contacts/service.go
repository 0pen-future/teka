package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/shared/validation"
)

// blockingNamesShown caps how many blocking student names the delete-conflict
// message spells out; the rest collapse into a count.
const blockingNamesShown = 5

// Service owns contact business rules: phone normalisation, tenant scoping,
// and the delete guard.
type Service struct {
	repo Repository
}

// NewService builds the contacts service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create inserts a contact with a normalised E.164 phone. Duplicate phones are
// detected by the partial unique index, never by a pre-check SELECT. The
// contact is always stamped as the caller's own — including an owner, who
// creates rows as themselves, never on behalf of another teacher.
func (s *Service) Create(ctx context.Context, sc authctx.Scope, req CreateRequest) (*Row, error) {
	c := &Contact{
		ID:        id.New(),
		TeacherID: sc.TeacherID,
		CenterID:  sc.CenterID,
		FullName:  req.FullName,
		Phone:     validation.NormalizePhone(req.Phone),
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, translate(err)
	}
	// A contact fresh out of Create cannot have students yet.
	return &Row{Contact: *c}, nil
}

// Get returns one contact with its live-student count.
func (s *Service) Get(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) (*Row, error) {
	row, err := s.repo.GetByID(ctx, sc, contactID)
	if err != nil {
		return nil, translate(err)
	}
	return row, nil
}

// List returns a page of contacts with student counts.
func (s *Service) List(ctx context.Context, sc authctx.Scope, filter ListFilter, p pagination.Params) ([]Row, int64, error) {
	return s.repo.List(ctx, sc, filter, p)
}

// Update replaces both editable fields, re-normalising the phone.
func (s *Service) Update(ctx context.Context, sc authctx.Scope, contactID uuid.UUID, req UpdateRequest) (*Row, error) {
	row, err := s.repo.GetByID(ctx, sc, contactID)
	if err != nil {
		return nil, translate(err)
	}
	c := row.Contact
	c.FullName = req.FullName
	c.Phone = validation.NormalizePhone(req.Phone)
	if err := s.repo.Update(ctx, &c); err != nil {
		return nil, translate(err)
	}
	row.Contact = c
	return row, nil
}

// Delete soft-deletes a contact unless live students still reference it. The
// count-then-delete sequence is racy in principle; that is acceptable — the FK
// only restricts hard deletes, the count exists for a helpful error, and the
// worst case (soft-deleted contact with a live student) is visible and
// reversible. Do not add locking for a single-teacher workload.
func (s *Service) Delete(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, sc, contactID); err != nil {
		return translate(err)
	}
	total, err := s.repo.CountActiveStudents(ctx, sc, contactID)
	if err != nil {
		return err
	}
	if total > 0 {
		names, err := s.repo.ListStudentNames(ctx, sc, contactID, blockingNamesShown)
		if err != nil {
			return err
		}
		appErr := apperror.Conflict("contact still has " + blockingDetail(total, names))
		appErr.Err = ErrHasStudents
		return appErr
	}
	return translate(s.repo.SoftDelete(ctx, sc, contactID))
}

// UpdateZaloMapping binds the contact to the Zalo friend the teacher picked
// and returns the updated row. Values are stored trimmed: the id is compared
// byte-for-byte when the sender resolves a contact, and the required binding
// tag cannot see that a padded value is really blank.
func (s *Service) UpdateZaloMapping(ctx context.Context, sc authctx.Scope, contactID uuid.UUID, req ZaloMappingRequest) (*Row, error) {
	req.ZaloUserID = strings.TrimSpace(req.ZaloUserID)
	req.ZaloName = strings.TrimSpace(req.ZaloName)
	fields := map[string]string{}
	if req.ZaloUserID == "" {
		fields["zalo_user_id"] = "must not be blank"
	}
	if req.ZaloName == "" {
		fields["zalo_name"] = "must not be blank"
	}
	if len(fields) > 0 {
		return nil, apperror.Invalid("validation failed", fields)
	}
	if err := s.repo.UpdateZaloMapping(ctx, sc, contactID, req.ZaloUserID, req.ZaloName); err != nil {
		return nil, translate(err)
	}
	row, err := s.repo.GetByID(ctx, sc, contactID)
	if err != nil {
		return nil, translate(err)
	}
	return row, nil
}

// ClearZaloMapping detaches the contact from its Zalo friend. Clearing an
// unmapped contact succeeds — the caller's intent is already true.
func (s *Service) ClearZaloMapping(ctx context.Context, sc authctx.Scope, contactID uuid.UUID) error {
	return translate(s.repo.ClearZaloMapping(ctx, sc, contactID))
}

// blockingDetail renders "3 student(s): An, Bình, Chi" with an "and N more"
// tail when the list is truncated.
func blockingDetail(total int64, names []string) string {
	detail := fmt.Sprintf("%d student(s): %s", total, strings.Join(names, ", "))
	if extra := total - int64(len(names)); extra > 0 {
		detail += fmt.Sprintf(" and %d more", extra)
	}
	return detail
}

// translate maps domain errors onto the API error contract, keeping the domain
// error as the cause so errors.Is still works.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apperror.NotFound("contact")
	case errors.Is(err, ErrDuplicatePhone):
		appErr := apperror.Conflict(ErrDuplicatePhone.Error())
		appErr.Err = err
		return appErr
	case errors.Is(err, ErrDuplicateZaloMapping):
		appErr := apperror.Conflict(ErrDuplicateZaloMapping.Error())
		appErr.Err = err
		return appErr
	default:
		return err
	}
}

// FindIDByPhone resolves a live contact by its exact E.164 phone inside the
// scope — the create-or-reuse seam for bulk flows, matching the shape of
// uq_contacts_phone(teacher_id, phone).
func (s *Service) FindIDByPhone(ctx context.Context, sc authctx.Scope, phone string) (uuid.UUID, bool, error) {
	return s.repo.FindIDByPhone(ctx, sc, validation.NormalizePhone(phone))
}
