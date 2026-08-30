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
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

type fakeContact struct {
	Contact
	deleted bool
}

// fakeRepository is an in-memory Repository enforcing the same invariants the
// SQL layer does: per-teacher phone uniqueness among non-deleted rows and
// center-scoped, oversight-gated reads.
type fakeRepository struct {
	rows     map[uuid.UUID]*fakeContact
	students map[uuid.UUID][]string // contactID -> live student names
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{rows: map[uuid.UUID]*fakeContact{}, students: map[uuid.UUID][]string{}}
}

// seed inserts a row directly, the way integration fixtures do — member
// writes cannot go through Create any more, but member-anchored rows still
// exist until the ownership migration re-anchors them.
func (f *fakeRepository) seed(teacherID, centerID uuid.UUID, name, phone string) *Contact {
	c := &Contact{ID: id.New(), TeacherID: teacherID, CenterID: centerID, FullName: name, Phone: phone}
	f.rows[c.ID] = &fakeContact{Contact: *c}
	return c
}

// visible mirrors scopedRead's oversight arm: the owner or a reports-oversight
// holder reads the whole center, whoever anchored the row. The hoc_vu reach
// arm is a SQL EXISTS (classscope.PhoneVisibleViaContact) the fake cannot
// model; the integration tests own it.
func visible(c *fakeContact, sc authctx.Scope) bool {
	if c.deleted || c.CenterID != sc.CenterID {
		return false
	}
	return sc.ReportsOversight()
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

func (f *fakeRepository) GetByID(_ context.Context, sc authctx.Scope, contactID uuid.UUID) (*Row, error) {
	c, ok := f.rows[contactID]
	if !ok || !visible(c, sc) {
		return nil, ErrNotFound
	}
	return &Row{Contact: c.Contact, StudentCount: int64(len(f.students[contactID]))}, nil
}

func (f *fakeRepository) List(_ context.Context, sc authctx.Scope, filter ListFilter, _ pagination.Params) ([]Row, int64, error) {
	var out []Row
	for _, c := range f.rows {
		if !visible(c, sc) {
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

func (f *fakeRepository) SoftDelete(_ context.Context, sc authctx.Scope, contactID uuid.UUID) error {
	c, ok := f.rows[contactID]
	if !ok || !visible(c, sc) {
		return ErrNotFound
	}
	c.deleted = true
	return nil
}

func (f *fakeRepository) CountActiveStudents(_ context.Context, _ authctx.Scope, contactID uuid.UUID) (int64, error) {
	return int64(len(f.students[contactID])), nil
}

func (f *fakeRepository) ListStudentNames(_ context.Context, _ authctx.Scope, contactID uuid.UUID, limit int) ([]string, error) {
	names := append([]string(nil), f.students[contactID]...)
	sort.Strings(names)
	if len(names) > limit {
		names = names[:limit]
	}
	return names, nil
}

func (f *fakeRepository) UpdateZaloMapping(_ context.Context, sc authctx.Scope, contactID uuid.UUID, zaloUserID, zaloName string) error {
	c, ok := f.rows[contactID]
	if !ok || !visible(c, sc) {
		return ErrNotFound
	}
	c.ZaloUserID = &zaloUserID
	c.ZaloName = &zaloName
	return nil
}

func (f *fakeRepository) ClearZaloMapping(_ context.Context, sc authctx.Scope, contactID uuid.UUID) error {
	c, ok := f.rows[contactID]
	if !ok || !visible(c, sc) {
		return ErrNotFound
	}
	c.ZaloUserID = nil
	c.ZaloName = nil
	return nil
}

// ownerScope returns a scope for a teacher who owns their own center.
func ownerScope() authctx.Scope {
	return authctx.Scope{TeacherID: id.New(), CenterID: id.New(), IsOwner: true}
}

// memberScope returns a scope for a non-owning member of some center.
func memberScope() authctx.Scope {
	return authctx.Scope{TeacherID: id.New(), CenterID: id.New(), IsOwner: false}
}

func TestCreateNormalisesPhone(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	sc := ownerScope()

	row, err := svc.Create(context.Background(), sc, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.Phone != "+84912345678" {
		t.Fatalf("local-form input must be stored as E.164, got %q", row.Phone)
	}
	if row.TeacherID != sc.TeacherID {
		t.Fatalf("teacher id must come from the caller's scope, got %s", row.TeacherID)
	}
	if row.CenterID != sc.CenterID {
		t.Fatalf("center id must come from the caller's scope, got %s", row.CenterID)
	}
	if row.StudentCount != 0 {
		t.Fatalf("fresh contact must report zero students, got %d", row.StudentCount)
	}
}

func TestCreateDuplicatePhoneIsConflict(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	sc := ownerScope()
	ctx := context.Background()

	if _, err := svc.Create(ctx, sc, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	// The E.164 spelling of the same number must collide with the local form.
	_, err := svc.Create(ctx, sc, CreateRequest{FullName: "Chị Hoa (bis)", Phone: "+84912345678"})
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
	sc := ownerScope()
	ctx := context.Background()

	first, err := svc.Create(ctx, sc, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := svc.Create(ctx, sc, CreateRequest{FullName: "Anh Tuấn", Phone: "0912345679"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	_, err = svc.Update(ctx, sc, second.ID, UpdateRequest{FullName: "Anh Tuấn", Phone: first.Phone})
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("want CONFLICT, got %v", err)
	}
}

func TestGetTranslatesNotFound(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.Get(context.Background(), memberScope(), id.New())
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("want NOT_FOUND, got %v", err)
	}
}

func TestDeleteBlockedByStudentsListsNames(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	sc := ownerScope()
	ctx := context.Background()

	row, err := svc.Create(ctx, sc, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 1; i <= 7; i++ {
		repo.students[row.ID] = append(repo.students[row.ID], fmt.Sprintf("Bé %d", i))
	}

	err = svc.Delete(ctx, sc, row.ID)
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

	if _, getErr := svc.Get(ctx, sc, row.ID); getErr != nil {
		t.Fatalf("blocked delete must leave the contact live: %v", getErr)
	}
}

func TestDeleteSoftDeletes(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	sc := ownerScope()
	ctx := context.Background()

	row, err := svc.Create(ctx, sc, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(ctx, sc, row.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(ctx, sc, row.ID); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("deleted contact must be gone, got %v", err)
	}
}

func TestDeleteOtherTenantsContactIsNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()

	row, err := svc.Create(ctx, ownerScope(), CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	err = svc.Delete(ctx, ownerScope(), row.ID)
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant delete must be NOT_FOUND, got %v", err)
	}
}

// An owner reads and manages a member-anchored contact — center oversight,
// not per-teacher isolation. The row is seeded directly because member writes
// are refused now; such rows exist until the ownership migration re-anchors
// them.
func TestOwnerScopeSeesAndDeletesMembersContact(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()
	center := id.New()
	member := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	owner := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: true}

	row := repo.seed(member.TeacherID, center, "Chị Hoa", "+84912345678")
	if _, err := svc.Get(ctx, owner, row.ID); err != nil {
		t.Fatalf("owner must read a member-anchored contact, got %v", err)
	}
	if err := svc.Delete(ctx, owner, row.ID); err != nil {
		t.Fatalf("owner must delete a member-anchored contact, got %v", err)
	}
}

// A peer in the same center without reports oversight has no contact reach —
// contacts are phone rows, and phones need oversight or an active hoc_vu
// stint (the latter is SQL-only, owned by the integration tests).
func TestPeerScopeCannotSeeAnotherMembersContact(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()
	center := id.New()
	author := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	peer := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}

	row := repo.seed(author.TeacherID, center, "Chị Hoa", "+84912345678")
	if _, err := svc.Get(ctx, peer, row.ID); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("peer must not read another member's contact, got %v", err)
	}
}

// Every contact write is the owner's: a member hits the gate before the
// repository is even consulted, so writability can never leak reach.
func TestMemberWritesAreForbidden(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()
	center := id.New()
	member := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	row := repo.seed(id.New(), center, "Chị Hoa", "+84912345678")

	_, err := svc.Create(ctx, member, CreateRequest{FullName: "Chị Lan", Phone: "0987654321"})
	if apperror.From(err).Code != apperror.CodeForbidden {
		t.Fatalf("member create must be FORBIDDEN, got %v", err)
	}
	_, err = svc.Update(ctx, member, row.ID, UpdateRequest{FullName: "Chị Hoa Sửa", Phone: "0912345678"})
	if apperror.From(err).Code != apperror.CodeForbidden {
		t.Fatalf("member update must be FORBIDDEN, got %v", err)
	}
	if err := svc.Delete(ctx, member, row.ID); apperror.From(err).Code != apperror.CodeForbidden {
		t.Fatalf("member delete must be FORBIDDEN, got %v", err)
	}
}

func TestUpdateZaloMappingSetsBothFields(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()
	sc := ownerScope()

	row, err := svc.Create(ctx, sc, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := svc.UpdateZaloMapping(ctx, sc, row.ID, ZaloMappingRequest{
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

	got, err := svc.Get(ctx, sc, row.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ZaloUserID == nil || *got.ZaloUserID != "8421000123456789" {
		t.Fatalf("mapping must persist, got %v", got.ZaloUserID)
	}
}

func TestUpdateZaloMappingOtherTenantsContactIsNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()

	row, err := svc.Create(ctx, ownerScope(), CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.UpdateZaloMapping(ctx, ownerScope(), row.ID, ZaloMappingRequest{
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
	sc := ownerScope()

	row, err := svc.Create(ctx, sc, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.UpdateZaloMapping(ctx, sc, row.ID, ZaloMappingRequest{
		ZaloUserID: "8421", ZaloName: "Hoa",
	}); err != nil {
		t.Fatalf("update mapping: %v", err)
	}

	if err := svc.ClearZaloMapping(ctx, sc, row.ID); err != nil {
		t.Fatalf("first clear: %v", err)
	}
	got, err := svc.Get(ctx, sc, row.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ZaloUserID != nil || got.ZaloName != nil {
		t.Fatalf("mapping must be cleared, got %v %v", got.ZaloUserID, got.ZaloName)
	}

	// Clearing an already-unmapped contact is a no-op success, mirroring the
	// unlink endpoint's idempotency.
	if err := svc.ClearZaloMapping(ctx, sc, row.ID); err != nil {
		t.Fatalf("second clear must succeed, got %v", err)
	}
}

func TestUpdateZaloMappingTrimsAndRejectsWhitespaceOnly(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	ctx := context.Background()
	sc := ownerScope()

	row, err := svc.Create(ctx, sc, CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Padded values are stored clean — the id is compared byte-for-byte when
	// the sender resolves a contact to a Zalo user.
	updated, err := svc.UpdateZaloMapping(ctx, sc, row.ID, ZaloMappingRequest{
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
	_, err = svc.UpdateZaloMapping(ctx, sc, row.ID, ZaloMappingRequest{
		ZaloUserID: "   ", ZaloName: "Hoa Nguyễn",
	})
	if apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("blank zalo_user_id must be VALIDATION_ERROR, got %v", err)
	}
	_, err = svc.UpdateZaloMapping(ctx, sc, row.ID, ZaloMappingRequest{
		ZaloUserID: "8421000123456789", ZaloName: "   ",
	})
	if apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("blank zalo_name must be VALIDATION_ERROR, got %v", err)
	}
}

func TestClearZaloMappingUnknownContactIsNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)

	err := svc.ClearZaloMapping(context.Background(), memberScope(), id.New())
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("unknown contact must be NOT_FOUND, got %v", err)
	}
}

// FindIDByPhone mirrors the SQL predicate: scope-visible, not soft-deleted,
// exact phone. Under owner-anchored contacts the visible set for an owner is
// the whole center, so the lookup is center-wide by phone.
func (f *fakeRepository) FindIDByPhone(_ context.Context, sc authctx.Scope, phone string) (uuid.UUID, bool, error) {
	for _, c := range f.rows {
		if visible(c, sc) && c.Phone == phone {
			return c.ID, true, nil
		}
	}
	return uuid.Nil, false, nil
}

// FindIDByPhone is the bulk-import lookup: it must normalise the way Create
// does and find one parent per phone center-wide, or an import would create a
// second row for a parent the center already knows.

func TestFindIDByPhoneNormalisesLikeCreate(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	sc := ownerScope()

	row, err := svc.Create(context.Background(), sc, CreateRequest{FullName: "Phạm Văn Hùng", Phone: "0901234567"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	for _, written := range []string{"0901234567", "+84901234567"} {
		got, found, err := svc.FindIDByPhone(context.Background(), sc, written)
		if err != nil || !found || got != row.ID {
			t.Fatalf("%s must resolve to the stored contact, got id=%v found=%v err=%v", written, got, found, err)
		}
	}
}

func TestFindIDByPhoneIsCenterWideForTheOwner(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	center := id.New()
	member := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	owner := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: true}

	// A member-anchored row left over from before the ownership migration: one
	// phone is one parent center-wide, so the owner's import lookup must find
	// it rather than plan a duplicate.
	row := repo.seed(member.TeacherID, center, "Phạm Văn Hùng", "+84901234567")

	got, found, err := svc.FindIDByPhone(context.Background(), owner, "0901234567")
	if err != nil || !found || got != row.ID {
		t.Fatalf("owner must find the member-anchored contact, got id=%v found=%v err=%v", got, found, err)
	}

	// A plain member has no oversight, so the same lookup misses.
	if _, found, err := svc.FindIDByPhone(context.Background(), member, "0901234567"); err != nil || found {
		t.Fatalf("a member without oversight must not find contacts, got found=%v err=%v", found, err)
	}
}

func TestFindIDByPhoneIgnoresDeletedContacts(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	sc := ownerScope()

	row, err := svc.Create(context.Background(), sc, CreateRequest{FullName: "Phạm Văn Hùng", Phone: "0901234567"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(context.Background(), sc, row.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// uq_contacts_phone is partial on deleted_at IS NULL, so a deleted row
	// holds no key and a fresh create succeeds — the lookup has to miss it or
	// the import would attach students to a contact nobody can see.
	if _, found, err := svc.FindIDByPhone(context.Background(), sc, "0901234567"); err != nil || found {
		t.Fatalf("a deleted contact must not be found, got found=%v err=%v", found, err)
	}
}
