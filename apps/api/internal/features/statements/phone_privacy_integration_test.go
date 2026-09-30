//go:build integration

package statements_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// The one phone rule on the statements surface: a statement row carries the
// contact's phone only to the owner or a contacts.view_all holder (a
// reports-oversight holder through the key it implies) — never through a
// class staff assignment, hoc_vu included. The family statement URL is stricter still —
// it is a public bearer token, so it goes only to owner/oversight, never to a
// class teacher or a hoc_vu (they get the per-class variant in a later phase).
func TestStatementPhoneAndURLFollowTheOnePhoneRule(t *testing.T) {
	t.Parallel()
	statementsSvc, billingSvc, db := newIntegrationDeps(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	memberB, _ := testutil.Teacher(t, db)
	hocVu, _ := testutil.Teacher(t, db)
	troGiang, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	center := ownerScope.CenterID
	testutil.JoinCenter(t, db, member.ID, center)
	testutil.JoinCenter(t, db, memberB.ID, center)
	testutil.JoinCenter(t, db, hocVu.ID, center)
	testutil.JoinCenter(t, db, troGiang.ID, center)
	_, secretary := testutil.Secretary(t, db, center)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	secScope := testutil.ScopeFor(t, db, secretary.ID)

	contact := testutil.Contact(t, db, member.ID, testutil.WithContactPhone("+84903334444"))
	classStart := date("2026-03-01")
	class := testutil.Class(t, db, member.ID, testutil.WithClassName("PrivacyA"), testutil.WithClassStartDate(classStart))
	student := testutil.Student(t, db, member.ID, contact.ID, testutil.WithStudentFullName("Privacy-student"))
	enrollment := testutil.Enrollment(t, db, member.ID, student.ID, class.ID, classStart)
	sess := testutil.Session(t, db, member.ID, class.ID, classStart.AddDate(0, 0, 1),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, member.ID, sess.ID, student.ID, enrollment.ID)

	// hoc_vu and tro_giang hold stints on the class — read reach elsewhere,
	// but no statements reach: statements stay period-owner + oversight until
	// the per-class variant exists.
	testutil.StaffAssignment(t, db, class, hocVu.ID, "hoc_vu")
	testutil.StaffAssignment(t, db, class, troGiang.ID, "tro_giang")

	period, err := billingSvc.EnsurePeriod(ctx, memberScope, 2026, 3)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, memberScope, period.ID)
	require.NoError(t, err)

	gen, err := statementsSvc.Generate(ctx, memberScope, period.ID)
	require.NoError(t, err)
	require.Len(t, gen.Statements, 1)

	// The generating class teacher is not oversight: the generate response
	// they get back carries neither the phone nor the family URL.
	memberResp := statementsSvc.ToResponse(memberScope, gen.Statements[0])
	require.Nil(t, memberResp.Phone, "a class teacher without hoc_vu must not see the contact's phone")
	require.Nil(t, memberResp.URL, "the family statement URL is a public bearer token — owner/oversight only")

	// Owner and secretary read both.
	rows, total, err := statementsSvc.List(ctx, ownerScope, period.ID, pagination.Params{Page: 1, PerPage: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	ownerResp := statementsSvc.ToResponse(ownerScope, rows[0])
	require.NotNil(t, ownerResp.Phone, "the owner sees every phone")
	require.Equal(t, "+84903334444", *ownerResp.Phone)
	require.NotNil(t, ownerResp.URL, "the owner gets the family URL")
	require.Contains(t, *ownerResp.URL, "/s/")

	secRows, _, err := statementsSvc.List(ctx, secScope, period.ID, pagination.Params{Page: 1, PerPage: 20})
	require.NoError(t, err)
	secResp := statementsSvc.ToResponse(secScope, secRows[0])
	require.NotNil(t, secResp.Phone, "reports oversight sees every phone")
	require.NotNil(t, secResp.URL, "reports oversight sends the links, so it gets them")

	// hoc_vu and tro_giang stints grant no statements reach at all: the
	// period is not theirs and they hold no oversight.
	_, _, err = statementsSvc.List(ctx, testutil.ScopeFor(t, db, hocVu.ID), period.ID, pagination.Params{Page: 1, PerPage: 20})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"a hoc_vu stint alone opens no family-statement listing")
	_, _, err = statementsSvc.List(ctx, testutil.ScopeFor(t, db, troGiang.ID), period.ID, pagination.Params{Page: 1, PerPage: 20})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"a tro_giang stint alone opens no family-statement listing")

	// Class assignments never widen the phone: give the period teacher an
	// active hoc_vu stint on another teacher's class where this contact's
	// student is actively enrolled, and their own statement read still
	// carries no phone.
	classB := testutil.Class(t, db, memberB.ID, testutil.WithClassName("PrivacyB"), testutil.WithClassStartDate(classStart))
	testutil.Enrollment(t, db, memberB.ID, student.ID, classB.ID, classStart)
	testutil.StaffAssignment(t, db, classB, member.ID, "hoc_vu")

	memberRows, _, err := statementsSvc.List(ctx, memberScope, period.ID, pagination.Params{Page: 1, PerPage: 20})
	require.NoError(t, err)
	require.Len(t, memberRows, 1)
	viaHocVu := statementsSvc.ToResponse(memberScope, memberRows[0])
	require.Nil(t, viaHocVu.Phone, "an active hoc_vu stint no longer unlocks the phone")
	require.Nil(t, viaHocVu.URL, "hoc_vu never receives the family URL — only owner/oversight do")

	// contacts.view_all is the one grant that opens the phone, and it still
	// never opens the family URL.
	viewerScope := memberScope
	viewerScope.Perms = authctx.BuildPermSet(nil, []string{authctx.PermContactsViewAll}, nil)
	viaGrant := statementsSvc.ToResponse(viewerScope, memberRows[0])
	require.NotNil(t, viaGrant.Phone, "contacts.view_all unlocks the phone")
	require.Equal(t, "+84903334444", *viaGrant.Phone)
	require.Nil(t, viaGrant.URL, "contacts.view_all is a read grant, never the family URL")
}

// TargetContacts reads rows by the period anchor alone: whose invoices are
// targeted is the anchor's business, and whether the phone may be shown is
// decided above the repository by Scope.PhoneVisible. Pin that the rows
// follow the anchor and always carry the stored phone, so a refactor that
// masks inside the repository fails here rather than in a send path.
func TestTargetContactsRowsFollowTheAnchor(t *testing.T) {
	t.Parallel()
	_, billingSvc, db := newIntegrationDeps(t)
	repo := statements.NewRepository(db)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	testutil.JoinCenter(t, db, member.ID, ownerScope.CenterID)
	memberScope := testutil.ScopeFor(t, db, member.ID)

	contact := testutil.Contact(t, db, member.ID, testutil.WithContactPhone("+84901234567"))
	seedChild(t, db, member.ID, contact.ID, "Anchor", date("2026-08-01"), 1)
	period, err := billingSvc.EnsurePeriod(ctx, memberScope, 2026, 8)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, memberScope, period.ID)
	require.NoError(t, err)

	rows, err := repo.TargetContacts(ctx, memberScope.Self(), period.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, contact.ID, rows[0].ContactID)
	require.Equal(t, "+84901234567", rows[0].Phone, "the repository returns the stored phone; masking is the service's job")
	require.True(t, ownerScope.PhoneVisible())
	require.False(t, memberScope.PhoneVisible(), "a plain member never sees another teacher's contact phone")

	rows, err = repo.TargetContacts(ctx, ownerScope.Self(), period.ID)
	require.NoError(t, err)
	require.Empty(t, rows, "the anchor decides whose invoices are targeted")
}
