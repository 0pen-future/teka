//go:build integration

package collections_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/collections"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// The one phone rule on the collection board: a contact-balance row carries
// the contact's phone only to the owner, a reports-oversight holder, or a
// caller with an ACTIVE hoc_vu stint on a live class where one of the
// contact's students is actively enrolled. Callers outside the period's reach
// get a 404, exactly as if the period did not exist.
func TestCollectionsPhoneFollowsTheOnePhoneRule(t *testing.T) {
	t.Parallel()
	collectionsSvc, billingSvc, _, db := newIntegrationDeps(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	memberB, _ := testutil.Teacher(t, db)
	hocVu, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	center := ownerScope.CenterID
	testutil.JoinCenter(t, db, member.ID, center)
	testutil.JoinCenter(t, db, memberB.ID, center)
	testutil.JoinCenter(t, db, hocVu.ID, center)
	_, secretary := testutil.Secretary(t, db, center)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	secScope := testutil.ScopeFor(t, db, secretary.ID)

	contact := testutil.Contact(t, db, member.ID, testutil.WithContactPhone("+84907778888"))
	classStart := date("2026-03-01")
	class := testutil.Class(t, db, member.ID, testutil.WithClassName("BoardPrivacyA"), testutil.WithClassStartDate(classStart))
	student := testutil.Student(t, db, member.ID, contact.ID, testutil.WithStudentFullName("Board-privacy-student"))
	enrollment := testutil.Enrollment(t, db, member.ID, student.ID, class.ID, classStart)
	sess := testutil.Session(t, db, member.ID, class.ID, classStart.AddDate(0, 0, 1),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	testutil.AttendanceRecord(t, db, member.ID, sess.ID, student.ID, enrollment.ID)
	testutil.StaffAssignment(t, db, class, hocVu.ID, "hoc_vu")

	period, err := billingSvc.EnsurePeriod(ctx, memberScope, 2026, 3)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, memberScope, period.ID)
	require.NoError(t, err)

	page := pagination.Params{Page: 1, PerPage: 20}

	ownerRes, err := collectionsSvc.List(ctx, ownerScope, period.ID, collections.ViewContact, collections.Filter{}, page)
	require.NoError(t, err)
	require.Len(t, ownerRes.ContactRows, 1)
	require.NotNil(t, ownerRes.ContactRows[0].Phone, "the owner sees every phone")
	require.Equal(t, "+84907778888", *ownerRes.ContactRows[0].Phone)

	secRes, err := collectionsSvc.List(ctx, secScope, period.ID, collections.ViewContact, collections.Filter{}, page)
	require.NoError(t, err)
	require.Len(t, secRes.ContactRows, 1)
	require.NotNil(t, secRes.ContactRows[0].Phone, "reports oversight sees every phone")

	// The period's own teacher reaches their own rows — masked.
	memberRes, err := collectionsSvc.List(ctx, memberScope, period.ID, collections.ViewContact, collections.Filter{}, page)
	require.NoError(t, err)
	require.Len(t, memberRes.ContactRows, 1)
	require.Nil(t, memberRes.ContactRows[0].Phone, "a class teacher without hoc_vu must not see the contact's phone")

	// A hoc_vu stint alone does not reach another teacher's period: 404, as
	// if it did not exist.
	_, err = collectionsSvc.List(ctx, testutil.ScopeFor(t, db, hocVu.ID), period.ID, collections.ViewContact, collections.Filter{}, page)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"another teacher's period must look nonexistent to a non-oversight caller")

	// The row grant is the ONE rule: an active hoc_vu stint on another
	// teacher's class with this contact's student actively enrolled unlocks
	// the phone on the period teacher's own board.
	classB := testutil.Class(t, db, memberB.ID, testutil.WithClassName("BoardPrivacyB"), testutil.WithClassStartDate(classStart))
	testutil.Enrollment(t, db, memberB.ID, student.ID, classB.ID, classStart)
	testutil.StaffAssignment(t, db, classB, member.ID, "hoc_vu")

	memberRes, err = collectionsSvc.List(ctx, memberScope, period.ID, collections.ViewContact, collections.Filter{}, page)
	require.NoError(t, err)
	require.Len(t, memberRes.ContactRows, 1)
	require.NotNil(t, memberRes.ContactRows[0].Phone, "an active hoc_vu stint over the contact's student unlocks the phone")
	require.Equal(t, "+84907778888", *memberRes.ContactRows[0].Phone)
}
