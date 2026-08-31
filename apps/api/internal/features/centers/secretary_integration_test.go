//go:build integration

package centers_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// The reports.send permission never touches membership management: a flag holder is
// still a plain member to the offboarding guard and is refused exactly like
// one — an explicit forbidden, since membership visibility is not a secret
// inside one's own center.
func TestSecretaryCannotRemoveMembers(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	_, secretary := testutil.Secretary(t, e.db, e.scope(t, owner.ID).CenterID)
	secScope := e.scope(t, secretary.ID)
	require.True(t, secScope.CanSendReports)
	require.False(t, secScope.IsOwner)

	err := e.centersSvc.RemoveMember(ctx, secScope, member.ID)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"the reports.send permission must not let a member offboard anyone")

	m := e.liveMembership(t, member.ID)
	require.Nil(t, m.LeftAt, "the refused removal must not have closed the membership")
}
