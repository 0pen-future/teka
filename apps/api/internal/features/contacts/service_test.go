package contacts

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

type fakeContact struct {
	Contact
	deleted bool
}

// fakeRepository is an in-memory Repository enforcing the same invariants the
// SQL layer does: per-teacher phone uniqueness among non-deleted rows and
// tenant-scoped reads.
type fakeRepository struct {
	rows     map[uuid.UUID]*fakeContact
	students map[uuid.UUID][]string // contactID -> live student names
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{rows: map[uuid.UUID]*fakeContact{}, students: map[uuid.UUID][]string{}}
}

func (f *fakeRepository) Create(_ context.Context, c *Contact) error {
	for _, existing := range f.rows {
		if !existing.deleted && existing.TeacherID == c.TeacherID && existing.Phone == c.Phone {
			return ErrDuplicatePhone
		}
	}
	f.rows[c.ID] = &fakeContact{Contact: *c}
	return nil
}

func (f *fakeRepository) GetByID(_ context.Context, teacherID, contactID uuid.UUID) (*Row, error) {
	c, ok := f.rows[contactID]
	if !ok || c.deleted || c.TeacherID != teacherID {
		return nil, ErrNotFound
	}
	return &Row{Contact: c.Contact, StudentCount: int64(len(f.students[contactID]))}, nil
}

func (f *fakeRepository) List(_ context.Context, teacherID uuid.UUID, filter ListFilter, _ pagination.Params) ([]Row, int64, error) {
	var out []Row
	for _, c := range f.rows {
		if c.deleted || c.TeacherID != teacherID {
			continue
		}
		if q := strings.ToLower(filter.Query); q != "" &&
			!strings.Contains(strings.ToLower(c.FullName), q) && !strings.Contains(c.Phone, q) {
			continue
		}
		out = append(out, Row{Contact: c.Contact, StudentCount: int64(len(f.students[c.ID]))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullName < out[j].FullName })
	return out, int64(len(out)), nil
}

func (f *fakeRepository) Update(_ context.Context, c *Contact) error {
	for otherID, existing := range f.rows {
		if otherID != c.ID && !existing.deleted && existing.TeacherID == c.TeacherID && existing.Phone == c.Phone {
			return ErrDuplicatePhone
		}
	}
	f.rows[c.ID] = &fakeContact{Contact: *c}
	return nil
}

func (f *fakeRepository) SoftDelete(_ context.Context, teacherID, contactID uuid.UUID) error {
	c, ok := f.rows[contactID]
	if !ok || c.deleted || c.TeacherID != teacherID {
		return ErrNotFound
	}
	c.deleted = true
	return nil
}

func (f *fakeRepository) CountActiveStudents(_ context.Context, _, contactID uuid.UUID) (int64, error) {
	return int64(len(f.students[contactID])), nil
}

func (f *fakeRepository) ListStudentNames(_ context.Context, _, contactID uuid.UUID, limit int) ([]string, error) {
	names := append([]string(nil), f.students[contactID]...)
	sort.Strings(names)
	if len(names) > limit {
		names = names[:limit]
	}
	return names, nil
}

func (f *fakeRepository) UpdateZaloMapping(_ context.Context, teacherID, contactID uuid.UUID, zaloUserID, zaloName string) error {
	c, ok := f.rows[contactID]
	if !ok || c.deleted || c.TeacherID != teacherID {
		return ErrNotFound
	}
	c.ZaloUserID = &zaloUserID
	c.ZaloName = &zaloName
	return nil
}

func (f *fakeRepository) ClearZaloMapping(_ context.Context, teacherID, contactID uuid.UUID) error {
	c, ok := f.rows[contactID]
	if !ok || c.deleted || c.TeacherID != teacherID {
		return ErrNotFound
	}
	c.ZaloUserID = nil
	c.ZaloName = nil
	return nil
}

func TestCreateNormalisesPhone(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	teacherID := id.New()

	row, err := svc.Create(context.Background(), teacherID, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.Phone != "+84912345678" {
		t.Fatalf("local-form input must be stored as E.164, got %q", row.Phone)
	}
	if row.TeacherID != teacherID {
		t.Fatalf("teacher id must come from the caller, got %s", row.TeacherID)
	}
	if row.StudentCount != 0 {
		t.Fatalf("fresh contact must report zero students, got %d", row.StudentCount)
	}
}

func TestCreateDuplicatePhoneIsConflict(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	teacherID := id.New()
	ctx := context.Background()

	if _, err := svc.Create(ctx, teacherID, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// The E.164 spelling of the same number must collide with the local form.
	_, err := svc.Create(ctx, teacherID, CreateRequest{FullName: "Chị Hoa (bis)", Phone: "+84912345678"})
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("want CONFLICT, got %v", err)
	}
	if !errors.Is(err, ErrDuplicatePhone) {
		t.Fatalf("conflict must keep ErrDuplicatePhone as cause, got %v", err)
	}
}

func TestUpdateDuplicatePhoneIsConflict(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	teacherID := id.New()
	ctx := context.Background()

	first, err := svc.Create(ctx, teacherID, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := svc.Create(ctx, teacherID, CreateRequest{FullName: "Anh Tuấn", Phone: "0912345679"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	_, err = svc.Update(ctx, teacherID, second.ID, UpdateRequest{FullName: "Anh Tuấn", Phone: first.Phone})
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("want CONFLICT, got %v", err)
	}
}

func TestGetTranslatesNotFound(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.Get(context.Background(), id.New(), id.New())
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("want NOT_FOUND, got %v", err)
	}
}

func TestDeleteBlockedByStudentsListsNames(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	teacherID := id.New()
	ctx := context.Background()

	row, err := svc.Create(ctx, teacherID, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 1; i <= 7; i++ {
		repo.students[row.ID] = append(repo.students[row.ID], fmt.Sprintf("Bé %d", i))
	}

	err = svc.Delete(ctx, teacherID, row.ID)
	appErr := apperror.From(err)
	if appErr.Code != apperror.CodeConflict {
		t.Fatalf("want CONFLICT, got %v", err)
	}
	if !errors.Is(err, ErrHasStudents) {
		t.Fatalf("conflict must keep ErrHasStudents as cause, got %v", err)
	}
	// Five names spelled out, the remaining two collapsed into a count.
	for i := 1; i <= 5; i++ {
		if !strings.Contains(appErr.Message, fmt.Sprintf("Bé %d", i)) {
			t.Fatalf("message must name blocking student %d: %q", i, appErr.Message)
		}
	}
	if !strings.Contains(appErr.Message, "7 student(s)") || !strings.Contains(appErr.Message, "and 2 more") {
		t.Fatalf("message must carry total and overflow count: %q", appErr.Message)
	}

	if _, getErr := svc.Get(ctx, teacherID, row.ID); getErr != nil {
		t.Fatalf("blocked delete must leave the contact live: %v", getErr)
	}
}

func TestDeleteSoftDeletes(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	teacherID := id.New()
	ctx := context.Background()

	row, err := svc.Create(ctx, teacherID, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(ctx, teacherID, row.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(ctx, teacherID, row.ID); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("deleted contact must be gone, got %v", err)
	}
}

func TestDeleteOtherTeachersContactIsNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()

	row, err := svc.Create(ctx, id.New(), CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	err = svc.Delete(ctx, id.New(), row.ID)
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant delete must be NOT_FOUND, got %v", err)
	}
}

func TestUpdateZaloMappingSetsBothFields(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()
	teacherID := id.New()

	row, err := svc.Create(ctx, teacherID, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := svc.UpdateZaloMapping(ctx, teacherID, row.ID, ZaloMappingRequest{
		ZaloUserID: "8421000123456789", ZaloName: "Hoa Nguyễn",
	})
	if err != nil {
		t.Fatalf("update mapping: %v", err)
	}
	if updated.ZaloUserID == nil || *updated.ZaloUserID != "8421000123456789" {
		t.Fatalf("zalo_user_id not stored, got %v", updated.ZaloUserID)
	}
	if updated.ZaloName == nil || *updated.ZaloName != "Hoa Nguyễn" {
		t.Fatalf("zalo_name not stored, got %v", updated.ZaloName)
	}

	got, err := svc.Get(ctx, teacherID, row.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ZaloUserID == nil || *got.ZaloUserID != "8421000123456789" {
		t.Fatalf("mapping must persist, got %v", got.ZaloUserID)
	}
}

func TestUpdateZaloMappingOtherTeachersContactIsNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()

	row, err := svc.Create(ctx, id.New(), CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.UpdateZaloMapping(ctx, id.New(), row.ID, ZaloMappingRequest{
		ZaloUserID: "8421", ZaloName: "Hoa",
	})
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant mapping must be NOT_FOUND, got %v", err)
	}
}

func TestClearZaloMappingIsIdempotent(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()
	teacherID := id.New()

	row, err := svc.Create(ctx, teacherID, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.UpdateZaloMapping(ctx, teacherID, row.ID, ZaloMappingRequest{
		ZaloUserID: "8421", ZaloName: "Hoa",
	}); err != nil {
		t.Fatalf("update mapping: %v", err)
	}

	if err := svc.ClearZaloMapping(ctx, teacherID, row.ID); err != nil {
		t.Fatalf("first clear: %v", err)
	}
	got, err := svc.Get(ctx, teacherID, row.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ZaloUserID != nil || got.ZaloName != nil {
		t.Fatalf("mapping must be cleared, got %v %v", got.ZaloUserID, got.ZaloName)
	}

	// Clearing an already-unmapped contact is a no-op success, mirroring the
	// unlink endpoint's idempotency.
	if err := svc.ClearZaloMapping(ctx, teacherID, row.ID); err != nil {
		t.Fatalf("second clear must succeed, got %v", err)
	}
}

func TestUpdateZaloMappingTrimsAndRejectsWhitespaceOnly(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()
	teacherID := id.New()

	row, err := svc.Create(ctx, teacherID, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Padded values are stored clean — the id is compared byte-for-byte when
	// the sender resolves a contact to a Zalo user.
	updated, err := svc.UpdateZaloMapping(ctx, teacherID, row.ID, ZaloMappingRequest{
		ZaloUserID: "  8421000123456789  ", ZaloName: "  Hoa Nguyễn  ",
	})
	if err != nil {
		t.Fatalf("update mapping: %v", err)
	}
	if updated.ZaloUserID == nil || *updated.ZaloUserID != "8421000123456789" {
		t.Fatalf("zalo_user_id must be trimmed, got %v", updated.ZaloUserID)
	}
	if updated.ZaloName == nil || *updated.ZaloName != "Hoa Nguyễn" {
		t.Fatalf("zalo_name must be trimmed, got %v", updated.ZaloName)
	}

	// Whitespace-only slips past the required binding but is still no mapping.
	_, err = svc.UpdateZaloMapping(ctx, teacherID, row.ID, ZaloMappingRequest{
		ZaloUserID: "   ", ZaloName: "Hoa Nguyễn",
	})
	if apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("blank zalo_user_id must be VALIDATION_ERROR, got %v", err)
	}
	_, err = svc.UpdateZaloMapping(ctx, teacherID, row.ID, ZaloMappingRequest{
		ZaloUserID: "8421000123456789", ZaloName: "   ",
	})
	if apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("blank zalo_name must be VALIDATION_ERROR, got %v", err)
	}
}

func TestClearZaloMappingUnknownContactIsNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)

	err := svc.ClearZaloMapping(context.Background(), id.New(), id.New())
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("unknown contact must be NOT_FOUND, got %v", err)
	}
}
