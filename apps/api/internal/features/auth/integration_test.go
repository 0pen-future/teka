//go:build integration

package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/users"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

func newIntegrationService(t *testing.T) (*auth.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	usersSvc := users.NewService(users.NewRepository(db))
	issuer := auth.NewTokenIssuer(config.JWTConfig{
		Secret:     testutil.JWTSecret,
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	})
	svc := auth.NewService(usersSvc, auth.NewRepository(db), issuer, database.NewTxManager(db))
	return svc, db
}

func liveTokenCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&auth.RefreshToken{}).Where("revoked_at IS NULL").Count(&n).Error)
	return n
}

func requireUnauthorized(t *testing.T, err error) {
	t.Helper()
	appErr := apperror.From(err)
	require.Equal(t, apperror.CodeUnauthorized, appErr.Code, "want UNAUTHORIZED, got %v", err)
}

func TestRefreshRotationAndReuseAgainstRealSQL(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	sess, err := svc.Register(ctx, auth.RegisterRequest{
		Email: "rotate@example.com", Password: "password-123", Name: "Rotate",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, liveTokenCount(t, db))

	rotated, err := svc.Refresh(ctx, sess.RefreshToken)
	require.NoError(t, err)
	require.NotEqual(t, sess.RefreshToken, rotated.RefreshToken)
	require.EqualValues(t, 1, liveTokenCount(t, db), "rotation must revoke the old token and issue exactly one new one")

	// Replaying the rotated-away token is reuse: the whole family dies.
	_, err = svc.Refresh(ctx, sess.RefreshToken)
	requireUnauthorized(t, err)
	require.EqualValues(t, 0, liveTokenCount(t, db), "reuse must revoke every live token in the family")

	_, err = svc.Refresh(ctx, rotated.RefreshToken)
	requireUnauthorized(t, err)
}

func TestRegisterDuplicateEmailRollsBack(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	_, err := svc.Register(ctx, auth.RegisterRequest{
		Email: "once@example.com", Password: "password-123", Name: "Once",
	})
	require.NoError(t, err)

	_, err = svc.Register(ctx, auth.RegisterRequest{
		Email: "once@example.com", Password: "password-123", Name: "Twice",
	})
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	var userCount int64
	require.NoError(t, db.Model(&users.User{}).Count(&userCount).Error)
	require.EqualValues(t, 1, userCount, "failed register must persist nothing")
	require.EqualValues(t, 1, liveTokenCount(t, db))
}

func TestLoginAgainstStoredHash(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	u := testutil.User(t, db, testutil.WithPassword("s3cret-pass!"))

	sess, err := svc.Login(ctx, auth.LoginRequest{Email: u.Email, Password: "s3cret-pass!"})
	require.NoError(t, err)
	require.Equal(t, u.ID, sess.User.ID)
	require.NotEmpty(t, sess.AccessToken)

	_, err = svc.Login(ctx, auth.LoginRequest{Email: u.Email, Password: "wrong-pass"})
	requireUnauthorized(t, err)
}
