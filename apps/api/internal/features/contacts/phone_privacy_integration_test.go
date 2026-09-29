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
// into a single predicate: the owner and a contacts.view_all holder (reports
// oversight through the key it implies) read the whole center; everyone else
// gets an honest empty list and 404s — including the member who anchored the
// row and an active hoc_vu on the class. Writes are the owner's alone (honest
// 403), except zalo-mapping, which is reports oversight's (owner or
// reports.send): a read grant never rewires where a family's messages go.
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
	testutil.StaffAssignment(t, db, class, hocVu.ID, authctx.StaffRoleHocVu)
	testutil.StaffAssignment(t, db, class, troGiang.ID, authctx.StaffRoleTroGiang)

	scGv := testutil.ScopeFor(t, db, gv.ID)
	scHocVu := testutil.ScopeFor(t, db, hocVu.ID)
	scTroGiang := testutil.ScopeFor(t, db, troGiang.ID)
	scSecretary := testutil.ScopeFor(t, db, secretary.ID)
	scViewer := testutil.ScopeFor(t, db, troGiang.ID)
	scViewer.Perms = authctx.BuildPermSet(nil, []string{authctx.PermContactsViewAll}, nil)

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
	total, err = reads(scViewer)
	require.NoError(t, err, "contacts.view_all reads center-wide")
	require.EqualValues(t, 1, total)
	row, err := svc.Get(ctx, scViewer, contact.ID)
	require.NoError(t, err)
	require.Equal(t, "+84911222333", row.Phone, "a reachable contact row carries its phone")

	total, err = reads(scHocVu)
	require.Equal(t, 404, apperror.From(err).Status, "an active hoc_vu stint no longer reaches contacts")
	require.EqualValues(t, 0, total)
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

	// Zalo mapping is reports oversight's: a read grant or a class stint gets
	// the same neutral 404 as an unreachable row.
	mapping := contacts.ZaloMappingRequest{ZaloUserID: "zalo-1", ZaloName: "Na's mom"}
	_, err = svc.UpdateZaloMapping(ctx, scSecretary, contact.ID, mapping)
	require.NoError(t, err, "reports oversight maps center-wide")
	require.NoError(t, svc.ClearZaloMapping(ctx, scSecretary, contact.ID))
	_, err = svc.UpdateZaloMapping(ctx, scOwner, contact.ID, mapping)
	require.NoError(t, err, "the owner maps center-wide")
	for name, sc := range map[string]authctx.Scope{
		"contacts.view_all": scViewer, "hoc_vu": scHocVu, "tro_giang": scTroGiang, "giao_vien": scGv,
	} {
		_, err = svc.UpdateZaloMapping(ctx, sc, contact.ID, mapping)
		require.Equal(t, 404, apperror.From(err).Status, "%s must not write the mapping", name)
		require.Equal(t, 404, apperror.From(svc.ClearZaloMapping(ctx, sc, contact.ID)).Status,
			"%s must not clear the mapping", name)
	}
}
