//go:build integration

package collections_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/collections"
	"teka/apps/api/internal/features/payments"
	"teka/apps/api/internal/testutil"
)

// A reports.send-holding member reads the collections debt center wide — another
// member's contact rows and period summary with real figures — exactly the
// oversight view the delegated reminder flow depends on.
func TestSecretarySeesMembersCollectionsWithFullContent(t *testing.T) {
	t.Parallel()
	collectionsSvc, billingSvc, paymentsSvc, db := newIntegrationDeps(t)
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

	contact := testutil.Contact(t, db, member.ID, testutil.WithContactFullName("Member Contact"))
	seedChild(t, db, member.ID, contact.ID, "S1", date("2026-01-01"), 1)

	period, err := billingSvc.EnsurePeriod(ctx, memberScope, 2026, 1)
	require.NoError(t, err)
	_, err = billingSvc.Close(ctx, memberScope, period.ID)
	require.NoError(t, err)
	_, err = paymentsSvc.Record(ctx, memberScope, payments.RecordPaymentRequest{
		ContactID: contact.ID, Amount: 60_000, Method: payments.MethodCash, ReceivedOn: "2026-01-20",
	})
	require.NoError(t, err)

	result, err := collectionsSvc.List(ctx, secScope, period.ID, collections.ViewContact, collections.Filter{}, contactParams(t, ""))
	require.NoError(t, err, "a send-reports holder must read a member's collections")
	require.Len(t, result.ContactRows, 1)
	row := result.ContactRows[0]
	require.Equal(t, contact.ID, row.ContactID)
	require.Equal(t, "Member Contact", row.FullName)
	require.EqualValues(t, 100_000, row.TotalDue)
	require.EqualValues(t, 60_000, row.TotalPaid)
	require.EqualValues(t, 40_000, row.Outstanding)

	summary, err := collectionsSvc.Summary(ctx, secScope, period.ID)
	require.NoError(t, err, "a send-reports holder must read a member's period summary")
	require.EqualValues(t, 1, summary.ContactCount)
	require.EqualValues(t, 40_000, summary.TotalOutstanding)
}
