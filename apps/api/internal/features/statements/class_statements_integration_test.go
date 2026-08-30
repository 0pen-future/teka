//go:build integration

package statements_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/billing"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// classCopyFixture is one center where a single contact has two children in
// two different classes of the same teacher, with the period closed: class A
// carries two confirmed sessions (200 000đ), class B one (100 000đ), so a
// class-A copy is distinguishable from the family bill (300 000đ) by money
// alone.
type classCopyFixture struct {
	teacher  uuid.UUID
	classA   *classes.Class
	classB   *classes.Class
	periodID uuid.UUID
}

func seedClassCopyFixture(t *testing.T, statementsSvc *statements.Service, billingSvc *billing.Service, db *gorm.DB) classCopyFixture {
	t.Helper()
	ctx := context.Background()
	_, teacher := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID, testutil.WithContactPhone("+84905556666"))
	classStart := date("2026-04-01")

	classA := testutil.Class(t, db, teacher.ID, testutil.WithClassName("CopyClassA"), testutil.WithClassStartDate(classStart))
	studentA := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("CopyChildA"))
	enrollA := testutil.Enrollment(t, db, teacher.ID, studentA.ID, classA.ID, classStart)
	for i := 0; i < 2; i++ {
		sess := testutil.Session(t, db, teacher.ID, classA.ID, classStart.AddDate(0, 0, 7*i+1),
			testutil.WithSessionAttendanceConfirmed(time.Now()))
		testutil.AttendanceRecord(t, db, teacher.ID, sess.ID, studentA.ID, enrollA.ID)
	}

	classB := testutil.Class(t, db, teacher.ID, testutil.WithClassName("CopyClassB"), testutil.WithClassStartDate(classStart))
	studentB := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("CopyChildB"))
	enrollB := testutil.Enrollment(t, db, teacher.ID, studentB.ID, classB.ID, classStart)
	sessB := testutil.Session(t, db, teacher.ID, classB.ID, classStart.AddDate(0, 0, 2),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, teacher.ID, sessB.ID, studentB.ID, enrollB.ID)

	period, err := billingSvc.EnsurePeriod(ctx, sc, 2026, 4)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, sc, period.ID)
	require.NoError(t, err)

	return classCopyFixture{teacher: teacher.ID, classA: classA, classB: classB, periodID: period.ID}
}

// A hoc_vu stint on one class mints a copy carrying ONLY that class's
// charges, listable through ListClass — while the same stint opens nothing
// on a sibling class (neutral 404) and a read-only stint (tro_giang) gets an
// honest 403 on the class it can read.
func TestClassCopyCarriesOnlyTheAssignedClassCharges(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	fx := seedClassCopyFixture(t, statementsSvc, billingSvc, db)
	center := testutil.ScopeFor(t, db, fx.teacher).CenterID

	hocVu, _ := testutil.Teacher(t, db)
	troGiang, _ := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, hocVu.ID, center)
	testutil.JoinCenter(t, db, troGiang.ID, center)
	testutil.StaffAssignment(t, db, fx.classA, hocVu.ID, "hoc_vu")
	testutil.StaffAssignment(t, db, fx.classA, troGiang.ID, "tro_giang")

	hocVuScope := testutil.ScopeFor(t, db, hocVu.ID)
	gen, err := statementsSvc.GenerateForSendClass(ctx, hocVuScope, fx.periodID, fx.classA.ID)
	require.NoError(t, err)
	require.Len(t, gen.Statements, 1)
	row := gen.Statements[0]
	require.NotNil(t, row.ClassID)
	require.Equal(t, fx.classA.ID, *row.ClassID)
	require.EqualValues(t, 200_000, row.TotalDue,
		"the class copy bills class A's two sessions only, never the family's 300 000đ")

	rows, total, err := statementsSvc.ListClass(ctx, hocVuScope, fx.periodID, fx.classA.ID, pagination.Params{Page: 1, PerPage: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.NotNil(t, rows[0].ClassID)
	require.Equal(t, fx.classA.ID, *rows[0].ClassID)

	// The same stint opens nothing on the sibling class: neutral 404, same
	// as a class that does not exist.
	_, err = statementsSvc.GenerateForSendClass(ctx, hocVuScope, fx.periodID, fx.classB.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, _, err = statementsSvc.ListClass(ctx, hocVuScope, fx.periodID, fx.classB.ID, pagination.Params{Page: 1, PerPage: 20})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// A read-only stint can see the class, so it gets an honest 403 instead
	// of the neutral 404.
	troScope := testutil.ScopeFor(t, db, troGiang.ID)
	_, err = statementsSvc.GenerateForSendClass(ctx, troScope, fx.periodID, fx.classA.ID)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
}

// The class copy is its own bearer credential: a distinct row with a distinct
// token hash, whose public render shows the class's charges only — the class
// link can never open the family statement.
func TestClassCopyTokenIsDistinctAndRendersClassChargesOnly(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()
	fx := seedClassCopyFixture(t, statementsSvc, billingSvc, db)
	sc := testutil.ScopeFor(t, db, fx.teacher)

	famGen, err := statementsSvc.Generate(ctx, sc, fx.periodID)
	require.NoError(t, err)
	require.Len(t, famGen.Statements, 1)
	classGen, err := statementsSvc.GenerateForSendClass(ctx, sc, fx.periodID, fx.classA.ID)
	require.NoError(t, err)
	require.Len(t, classGen.Statements, 1)

	famRow, classRow := famGen.Statements[0], classGen.Statements[0]
	require.NotEqual(t, famRow.ID, classRow.ID, "family and class copies are separate rows")
	require.NotEqual(t, famRow.TokenHash, classRow.TokenHash, "the class copy never shares the family token")

	const prefix = "https://parent.example.com/s/"
	famURL := statementsSvc.ToResponseForSend(sc, famRow).URL
	classURL := statementsSvc.ToResponseForSend(sc, classRow).URL
	require.NotNil(t, famURL)
	require.NotNil(t, classURL)
	famToken := (*famURL)[len(prefix):]
	classToken := (*classURL)[len(prefix):]
	require.NotEqual(t, famToken, classToken)

	// The class token resolves the class row — never the family one — and
	// renders class A's money only.
	stmt, err := statementsSvc.LookupPublic(ctx, classToken)
	require.NoError(t, err)
	require.Equal(t, classRow.ID, stmt.ID)
	require.NotNil(t, stmt.ClassID)

	payload, qrPayload, err := statementsSvc.RenderPublic(ctx, stmt)
	require.NoError(t, err)
	require.NotEmpty(t, qrPayload)
	require.EqualValues(t, 200_000, payload.Totals.TotalDue)
	require.EqualValues(t, 0, payload.Totals.OpeningBalance, "family-level money stays off a class copy")
	require.Len(t, payload.Children, 1)
	require.Len(t, payload.Children[0].Classes, 1)
	require.Equal(t, "CopyClassA", payload.Children[0].Classes[0].ClassName)

	// The family token still renders the whole family's 300 000đ.
	famStmt, err := statementsSvc.LookupPublic(ctx, famToken)
	require.NoError(t, err)
	require.Equal(t, famRow.ID, famStmt.ID)
	famPayload, _, err := statementsSvc.RenderPublic(ctx, famStmt)
	require.NoError(t, err)
	require.EqualValues(t, 300_000, famPayload.Totals.TotalDue)
	require.Len(t, famPayload.Children, 2)
}
