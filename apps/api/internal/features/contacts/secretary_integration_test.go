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

// The send-reports flag grants billing/statement/debt READS only — contacts
// stay peer territory. A flag holder gets the same neutral not-found a plain
// peer gets, on the read and on every mutation.
func TestSecretaryCannotReadOrMutateMembersContacts(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID
	testutil.JoinCenter(t, db, member.ID, ownerCenter)
	_, secretary := testutil.Secretary(t, db, ownerCenter)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	secScope := testutil.ScopeFor(t, db, secretary.ID)
	require.True(t, secScope.CanSendReports)
	require.False(t, secScope.IsOwner)

	contact := testutil.Contact(t, db, member.ID)

	_, err := svc.Get(ctx, secScope, contact.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the send-reports flag must not open another member's contact")

	_, err = svc.Update(ctx, secScope, contact.ID, contacts.UpdateRequest{
		FullName: "Renamed", Phone: "+84901239876",
	})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the send-reports flag must not let anyone edit another member's contact")

	_, err = svc.UpdateZaloMapping(ctx, secScope, contact.ID, contacts.ZaloMappingRequest{
		ZaloUserID: "zuid-secretary", ZaloName: "Should Not Stick",
	})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the send-reports flag must not let anyone remap another member's contact")

	err = svc.Delete(ctx, secScope, contact.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the send-reports flag must not let anyone delete another member's contact")

	got, err := svc.Get(ctx, memberScope, contact.ID)
	require.NoError(t, err, "the contact must survive every refused mutation")
	require.Equal(t, contact.FullName, got.FullName)
	require.Nil(t, got.ZaloUserID, "the refused mapping must not have stuck")
}
