//go:build integration

package users_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/users"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// parseParams builds pagination.Params from a raw query string, the same way
// handlers do, so repository tests exercise the real sort whitelist logic.
func parseParams(t *testing.T, rawQuery string) pagination.Params {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
	return pagination.Parse(c, "-created_at", map[string]string{
		"created_at": "created_at", "name": "name", "email": "email",
	})
}

func TestRepositoryUniqueEmailAcrossSoftDelete(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	repo := users.NewRepository(db)
	ctx := context.Background()

	first := testutil.User(t, db, testutil.WithEmail("dup@example.com"))

	err := repo.Create(ctx, &users.User{
		Email: "DUP@example.com", PasswordHash: "x", Name: "Dup", Role: users.RoleUser,
	})
	require.ErrorIs(t, err, users.ErrDuplicateEmail,
		"citext unique index must reject the same email case-insensitively")

	require.NoError(t, repo.SoftDelete(ctx, first.ID))

	_, err = repo.GetByEmail(ctx, "dup@example.com")
	require.ErrorIs(t, err, users.ErrNotFound, "soft-deleted users must be invisible")

	require.NoError(t, repo.Create(ctx, &users.User{
		Email: "dup@example.com", PasswordHash: "x", Name: "Again", Role: users.RoleUser,
	}), "partial unique index must allow re-registering a soft-deleted email")

	u, err := repo.GetByEmail(ctx, "dup@example.com")
	require.NoError(t, err)
	require.Equal(t, "Again", u.Name)
}

func TestRepositoryListFilterSortPagination(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	repo := users.NewRepository(db)
	ctx := context.Background()

	// Insertion order fixes created_at order: eve is newest. Alice's surname
	// appears only in her NAME so the case-insensitive search assertion pins
	// ILIKE on name — her citext email alone would also match a plain LIKE.
	testutil.User(t, db, testutil.WithName("Alice Liddell"), testutil.WithEmail("alice@example.com"))
	testutil.User(t, db, testutil.WithName("Bob"), testutil.WithEmail("bob@example.com"))
	testutil.User(t, db, testutil.WithName("Carol"), testutil.WithEmail("carol@example.com"), testutil.WithRole(users.RoleAdmin))
	testutil.User(t, db, testutil.WithName("Dave"), testutil.WithEmail("dave@example.com"))
	testutil.User(t, db, testutil.WithName("Eve"), testutil.WithEmail("eve@example.com"))

	rows, total, err := repo.List(ctx, users.ListFilter{Query: "LIDDELL"}, parseParams(t, ""))
	require.NoError(t, err)
	require.EqualValues(t, 1, total, "name search must match case-insensitively (ILIKE)")
	require.Equal(t, "Alice Liddell", rows[0].Name)

	rows, total, err = repo.List(ctx, users.ListFilter{Role: users.RoleAdmin}, parseParams(t, ""))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "Carol", rows[0].Name)

	// Page 2 of size 2 sorted by -created_at over [Eve Dave Carol Bob Alice].
	rows, total, err = repo.List(ctx, users.ListFilter{}, parseParams(t, "page=2&per_page=2&sort=-created_at"))
	require.NoError(t, err)
	require.EqualValues(t, 5, total)
	require.Len(t, rows, 2)
	require.Equal(t, "Carol", rows[0].Name)
	require.Equal(t, "Bob", rows[1].Name)

	// Ascending name sort.
	rows, _, err = repo.List(ctx, users.ListFilter{}, parseParams(t, "per_page=1&sort=name"))
	require.NoError(t, err)
	require.Equal(t, "Alice Liddell", rows[0].Name)
}

func TestWithinTxRollsBack(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	repo := users.NewRepository(db)
	txMgr := database.NewTxManager(db)
	ctx := context.Background()

	boom := errors.New("boom")
	err := txMgr.WithinTx(ctx, func(ctx context.Context) error {
		if err := repo.Create(ctx, &users.User{
			Email: "rollback@example.com", PasswordHash: "x", Name: "Roll", Role: users.RoleUser,
		}); err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)

	_, err = repo.GetByEmail(ctx, "rollback@example.com")
	require.ErrorIs(t, err, users.ErrNotFound, "a failed transaction must persist nothing")
}
