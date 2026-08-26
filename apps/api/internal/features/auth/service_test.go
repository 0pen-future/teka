package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/token"
	"teka/apps/api/internal/shared/validation"
)

// fakeAccountService implements AccountService in memory. Like the real
// teachers.Service, it normalizes phones before lookup and storage.
type fakeAccountService struct {
	byID map[uuid.UUID]*teachers.Profile
}

func newFakeAccountService() *fakeAccountService {
	return &fakeAccountService{byID: map[uuid.UUID]*teachers.Profile{}}
}

func (f *fakeAccountService) add(t *testing.T, phone, password, status string) *teachers.Profile {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	hashStr := string(hash)
	accountID := id.New()
	p := &teachers.Profile{
		Account: teachers.Account{
			ID:           accountID,
			Role:         authctx.RoleTeacher,
			Phone:        validation.NormalizePhone(phone),
			PasswordHash: &hashStr,
			Status:       status,
		},
		Teacher: teachers.Teacher{ID: accountID, FullName: "Test", Timezone: teachers.DefaultTimezone},
	}
	f.byID[accountID] = p
	return p
}

func (f *fakeAccountService) Disable(_ context.Context, accountID uuid.UUID) error {
	p, ok := f.byID[accountID]
	if !ok {
		return apperror.NotFound("teacher")
	}
	p.Account.Status = teachers.StatusDisabled
	return nil
}

func (f *fakeAccountService) GetByPhone(_ context.Context, phone string) (*teachers.Profile, error) {
	phone = validation.NormalizePhone(phone)
	for _, p := range f.byID {
		if p.Account.Phone == phone {
			return p, nil
		}
	}
	return nil, apperror.NotFound("teacher")
}

func (f *fakeAccountService) GetByID(_ context.Context, accountID uuid.UUID) (*teachers.Profile, error) {
	p, ok := f.byID[accountID]
	if !ok {
		return nil, apperror.NotFound("teacher")
	}
	return p, nil
}

func (f *fakeAccountService) TouchLastLogin(_ context.Context, accountID uuid.UUID) error {
	p, ok := f.byID[accountID]
	if !ok {
		return apperror.NotFound("teacher")
	}
	now := time.Now()
	p.Account.LastLoginAt = &now
	return nil
}

// SetPassword mirrors teachers.Service.SetPassword: it rewrites the hash only
// for a currently-active account and returns the raw teachers.ErrNotFound
// sentinel otherwise, the same guard ResetPassword folds into its generic
// anti-enumeration rejection.
func (f *fakeAccountService) SetPassword(_ context.Context, accountID uuid.UUID, password string) error {
	p, ok := f.byID[accountID]
	if !ok || p.Account.Status != teachers.StatusActive {
		return teachers.ErrNotFound
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		return err
	}
	hashStr := string(hash)
	p.Account.PasswordHash = &hashStr
	return nil
}

// fakeTokenRepository implements Repository in memory.
type fakeTokenRepository struct {
	byHash      map[string]*RefreshToken
	resetByHash map[string]*PasswordResetToken
}

func newFakeTokenRepository() *fakeTokenRepository {
	return &fakeTokenRepository{
		byHash:      map[string]*RefreshToken{},
		resetByHash: map[string]*PasswordResetToken{},
	}
}

func (r *fakeTokenRepository) Create(_ context.Context, t *RefreshToken) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	cp := *t
	r.byHash[t.TokenHash] = &cp
	return nil
}

func (r *fakeTokenRepository) GetByHash(_ context.Context, hash string) (*RefreshToken, error) {
	t, ok := r.byHash[hash]
	if !ok {
		return nil, ErrTokenNotFound
	}
	cp := *t
	return &cp, nil
}

func (r *fakeTokenRepository) Revoke(_ context.Context, id uuid.UUID, at time.Time) error {
	for _, t := range r.byHash {
		if t.ID == id && t.RevokedAt == nil {
			revokedAt := at
			t.RevokedAt = &revokedAt
			return nil
		}
	}
	return ErrTokenAlreadyRevoked
}

func (r *fakeTokenRepository) RevokeFamily(_ context.Context, familyID uuid.UUID, at time.Time) error {
	for _, t := range r.byHash {
		if t.FamilyID == familyID && t.RevokedAt == nil {
			revokedAt := at
			t.RevokedAt = &revokedAt
		}
	}
	return nil
}

func (r *fakeTokenRepository) RevokeAllForUser(_ context.Context, userID uuid.UUID, at time.Time) error {
	for _, t := range r.byHash {
		if t.UserID == userID && t.RevokedAt == nil {
			revokedAt := at
			t.RevokedAt = &revokedAt
		}
	}
	return nil
}

func (r *fakeTokenRepository) CreateResetToken(_ context.Context, t *PasswordResetToken) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	cp := *t
	r.resetByHash[t.TokenHash] = &cp
	return nil
}

func (r *fakeTokenRepository) LatestResetToken(_ context.Context, userID uuid.UUID) (*PasswordResetToken, error) {
	var latest *PasswordResetToken
	for _, t := range r.resetByHash {
		if t.UserID != userID {
			continue
		}
		if latest == nil || t.CreatedAt.After(latest.CreatedAt) {
			latest = t
		}
	}
	if latest == nil {
		return nil, ErrResetTokenNotFound
	}
	cp := *latest
	return &cp, nil
}

func (r *fakeTokenRepository) SupersedeResetTokens(_ context.Context, userID uuid.UUID, at time.Time) error {
	for _, t := range r.resetByHash {
		if t.UserID == userID && t.UsedAt == nil && t.SupersededAt == nil {
			supersededAt := at
			t.SupersededAt = &supersededAt
		}
	}
	return nil
}

func (r *fakeTokenRepository) ConsumeResetTokenForUpdate(_ context.Context, hash string) (*PasswordResetToken, error) {
	t, ok := r.resetByHash[hash]
	if !ok {
		return nil, ErrResetTokenNotFound
	}
	cp := *t
	return &cp, nil
}

func (r *fakeTokenRepository) MarkResetTokenUsed(_ context.Context, id uuid.UUID, at time.Time) error {
	for _, t := range r.resetByHash {
		if t.ID == id {
			usedAt := at
			t.UsedAt = &usedAt
			return nil
		}
	}
	return ErrResetTokenNotFound
}

// fakeOwnerResolver is a scripted OwnerResolver keyed by teacher id,
// mirroring the real centers.Service.CenterOwner ForgotPassword consumes to
// exclude center owners and anchor the reset DM on the owner's Zalo.
type fakeOwnerResolver struct {
	byTeacher map[uuid.UUID]ownerRow
	err       error
}

type ownerRow struct {
	ownerID uuid.UUID
	isOwner bool
}

func newFakeOwnerResolver() *fakeOwnerResolver {
	return &fakeOwnerResolver{byTeacher: map[uuid.UUID]ownerRow{}}
}

// setMember records teacherID as a member (not the owner) of a center owned
// by ownerID.
func (f *fakeOwnerResolver) setMember(teacherID, ownerID uuid.UUID) {
	f.byTeacher[teacherID] = ownerRow{ownerID: ownerID, isOwner: false}
}

// setOwner records teacherID as the owner of their own center.
func (f *fakeOwnerResolver) setOwner(teacherID uuid.UUID) {
	f.byTeacher[teacherID] = ownerRow{ownerID: teacherID, isOwner: true}
}

func (f *fakeOwnerResolver) CenterOwner(_ context.Context, teacherID uuid.UUID) (uuid.UUID, bool, error) {
	if f.err != nil {
		return uuid.Nil, false, f.err
	}
	row, ok := f.byTeacher[teacherID]
	if !ok {
		return uuid.Nil, false, apperror.NotFound("teacher")
	}
	return row.ownerID, row.isOwner, nil
}

// fakeResetDMSender is a scripted ResetDMSender, the same shape
// invitations.fakeZaloSender scripts for the accept flow's best-effort DM.
type fakeResetDMSender struct {
	lookupUID    string
	lookupOK     bool
	lookupErr    error
	sendErr      error
	lookupCalled bool
	sendCalled   bool
	// lastText is the message SendDM was last called with — the only place
	// the reset link (and thus the plaintext token) is observable in a unit
	// test, since ForgotPassword's response never carries it.
	lastText string
}

func (f *fakeResetDMSender) LookupPhone(_ context.Context, _ uuid.UUID, _ string) (string, bool, error) {
	f.lookupCalled = true
	return f.lookupUID, f.lookupOK, f.lookupErr
}

func (f *fakeResetDMSender) SendDM(_ context.Context, _ uuid.UUID, _, text string) (string, error) {
	f.sendCalled = true
	f.lastText = text
	if f.sendErr != nil {
		return "", f.sendErr
	}
	return "msg-1", nil
}

// noopTxManager satisfies database.TxManager without a database.
type noopTxManager struct{}

func (noopTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// testResetConfig is the shared OnboardingConfig every test service builds
// with — only the reset TTL/cooldown matter to auth, the same values
// production wires from cfg.Onboarding.
func testResetConfig() config.OnboardingConfig {
	return config.OnboardingConfig{ResetTTL: 48 * time.Hour, ResetCooldown: 15 * time.Minute}
}

// newTestAuthService builds a Service for the login/refresh/logout/disable
// tests below, which never touch ForgotPassword/ResetPassword — fresh no-op
// owner/DM fakes are enough to satisfy the constructor.
func newTestAuthService(t *testing.T) (*Service, *fakeAccountService, *fakeTokenRepository) {
	t.Helper()
	accounts := newFakeAccountService()
	repo := newFakeTokenRepository()
	issuer := NewTokenIssuer(config.JWTConfig{
		Secret:     "test-secret-at-least-32-characters!!",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	})
	svc := NewService(accounts, repo, issuer, noopTxManager{},
		newFakeOwnerResolver(), &fakeResetDMSender{}, testResetConfig(), "https://app.example.com", nil)
	return svc, accounts, repo
}

// newResetTestService builds a Service for the ForgotPassword/ResetPassword
// tests, where the owner resolver and DM sender are the fakes under test.
func newResetTestService(t *testing.T) (*Service, *fakeAccountService, *fakeTokenRepository, *fakeOwnerResolver, *fakeResetDMSender) {
	t.Helper()
	accounts := newFakeAccountService()
	repo := newFakeTokenRepository()
	owners := newFakeOwnerResolver()
	dmSender := &fakeResetDMSender{}
	issuer := NewTokenIssuer(config.JWTConfig{
		Secret:     "test-secret-at-least-32-characters!!",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	})
	svc := NewService(accounts, repo, issuer, noopTxManager{}, owners, dmSender, testResetConfig(), "https://app.example.com", nil)
	return svc, accounts, repo, owners, dmSender
}

func wantUnauthorized(t *testing.T, err error) {
	t.Helper()
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeUnauthorized {
		t.Fatalf("want UNAUTHORIZED, got %v", err)
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	svc, accounts, _ := newTestAuthService(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)

	_, err := svc.Login(context.Background(), LoginRequest{Phone: "+84901234567", Password: "wrong-password"}, ClientMeta{})
	wantUnauthorized(t, err)

	_, err = svc.Login(context.Background(), LoginRequest{Phone: "+84909999999", Password: "whatever-123"}, ClientMeta{})
	wantUnauthorized(t, err)
}

func TestLoginRejectsDisabledAccount(t *testing.T) {
	svc, accounts, _ := newTestAuthService(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusDisabled)

	// Correct password on a disabled account must look identical to bad
	// credentials.
	_, err := svc.Login(context.Background(), LoginRequest{Phone: "+84901234567", Password: "correct-password"}, ClientMeta{})
	wantUnauthorized(t, err)
}

func TestLoginRejectsPasswordlessAccount(t *testing.T) {
	svc, accounts, _ := newTestAuthService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	p.Account.PasswordHash = nil

	_, err := svc.Login(context.Background(), LoginRequest{Phone: "+84901234567", Password: "correct-password"}, ClientMeta{})
	wantUnauthorized(t, err)
}

func TestLoginSucceedsWithEitherPhoneForm(t *testing.T) {
	svc, accounts, _ := newTestAuthService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)

	for _, phone := range []string{"+84901234567", "0901234567"} {
		sess, err := svc.Login(context.Background(), LoginRequest{Phone: phone, Password: "correct-password"}, ClientMeta{})
		if err != nil {
			t.Fatalf("login with %q: %v", phone, err)
		}
		if sess.Teacher.Account.ID != p.Account.ID {
			t.Fatalf("login with %q: session teacher mismatch", phone)
		}
	}
	if p.Account.LastLoginAt == nil {
		t.Fatal("login must stamp last_login_at")
	}
}

func TestRefreshRotatesWithinFamily(t *testing.T) {
	svc, accounts, repo := newTestAuthService(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"}, ClientMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	first := repo.byHash[HashToken(sess.RefreshToken)]

	rotated, err := svc.Refresh(ctx, sess.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if rotated.RefreshToken == sess.RefreshToken {
		t.Fatal("refresh must issue a new token")
	}
	if repo.byHash[HashToken(sess.RefreshToken)].RevokedAt == nil {
		t.Fatal("old token must be revoked after rotation")
	}
	second := repo.byHash[HashToken(rotated.RefreshToken)]
	if second.FamilyID != first.FamilyID {
		t.Fatal("rotated token must stay in the same family")
	}
}

func TestRefreshRejectsDisabledAccount(t *testing.T) {
	svc, accounts, _ := newTestAuthService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"}, ClientMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// Disabling the account must invalidate its outstanding refresh tokens.
	p.Account.Status = teachers.StatusDisabled
	_, err = svc.Refresh(ctx, sess.RefreshToken)
	wantUnauthorized(t, err)
}

func TestRefreshReuseRevokesFamily(t *testing.T) {
	svc, accounts, repo := newTestAuthService(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"}, ClientMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	rotated, err := svc.Refresh(ctx, sess.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Replaying the rotated-away token is reuse: the whole family dies.
	_, err = svc.Refresh(ctx, sess.RefreshToken)
	wantUnauthorized(t, err)
	if repo.byHash[HashToken(rotated.RefreshToken)].RevokedAt == nil {
		t.Fatal("reuse must revoke the newest token in the family too")
	}

	_, err = svc.Refresh(ctx, rotated.RefreshToken)
	wantUnauthorized(t, err)
}

// staleReadRepository simulates the rotation race: GetByHash returns a
// not-yet-revoked snapshot even after another request revoked the token, the
// way a second concurrent refresh sees the pre-commit row.
type staleReadRepository struct {
	*fakeTokenRepository
}

func (r *staleReadRepository) GetByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	t, err := r.fakeTokenRepository.GetByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	t.RevokedAt = nil
	return t, nil
}

func TestRefreshConcurrentRotationRevokesFamily(t *testing.T) {
	accounts := newFakeAccountService()
	repo := newFakeTokenRepository()
	issuer := NewTokenIssuer(config.JWTConfig{
		Secret:     "test-secret-at-least-32-characters!!",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	})
	svc := NewService(accounts, &staleReadRepository{repo}, issuer, noopTxManager{},
		newFakeOwnerResolver(), &fakeResetDMSender{}, testResetConfig(), "https://app.example.com", nil)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"}, ClientMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	// First refresh wins the race and rotates normally.
	rotated, err := svc.Refresh(ctx, sess.RefreshToken)
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	// Second refresh of the SAME token reads a stale unrevoked snapshot, so
	// its Revoke hits zero rows — that loss must revoke the whole family.
	_, err = svc.Refresh(ctx, sess.RefreshToken)
	wantUnauthorized(t, err)
	if repo.byHash[HashToken(rotated.RefreshToken)].RevokedAt == nil {
		t.Fatal("losing the rotation race must revoke the family")
	}
}

func TestRefreshRejectsExpiredAndUnknown(t *testing.T) {
	svc, accounts, _ := newTestAuthService(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"}, ClientMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	// Jump past the refresh TTL.
	svc.now = func() time.Time { return time.Now().Add(721 * time.Hour) }
	_, err = svc.Refresh(ctx, sess.RefreshToken)
	wantUnauthorized(t, err)

	svc.now = time.Now
	_, err = svc.Refresh(ctx, "never-issued-token")
	wantUnauthorized(t, err)
}

func TestLogoutRevokesFamilyAndIsIdempotent(t *testing.T) {
	svc, accounts, repo := newTestAuthService(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"}, ClientMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := svc.Logout(ctx, sess.RefreshToken, ClientMeta{}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if repo.byHash[HashToken(sess.RefreshToken)].RevokedAt == nil {
		t.Fatal("logout must revoke the token family")
	}

	if err := svc.Logout(ctx, sess.RefreshToken, ClientMeta{}); err != nil {
		t.Fatalf("second logout must be a no-op, got %v", err)
	}
	if err := svc.Logout(ctx, "", ClientMeta{}); err != nil {
		t.Fatalf("logout without cookie must be a no-op, got %v", err)
	}
}

func TestDisableFlipsStatusAndRevokesEveryFamily(t *testing.T) {
	svc, accounts, repo := newTestAuthService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	ctx := context.Background()

	// Two separate logins open two independent token families — Disable must
	// revoke both, not just the latest.
	sessA, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"}, ClientMeta{})
	if err != nil {
		t.Fatalf("login A: %v", err)
	}
	p.Account.Status = teachers.StatusActive // Login doesn't change status; keep active for the 2nd login
	sessB, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"}, ClientMeta{})
	if err != nil {
		t.Fatalf("login B: %v", err)
	}

	if err := svc.Disable(ctx, p.Account.ID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if p.Account.Status != teachers.StatusDisabled {
		t.Fatalf("account must be disabled, got %q", p.Account.Status)
	}
	if repo.byHash[HashToken(sessA.RefreshToken)].RevokedAt == nil {
		t.Fatal("disable must revoke token family A")
	}
	if repo.byHash[HashToken(sessB.RefreshToken)].RevokedAt == nil {
		t.Fatal("disable must revoke token family B too")
	}
}

func TestRevokeAllForUserRevokesOnlyThatUsersTokens(t *testing.T) {
	svc, accounts, repo := newTestAuthService(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	other := accounts.add(t, "+84909999999", "correct-password", teachers.StatusActive)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"}, ClientMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	otherSess, err := svc.Login(ctx, LoginRequest{Phone: "+84909999999", Password: "correct-password"}, ClientMeta{})
	if err != nil {
		t.Fatalf("login other: %v", err)
	}

	if err := svc.RevokeAllForUser(ctx, other.Account.ID); err != nil {
		t.Fatalf("revoke all for user: %v", err)
	}
	if repo.byHash[HashToken(otherSess.RefreshToken)].RevokedAt == nil {
		t.Fatal("the target user's token must be revoked")
	}
	if repo.byHash[HashToken(sess.RefreshToken)].RevokedAt != nil {
		t.Fatal("another user's token must be untouched")
	}
}

// mintResetToken inserts a live (by default) reset token row directly into
// the fake repository for userID and returns its plaintext, letting a
// ResetPassword test drive the flow exactly the way an HTTP caller would:
// knowing only the plaintext. opts, when set, mutates the row before
// insertion (used/expired/superseded).
func mintResetToken(t *testing.T, repo *fakeTokenRepository, userID uuid.UUID, opts func(*PasswordResetToken)) string {
	t.Helper()
	plaintext, hash, err := token.New()
	if err != nil {
		t.Fatalf("mint reset token: %v", err)
	}
	rt := &PasswordResetToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}
	if opts != nil {
		opts(rt)
	}
	repo.resetByHash[hash] = rt
	return plaintext
}

// extractResetToken recovers the plaintext token from the message text
// attemptResetDM sent — the only place the plaintext is observable, since
// ForgotPassword's own response never carries it.
func extractResetToken(t *testing.T, text string) string {
	t.Helper()
	const marker = "/reset-password/"
	idx := strings.Index(text, marker)
	if idx == -1 {
		t.Fatalf("dm text missing reset link: %q", text)
	}
	return text[idx+len(marker):]
}

func TestForgotPasswordMemberMintsTokenAndAttemptsDM(t *testing.T) {
	svc, accounts, repo, owners, dmSender := newResetTestService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	owners.setMember(p.Account.ID, uuid.New())
	dmSender.lookupOK = true
	dmSender.lookupUID = "u1"

	if err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{Phone: "+84901234567"}); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	if len(repo.resetByHash) != 1 {
		t.Fatalf("want exactly one reset token, got %d", len(repo.resetByHash))
	}
	if !dmSender.lookupCalled || !dmSender.sendCalled {
		t.Fatal("a member request must attempt the reset DM")
	}
}

func TestForgotPasswordOwnerIsANoOpWithNoTokenOrSend(t *testing.T) {
	svc, accounts, repo, owners, dmSender := newResetTestService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	owners.setOwner(p.Account.ID)

	if err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{Phone: "+84901234567"}); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	if len(repo.resetByHash) != 0 {
		t.Fatal("a center owner must never receive a reset token")
	}
	if dmSender.lookupCalled {
		t.Fatal("a center owner must never trigger a reset DM")
	}
}

func TestForgotPasswordUnknownPhoneIsANoOp(t *testing.T) {
	svc, _, repo, _, dmSender := newResetTestService(t)

	if err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{Phone: "+84909999999"}); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	if len(repo.resetByHash) != 0 || dmSender.lookupCalled {
		t.Fatal("an unknown phone must never mint a token or attempt a DM")
	}
}

func TestForgotPasswordDisabledAccountIsANoOp(t *testing.T) {
	svc, accounts, repo, owners, dmSender := newResetTestService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusDisabled)
	owners.setMember(p.Account.ID, uuid.New())

	if err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{Phone: "+84901234567"}); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	if len(repo.resetByHash) != 0 || dmSender.lookupCalled {
		t.Fatal("a disabled account must never mint a token or attempt a DM")
	}
}

func TestForgotPasswordCooldownBlocksSecondRequest(t *testing.T) {
	svc, accounts, repo, owners, _ := newResetTestService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	owners.setMember(p.Account.ID, uuid.New())

	if err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{Phone: "+84901234567"}); err != nil {
		t.Fatalf("first request: %v", err)
	}
	if len(repo.resetByHash) != 1 {
		t.Fatalf("want exactly one token after the first request, got %d", len(repo.resetByHash))
	}

	if err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{Phone: "+84901234567"}); err != nil {
		t.Fatalf("second request: %v", err)
	}
	if len(repo.resetByHash) != 1 {
		t.Fatal("a request inside the cooldown window must not mint a second token")
	}
}

func TestForgotPasswordAfterCooldownSupersedesPreviousToken(t *testing.T) {
	svc, accounts, repo, owners, _ := newResetTestService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	owners.setMember(p.Account.ID, uuid.New())

	if err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{Phone: "+84901234567"}); err != nil {
		t.Fatalf("first request: %v", err)
	}
	var first *PasswordResetToken
	for _, rt := range repo.resetByHash {
		first = rt
	}

	// Advance the clock past the cooldown so the second request is allowed.
	svc.now = func() time.Time { return time.Now().Add(time.Hour) }
	if err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{Phone: "+84901234567"}); err != nil {
		t.Fatalf("second request: %v", err)
	}

	if len(repo.resetByHash) != 2 {
		t.Fatalf("want two token rows (one superseded), got %d", len(repo.resetByHash))
	}
	if repo.resetByHash[first.TokenHash].SupersededAt == nil {
		t.Fatal("the previous live token must be superseded once the cooldown passes")
	}
}

func TestForgotPasswordDMFailureDoesNotInvalidateToken(t *testing.T) {
	svc, accounts, repo, owners, dmSender := newResetTestService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	owners.setMember(p.Account.ID, uuid.New())
	dmSender.lookupErr = zalo.ErrNotLinked

	if err := svc.ForgotPassword(context.Background(), ForgotPasswordRequest{Phone: "+84901234567"}); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	if len(repo.resetByHash) != 1 {
		t.Fatal("a best-effort DM delivery failure must not roll back the already-committed token")
	}
}

func TestResetPasswordHappyPathSetsPasswordAndRevokesTokens(t *testing.T) {
	svc, accounts, repo, owners, dmSender := newResetTestService(t)
	p := accounts.add(t, "+84901234567", "old-password", teachers.StatusActive)
	owners.setMember(p.Account.ID, uuid.New())
	dmSender.lookupOK = true
	dmSender.lookupUID = "u1"
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "old-password"}, ClientMeta{})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if err := svc.ForgotPassword(ctx, ForgotPasswordRequest{Phone: "+84901234567"}); err != nil {
		t.Fatalf("forgot password: %v", err)
	}
	plaintext := extractResetToken(t, dmSender.lastText)

	if err := svc.ResetPassword(ctx, ResetPasswordRequest{Token: plaintext, Password: "new-password"}); err != nil {
		t.Fatalf("reset password: %v", err)
	}

	if _, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "old-password"}, ClientMeta{}); err == nil {
		t.Fatal("the old password must be rejected after a reset")
	}
	if _, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "new-password"}, ClientMeta{}); err != nil {
		t.Fatalf("the new password must work after a reset: %v", err)
	}

	if repo.byHash[HashToken(sess.RefreshToken)].RevokedAt == nil {
		t.Fatal("reset must revoke every refresh token the account held")
	}

	// The token was consumed: a second redemption must be rejected.
	if err := svc.ResetPassword(ctx, ResetPasswordRequest{Token: plaintext, Password: "another-password"}); err != errResetRejected {
		t.Fatalf("a reused token must answer the generic rejection, got %v", err)
	}
}

// TestResetPasswordRejectionIsIdenticalAcrossEveryFailureReason proves the
// anti-enumeration guarantee: an unknown token, a used token, an expired
// token, a superseded token, and a token whose account is no longer active
// all answer the exact same shared errResetRejected value.
func TestResetPasswordRejectionIsIdenticalAcrossEveryFailureReason(t *testing.T) {
	scenarios := map[string]func(t *testing.T, accounts *fakeAccountService, repo *fakeTokenRepository) string{
		"unknown token": func(_ *testing.T, _ *fakeAccountService, _ *fakeTokenRepository) string {
			return "does-not-exist"
		},
		"used token": func(t *testing.T, accounts *fakeAccountService, repo *fakeTokenRepository) string {
			p := accounts.add(t, "+84901111111", "password1", teachers.StatusActive)
			return mintResetToken(t, repo, p.Account.ID, func(rt *PasswordResetToken) {
				used := time.Now()
				rt.UsedAt = &used
			})
		},
		"expired token": func(t *testing.T, accounts *fakeAccountService, repo *fakeTokenRepository) string {
			p := accounts.add(t, "+84902222222", "password1", teachers.StatusActive)
			return mintResetToken(t, repo, p.Account.ID, func(rt *PasswordResetToken) {
				rt.ExpiresAt = time.Now().Add(-time.Hour)
			})
		},
		"superseded token": func(t *testing.T, accounts *fakeAccountService, repo *fakeTokenRepository) string {
			p := accounts.add(t, "+84903333333", "password1", teachers.StatusActive)
			return mintResetToken(t, repo, p.Account.ID, func(rt *PasswordResetToken) {
				superseded := time.Now()
				rt.SupersededAt = &superseded
			})
		},
		"disabled account": func(t *testing.T, accounts *fakeAccountService, repo *fakeTokenRepository) string {
			p := accounts.add(t, "+84904444444", "password1", teachers.StatusDisabled)
			return mintResetToken(t, repo, p.Account.ID, nil)
		},
	}

	for name, setup := range scenarios {
		t.Run(name, func(t *testing.T) {
			svc, accounts, repo, _, _ := newResetTestService(t)
			plaintext := setup(t, accounts, repo)
			err := svc.ResetPassword(context.Background(), ResetPasswordRequest{Token: plaintext, Password: "new-password"})
			if err != errResetRejected {
				t.Fatalf("every rejection branch must answer the identical shared value, got %v", err)
			}
		})
	}
}
