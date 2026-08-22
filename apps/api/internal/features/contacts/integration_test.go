//go:build integration

package contacts_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

func newIntegrationService(t *testing.T) (*contacts.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	return contacts.NewService(contacts.NewRepository(db)), db
}

// listParams builds pagination params the way a handler would, from a raw
// query string.
func listParams(t *testing.T, query string) pagination.Params {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	return pagination.Parse(c, "full_name", map[string]string{"full_name": "contacts.full_name"})
}

// insertStudent writes a students row directly; the students feature package
// arrives in phase 2, but the count join must be proven against real rows now.
func insertStudent(t *testing.T, db *gorm.DB, teacherID, centerID, contactID any, name string) (studentID any) {
	t.Helper()
	sid := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO students (id, teacher_id, center_id, contact_id, full_name) VALUES (?, ?, ?, ?, ?)",
		sid, teacherID, centerID, contactID, name,
	).Error)
	return sid
}

func TestPhoneReusableAcrossTeachersButUniqueWithinOne(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)

	_, err := svc.Create(ctx, scopeA, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)

	// Same number under a second teacher's own center is a separate tenant's
	// customer.
	_, err = svc.Create(ctx, scopeB, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err, "phone uniqueness must be per-teacher, not global")

	// The E.164 spelling of the number already registered under teacher A must
	// collide with the local-form original — one number, one contact.
	_, err = svc.Create(ctx, scopeA, contacts.CreateRequest{FullName: "Chị Hoa (bis)", Phone: "+84912345678"})
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code,
		"0912345678 and +84912345678 must be the same number")
}

func TestSoftDeleteFreesPhoneForReuse(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	first, err := svc.Create(ctx, sc, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, sc, first.ID))

	// The partial unique index only covers non-deleted rows, so the number is
	// free again.
	again, err := svc.Create(ctx, sc, contacts.CreateRequest{FullName: "Chị Hoa mới", Phone: "0912345678"})
	require.NoError(t, err, "soft delete must free the phone for reuse")
	require.NotEqual(t, first.ID, again.ID)
}

func TestCrossCenterReadsAreNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)

	row, err := svc.Create(ctx, scopeA, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)

	// 404, never 403: a 403 would confirm the id exists in another center.
	_, err = svc.Get(ctx, scopeB, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	err = svc.Delete(ctx, scopeB, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.Update(ctx, scopeB, row.ID, contacts.UpdateRequest{FullName: "X", Phone: "0912345679"})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// Never shows up in the other center's list either.
	rows, _, err := svc.List(ctx, scopeB, contacts.ListFilter{}, listParams(t, ""))
	require.NoError(t, err)
	for _, r := range rows {
		require.NotEqual(t, row.ID, r.ID, "another center's list must not include this contact")
	}
}

// An owner sees, reads, updates, and deletes a contact created by a teacher
// who joined their center — center-wide oversight, not per-teacher isolation.
// An owner-created contact is stamped as the owner's own, never on behalf of
// someone else.
func TestOwnerHasFullOversightOfMembersContacts(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID

	testutil.JoinCenter(t, db, member.ID, ownerCenter)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	require.Equal(t, ownerScope.CenterID, memberScope.CenterID, "member must have joined the owner's center")

	row, err := svc.Create(ctx, memberScope, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)

	got, err := svc.Get(ctx, ownerScope, row.ID)
	require.NoError(t, err, "owner must read a member's contact")
	require.Equal(t, row.ID, got.ID)

	rows, total, err := svc.List(ctx, ownerScope, contacts.ListFilter{}, listParams(t, ""))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, row.ID, rows[0].ID, "owner's list must include the member's contact")

	updated, err := svc.Update(ctx, ownerScope, row.ID, contacts.UpdateRequest{FullName: "Chị Hoa (updated)", Phone: "0912345678"})
	require.NoError(t, err, "owner must update a member's contact")
	require.Equal(t, "Chị Hoa (updated)", updated.FullName)

	require.NoError(t, svc.Delete(ctx, ownerScope, row.ID), "owner must delete a member's contact")
	_, err = svc.Get(ctx, ownerScope, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// An owner creates rows as themselves, never on behalf of a member.
	ownerRow, err := svc.Create(ctx, ownerScope, contacts.CreateRequest{FullName: "Anh Tuấn", Phone: "0987654321"})
	require.NoError(t, err)
	require.Equal(t, owner.ID, ownerRow.TeacherID, "owner-created contact must be stamped as the owner's own")
}

// Two non-owning teachers in the same center are still isolated from each
// other: center scope grants the owner oversight, not peer-to-peer access.
func TestPeersInSameCenterCannotSeeEachOthersContacts(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	memberB, _ := testutil.Teacher(t, db)
	memberC, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID

	testutil.JoinCenter(t, db, memberB.ID, ownerCenter)
	testutil.JoinCenter(t, db, memberC.ID, ownerCenter)
	scopeB := testutil.ScopeFor(t, db, memberB.ID)
	scopeC := testutil.ScopeFor(t, db, memberC.ID)

	row, err := svc.Create(ctx, scopeB, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)

	_, err = svc.Get(ctx, scopeC, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not read another member's contact")

	rows, total, err := svc.List(ctx, scopeC, contacts.ListFilter{}, listParams(t, ""))
	require.NoError(t, err)
	require.EqualValues(t, 0, total)
	for _, r := range rows {
		require.NotEqual(t, row.ID, r.ID, "a peer's list must not include another member's contact")
	}
}

func TestUpdateRenormalisesPhoneAndDetectsCollision(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	kept, err := svc.Create(ctx, sc, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)
	moved, err := svc.Create(ctx, sc, contacts.CreateRequest{FullName: "Anh Tuấn", Phone: "0912345679"})
	require.NoError(t, err)

	updated, err := svc.Update(ctx, sc, moved.ID, contacts.UpdateRequest{FullName: "Anh Tuấn", Phone: "0399999999"})
	require.NoError(t, err)
	require.Equal(t, "+84399999999", updated.Phone)

	_, err = svc.Update(ctx, sc, moved.ID, contacts.UpdateRequest{FullName: "Anh Tuấn", Phone: kept.Phone})
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
}

func TestDeleteBlockedByLiveStudentsNamesThem(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	row, err := svc.Create(ctx, sc, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)
	studentID := insertStudent(t, db, teacher.ID, sc.CenterID, row.ID, "Bé An")

	err = svc.Delete(ctx, sc, row.ID)
	appErr := apperror.From(err)
	require.Equal(t, apperror.CodeConflict, appErr.Code)
	require.Contains(t, appErr.Message, "Bé An", "the 409 must name the blocking student")

	// Soft-deleting the student clears the block.
	require.NoError(t, db.Exec("UPDATE students SET deleted_at = now() WHERE id = ?", studentID).Error)
	require.NoError(t, svc.Delete(ctx, sc, row.ID))
}

func TestListCountsStudentsWithoutNPlusOne(t *testing.T) {
	t.Parallel()
	_, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	// 150 contacts — the PRD's roster scale. The one carrying students gets a
	// name that sorts first so the default full_name ordering keeps it inside
	// the asserted page.
	first := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("An An"))
	for i := 0; i < 149; i++ {
		testutil.Contact(t, db, teacher.ID)
	}
	insertStudent(t, db, teacher.ID, sc.CenterID, first.ID, "Bé An")
	insertStudent(t, db, teacher.ID, sc.CenterID, first.ID, "Bé Bình")
	gone := insertStudent(t, db, teacher.ID, sc.CenterID, first.ID, "Bé Cũ")
	require.NoError(t, db.Exec("UPDATE students SET deleted_at = now() WHERE id = ?", gone).Error)

	counter := &sqlCounter{Interface: gormlogger.Discard}
	counted := contacts.NewService(contacts.NewRepository(db.Session(&gorm.Session{Logger: counter})))

	rows, total, err := counted.List(ctx, sc, contacts.ListFilter{}, listParams(t, "per_page=100"))
	require.NoError(t, err)
	require.EqualValues(t, 150, total)
	require.Len(t, rows, 100)
	require.LessOrEqual(t, counter.n.Load(), int64(2),
		"listing must stay at count+select regardless of row count — no per-row student query")

	byID := map[string]int64{}
	for _, r := range rows {
		byID[r.ID.String()] = r.StudentCount
	}
	require.EqualValues(t, 2, byID[first.ID.String()],
		"student_count must count live students only")
}

func TestListSearchMatchesNameAndPhone(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	_, err := svc.Create(ctx, sc, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)
	_, err = svc.Create(ctx, sc, contacts.CreateRequest{FullName: "Anh Tuấn", Phone: "0987654321"})
	require.NoError(t, err)

	rows, total, err := svc.List(ctx, sc, contacts.ListFilter{Query: "hoa"}, listParams(t, ""))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "Chị Hoa", rows[0].FullName)

	rows, total, err = svc.List(ctx, sc, contacts.ListFilter{Query: "8765"}, listParams(t, ""))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "Anh Tuấn", rows[0].FullName)
}

func TestZaloMappingPersistsAndClears(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	row, err := svc.Create(ctx, sc, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)

	mapped, err := svc.UpdateZaloMapping(ctx, sc, row.ID, contacts.ZaloMappingRequest{
		ZaloUserID: "8421113355",
		ZaloName:   "Hoa Nguyễn",
	})
	require.NoError(t, err)
	require.NotNil(t, mapped.ZaloUserID)
	require.Equal(t, "8421113355", *mapped.ZaloUserID)
	require.NotNil(t, mapped.ZaloName)
	require.Equal(t, "Hoa Nguyễn", *mapped.ZaloName)

	// A fresh read proves the columns persisted, not just the returned struct.
	got, err := svc.Get(ctx, sc, row.ID)
	require.NoError(t, err)
	require.NotNil(t, got.ZaloUserID)
	require.Equal(t, "8421113355", *got.ZaloUserID)

	// Remapping replaces both fields together.
	remapped, err := svc.UpdateZaloMapping(ctx, sc, row.ID, contacts.ZaloMappingRequest{
		ZaloUserID: "8429990001",
		ZaloName:   "Hoa (mẹ bé An)",
	})
	require.NoError(t, err)
	require.Equal(t, "8429990001", *remapped.ZaloUserID)
	require.Equal(t, "Hoa (mẹ bé An)", *remapped.ZaloName)

	require.NoError(t, svc.ClearZaloMapping(ctx, sc, row.ID))
	cleared, err := svc.Get(ctx, sc, row.ID)
	require.NoError(t, err)
	require.Nil(t, cleared.ZaloUserID)
	require.Nil(t, cleared.ZaloName)

	// Clearing again is still fine — the caller's intent is already true.
	require.NoError(t, svc.ClearZaloMapping(ctx, sc, row.ID))
}

// One Zalo friend maps to at most one live contact per teacher: a duplicate
// mapping would send one person the statement links — and the debt data — of
// two different families.
func TestZaloMappingRefusesAFriendAlreadyMappedToAnotherContact(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	otherTeacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	otherScope := testutil.ScopeFor(t, db, otherTeacher.ID)

	first, err := svc.Create(ctx, sc, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)
	second, err := svc.Create(ctx, sc, contacts.CreateRequest{FullName: "Anh Tuấn", Phone: "0912345679"})
	require.NoError(t, err)

	friend := contacts.ZaloMappingRequest{ZaloUserID: "8421113355", ZaloName: "Hoa Nguyễn"}
	_, err = svc.UpdateZaloMapping(ctx, sc, first.ID, friend)
	require.NoError(t, err)

	_, err = svc.UpdateZaloMapping(ctx, sc, second.ID, friend)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code,
		"the same Zalo friend must not map to two contacts of one teacher")

	// Re-saving the same mapping on the same contact is not a duplicate.
	_, err = svc.UpdateZaloMapping(ctx, sc, first.ID, friend)
	require.NoError(t, err)

	// Another teacher's roster is a separate world.
	foreign, err := svc.Create(ctx, otherScope, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)
	_, err = svc.UpdateZaloMapping(ctx, otherScope, foreign.ID, friend)
	require.NoError(t, err, "uniqueness must be per-teacher, not global")

	// A soft-deleted contact releases its friend, like uq_contacts_phone.
	require.NoError(t, svc.ClearZaloMapping(ctx, sc, first.ID))
	require.NoError(t, svc.Delete(ctx, sc, first.ID))
	_, err = svc.UpdateZaloMapping(ctx, sc, second.ID, friend)
	require.NoError(t, err)
}

func TestZaloMappingIsTenantScoped(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)

	row, err := svc.Create(ctx, scopeA, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)

	_, err = svc.UpdateZaloMapping(ctx, scopeB, row.ID, contacts.ZaloMappingRequest{
		ZaloUserID: "8421113355",
		ZaloName:   "Hoa Nguyễn",
	})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	err = svc.ClearZaloMapping(ctx, scopeB, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// The other tenant's probe must not have touched the row.
	got, err := svc.Get(ctx, scopeA, row.ID)
	require.NoError(t, err)
	require.Nil(t, got.ZaloUserID)
}

// sqlCounter counts executed statements through the GORM logger's Trace hook.
type sqlCounter struct {
	gormlogger.Interface
	n atomic.Int64
}

func (c *sqlCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	c.n.Add(1)
	c.Interface.Trace(ctx, begin, fc, err)
}
