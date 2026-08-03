//go:build integration

package seeds_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/testutil"
	"teka/apps/api/seeds"
)

func TestRunIsIdempotent(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	ctx := context.Background()
	log := slog.New(slog.DiscardHandler)

	require.NoError(t, seeds.Run(ctx, db, log))

	var accounts, teachers int64
	require.NoError(t, db.Raw("SELECT count(*) FROM user_accounts").Scan(&accounts).Error)
	require.NoError(t, db.Raw("SELECT count(*) FROM teachers").Scan(&teachers).Error)
	require.Positive(t, accounts, "seed inserted no accounts")
	require.Equal(t, accounts, teachers, "every account needs a matching teachers row")

	// Second run must not add or modify anything.
	require.NoError(t, seeds.Run(ctx, db, log))
	var accountsAfter int64
	require.NoError(t, db.Raw("SELECT count(*) FROM user_accounts").Scan(&accountsAfter).Error)
	require.Equal(t, accounts, accountsAfter, "reseed must be a no-op")
}
