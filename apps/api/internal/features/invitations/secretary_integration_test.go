//go:build integration

package invitations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/invitations"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// The reports.send permission never touches membership management: inviting a new
// teacher stays owner-only, and a flag holder is refused exactly like a plain
// member — an explicit forbidden from the invitation write guard.
func TestSecretaryCannotCreateInvitations(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	ownerCenter := testutil.ScopeFor(t, e.db, owner.ID).CenterID
	_, secretary := testutil.Secretary(t, e.db, ownerCenter)
	secScope := testutil.ScopeFor(t, e.db, secretary.ID)
	require.True(t, secScope.CanSendReports)
	require.False(t, secScope.IsOwner)

	_, err := e.invitationsSvc.Create(ctx, secScope, invitations.CreateRequest{Phone: "+84901234567"})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"the reports.send permission must not let a member invite teachers")

	var pending int64
	require.NoError(t, e.db.Table("invitations").
		Where("center_id = ?", ownerCenter).Count(&pending).Error)
	require.Zero(t, pending, "the refused create must not have written an invitation")
}
