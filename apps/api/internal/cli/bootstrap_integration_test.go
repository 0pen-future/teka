//go:build integration

package cli

import (
	"context"
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

// stubZaloSender never links an owner, so it satisfies auth.ResetDMSender
// without ever sending anything — these tests exercise bootstrapCenter and
// resetPassword, not Zalo delivery.
type stubZaloSender struct{}

func (stubZaloSender) LookupPhone(context.Context, uuid.UUID, string) (string, bool, error) {
	return "", false, nil
}

func (stubZaloSender) SendDM(context.Context, uuid.UUID, string, string) (string, error) {
	return "", nil
}

// cliEnv wires the exact same teachers/centers/auth stack app.Container
// builds in production, against a real Postgres — so these tests exercise
// the real bootstrap/reset write paths, not fakes.
type cliEnv struct {
	db          *gorm.DB
	tx          database.TxManager
	teachersSvc *teachers.Service
	centersSvc  *centers.Service
	authSvc     *auth.Service
}

func newCLIEnv(t *testing.T) *cliEnv {
	t.Helper()
	db := testutil.StartPostgres(t)
	tx := database.NewTxManager(db)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	centersSvc := centers.NewService(centers.NewRepository(db), tx)
	jwtCfg := config.JWTConfig{Secret: testutil.JWTSecret, AccessTTL: 15 * time.Minute}
	onboardingCfg := config.OnboardingConfig{InviteTTL: 72 * time.Hour, ResetTTL: 48 * time.Hour, ResetCooldown: 15 * time.Minute}
	authSvc := auth.NewService(teachersSvc, auth.NewRepository(db), auth.NewTokenIssuer(jwtCfg), tx,
		centersSvc, stubZaloSender{}, onboardingCfg, "https://app.example.com")
	centersSvc.SetAccountDisabler(authSvc)
	teachersSvc.SetTokenRevoker(authSvc)
	return &cliEnv{db: db, tx: tx, teachersSvc: teachersSvc, centersSvc: centersSvc, authSvc: authSvc}
}

// TestBootstrapCenterFreshDBOwnerCanLogInAndIsOwner proves the happy path an
// operator relies on to stand up a brand-new customer: after bootstrapCenter
// returns, the owner can log in with the password it was given, and
// ResolveScope reports IsOwner=true for the center it just created.
func TestBootstrapCenterFreshDBOwnerCanLogInAndIsOwner(t *testing.T) {
	t.Parallel()
	e := newCLIEnv(t)
	ctx := context.Background()

	centerID, accountID, err := bootstrapCenter(ctx, e.db, e.tx, e.teachersSvc, e.centersSvc,
		"Trung Tâm Anh Ngữ ABC", "+84901234567", "Nguyễn Văn A", "owner-password-1")
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, centerID)
	require.NotEqual(t, uuid.Nil, accountID)

	sess, err := e.authSvc.Login(ctx, auth.LoginRequest{Phone: "+84901234567", Password: "owner-password-1"})
	require.NoError(t, err, "the owner must be able to log in immediately after bootstrap")
	require.Equal(t, accountID, sess.Teacher.Account.ID)

	scope, err := e.centersSvc.ResolveScope(ctx, accountID)
	require.NoError(t, err)
	require.Equal(t, centerID, scope.CenterID)
	require.True(t, scope.IsOwner, "the bootstrapped account must resolve as the center's owner")

	var storedName string
	require.NoError(t, e.db.Raw("SELECT name FROM centers WHERE id = ?", centerID).Scan(&storedName).Error)
	require.Equal(t, "Trung Tâm Anh Ngữ ABC", storedName)
}

// TestBootstrapCenterDuplicatePhoneRollsBackEverything proves there is never
// an ownerless-center or centerless-account window: when the owner phone
// already has an account, the whole transaction — including the new center
// row — rolls back, leaving no trace of the failed attempt.
func TestBootstrapCenterDuplicatePhoneRollsBackEverything(t *testing.T) {
	t.Parallel()
	e := newCLIEnv(t)
	ctx := context.Background()
	existing, _ := testutil.Teacher(t, e.db)

	var centersBefore, accountsBefore int64
	require.NoError(t, e.db.Raw("SELECT count(*) FROM centers").Scan(&centersBefore).Error)
	require.NoError(t, e.db.Raw("SELECT count(*) FROM user_accounts").Scan(&accountsBefore).Error)

	_, _, err := bootstrapCenter(ctx, e.db, e.tx, e.teachersSvc, e.centersSvc,
		"Another Center", existing.Phone, "Someone Else", "some-password-1")
	require.Error(t, err, "bootstrapping with an already-registered phone must fail")
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	var centersAfter, accountsAfter int64
	require.NoError(t, e.db.Raw("SELECT count(*) FROM centers").Scan(&centersAfter).Error)
	require.NoError(t, e.db.Raw("SELECT count(*) FROM user_accounts").Scan(&accountsAfter).Error)
	require.Equal(t, centersBefore, centersAfter, "the placeholder center row must not survive a rollback")
	require.Equal(t, accountsBefore, accountsAfter, "no partial account row must survive a rollback")
}

// TestResetPasswordRevokesSessionAndAllowsReloginWithNewPassword proves the
// operator recovery path end to end: after reset, the account's pre-existing
// refresh token is dead, the old password no longer works, and the new one
// logs in immediately.
func TestResetPasswordRevokesSessionAndAllowsReloginWithNewPassword(t *testing.T) {
	t.Parallel()
	e := newCLIEnv(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, e.db, testutil.WithPassword("old-password-1"))

	oldSess, err := e.authSvc.Login(ctx, auth.LoginRequest{Phone: owner.Phone, Password: "old-password-1"})
	require.NoError(t, err)

	accountID, err := resetPassword(ctx, e.tx, e.teachersSvc, e.authSvc, owner.Phone, "new-password-1")
	require.NoError(t, err)
	require.Equal(t, owner.ID, accountID)

	_, err = e.authSvc.Refresh(ctx, oldSess.RefreshToken)
	require.Error(t, err, "the pre-existing refresh token must be revoked by reset-password")

	_, err = e.authSvc.Login(ctx, auth.LoginRequest{Phone: owner.Phone, Password: "old-password-1"})
	require.Error(t, err, "the old password must no longer work")

	sess, err := e.authSvc.Login(ctx, auth.LoginRequest{Phone: owner.Phone, Password: "new-password-1"})
	require.NoError(t, err, "the new password must work immediately")
	require.Equal(t, owner.ID, sess.Teacher.Account.ID)
}

// TestResetPasswordWorksOnDisabledAccountWithoutReactivatingIt proves the
// documented disabled-account caveat: reset-password can rewrite a disabled
// account's password (e.g. a locked-out owner recovering after an accidental
// offboard), but it must not flip status back to active — login stays
// blocked until something else reactivates the account.
func TestResetPasswordWorksOnDisabledAccountWithoutReactivatingIt(t *testing.T) {
	t.Parallel()
	e := newCLIEnv(t)
	ctx := context.Background()
	member, _ := testutil.Teacher(t, e.db, testutil.WithStatus(teachers.StatusDisabled))

	accountID, err := resetPassword(ctx, e.tx, e.teachersSvc, e.authSvc, member.Phone, "recovered-password-1")
	require.NoError(t, err)
	require.Equal(t, member.ID, accountID)

	var status string
	require.NoError(t, e.db.Raw("SELECT status FROM user_accounts WHERE id = ?", member.ID).Scan(&status).Error)
	require.Equal(t, teachers.StatusDisabled, status, "status must stay disabled")

	_, err = e.authSvc.Login(ctx, auth.LoginRequest{Phone: member.Phone, Password: "recovered-password-1"})
	require.Error(t, err, "login must stay blocked for a disabled account even with the right password")
}
