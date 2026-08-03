package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
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

func (f *fakeAccountService) CreateTeacher(_ context.Context, req teachers.CreateRequest) (*teachers.Profile, error) {
	phone := validation.NormalizePhone(req.Phone)
	for _, p := range f.byID {
		if p.Account.Phone == phone {
			return nil, apperror.Conflict("phone already registered")
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.MinCost)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	hashStr := string(hash)
	accountID := id.New()
	p := &teachers.Profile{
		Account: teachers.Account{
			ID:           accountID,
			Role:         authctx.RoleTeacher,
			Phone:        phone,
			PasswordHash: &hashStr,
			Status:       teachers.StatusActive,
		},
		Teacher: teachers.Teacher{ID: accountID, FullName: req.FullName, Timezone: teachers.DefaultTimezone},
	}
	f.byID[accountID] = p
	return p, nil
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

// fakeTokenRepository implements Repository in memory.
type fakeTokenRepository struct {
	byHash map[string]*RefreshToken
}

func newFakeTokenRepository() *fakeTokenRepository {
	return &fakeTokenRepository{byHash: map[string]*RefreshToken{}}
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

// noopTxManager satisfies database.TxManager without a database.
type noopTxManager struct{}

func (noopTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func newTestAuthService(t *testing.T) (*Service, *fakeAccountService, *fakeTokenRepository) {
	t.Helper()
	accounts := newFakeAccountService()
	repo := newFakeTokenRepository()
	issuer := NewTokenIssuer(config.JWTConfig{
		Secret:     "test-secret-at-least-32-characters!!",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	})
	return NewService(accounts, repo, issuer, noopTxManager{}), accounts, repo
}

func wantUnauthorized(t *testing.T, err error) {
	t.Helper()
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeUnauthorized {
		t.Fatalf("want UNAUTHORIZED, got %v", err)
	}
}

func TestRegisterCreatesTeacherRoleAndSession(t *testing.T) {
	svc, _, repo := newTestAuthService(t)

	sess, err := svc.Register(context.Background(), RegisterRequest{
		Phone: "0901234567", Password: "password-123", FullName: "Cô Lan",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if sess.Teacher.Account.Role != authctx.RoleTeacher {
		t.Fatalf("registered role must be %q, got %q", authctx.RoleTeacher, sess.Teacher.Account.Role)
	}
	if sess.Teacher.Account.Phone != "+84901234567" {
		t.Fatalf("phone must be stored in E.164, got %q", sess.Teacher.Account.Phone)
	}
	if sess.AccessToken == "" || sess.RefreshToken == "" {
		t.Fatal("session missing tokens")
	}
	if _, ok := repo.byHash[sess.RefreshToken]; ok {
		t.Fatal("refresh token stored in plaintext")
	}
	if _, ok := repo.byHash[HashToken(sess.RefreshToken)]; !ok {
		t.Fatal("refresh token hash not stored")
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	svc, accounts, _ := newTestAuthService(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)

	_, err := svc.Login(context.Background(), LoginRequest{Phone: "+84901234567", Password: "wrong-password"})
	wantUnauthorized(t, err)

	_, err = svc.Login(context.Background(), LoginRequest{Phone: "+84909999999", Password: "whatever-123"})
	wantUnauthorized(t, err)
}

func TestLoginRejectsDisabledAccount(t *testing.T) {
	svc, accounts, _ := newTestAuthService(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusDisabled)

	// Correct password on a disabled account must look identical to bad
	// credentials.
	_, err := svc.Login(context.Background(), LoginRequest{Phone: "+84901234567", Password: "correct-password"})
	wantUnauthorized(t, err)
}

func TestLoginRejectsPasswordlessAccount(t *testing.T) {
	svc, accounts, _ := newTestAuthService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	p.Account.PasswordHash = nil

	_, err := svc.Login(context.Background(), LoginRequest{Phone: "+84901234567", Password: "correct-password"})
	wantUnauthorized(t, err)
}

func TestLoginSucceedsWithEitherPhoneForm(t *testing.T) {
	svc, accounts, _ := newTestAuthService(t)
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)

	for _, phone := range []string{"+84901234567", "0901234567"} {
		sess, err := svc.Login(context.Background(), LoginRequest{Phone: phone, Password: "correct-password"})
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

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"})
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

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"})
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

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"})
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
	svc := NewService(accounts, &staleReadRepository{repo}, issuer, noopTxManager{})
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"})
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

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"})
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

	sess, err := svc.Login(ctx, LoginRequest{Phone: "+84901234567", Password: "correct-password"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := svc.Logout(ctx, sess.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if repo.byHash[HashToken(sess.RefreshToken)].RevokedAt == nil {
		t.Fatal("logout must revoke the token family")
	}

	if err := svc.Logout(ctx, sess.RefreshToken); err != nil {
		t.Fatalf("second logout must be a no-op, got %v", err)
	}
	if err := svc.Logout(ctx, ""); err != nil {
		t.Fatalf("logout without cookie must be a no-op, got %v", err)
	}
}
