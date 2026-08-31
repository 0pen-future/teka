//go:build integration

package classes_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// The reports.send permission grants billing/statement/debt READS only — classes
// stay peer territory. A flag holder gets the same neutral not-found a plain
// peer gets, on the read and on every mutation.
func TestSecretaryCannotReadOrMutateMembersClasses(t *testing.T) {
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

	created, err := svc.Create(ctx, memberScope, createRequest())
	require.NoError(t, err)

	_, err = svc.Get(ctx, secScope, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the reports.send permission must not open another member's class")

	_, err = svc.Update(ctx, secScope, created.ID, classes.UpdateClassRequest{
		Name: "Renamed", StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000),
	})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the reports.send permission must not let anyone edit another member's class")

	_, err = svc.Archive(ctx, secScope, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the reports.send permission must not let anyone archive another member's class")

	err = svc.Delete(ctx, secScope, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the reports.send permission must not let anyone delete another member's class")

	got, err := svc.Get(ctx, memberScope, created.ID)
	require.NoError(t, err, "the class must survive every refused mutation")
	require.Equal(t, "Toán 8", got.Name)
}
