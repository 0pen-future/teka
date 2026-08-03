package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/users"
	"teka/apps/api/internal/shared/apperror"
)

// fakeUserService implements UserService in memory.
type fakeUserService struct {
	byID map[uuid.UUID]*users.User
}

func newFakeUserService() *fakeUserService {
	return &fakeUserService{byID: map[uuid.UUID]*users.User{}}
}

func (f *fakeUserService) add(t *testing.T, email, password, role string) *users.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u := &users.User{ID: uuid.New(), Email: email, PasswordHash: string(hash), Name: "Test", Role: role}
	f.byID[u.ID] = u
	return u
}

func (f *fakeUserService) Create(_ context.Context, req users.CreateRequest) (*users.User, error) {
	for _, u := range f.byID {
		if u.Email == req.Email {
			return nil, apperror.Conflict("email already in use")
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.MinCost)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	u := &users.User{ID: uuid.New(), Email: req.Email, PasswordHash: string(hash), Name: req.Name, Role: req.Role}
	f.byID[u.ID] = u
	return u, nil
}

func (f *fakeUserService) GetByEmail(_ context.Context, email string) (*users.User, error) {
	for _, u := range f.byID {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, apperror.NotFound("user")
}

func (f *fakeUserService) GetByID(_ context.Context, id uuid.UUID) (*users.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, apperror.NotFound("user")
	}
	return u, nil
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

func newTestAuthService(t *testing.T) (*Service, *fakeUserService, *fakeTokenRepository) {
	t.Helper()
	usersSvc := newFakeUserService()
	repo := newFakeTokenRepository()
	issuer := NewTokenIssuer(config.JWTConfig{
		Secret:     "test-secret-at-least-32-characters!!",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	})
	return NewService(usersSvc, repo, issuer, noopTxManager{}), usersSvc, repo
}

func wantUnauthorized(t *testing.T, err error) {
	t.Helper()
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeUnauthorized {
		t.Fatalf("want UNAUTHORIZED, got %v", err)
	}
}

func TestRegisterCreatesUserRoleAndSession(t *testing.T) {
	svc, _, repo := newTestAuthService(t)

	sess, err := svc.Register(context.Background(), RegisterRequest{
		Email: "new@example.com", Password: "password-123", Name: "New",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if sess.User.Role != users.RoleUser {
		t.Fatalf("registered role must be %q, got %q", users.RoleUser, sess.User.Role)
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
	svc, usersSvc, _ := newTestAuthService(t)
	usersSvc.add(t, "a@example.com", "correct-password", users.RoleUser)

	_, err := svc.Login(context.Background(), LoginRequest{Email: "a@example.com", Password: "wrong-password"})
	wantUnauthorized(t, err)

	_, err = svc.Login(context.Background(), LoginRequest{Email: "missing@example.com", Password: "whatever-123"})
	wantUnauthorized(t, err)
}

func TestLoginSucceeds(t *testing.T) {
	svc, usersSvc, _ := newTestAuthService(t)
	u := usersSvc.add(t, "a@example.com", "correct-password", users.RoleAdmin)

	sess, err := svc.Login(context.Background(), LoginRequest{Email: "a@example.com", Password: "correct-password"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.User.ID != u.ID {
		t.Fatal("session user mismatch")
	}
}

func TestRefreshRotatesWithinFamily(t *testing.T) {
	svc, usersSvc, repo := newTestAuthService(t)
	usersSvc.add(t, "a@example.com", "correct-password", users.RoleUser)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Email: "a@example.com", Password: "correct-password"})
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

func TestRefreshReuseRevokesFamily(t *testing.T) {
	svc, usersSvc, repo := newTestAuthService(t)
	usersSvc.add(t, "a@example.com", "correct-password", users.RoleUser)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Email: "a@example.com", Password: "correct-password"})
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
	usersSvc := newFakeUserService()
	repo := newFakeTokenRepository()
	issuer := NewTokenIssuer(config.JWTConfig{
		Secret:     "test-secret-at-least-32-characters!!",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	})
	svc := NewService(usersSvc, &staleReadRepository{repo}, issuer, noopTxManager{})
	usersSvc.add(t, "a@example.com", "correct-password", users.RoleUser)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Email: "a@example.com", Password: "correct-password"})
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
	svc, usersSvc, _ := newTestAuthService(t)
	usersSvc.add(t, "a@example.com", "correct-password", users.RoleUser)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Email: "a@example.com", Password: "correct-password"})
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
	svc, usersSvc, repo := newTestAuthService(t)
	usersSvc.add(t, "a@example.com", "correct-password", users.RoleUser)
	ctx := context.Background()

	sess, err := svc.Login(ctx, LoginRequest{Email: "a@example.com", Password: "correct-password"})
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
