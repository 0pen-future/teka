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
	phone := c.phone
	return Row{Student: s.Student, ContactName: c.name, ContactPhone: &phone}
}

// addStudent seeds a row directly, bypassing the service's owner gate — the
// shape of pre-migration data still anchored to a member.
func (f *fakeRepository) addStudent(teacherID, centerID, contactID uuid.UUID, name string) uuid.UUID {
	sid := id.New()
	f.rows[sid] = &fakeStudent{Student: Student{
		ID: sid, TeacherID: teacherID, CenterID: centerID,
		ContactID: contactID, FullName: name,
	}}
	return sid
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
	sc := ownerScope()
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
	sc := ownerScope()
	contactID := repo.addContactIn(sc.TeacherID, sc.CenterID)

	row, err := svc.Create(context.Background(), sc, CreateRequest{
		FullName: "Bé An", ContactID: contactID, DisplayNote: "An lớp 9A",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.ContactName != "Chị Hoa" || row.ContactPhone == nil || *row.ContactPhone != "+84912345678" {
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
	sc := ownerScope()
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
	sc := ownerScope()
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
	sc := ownerScope()
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
	owner := ownerScope()
	contactID := repo.addContactIn(owner.TeacherID, owner.CenterID)
	row, err := svc.Create(context.Background(), owner, CreateRequest{FullName: "Bé An", ContactID: contactID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	strangerOwner := ownerScope()
	var appErr *apperror.AppError
	if _, err := svc.Get(context.Background(), strangerOwner, row.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be NOT_FOUND, got %v", err)
	}
	if err := svc.Delete(context.Background(), strangerOwner, row.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant delete must be NOT_FOUND, got %v", err)
	}
}

// Student CRUD is the owner's alone: every member — the row's legacy creator
// included — gets an honest 403, before any lookup can leak existence.
func TestMemberWritesForbidden(t *testing.T) {
	svc, repo, _ := newTestService()
	center := id.New()
	member := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	contactID := repo.addContactIn(member.TeacherID, center)
	sid := repo.addStudent(member.TeacherID, center, contactID, "Bé An")

	if _, err := svc.Create(context.Background(), member, CreateRequest{FullName: "Bé Bình", ContactID: contactID}); apperror.From(err).Status != 403 {
		t.Fatalf("member create must be 403, got %v", err)
	}
	if _, err := svc.Update(context.Background(), member, sid, UpdateRequest{FullName: "Bé An (sửa)", ContactID: contactID}); apperror.From(err).Status != 403 {
		t.Fatalf("member update must be 403, got %v", err)
	}
	if err := svc.Delete(context.Background(), member, sid); apperror.From(err).Status != 403 {
		t.Fatalf("member delete must be 403, got %v", err)
	}
}

// The owner manages rows still anchored to a member (pre-migration shape) and
// creates under any of the center's contacts — a member's included, since
// contacts anchor to the owner going forward.
func TestOwnerManagesMemberAnchoredData(t *testing.T) {
	svc, repo, _ := newTestService()
	center := id.New()
	memberID := id.New()
	owner := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: true}
	contactID := repo.addContactIn(memberID, center)
	sid := repo.addStudent(memberID, center, contactID, "Bé An")

	if _, err := svc.Get(context.Background(), owner, sid); err != nil {
		t.Fatalf("owner must read a member-anchored student, got %v", err)
	}
	created, err := svc.Create(context.Background(), owner, CreateRequest{FullName: "Bé Bình", ContactID: contactID})
	if err != nil {
		t.Fatalf("owner must create under any center contact, got %v", err)
	}
	if created.TeacherID != owner.TeacherID {
		t.Fatalf("owner-created student must be stamped as the owner's own, got %s", created.TeacherID)
	}
	if err := svc.Delete(context.Background(), owner, sid); err != nil {
		t.Fatalf("owner must delete a member-anchored student, got %v", err)
	}
}

// A peer in the same center but not the creator, and not the owner, must not
// see the student — center scope alone is not enough, isolation still holds
// between non-owning members.
func TestPeerScopeCannotSeeAnotherMembersStudent(t *testing.T) {
	svc, repo, _ := newTestService()
	center := id.New()
	authorID := id.New()
	peer := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	contactID := repo.addContactIn(authorID, center)
	sid := repo.addStudent(authorID, center, contactID, "Bé An")

	if _, err := svc.Get(context.Background(), peer, sid); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("peer must not read another member's student, got %v", err)
	}
}

// FindIDByName mirrors the SQL predicate, including the NULL-safe note
// comparison: display_note is NULL when unset, so a plain equality test would
// miss every student without a distinguishing note.
func (f *fakeRepository) FindIDByName(_ context.Context, sc authctx.Scope, contactID uuid.UUID, fullName string, note *string) (uuid.UUID, bool, error) {
	for _, s := range f.rows {
		if !visible(s, sc) || s.ContactID != contactID || s.FullName != fullName {
			continue
		}
		if sameNote(s.DisplayNote, note) {
			return s.ID, true, nil
		}
	}
	return uuid.Nil, false, nil
}

// sameNote is SQL's IS NOT DISTINCT FROM for a nullable text column.
func sameNote(a, b *string) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

// FindIDByName is the bulk-import lookup. Its whole difficulty is the note:
// display_note is NULL when unset, and NULL never equals anything.

func TestFindIDByNameMatchesAStudentWithNoNote(t *testing.T) {
	svc, repo, _ := newTestService()
	sc := ownerScope()
	contactID := repo.addContactIn(sc.TeacherID, sc.CenterID)

	row, err := svc.Create(context.Background(), sc, CreateRequest{FullName: "Bé An", ContactID: contactID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Most students carry no note, so this is the common case a re-import
	// hits — an equality test on display_note would miss every one of them and
	// duplicate the whole roster.
	got, found, err := svc.FindIDByName(context.Background(), sc, contactID, "Bé An", nil)
	if err != nil || !found || got != row.ID {
		t.Fatalf("a note-less student must be found with a nil note, got id=%v found=%v err=%v", got, found, err)
	}

	note := "An lớn"
	if _, found, err := svc.FindIDByName(context.Background(), sc, contactID, "Bé An", &note); err != nil || found {
		t.Fatalf("a note-less student must not match a note, got found=%v err=%v", found, err)
	}
}

func TestFindIDByNameDistinguishesNamesakesByNote(t *testing.T) {
	svc, repo, _ := newTestService()
	sc := ownerScope()
	contactID := repo.addContactIn(sc.TeacherID, sc.CenterID)

	big, err := svc.Create(context.Background(), sc, CreateRequest{
		FullName: "Bé An", ContactID: contactID, DisplayNote: "An lớn",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	small, err := svc.Create(context.Background(), sc, CreateRequest{
		FullName: "Bé An", ContactID: contactID, DisplayNote: "An nhỏ",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	for _, tc := range []struct {
		note string
		want uuid.UUID
	}{{"An lớn", big.ID}, {"An nhỏ", small.ID}} {
		note := tc.note
		got, found, err := svc.FindIDByName(context.Background(), sc, contactID, "Bé An", &note)
		if err != nil || !found || got != tc.want {
			t.Fatalf("%s must resolve to its own student, got id=%v found=%v err=%v", tc.note, got, found, err)
		}
	}
}

func TestFindIDByNameStaysWithinTheAnchorTeacher(t *testing.T) {
	svc, repo, _ := newTestService()
	center := id.New()
	authorID := id.New()
	peer := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	contactID := repo.addContactIn(authorID, center)
	repo.addStudent(authorID, center, contactID, "Bé An")

	if _, found, err := svc.FindIDByName(context.Background(), peer, contactID, "Bé An", nil); err != nil || found {
		t.Fatalf("another teacher's student must not be found, got found=%v err=%v", found, err)
	}
}
