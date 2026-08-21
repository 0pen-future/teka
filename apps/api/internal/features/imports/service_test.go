package imports

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

// importFixture wires the service over the in-memory roster with an owner
// caller whose center holds both teachers the sample workbook names.
type importFixture struct {
	svc      *Service
	store    *fakeRoster
	locker   *fakeLocker
	scope    authctx.Scope
	nam, lan uuid.UUID
}

func newImportFixture() importFixture {
	nam, lan := uuid.New(), uuid.New()
	store := newFakeRoster()
	dir := &countingDirectory{dir: map[string]uuid.UUID{namPhone: nam, lanPhone: lan}}
	svc, locker := newTestService(dir, store)
	return importFixture{
		svc:    svc,
		store:  store,
		locker: locker,
		scope:  authctx.Scope{TeacherID: uuid.New(), CenterID: uuid.New(), IsOwner: true},
		nam:    nam, lan: lan,
	}
}

func (f importFixture) importFile(file []byte, dryRun bool) (*Report, error) {
	return f.svc.Import(context.Background(), f.scope, file, dryRun)
}

// rowErrorCodes pulls the codes out of a 422 carrying row defects, failing the
// test when err is anything else.
func rowErrorCodes(t *testing.T, err error) []string {
	t.Helper()
	var rowErrs *RowErrorsError
	require.ErrorAs(t, err, &rowErrs)
	require.Equal(t, http.StatusUnprocessableEntity, apperror.From(err).Status)
	codes := make([]string, 0, len(rowErrs.Payload.Errors))
	for _, e := range rowErrs.Payload.Errors {
		codes = append(codes, e.Code)
	}
	return codes
}

func TestImportCommitCreatesEveryEntity(t *testing.T) {
	t.Parallel()
	f := newImportFixture()

	rep, err := f.importFile(validWorkbook(t), false)
	require.NoError(t, err)
	require.True(t, rep.Committed)

	require.Equal(t, ReportEntity{Created: 2}, rep.Classes)
	require.Equal(t, ReportEntity{Created: 3}, rep.Schedules, "Toán 9A runs twice a week")
	require.Equal(t, ReportEntity{Created: 3}, rep.Contacts)
	require.Equal(t, ReportEntity{Created: 3}, rep.Students)
	require.Equal(t, ReportEntity{Created: 3}, rep.Enrollments)

	require.Len(t, f.store.classRows, 2)
	require.Len(t, f.store.scheduleRows, 3)
	require.Len(t, f.store.contactRows, 3)
	require.Len(t, f.store.studentRows, 3)
	require.Len(t, f.store.enrollRows, 3)

	// Every row lands on the class's teacher, not on the importing owner.
	for _, c := range f.store.classRows {
		require.NotEqual(t, f.scope.TeacherID, c.TeacherID)
		require.Equal(t, f.scope.CenterID, c.CenterID)
	}
	require.Equal(t, f.nam, f.store.classRows[0].TeacherID)
	require.Equal(t, f.lan, f.store.classRows[1].TeacherID)

	// One parent with children under two teachers becomes two contacts:
	// uq_contacts_phone is (teacher_id, phone), so each teacher gets their own
	// row, their own statement link, and their own balance.
	var hung []*struct{ teacher uuid.UUID }
	for _, c := range f.store.contactRows {
		if c.Phone == "+84901234567" {
			hung = append(hung, &struct{ teacher uuid.UUID }{c.TeacherID})
		}
	}
	require.Len(t, hung, 2)
	require.NotEqual(t, hung[0].teacher, hung[1].teacher)
}

func TestImportSecondRunReusesEverything(t *testing.T) {
	t.Parallel()
	f := newImportFixture()

	_, err := f.importFile(validWorkbook(t), false)
	require.NoError(t, err)
	before := f.store.snapshot()

	// Re-uploading the same file is the common operator mistake. Two of the
	// three students carry a blank Ghi chú phân biệt, which is stored NULL —
	// the lookup has to match NULL to NULL or they would be created twice.
	rep, err := f.importFile(validWorkbook(t), false)
	require.NoError(t, err)

	require.Equal(t, ReportEntity{Reused: 2}, rep.Classes)
	require.Equal(t, ReportEntity{Reused: 3}, rep.Schedules)
	require.Equal(t, ReportEntity{Reused: 3}, rep.Contacts)
	require.Equal(t, ReportEntity{Reused: 3}, rep.Students)
	require.Equal(t, ReportEntity{Reused: 3}, rep.Enrollments)
	require.Equal(t, before, f.store.snapshot(), "a re-import writes nothing")
}

func TestImportRejectsClassWhoseStoredFieldsDiffer(t *testing.T) {
	t.Parallel()
	f := newImportFixture()
	_, err := f.importFile(validWorkbook(t), false)
	require.NoError(t, err)
	before := f.store.snapshot()

	// The operator edits the price and re-uploads. Keeping the stored price
	// would invoice at a rate nobody typed; overwriting it would edit a class
	// through an endpoint whose job is to create.
	rep, err := f.importFile(workbookWithClassPrice(t, "160000"), false)
	require.Nil(t, rep)
	require.Equal(t, []string{CodeClassExistsMismatch}, rowErrorCodes(t, err))
	require.Equal(t, before, f.store.snapshot())
}

func TestImportRejectsStudentWhoAlreadyLeftTheClass(t *testing.T) {
	t.Parallel()
	f := newImportFixture()
	_, err := f.importFile(validWorkbook(t), false)
	require.NoError(t, err)

	// uq_enrollments_active only covers open rows, so an ended enrollment is
	// invisible to the database. Re-creating it would backdate started_on and
	// invoice months of sessions for a child who left.
	ended := time.Date(2025, 11, 30, 0, 0, 0, 0, time.UTC)
	f.store.enrollRows[0].EndedOn = &ended
	before := f.store.snapshot()

	rep, err := f.importFile(validWorkbook(t), false)
	require.Nil(t, rep)
	require.Equal(t, []string{CodeEnrollmentEnded}, rowErrorCodes(t, err))
	require.Equal(t, before, f.store.snapshot())
}

func TestImportWriterFailureRollsBackAndStops(t *testing.T) {
	t.Parallel()
	f := newImportFixture()
	f.store.failOn["students.Create"] = errWriter

	rep, err := f.importFile(validWorkbook(t), false)
	require.Nil(t, rep)
	require.ErrorIs(t, err, errWriter)

	require.Equal(t, rosterSnapshot{}, f.store.snapshot(), "the whole import unwinds")
	require.NotContains(t, f.store.calls, "enrollments.Create",
		"the failing row must not be followed by writes that depend on it")
}

func TestDryRunMatchesTheCommitAndWritesNothing(t *testing.T) {
	t.Parallel()
	dry := newImportFixture()
	dryRep, err := dry.importFile(validWorkbook(t), true)
	require.NoError(t, err)
	require.False(t, dryRep.Committed)
	require.Empty(t, dry.store.calls, "a check that writes is not a check")
	require.Equal(t, rosterSnapshot{}, dry.store.snapshot())

	wet := newImportFixture()
	wetRep, err := wet.importFile(validWorkbook(t), false)
	require.NoError(t, err)

	// A clean check must not be followed by a surprise at commit.
	dryRep.Committed = true
	require.Equal(t, wetRep, dryRep)
}

func TestDryRunSeesTheSameConflictsAsTheCommit(t *testing.T) {
	t.Parallel()
	f := newImportFixture()
	_, err := f.importFile(validWorkbook(t), false)
	require.NoError(t, err)

	_, err = f.importFile(workbookWithClassPrice(t, "160000"), true)
	require.Equal(t, []string{CodeClassExistsMismatch}, rowErrorCodes(t, err))
}

func TestImportRefusesWhenAnotherImportHoldsTheCenter(t *testing.T) {
	t.Parallel()
	f := newImportFixture()
	f.locker.locked = false

	rep, err := f.importFile(validWorkbook(t), false)
	require.Nil(t, rep)
	require.Equal(t, http.StatusConflict, apperror.From(err).Status)
	require.Empty(t, f.store.calls)
	require.Equal(t, []uuid.UUID{f.scope.CenterID}, f.locker.centers,
		"the lock is keyed on the caller's center, never on anything in the file")
}

func TestDryRunTakesTheLockToo(t *testing.T) {
	t.Parallel()
	f := newImportFixture()
	f.locker.locked = false

	// A check whose answer could change under a concurrent import is not a
	// check, so the dry run refuses on the same lock the commit needs.
	_, err := f.importFile(validWorkbook(t), true)
	require.Equal(t, http.StatusConflict, apperror.From(err).Status)
}

func TestImportSurfacesLockerFailure(t *testing.T) {
	t.Parallel()
	f := newImportFixture()
	f.locker.lockErr = errors.New("connection reset")

	_, err := f.importFile(validWorkbook(t), false)
	require.ErrorIs(t, err, f.locker.lockErr)
	require.Empty(t, f.store.calls)
}

func TestImportRequiresOwner(t *testing.T) {
	t.Parallel()
	f := newImportFixture()
	f.scope.IsOwner = false

	_, err := f.importFile(validWorkbook(t), true)
	require.Equal(t, http.StatusForbidden, apperror.From(err).Status)
	require.Empty(t, f.locker.centers, "the gate runs before any work")
}

// workbookWithClassPrice is validWorkbook with a different Đơn giá/buổi on
// both Toán 9A rows.
func workbookWithClassPrice(t *testing.T, price string) []byte {
	t.Helper()
	return buildWorkbook(t, map[string][][]string{
		SheetClasses: {
			classHeaders,
			exampleClassRow,
			{"Toán 9A", "0912345678", "01/09/2025", price, "2", "18:00", "90", ""},
			{"Toán 9A", "0912345678", "01/09/2025", price, "5", "18:00", "90", ""},
			{"Văn 8", "0987654321", "15/09/2025", "120000", "CN", "08:30", "120", ""},
		},
		SheetStudents: {
			studentHeaders,
			exampleStudentRow,
			{"Phạm Gia An", "Phạm Văn Hùng", "0901234567", "Toán 9A", "0912345678", "", ""},
			{"Phạm Gia Bảo", "Phạm Văn Hùng", "0901234567", "Văn 8", "0987654321", "", ""},
			{"Lê Thu Hà", "Lê Thị Mai", "0977888999", "Toán 9A", "0912345678", "05/10/2025", "Hà nhỏ"},
		},
	})
}

// siblingWorkbook puts two children of one parent in one class, and one of
// them in a second class run by the same teacher — the two shapes where a
// natural key repeats inside a single file.
func siblingWorkbook(t *testing.T) []byte {
	t.Helper()
	return buildWorkbook(t, map[string][][]string{
		SheetClasses: {
			classHeaders,
			exampleClassRow,
			{"Toán 6A", namPhone, "01/09/2025", "150000", "2", "18:00", "90", ""},
			{"Lý 11", namPhone, "01/09/2025", "200000", "5", "19:30", "90", ""},
		},
		SheetStudents: {
			studentHeaders,
			exampleStudentRow,
			{"Trần Gia Bảo", "Trần Văn Hùng", "0912000111", "Toán 6A", namPhone, "", ""},
			{"Trần Gia Hân", "Trần Văn Hùng", "0912000111", "Toán 6A", namPhone, "", ""},
			{"Trần Gia Bảo", "Trần Văn Hùng", "0912000111", "Lý 11", namPhone, "", ""},
		},
	})
}

func TestDryRunCountsRepeatedKeysOnceLikeTheCommit(t *testing.T) {
	t.Parallel()

	// Two siblings share a parent and one of them is in two classes. Nothing
	// is written during a check, so every lookup misses; without carrying the
	// run's own decisions forward the check would promise three parents and
	// three students where the commit makes one and two.
	dry := newImportFixture()
	dryRep, err := dry.importFile(siblingWorkbook(t), true)
	require.NoError(t, err)

	wet := newImportFixture()
	wetRep, err := wet.importFile(siblingWorkbook(t), false)
	require.NoError(t, err)

	require.Equal(t, ReportEntity{Created: 1, Reused: 2}, wetRep.Contacts)
	require.Equal(t, ReportEntity{Created: 2, Reused: 1}, wetRep.Students)
	require.Equal(t, ReportEntity{Created: 3}, wetRep.Enrollments)

	dryRep.Committed = true
	require.Equal(t, wetRep, dryRep, "a check must predict the commit it precedes")
}
