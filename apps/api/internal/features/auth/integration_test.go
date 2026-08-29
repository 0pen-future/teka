//go:build integration

package auth_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// capturingDMSender is a scripted auth.ResetDMSender that records the last
// message text so a test can recover the plaintext reset token from the link
// it carries — the only place it's observable, since ForgotPassword's own
// response never carries it.
type capturingDMSender struct {
	lookupOK     bool
	lookupCalled bool
	sendCalled   bool
	lastText     string
}

func (s *capturingDMSender) LookupPhone(_ context.Context, _ uuid.UUID, _ string) (string, bool, error) {
	s.lookupCalled = true
	return "u1", s.lookupOK, nil
}

func (s *capturingDMSender) SendDM(_ context.Context, _ uuid.UUID, _, text string) (string, error) {
	s.sendCalled = true
	s.lastText = text
	return "msg-1", nil
}

// extractResetToken recovers the plaintext token from the message text
// attemptResetDM sent.
func extractResetToken(t *testing.T, text string) string {
	t.Helper()
	const marker = "/reset-password/"
	i := strings.Index(text, marker)
	require.NotEqual(t, -1, i, "dm text missing reset link: %s", text)
	return text[i+len(marker):]
}

// newIntegrationService wires auth the way the router does: teachersSvc is
// its AccountService, centersSvc is its OwnerResolver (owner-exclusion + DM
// anchor for forgot-password), and the returned capturingDMSender is a
// scripted ResetDMSender a test can inspect afterward.
func newIntegrationService(t *testing.T) (*auth.Service, *gorm.DB, *centers.Service, *capturingDMSender) {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	centersSvc := centers.NewService(centers.NewRepository(db), txMgr, nil)
	dmSender := &capturingDMSender{lookupOK: true}
	issuer := auth.NewTokenIssuer(config.JWTConfig{
		Secret:     testutil.JWTSecret,
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	})
	cfg := config.OnboardingConfig{ResetTTL: 48 * time.Hour, ResetCooldown: 15 * time.Minute}
	svc := auth.NewService(teachersSvc, auth.NewRepository(db), issuer, txMgr, centersSvc, dmSender, cfg, "https://app.example.com", nil)
	return svc, db, centersSvc, dmSender
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
	svc, db, _, _ := newIntegrationService(t)
	ctx := context.Background()

	acct, _ := testutil.Teacher(t, db, testutil.WithPassword("password-123"))
	sess, err := svc.Login(ctx, auth.LoginRequest{Phone: acct.Phone, Password: "password-123"}, auth.ClientMeta{})
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

func TestLoginAgainstStoredHash(t *testing.T) {
	t.Parallel()
	svc, db, _, _ := newIntegrationService(t)
	ctx := context.Background()

	acct, _ := testutil.Teacher(t, db, testutil.WithPassword("s3cret-pass!"))

	sess, err := svc.Login(ctx, auth.LoginRequest{Phone: acct.Phone, Password: "s3cret-pass!"}, auth.ClientMeta{})
	require.NoError(t, err)
	require.Equal(t, acct.ID, sess.Teacher.Account.ID)
	require.NotEmpty(t, sess.AccessToken)

	var reloaded teachers.Account
	require.NoError(t, db.First(&reloaded, "id = ?", acct.ID).Error)
	require.NotNil(t, reloaded.LastLoginAt, "login must stamp last_login_at")

	_, err = svc.Login(ctx, auth.LoginRequest{Phone: acct.Phone, Password: "wrong-pass"}, auth.ClientMeta{})
	requireUnauthorized(t, err)
}

func TestLoginRejectsDisabledAccountAgainstRealSQL(t *testing.T) {
	t.Parallel()
	svc, db, _, _ := newIntegrationService(t)
	ctx := context.Background()

	acct, _ := testutil.Teacher(t, db, testutil.WithStatus(teachers.StatusDisabled))

	_, err := svc.Login(ctx, auth.LoginRequest{Phone: acct.Phone, Password: testutil.DefaultPassword}, auth.ClientMeta{})
	requireUnauthorized(t, err)
}

func TestDisableAgainstRealSQLFlipsStatusAndRevokesTokens(t *testing.T) {
	t.Parallel()
	svc, db, _, _ := newIntegrationService(t)
	ctx := context.Background()

	acct, _ := testutil.Teacher(t, db, testutil.WithPassword("password-123"))
	sess, err := svc.Login(ctx, auth.LoginRequest{Phone: acct.Phone, Password: "password-123"}, auth.ClientMeta{})
	require.NoError(t, err)
	require.EqualValues(t, 1, liveTokenCount(t, db))

	require.NoError(t, svc.Disable(ctx, acct.ID))

	var reloaded teachers.Account
	require.NoError(t, db.First(&reloaded, "id = ?", acct.ID).Error)
	require.Equal(t, teachers.StatusDisabled, reloaded.Status)
	require.EqualValues(t, 0, liveTokenCount(t, db), "disable must revoke every live token")

	_, err = svc.Refresh(ctx, sess.RefreshToken)
	requireUnauthorized(t, err)
}

// TestForgotPasswordAndResetRoundTripAgainstRealSQL proves the full member
// forgot -> reset -> login path against real Postgres: the mailed link's
// token resets the password and revokes every pre-existing session.
func TestForgotPasswordAndResetRoundTripAgainstRealSQL(t *testing.T) {
	t.Parallel()
	svc, db, _, dmSender := newIntegrationService(t)
	ctx := context.Background()

	_, ownerTeacher := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db, testutil.WithPassword("old-password-1"))
	testutil.JoinCenter(t, db, member.ID, ownerTeacher.CenterID)

	oldSess, err := svc.Login(ctx, auth.LoginRequest{Phone: member.Phone, Password: "old-password-1"}, auth.ClientMeta{})
	require.NoError(t, err)

	require.NoError(t, svc.ForgotPassword(ctx, auth.ForgotPasswordRequest{Phone: member.Phone}))
	require.True(t, dmSender.lookupCalled)
	require.True(t, dmSender.sendCalled)
	plaintext := extractResetToken(t, dmSender.lastText)

	require.NoError(t, svc.ResetPassword(ctx, auth.ResetPasswordRequest{Token: plaintext, Password: "new-password-1"}))

	_, err = svc.Login(ctx, auth.LoginRequest{Phone: member.Phone, Password: "old-password-1"}, auth.ClientMeta{})
	require.Error(t, err, "the old password must be rejected after a reset")

	sess, err := svc.Login(ctx, auth.LoginRequest{Phone: member.Phone, Password: "new-password-1"}, auth.ClientMeta{})
	require.NoError(t, err, "the new password must work after a reset")
	require.Equal(t, member.ID, sess.Teacher.Account.ID)

	_, err = svc.Refresh(ctx, oldSess.RefreshToken)
	requireUnauthorized(t, err)
}

// TestForgotPasswordExcludesCenterOwnerAgainstRealSQL proves an owner's own
// phone never mints a reset token or triggers a DM — owners recover via
// operator CLI only.
func TestForgotPasswordExcludesCenterOwnerAgainstRealSQL(t *testing.T) {
	t.Parallel()
	svc, db, _, dmSender := newIntegrationService(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)

	require.NoError(t, svc.ForgotPassword(ctx, auth.ForgotPasswordRequest{Phone: owner.Phone}))
	require.False(t, dmSender.lookupCalled, "a center owner must never trigger a reset DM")

	var tokenCount int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM password_reset_tokens WHERE user_id = ?", owner.ID,
	).Scan(&tokenCount).Error)
	require.Equal(t, int64(0), tokenCount, "a center owner must never receive a reset token")
}

// TestForgotPasswordConcurrentRequestsLeaveExactlyOneLiveToken proves the
// partial unique index uq_password_reset_active is honored under a real
// race: two concurrent forgot-password requests for the same account never
// leave two live token rows.
func TestForgotPasswordConcurrentRequestsLeaveExactlyOneLiveToken(t *testing.T) {
	t.Parallel()
	svc, db, _, _ := newIntegrationService(t)
	ctx := context.Background()

	_, ownerTeacher := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, member.ID, ownerTeacher.CenterID)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = svc.ForgotPassword(ctx, auth.ForgotPasswordRequest{Phone: member.Phone})
		}(i)
	}
	wg.Wait()

	require.NoError(t, errs[0], "a concurrent forgot-password race must never surface as an error")
	require.NoError(t, errs[1], "a concurrent forgot-password race must never surface as an error")

	var liveCount int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM password_reset_tokens WHERE user_id = ? AND used_at IS NULL AND superseded_at IS NULL",
		member.ID,
	).Scan(&liveCount).Error)
	require.Equal(t, int64(1), liveCount, "exactly one live token must survive the race")
}

// TestResetPasswordConcurrentDoubleConsumeOnlyOneWins proves the
// SELECT ... FOR UPDATE lock on the reset token row: two goroutines racing to
// redeem the same token against real Postgres serialize, and exactly one
// wins the live->used transition.
func TestResetPasswordConcurrentDoubleConsumeOnlyOneWins(t *testing.T) {
	t.Parallel()
	svc, db, _, dmSender := newIntegrationService(t)
	ctx := context.Background()

	_, ownerTeacher := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db, testutil.WithPassword("old-password-1"))
	testutil.JoinCenter(t, db, member.ID, ownerTeacher.CenterID)

	require.NoError(t, svc.ForgotPassword(ctx, auth.ForgotPasswordRequest{Phone: member.Phone}))
	plaintext := extractResetToken(t, dmSender.lastText)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = svc.ResetPassword(ctx, auth.ResetPasswordRequest{
				Token: plaintext, Password: "new-password-1",
			})
		}(i)
	}
	wg.Wait()

	successes, failures := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		default:
			failures++
			require.Equal(t, apperror.CodeBadRequest, apperror.From(err).Code,
				"the losing reset must answer the generic rejection, not a race artifact")
		}
	}
	require.Equal(t, 1, successes, "exactly one concurrent redemption of the same token must win")
	require.Equal(t, 1, failures)
}
