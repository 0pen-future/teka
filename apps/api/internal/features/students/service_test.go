package students

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

type fakeStudent struct {
	Student
	deleted bool
}

type fakeContact struct {
	teacherID uuid.UUID
	name      string
	phone     string
}

// fakeRepository is an in-memory Repository enforcing the same invariants the
// SQL layer does: tenant-scoped reads, soft-delete filtering, and the
// composite-FK refusal of foreign contacts.
type fakeRepository struct {
	rows     map[uuid.UUID]*fakeStudent
	contacts map[uuid.UUID]fakeContact
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{rows: map[uuid.UUID]*fakeStudent{}, contacts: map[uuid.UUID]fakeContact{}}
}

func (f *fakeRepository) addContact(teacherID uuid.UUID) uuid.UUID {
	contactID := id.New()
	f.contacts[contactID] = fakeContact{teacherID: teacherID, name: "Chị Hoa", phone: "+84912345678"}
	return contactID
}

func (f *fakeRepository) row(s *fakeStudent) Row {
	c := f.contacts[s.ContactID]
	return Row{Student: s.Student, ContactName: c.name, ContactPhone: c.phone}
}

func (f *fakeRepository) Create(_ context.Context, s *Student) error {
	if c, ok := f.contacts[s.ContactID]; !ok || c.teacherID != s.TeacherID {
		return ErrContactNotOwned
	}
	f.rows[s.ID] = &fakeStudent{Student: *s}
	return nil
}

func (f *fakeRepository) GetByID(_ context.Context, teacherID, studentID uuid.UUID) (*Row, error) {
	s, ok := f.rows[studentID]
	if !ok || s.deleted || s.TeacherID != teacherID {
		return nil, ErrNotFound
	}
	row := f.row(s)
	return &row, nil
}

func (f *fakeRepository) List(_ context.Context, teacherID uuid.UUID, filter ListFilter, _ pagination.Params) ([]Row, int64, error) {
	var out []Row
	for _, s := range f.rows {
		if s.deleted || s.TeacherID != teacherID {
			continue
		}
		if filter.ContactID != uuid.Nil && s.ContactID != filter.ContactID {
			continue
		}
		out = append(out, f.row(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullName < out[j].FullName })
	return out, int64(len(out)), nil
}

func (f *fakeRepository) Update(_ context.Context, s *Student) error {
	if c, ok := f.contacts[s.ContactID]; !ok || c.teacherID != s.TeacherID {
		return ErrContactNotOwned
	}
	f.rows[s.ID] = &fakeStudent{Student: *s}
	return nil
}

func (f *fakeRepository) ContactExists(_ context.Context, teacherID, contactID uuid.UUID) (bool, error) {
	c, ok := f.contacts[contactID]
	return ok && c.teacherID == teacherID, nil
}

func (f *fakeRepository) AnonymizeAndDelete(_ context.Context, teacherID, studentID uuid.UUID, placeholder string) error {
	s, ok := f.rows[studentID]
	if !ok || s.deleted || s.TeacherID != teacherID {
		return ErrNotFound
	}
	now := time.Now()
	s.FullName = placeholder
	s.DisplayNote = nil
	s.AnonymizedAt = &now
	s.deleted = true
	return nil
}

// fakeEnder records EndOpenEnrollments calls so tests can assert the delete
// flow closes enrollments.
type fakeEnder struct {
	calls []uuid.UUID
	err   error
}

func (f *fakeEnder) EndOpenEnrollments(_ context.Context, _, studentID uuid.UUID, _ time.Time) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, studentID)
	return nil
}

type noopTx struct{}

func (noopTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func newTestService() (*Service, *fakeRepository, *fakeEnder) {
	repo := newFakeRepository()
	ender := &fakeEnder{}
	return NewService(repo, ender, noopTx{}), repo, ender
}

func TestCreateRejectsForeignContact(t *testing.T) {
	svc, repo, _ := newTestService()
	teacherID := id.New()
	foreignContact := repo.addContact(id.New())

	_, err := svc.Create(context.Background(), teacherID, CreateRequest{
		FullName: "Bé An", ContactID: foreignContact,
	})
	if !errors.Is(err, ErrContactNotOwned) {
		t.Fatalf("want ErrContactNotOwned cause, got %v", err)
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation {
		t.Fatalf("foreign contact must be a 422 validation error, got %v", err)
	}
	if appErr.Fields["contact_id"] == "" {
		t.Fatalf("the error must name the contact_id field, got %+v", appErr.Fields)
	}
}

func TestCreateReturnsContactDetails(t *testing.T) {
	svc, repo, _ := newTestService()
	teacherID := id.New()
	contactID := repo.addContact(teacherID)

	row, err := svc.Create(context.Background(), teacherID, CreateRequest{
		FullName: "Bé An", ContactID: contactID, DisplayNote: "An lớp 9A",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.ContactName != "Chị Hoa" || row.ContactPhone != "+84912345678" {
		t.Fatalf("row must carry the contact's details, got %+v", row)
	}
	if row.DisplayNote == nil || *row.DisplayNote != "An lớp 9A" {
		t.Fatalf("display note must persist, got %v", row.DisplayNote)
	}
}

func TestUpdateRechecksContactOnlyWhenChanged(t *testing.T) {
	svc, repo, _ := newTestService()
	teacherID := id.New()
	contactID := repo.addContact(teacherID)
	row, err := svc.Create(context.Background(), teacherID, CreateRequest{FullName: "Bé An", ContactID: contactID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Same contact: fine.
	if _, err := svc.Update(context.Background(), teacherID, row.ID, UpdateRequest{
		FullName: "Bé An (sửa)", ContactID: contactID,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Switching to a foreign contact: 422.
	foreignContact := repo.addContact(id.New())
	_, err = svc.Update(context.Background(), teacherID, row.ID, UpdateRequest{
		FullName: "Bé An", ContactID: foreignContact,
	})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation {
		t.Fatalf("foreign contact on update must be 422, got %v", err)
	}
}

func TestDeleteScrubsAndEndsEnrollments(t *testing.T) {
	svc, repo, ender := newTestService()
	teacherID := id.New()
	contactID := repo.addContact(teacherID)
	row, err := svc.Create(context.Background(), teacherID, CreateRequest{
		FullName: "Bé An", ContactID: contactID, DisplayNote: "An lớp 9A",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.Delete(context.Background(), teacherID, row.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	stored := repo.rows[row.ID]
	if stored.FullName != AnonymizedName {
		t.Fatalf("full name must become the placeholder %q, got %q", AnonymizedName, stored.FullName)
	}
	if stored.DisplayNote != nil {
		t.Fatalf("display note must be scrubbed, got %v", *stored.DisplayNote)
	}
	if stored.AnonymizedAt == nil {
		t.Fatal("anonymized_at must be stamped")
	}
	if !stored.deleted {
		t.Fatal("the row must be soft-deleted")
	}
	if len(ender.calls) != 1 || ender.calls[0] != row.ID {
		t.Fatalf("delete must end the student's open enrollments, calls: %v", ender.calls)
	}
}

func TestDeleteAbortsWhenEnrollmentClosureFails(t *testing.T) {
	svc, repo, ender := newTestService()
	teacherID := id.New()
	contactID := repo.addContact(teacherID)
	row, err := svc.Create(context.Background(), teacherID, CreateRequest{FullName: "Bé An", ContactID: contactID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	boom := errors.New("enrollments unavailable")
	ender.err = boom

	if err := svc.Delete(context.Background(), teacherID, row.ID); !errors.Is(err, boom) {
		t.Fatalf("want the ender error, got %v", err)
	}
	if repo.rows[row.ID].FullName != "Bé An" {
		t.Fatal("a failed enrollment closure must leave the student untouched")
	}
}

func TestCrossTenantReadsAsNotFound(t *testing.T) {
	svc, repo, _ := newTestService()
	owner := id.New()
	contactID := repo.addContact(owner)
	row, err := svc.Create(context.Background(), owner, CreateRequest{FullName: "Bé An", ContactID: contactID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	stranger := id.New()
	var appErr *apperror.AppError
	if _, err := svc.Get(context.Background(), stranger, row.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be NOT_FOUND, got %v", err)
	}
	if err := svc.Delete(context.Background(), stranger, row.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant delete must be NOT_FOUND, got %v", err)
	}
}
