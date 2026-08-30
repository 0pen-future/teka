//go:build integration

package contacts_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// TestContactAccessAcrossRoles pins the contacts surface of the one phone
// rule. Contacts ARE phone rows, so read reach and phone visibility collapse
// into a single predicate: the owner and reports oversight read the whole
// center; an ACTIVE hoc_vu stint reaches exactly the contacts whose students
// are actively enrolled in the assigned class; everyone else gets an honest
// empty list and 404s — including the member who anchored the row. Writes are
// the owner's alone (honest 403), except zalo-mapping, which follows the read
// predicate so hoc_vu can wire up the parents of their own class.
func TestContactAccessAcrossRoles(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	_, owner := testutil.Teacher(t, db)
	scOwner := testutil.ScopeFor(t, db, owner.ID)
	_, gv := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, gv.ID, scOwner.CenterID)
	_, hocVu := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, hocVu.ID, scOwner.CenterID)
	_, troGiang := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, troGiang.ID, scOwner.CenterID)
	_, secretary := testutil.Secretary(t, db, scOwner.CenterID)

	// Pre-migration shape on purpose: the row anchors to the member who
	// created it, and access must already follow the new rules regardless.
	class := testutil.Class(t, db, gv.ID)
	contact := testutil.Contact(t, db, gv.ID, testutil.WithContactPhone("+84911222333"))
	student := testutil.Student(t, db, gv.ID, contact.ID)
	testutil.Enrollment(t, db, gv.ID, student.ID, class.ID,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	hocVuStint := testutil.StaffAssignment(t, db, class, hocVu.ID, authctx.StaffRoleHocVu)
	testutil.StaffAssignment(t, db, class, troGiang.ID, authctx.StaffRoleTroGiang)

	scGv := testutil.ScopeFor(t, db, gv.ID)
	scHocVu := testutil.ScopeFor(t, db, hocVu.ID)
	scTroGiang := testutil.ScopeFor(t, db, troGiang.ID)
	scSecretary := testutil.ScopeFor(t, db, secretary.ID)

	reads := func(sc authctx.Scope) (int64, error) {
		t.Helper()
		_, total, err := svc.List(ctx, sc, contacts.ListFilter{}, listParams(t, ""))
		require.NoError(t, err, "list never errors — it narrows")
		_, getErr := svc.Get(ctx, sc, contact.ID)
		return total, getErr
	}

	total, err := reads(scOwner)
	require.NoError(t, err, "owner reads every contact")
	require.EqualValues(t, 1, total)
	total, err = reads(scSecretary)
	require.NoError(t, err, "reports oversight reads center-wide")
	require.EqualValues(t, 1, total)
	total, err = reads(scHocVu)
	require.NoError(t, err, "active hoc_vu reaches the assigned class's contacts")
	require.EqualValues(t, 1, total)
	row, err := svc.Get(ctx, scHocVu, contact.ID)
	require.NoError(t, err)
	require.Equal(t, "+84911222333", row.Phone, "a reachable contact row carries its phone")

	total, err = reads(scGv)
	require.Equal(t, 404, apperror.From(err).Status, "the anchoring giao_vien is a plain member now")
	require.EqualValues(t, 0, total)
	total, err = reads(scTroGiang)
	require.Equal(t, 404, apperror.From(err).Status)
	require.EqualValues(t, 0, total)

	// Writes: owner-only, honest 403 for every member — reachability included.
	_, err = svc.Create(ctx, scGv, contacts.CreateRequest{FullName: "Chị Hai", Phone: "0902333444"})
	require.Equal(t, 403, apperror.From(err).Status)
	_, err = svc.Update(ctx, scHocVu, contact.ID,
		contacts.UpdateRequest{FullName: "Đổi tên", Phone: "0911222333"})
	require.Equal(t, 403, apperror.From(err).Status)
	require.Equal(t, 403, apperror.From(svc.Delete(ctx, scGv, contact.ID)).Status)
	_, err = svc.Update(ctx, scOwner, contact.ID,
		contacts.UpdateRequest{FullName: "Phụ huynh Na", Phone: "0911222333"})
	require.NoError(t, err, "the owner edits member-anchored rows")

	// Zalo mapping follows the read predicate: hoc_vu and oversight may wire
	// their reachable contacts; an unreachable member gets a neutral 404.
	mapping := contacts.ZaloMappingRequest{ZaloUserID: "zalo-1", ZaloName: "Na's mom"}
	_, err = svc.UpdateZaloMapping(ctx, scHocVu, contact.ID, mapping)
	require.NoError(t, err, "active hoc_vu maps the contacts of the assigned class")
	require.NoError(t, svc.ClearZaloMapping(ctx, scHocVu, contact.ID))
	_, err = svc.UpdateZaloMapping(ctx, scSecretary, contact.ID, mapping)
	require.NoError(t, err, "reports oversight maps center-wide")
	_, err = svc.UpdateZaloMapping(ctx, scTroGiang, contact.ID, mapping)
	require.Equal(t, 404, apperror.From(err).Status)
	_, err = svc.UpdateZaloMapping(ctx, scGv, contact.ID, mapping)
	require.Equal(t, 404, apperror.From(err).Status)

	// Ending the stint ends the reach: unlike student rows, contact rows are
	// pure phone data, so no history-read survives.
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE id = ?", hocVuStint).Error)
	_, err = svc.Get(ctx, scHocVu, contact.ID)
	require.Equal(t, 404, apperror.From(err).Status, "an ended stint drops contact reach")
}
