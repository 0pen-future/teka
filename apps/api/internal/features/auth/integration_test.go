//go:build integration

package auth_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

func newIntegrationService(t *testing.T) (*auth.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	issuer := auth.NewTokenIssuer(config.JWTConfig{
		Secret:     testutil.JWTSecret,
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	})
	svc := auth.NewService(teachersSvc, auth.NewRepository(db), issuer, database.NewTxManager(db))
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
		Phone: "0901234567", Password: "password-123", FullName: "Rotate",
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

func TestRegisterDuplicatePhoneRollsBack(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	sess, err := svc.Register(ctx, auth.RegisterRequest{
		Phone: "0901234567", Password: "password-123", FullName: "Once",
	})
	require.NoError(t, err)
	require.Equal(t, "+84901234567", sess.Teacher.Account.Phone,
		"local-form input must be stored as E.164")

	// The E.164 spelling of the same number must collide with the local-form
	// registration above — one number, one account.
	_, err = svc.Register(ctx, auth.RegisterRequest{
		Phone: "+84901234567", Password: "password-123", FullName: "Twice",
	})
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	var accountCount, teacherCount int64
	require.NoError(t, db.Model(&teachers.Account{}).Count(&accountCount).Error)
	require.NoError(t, db.Model(&teachers.Teacher{}).Count(&teacherCount).Error)
	require.EqualValues(t, 1, accountCount, "failed register must persist no account")
	require.EqualValues(t, 1, teacherCount, "failed register must persist no teacher profile")
	require.EqualValues(t, 1, liveTokenCount(t, db))
}

func TestRegisterRollsBackAccountWhenTeacherInsertFails(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	// Calling the service directly bypasses the handler's max=100 binding tag.
	// 101 characters exceeds teachers.full_name VARCHAR(100), so the second
	// INSERT of the transaction fails after the user_accounts row is written —
	// the only path that actually exercises the rollback.
	_, err := svc.Register(ctx, auth.RegisterRequest{
		Phone: "0901234567", Password: "password-123", FullName: strings.Repeat("x", 101),
	})
	require.Error(t, err, "over-length full_name must fail the teachers insert")

	var accountCount int64
	require.NoError(t, db.Unscoped().Model(&teachers.Account{}).
		Where("phone = ?", "+84901234567").Count(&accountCount).Error)
	require.EqualValues(t, 0, accountCount,
		"failed teachers insert must roll back the user_accounts row")
	require.EqualValues(t, 0, liveTokenCount(t, db))
}

func TestRegisterCreatesMatchingAccountAndTeacherRows(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	sess, err := svc.Register(ctx, auth.RegisterRequest{
		Phone: "0912345678", Password: "password-123", FullName: "Cô Lan",
	})
	require.NoError(t, err)

	var acct teachers.Account
	require.NoError(t, db.First(&acct, "phone = ?", "+84912345678").Error)
	var teacher teachers.Teacher
	require.NoError(t, db.First(&teacher, "id = ?", acct.ID).Error)
	require.Equal(t, acct.ID, teacher.ID, "account and teacher must share one id")
	require.Equal(t, sess.Teacher.Account.ID, acct.ID)
	require.Equal(t, "Cô Lan", teacher.FullName)
	require.Equal(t, teachers.DefaultTimezone, teacher.Timezone)
}

func TestLoginAgainstStoredHash(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	acct, _ := testutil.Teacher(t, db, testutil.WithPassword("s3cret-pass!"))

	sess, err := svc.Login(ctx, auth.LoginRequest{Phone: acct.Phone, Password: "s3cret-pass!"})
	require.NoError(t, err)
	require.Equal(t, acct.ID, sess.Teacher.Account.ID)
	require.NotEmpty(t, sess.AccessToken)

	var reloaded teachers.Account
	require.NoError(t, db.First(&reloaded, "id = ?", acct.ID).Error)
	require.NotNil(t, reloaded.LastLoginAt, "login must stamp last_login_at")

	_, err = svc.Login(ctx, auth.LoginRequest{Phone: acct.Phone, Password: "wrong-pass"})
	requireUnauthorized(t, err)
}

func TestLoginRejectsDisabledAccountAgainstRealSQL(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	acct, _ := testutil.Teacher(t, db, testutil.WithStatus(teachers.StatusDisabled))

	_, err := svc.Login(ctx, auth.LoginRequest{Phone: acct.Phone, Password: testutil.DefaultPassword})
	requireUnauthorized(t, err)
}
