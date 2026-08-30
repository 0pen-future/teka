//go:build integration

package contacts_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// The send-reports flag is reports oversight: the secretary sends every
// statement, so they read every contact in the center — including one still
// anchored on a member from before the ownership migration — and may fix its
// zalo mapping. Managing the directory itself stays the owner's: renames and
// deletes come back as an explicit 403, not a neutral miss, because the row is
// visibly there.
func TestSecretaryReadsCenterWideButCannotManageContacts(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	testutil.JoinCenter(t, db, member.ID, ownerScope.CenterID)
	_, secretary := testutil.Secretary(t, db, ownerScope.CenterID)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	secScope := testutil.ScopeFor(t, db, secretary.ID)
	require.True(t, secScope.CanSendReports)
	require.False(t, secScope.IsOwner)

	contact := testutil.Contact(t, db, member.ID)

	got, err := svc.Get(ctx, secScope, contact.ID)
	require.NoError(t, err, "reports oversight reads every phone row in the center")
	require.Equal(t, contact.FullName, got.FullName)
	require.Equal(t, contact.Phone, got.Phone, "a contact row IS a phone row — no masking")

	// A member without oversight or an active hoc_vu stint has no contact
	// reach, not even to a row still anchored on them.
	_, err = svc.Get(ctx, memberScope, contact.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"contact reach needs oversight or an active hoc_vu stint")

	// Directory management is the owner's data ownership, not oversight.
	_, err = svc.Update(ctx, secScope, contact.ID, contacts.UpdateRequest{
		FullName: "Renamed", Phone: "+84901239876",
	})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"only the owner edits the directory")

	err = svc.Delete(ctx, secScope, contact.ID)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"only the owner deletes from the directory")

	// The zalo mapping is how reports get delivered, so oversight maps it.
	mapped, err := svc.UpdateZaloMapping(ctx, secScope, contact.ID, contacts.ZaloMappingRequest{
		ZaloUserID: "zuid-secretary", ZaloName: "Hoa Zalo",
	})
	require.NoError(t, err, "reports oversight manages zalo delivery mappings")
	require.NotNil(t, mapped.ZaloUserID)
	require.Equal(t, "zuid-secretary", *mapped.ZaloUserID)

	got, err = svc.Get(ctx, ownerScope, contact.ID)
	require.NoError(t, err, "the contact must survive every refused mutation")
	require.Equal(t, contact.FullName, got.FullName)
	require.NotNil(t, got.ZaloUserID, "the secretary's mapping stuck")
}
