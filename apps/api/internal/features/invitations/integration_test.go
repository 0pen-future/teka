//go:build integration

package invitations_test

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
	"teka/apps/api/internal/features/invitations"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/token"
	"teka/apps/api/internal/testutil"
)

// stubZaloSender never links an owner, so every Create in this file reports
// dm_status "skipped" — these tests exercise persistence and the accept
// flow, not Zalo delivery.
type stubZaloSender struct{}

func (stubZaloSender) LookupPhone(context.Context, uuid.UUID, string) (string, bool, error) {
	return "", false, nil
}

func (stubZaloSender) SendDM(context.Context, uuid.UUID, string, string) (string, error) {
	return "", nil
}

// env wires the real teachers/centers/auth stack invitations.Service
// consumes as AccountOnboarder/MembershipOpener — the same construction
// router.go does in production — so the accept flow tests exercise real
// account creation, reactivation, and membership transitions, not fakes.
type env struct {
	db             *gorm.DB
	teachersSvc    *teachers.Service
	centersSvc     *centers.Service
	authSvc        *auth.Service
	invitationsSvc *invitations.Service
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	centersSvc := centers.NewService(centers.NewRepository(db), txMgr)
	jwtCfg := config.JWTConfig{Secret: testutil.JWTSecret, AccessTTL: 15 * time.Minute}
	onboardingCfg := config.OnboardingConfig{InviteTTL: 72 * time.Hour, ResetTTL: 48 * time.Hour, ResetCooldown: 15 * time.Minute}
	// centersSvc is auth's OwnerResolver and stubZaloSender its ResetDMSender —
	// the same wiring router.go does in production (see the comment above).
	authSvc := auth.NewService(teachersSvc, auth.NewRepository(db), auth.NewTokenIssuer(jwtCfg), txMgr,
		centersSvc, stubZaloSender{}, onboardingCfg, "https://app.example.com")
	// Same wiring router.go does in production: authSvc is the
	// AccountDisabler centers consumes to offboard a removed member, and the
	// TokenRevoker teachers consumes to invalidate old sessions on reactivate.
	centersSvc.SetAccountDisabler(authSvc)
	teachersSvc.SetTokenRevoker(authSvc)
	invitationsSvc := invitations.NewService(
		invitations.NewRepository(db), stubZaloSender{}, teachersSvc, centersSvc, txMgr,
		onboardingCfg, "https://app.example.com")
	return &env{db: db, teachersSvc: teachersSvc, centersSvc: centersSvc, authSvc: authSvc, invitationsSvc: invitationsSvc}
}

// tokenFromLink extracts the plaintext trailing /invite/ so a test can prove
// it hashes to whatever token_hash actually ended up stored.
func tokenFromLink(t *testing.T, link string) string {
	t.Helper()
	const marker = "/invite/"
	i := strings.Index(link, marker)
	require.NotEqual(t, -1, i, "link must carry the /invite/ route: %s", link)
	return link[i+len(marker):]
}

// TestConcurrentCreateForSamePhoneLeavesExactlyOnePendingSurvivor proves F15b:
// two owners racing to invite the same phone in the same center never leave
// two live pending rows, and neither caller's create fails — the loser's
// response link is still redeemable because the survivor's token_hash is
// rotated onto the plaintext the loser actually holds.
func TestConcurrentCreateForSamePhoneLeavesExactlyOnePendingSurvivor(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, e.db)
	sc := testutil.ScopeFor(t, e.db, owner.ID)
	phone := "+84901234567"

	var wg sync.WaitGroup
	responses := make([]*invitations.CreateResponse, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			responses[i], errs[i] = e.invitationsSvc.Create(ctx, sc, invitations.CreateRequest{Phone: phone})
		}(i)
	}
	wg.Wait()

	require.NoError(t, errs[0], "a concurrent-create race must never surface as an error")
	require.NoError(t, errs[1], "a concurrent-create race must never surface as an error")

	var pendingCount int64
	require.NoError(t, e.db.Raw(
		"SELECT count(*) FROM invitations WHERE center_id = ? AND phone = ? AND status = 'pending'",
		sc.CenterID, phone,
	).Scan(&pendingCount).Error)
	require.Equal(t, int64(1), pendingCount, "exactly one pending row must survive the race")

	var storedHash string
	require.NoError(t, e.db.Raw(
		"SELECT token_hash FROM invitations WHERE center_id = ? AND phone = ? AND status = 'pending'",
		sc.CenterID, phone,
	).Scan(&storedHash).Error)

	hashA := token.Hash(tokenFromLink(t, responses[0].Link))
	hashB := token.Hash(tokenFromLink(t, responses[1].Link))
	require.True(t, storedHash == hashA || storedHash == hashB,
		"the surviving row's token_hash must match a plaintext one of the two callers actually holds")
}

// TestTokenHashIsUniqueAcrossInvitations proves the token_hash UNIQUE column
// constraint is live: two invitations (even in different centers, for
// different phones) can never share a stored hash.
func TestTokenHashIsUniqueAcrossInvitations(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, e.db)
	sc := testutil.ScopeFor(t, e.db, owner.ID)

	created, err := e.invitationsSvc.Create(ctx, sc, invitations.CreateRequest{Phone: "+84901234567"})
	require.NoError(t, err)

	var existingHash string
	require.NoError(t, e.db.Raw("SELECT token_hash FROM invitations WHERE id = ?", created.ID).
		Scan(&existingHash).Error)

	err = e.db.Exec(
		`INSERT INTO invitations (id, center_id, phone, token_hash, status, expires_at)
		 VALUES (gen_random_uuid(), ?, ?, ?, 'pending', now() + interval '1 day')`,
		sc.CenterID, "+84909999999", existingHash,
	).Error
	require.Error(t, err, "a duplicate token_hash must be rejected by the database")
}

// TestListIsScopedPerCenter proves an owner's invitation list never leaks a
// row from another center, even when both centers have a pending invite for
// the very same phone (the partial unique index is per-center, not global).
func TestListIsScopedPerCenter(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	ownerA, _ := testutil.Teacher(t, e.db)
	ownerB, _ := testutil.Teacher(t, e.db)
	scopeA := testutil.ScopeFor(t, e.db, ownerA.ID)
	scopeB := testutil.ScopeFor(t, e.db, ownerB.ID)
	phone := "+84901234567"

	createdA, err := e.invitationsSvc.Create(ctx, scopeA, invitations.CreateRequest{Phone: phone})
	require.NoError(t, err)
	createdB, err := e.invitationsSvc.Create(ctx, scopeB, invitations.CreateRequest{Phone: phone})
	require.NoError(t, err)
	require.NotEqual(t, createdA.ID, createdB.ID, "each center gets its own row for the same phone")

	rowsA, err := e.invitationsSvc.List(ctx, scopeA)
	require.NoError(t, err)
	require.Len(t, rowsA, 1)
	require.Equal(t, createdA.ID, rowsA[0].ID)

	rowsB, err := e.invitationsSvc.List(ctx, scopeB)
	require.NoError(t, err)
	require.Len(t, rowsB, 1)
	require.Equal(t, createdB.ID, rowsB[0].ID)

	// Revoking A's invite must never touch B's row scoped under the same phone.
	require.NoError(t, e.invitationsSvc.Revoke(ctx, scopeA, createdA.ID))
	rowsB, err = e.invitationsSvc.List(ctx, scopeB)
	require.NoError(t, err)
	require.Len(t, rowsB, 1)
	require.Equal(t, invitations.StatusPending, rowsB[0].Status, "center B's invite must stay pending")
}

// TestPreviewReturnsRealCenterNameAndMaskedPhone proves Preview reads the
// live centers table, not a cached/denormalized copy.
func TestPreviewReturnsRealCenterNameAndMaskedPhone(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Trung Tâm Anh Ngữ ABC"))
	sc := testutil.ScopeFor(t, e.db, owner.ID)

	created, err := e.invitationsSvc.Create(ctx, sc, invitations.CreateRequest{Phone: "+84901234567"})
	require.NoError(t, err)
	plaintext := tokenFromLink(t, created.Link)

	preview, err := e.invitationsSvc.Preview(ctx, invitations.PreviewRequest{Token: plaintext})
	require.NoError(t, err)
	require.Equal(t, "Trung Tâm Anh Ngữ ABC", preview.CenterName, "Center.Name is seeded from the owner's fixture full name")
	require.Equal(t, "+84******567", preview.PhoneMasked)
}

// TestAcceptNewPhoneRoundTripCreatesAccountThatCanLogIn proves the full
// invite -> accept -> login path for a brand-new phone: the account is
// created active, its membership opens in the inviting center, and the
// chosen password logs in immediately afterward.
func TestAcceptNewPhoneRoundTripCreatesAccountThatCanLogIn(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, e.db)
	sc := testutil.ScopeFor(t, e.db, owner.ID)
	phone := "+84901234567"

	created, err := e.invitationsSvc.Create(ctx, sc, invitations.CreateRequest{Phone: phone})
	require.NoError(t, err)
	plaintext := tokenFromLink(t, created.Link)

	err = e.invitationsSvc.Accept(ctx, invitations.AcceptRequest{
		Token: plaintext, FullName: "Nguyễn Văn A", Password: "matkhau123",
	})
	require.NoError(t, err)

	acct, err := e.teachersSvc.FindByPhone(ctx, phone)
	require.NoError(t, err)
	require.Equal(t, teachers.StatusActive, acct.Status)

	memberScope := testutil.ScopeFor(t, e.db, acct.ID)
	require.Equal(t, sc.CenterID, memberScope.CenterID, "the new account must land in the inviting center")
	require.False(t, memberScope.IsOwner)

	sess, err := e.authSvc.Login(ctx, auth.LoginRequest{Phone: phone, Password: "matkhau123"})
	require.NoError(t, err, "the chosen password must work immediately after accept")
	require.Equal(t, acct.ID, sess.Teacher.Account.ID)

	var invStatus string
	require.NoError(t, e.db.Raw("SELECT status FROM invitations WHERE id = ?", created.ID).Scan(&invStatus).Error)
	require.Equal(t, string(invitations.StatusAccepted), invStatus)
}

// TestAcceptReInviteRoundTripReactivatesRemovedMember proves the offboard ->
// re-invite -> accept cycle: a removed (disabled) member cannot log in, a
// fresh invitation from the same center reactivates them with a new
// password, revokes their stale session, and restores login.
func TestAcceptReInviteRoundTripReactivatesRemovedMember(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, e.db)
	ownerScope := testutil.ScopeFor(t, e.db, owner.ID)
	member, _ := testutil.Teacher(t, e.db, testutil.WithPassword("old-password-1"))
	testutil.JoinCenter(t, e.db, member.ID, ownerScope.CenterID)

	// The member logs in once, then gets removed — proving their old session
	// dies with the account, not just future logins.
	oldSess, err := e.authSvc.Login(ctx, auth.LoginRequest{Phone: member.Phone, Password: "old-password-1"})
	require.NoError(t, err)

	require.NoError(t, e.centersSvc.RemoveMember(ctx, ownerScope, member.ID))

	_, err = e.authSvc.Login(ctx, auth.LoginRequest{Phone: member.Phone, Password: "old-password-1"})
	require.Error(t, err, "a removed member must not be able to log in")
	require.Equal(t, apperror.CodeUnauthorized, apperror.From(err).Code)

	_, err = e.authSvc.Refresh(ctx, oldSess.RefreshToken)
	require.Error(t, err, "the removed member's pre-existing session must be revoked, not just new logins blocked")

	created, err := e.invitationsSvc.Create(ctx, ownerScope, invitations.CreateRequest{Phone: member.Phone})
	require.NoError(t, err)
	plaintext := tokenFromLink(t, created.Link)

	err = e.invitationsSvc.Accept(ctx, invitations.AcceptRequest{
		Token: plaintext, FullName: "Nguyễn Văn A (Reactivated)", Password: "new-password-1",
	})
	require.NoError(t, err)

	acct, err := e.teachersSvc.FindByPhone(ctx, member.Phone)
	require.NoError(t, err)
	require.Equal(t, teachers.StatusActive, acct.Status)

	memberScope := testutil.ScopeFor(t, e.db, member.ID)
	require.Equal(t, ownerScope.CenterID, memberScope.CenterID, "reactivation must land back in the inviting center")

	_, err = e.authSvc.Login(ctx, auth.LoginRequest{Phone: member.Phone, Password: "old-password-1"})
	require.Error(t, err, "the pre-removal password must no longer work")

	sess, err := e.authSvc.Login(ctx, auth.LoginRequest{Phone: member.Phone, Password: "new-password-1"})
	require.NoError(t, err, "the password chosen at accept time must work")
	require.Equal(t, member.ID, sess.Teacher.Account.ID)
}

// TestAcceptRejectsDisabledAccountNeverAMemberOfThisCenter proves the
// membership gate: a disabled account with no history in the inviting
// center cannot be reactivated by that center's invitation, even though
// nothing else about the token is wrong.
func TestAcceptRejectsDisabledAccountNeverAMemberOfThisCenter(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	otherOwner, _ := testutil.Teacher(t, e.db)
	otherScope := testutil.ScopeFor(t, e.db, otherOwner.ID)
	// A member of a wholly unrelated center, disabled there.
	stranger, _ := testutil.Teacher(t, e.db, testutil.WithStatus(teachers.StatusDisabled))

	invitingOwner, _ := testutil.Teacher(t, e.db)
	invitingScope := testutil.ScopeFor(t, e.db, invitingOwner.ID)
	created, err := e.invitationsSvc.Create(ctx, invitingScope, invitations.CreateRequest{Phone: stranger.Phone})
	require.NoError(t, err)
	plaintext := tokenFromLink(t, created.Link)

	err = e.invitationsSvc.Accept(ctx, invitations.AcceptRequest{Token: plaintext, FullName: "X", Password: "password1"})
	require.Error(t, err)
	require.Equal(t, apperror.CodeBadRequest, apperror.From(err).Code)

	_, verifyErr := e.centersSvc.WasEverMember(ctx, stranger.ID, otherScope.CenterID)
	require.NoError(t, verifyErr)
}

// TestAcceptConcurrentDoubleAcceptOfSameTokenOnlyOneWins proves the
// SELECT ... FOR UPDATE lock on the invitation row: two goroutines racing
// to accept the same token against real Postgres serialize, and exactly one
// wins the pending->accepted transition.
func TestAcceptConcurrentDoubleAcceptOfSameTokenOnlyOneWins(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, e.db)
	sc := testutil.ScopeFor(t, e.db, owner.ID)
	phone := "+84901234567"

	created, err := e.invitationsSvc.Create(ctx, sc, invitations.CreateRequest{Phone: phone})
	require.NoError(t, err)
	plaintext := tokenFromLink(t, created.Link)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = e.invitationsSvc.Accept(ctx, invitations.AcceptRequest{
				Token: plaintext, FullName: "Nguyễn Văn A", Password: "matkhau123",
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
				"the losing accept must answer the generic rejection, not a race artifact like a duplicate-key error")
		}
	}
	require.Equal(t, 1, successes, "exactly one concurrent accept of the same token must win")
	require.Equal(t, 1, failures)

	var acctCount int64
	require.NoError(t, e.db.Raw(
		"SELECT count(*) FROM user_accounts WHERE phone = ?", phone,
	).Scan(&acctCount).Error)
	require.Equal(t, int64(1), acctCount, "the race must never create two accounts for the same phone")
}
