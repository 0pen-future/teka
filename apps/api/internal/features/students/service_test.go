package students

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

type fakeStudent struct {
	Student
	deleted bool
}

type fakeContact struct {
	teacherID uuid.UUID
	centerID  uuid.UUID
	name      string
	phone     string
}

// fakeRepository is an in-memory Repository enforcing the same invariants the
// SQL layer does: center-scoped reads (owner sees the whole center, a member
// only their own rows), soft-delete filtering, and the composite-FK refusal
// of foreign contacts.
type fakeRepository struct {
	rows     map[uuid.UUID]*fakeStudent
	contacts map[uuid.UUID]fakeContact
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{rows: map[uuid.UUID]*fakeStudent{}, contacts: map[uuid.UUID]fakeContact{}}
}

func (f *fakeRepository) addContact(teacherID uuid.UUID) uuid.UUID {
	return f.addContactIn(teacherID, teacherID)
}

// addContactIn registers a fixture contact under an explicit center, letting
// tests build an owner/member scope pair that shares one center.
func (f *fakeRepository) addContactIn(teacherID, centerID uuid.UUID) uuid.UUID {
	contactID := id.New()
	f.contacts[contactID] = fakeContact{teacherID: teacherID, centerID: centerID, name: "Chị Hoa", phone: "+84912345678"}
	return contactID
}

func (f *fakeRepository) row(s *fakeStudent) Row {
	c := f.contacts[s.ContactID]
	return Row{Student: s.Student, ContactName: c.name, ContactPhone: c.phone}
}

// visible mirrors the real scoped() predicate: always the center, plus the
// teacher when the caller is not an owner.
func visible(s *fakeStudent, sc authctx.Scope) bool {
	if s.deleted || s.CenterID != sc.CenterID {
		return false
	}
	return sc.IsOwner || s.TeacherID == sc.TeacherID
}

func (f *fakeRepository) Create(_ context.Context, s *Student) error {
	if c, ok := f.contacts[s.ContactID]; !ok || c.centerID != s.CenterID {
		return ErrContactNotOwned
	}
	f.rows[s.ID] = &fakeStudent{Student: *s}
	return nil
}

func (f *fakeRepository) GetByID(_ context.Context, sc authctx.Scope, studentID uuid.UUID) (*Row, error) {
	s, ok := f.rows[studentID]
	if !ok || !visible(s, sc) {
		return nil, ErrNotFound
	}
	row := f.row(s)
	return &row, nil
}

func (f *fakeRepository) List(_ context.Context, sc authctx.Scope, filter ListFilter, _ pagination.Params) ([]Row, int64, error) {
	var out []Row
	for _, s := range f.rows {
		if !visible(s, sc) {
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
	if c, ok := f.contacts[s.ContactID]; !ok || c.centerID != s.CenterID {
		return ErrContactNotOwned
	}
	f.rows[s.ID] = &fakeStudent{Student: *s}
	return nil
}

func (f *fakeRepository) ContactExists(_ context.Context, sc authctx.Scope, contactID uuid.UUID) (bool, error) {
	c, ok := f.contacts[contactID]
	if !ok || c.centerID != sc.CenterID {
		return false, nil
	}
	return sc.IsOwner || c.teacherID == sc.TeacherID, nil
}

func (f *fakeRepository) AnonymizeAndDelete(_ context.Context, sc authctx.Scope, studentID uuid.UUID, placeholder string) error {
	s, ok := f.rows[studentID]
	if !ok || !visible(s, sc) {
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

func (f *fakeEnder) EndOpenEnrollments(_ context.Context, _ authctx.Scope, studentID uuid.UUID, _ time.Time) error {
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

// ownerScope returns a scope for a teacher who owns their own center.
func ownerScope() authctx.Scope {
	return authctx.Scope{TeacherID: id.New(), CenterID: id.New(), IsOwner: true}
}

// memberScope returns a scope for a non-owning member of some center.
func memberScope() authctx.Scope {
	return authctx.Scope{TeacherID: id.New(), CenterID: id.New(), IsOwner: false}
}

func TestCreateRejectsForeignContact(t *testing.T) {
	svc, repo, _ := newTestService()
	sc := memberScope()
	foreignContact := repo.addContact(id.New())

	_, err := svc.Create(context.Background(), sc, CreateRequest{
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

func TestCreateReturnsContactDetailsAndStampsScope(t *testing.T) {
	svc, repo, _ := newTestService()
	sc := memberScope()
	contactID := repo.addContactIn(sc.TeacherID, sc.CenterID)

	row, err := svc.Create(context.Background(), sc, CreateRequest{
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
	if row.TeacherID != sc.TeacherID {
		t.Fatalf("teacher id must come from the caller's scope, got %s", row.TeacherID)
	}
	if row.CenterID != sc.CenterID {
		t.Fatalf("center id must come from the caller's scope, got %s", row.CenterID)
	}
}

func TestUpdateRechecksContactOnlyWhenChanged(t *testing.T) {
	svc, repo, _ := newTestService()
	sc := memberScope()
	contactID := repo.addContactIn(sc.TeacherID, sc.CenterID)
	row, err := svc.Create(context.Background(), sc, CreateRequest{FullName: "Bé An", ContactID: contactID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Same contact: fine.
	if _, err := svc.Update(context.Background(), sc, row.ID, UpdateRequest{
		FullName: "Bé An (sửa)", ContactID: contactID,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Switching to a foreign contact: 422.
	foreignContact := repo.addContact(id.New())
	_, err = svc.Update(context.Background(), sc, row.ID, UpdateRequest{
		FullName: "Bé An", ContactID: foreignContact,
	})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation {
		t.Fatalf("foreign contact on update must be 422, got %v", err)
	}
}

func TestDeleteScrubsAndEndsEnrollments(t *testing.T) {
	svc, repo, ender := newTestService()
	sc := memberScope()
	contactID := repo.addContactIn(sc.TeacherID, sc.CenterID)
	row, err := svc.Create(context.Background(), sc, CreateRequest{
		FullName: "Bé An", ContactID: contactID, DisplayNote: "An lớp 9A",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.Delete(context.Background(), sc, row.ID); err != nil {
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
	sc := memberScope()
	contactID := repo.addContactIn(sc.TeacherID, sc.CenterID)
	row, err := svc.Create(context.Background(), sc, CreateRequest{FullName: "Bé An", ContactID: contactID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	boom := errors.New("enrollments unavailable")
	ender.err = boom

	if err := svc.Delete(context.Background(), sc, row.ID); !errors.Is(err, boom) {
		t.Fatalf("want the ender error, got %v", err)
	}
	if repo.rows[row.ID].FullName != "Bé An" {
		t.Fatal("a failed enrollment closure must leave the student untouched")
	}
}

func TestCrossTenantReadsAsNotFound(t *testing.T) {
	svc, repo, _ := newTestService()
	owner := memberScope()
	contactID := repo.addContactIn(owner.TeacherID, owner.CenterID)
	row, err := svc.Create(context.Background(), owner, CreateRequest{FullName: "Bé An", ContactID: contactID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	stranger := memberScope()
	var appErr *apperror.AppError
	if _, err := svc.Get(context.Background(), stranger, row.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be NOT_FOUND, got %v", err)
	}
	if err := svc.Delete(context.Background(), stranger, row.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant delete must be NOT_FOUND, got %v", err)
	}
}

// An owner reads and manages a member's student — center oversight, not
// per-teacher isolation. Creating is stricter: the row is always stamped as
// the caller's own, so the owner may only reference their own contacts.
func TestOwnerScopeSeesAndDeletesMembersStudent(t *testing.T) {
	svc, repo, _ := newTestService()
	center := id.New()
	member := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	owner := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: true}
	contactID := repo.addContactIn(member.TeacherID, center)

	row, err := svc.Create(context.Background(), member, CreateRequest{FullName: "Bé An", ContactID: contactID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Get(context.Background(), owner, row.ID); err != nil {
		t.Fatalf("owner must read a member's student, got %v", err)
	}
	if err := svc.Delete(context.Background(), owner, row.ID); err != nil {
		t.Fatalf("owner must delete a member's student, got %v", err)
	}

	// A member's contact is view-only for creation: the owner gets the same
	// 422 a foreign contact would produce.
	if _, err := svc.Create(context.Background(), owner, CreateRequest{FullName: "Bé Bình", ContactID: contactID}); apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("owner creating under a member's contact must be a 422, got %v", err)
	}
	ownContact := repo.addContactIn(owner.TeacherID, center)
	created, err := svc.Create(context.Background(), owner, CreateRequest{FullName: "Bé Bình", ContactID: ownContact})
	if err != nil {
		t.Fatalf("owner must still create under their own contact, got %v", err)
	}
	if created.TeacherID != owner.TeacherID {
		t.Fatalf("owner-created student must be stamped as the owner's own, got %s", created.TeacherID)
	}
}

// A peer in the same center but not the creator, and not the owner, must not
// see the student — center scope alone is not enough, isolation still holds
// between non-owning members.
func TestPeerScopeCannotSeeAnotherMembersStudent(t *testing.T) {
	svc, repo, _ := newTestService()
	center := id.New()
	author := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	peer := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	contactID := repo.addContactIn(author.TeacherID, center)

	row, err := svc.Create(context.Background(), author, CreateRequest{FullName: "Bé An", ContactID: contactID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Get(context.Background(), peer, row.ID); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("peer must not read another member's student, got %v", err)
	}
}
