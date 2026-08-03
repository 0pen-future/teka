package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/users"
	"teka/apps/api/internal/shared/apperror"
)

// UserService is the slice of the users feature this service consumes
// (consumer-defined interface; implemented by *users.Service).
type UserService interface {
	Create(ctx context.Context, req users.CreateRequest) (*users.User, error)
	GetByEmail(ctx context.Context, email string) (*users.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*users.User, error)
}

// Session is the outcome of register/login/refresh: the user, a signed
// access token, and the refresh-token plaintext destined for the cookie.
type Session struct {
	User         *users.User
	AccessToken  string
	RefreshToken string
}

// Service implements the authentication flows.
type Service struct {
	usersSvc UserService
	repo     Repository
	issuer   *TokenIssuer
	tx       database.TxManager
	now      func() time.Time
}

// NewService builds the auth service.
func NewService(usersSvc UserService, repo Repository, issuer *TokenIssuer, tx database.TxManager) *Service {
	return &Service{usersSvc: usersSvc, repo: repo, issuer: issuer, tx: tx, now: time.Now}
}

// Register creates a user (always role "user") and opens a session; user and
// refresh token are created atomically.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*Session, error) {
	var sess *Session
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		u, err := s.usersSvc.Create(ctx, users.CreateRequest{
			Email:    req.Email,
			Password: req.Password,
			Name:     req.Name,
			Role:     users.RoleUser,
		})
		if err != nil {
			return err
		}
		sess, err = s.openSession(ctx, u)
		return err
	})
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// Login verifies credentials and opens a new session (new token family).
// Wrong email and wrong password are indistinguishable to the caller.
func (s *Service) Login(ctx context.Context, req LoginRequest) (*Session, error) {
	invalid := apperror.Unauthorized("invalid email or password")

	u, err := s.usersSvc.GetByEmail(ctx, req.Email)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == apperror.CodeNotFound {
			// Burn comparable time so the error does not leak which field
			// was wrong via response latency.
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$12$000000000000000000000uGyHY.Pw6X8O0nMYcMHM1v1EYkg/aG6i"), []byte(req.Password))
			return nil, invalid
		}
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		return nil, invalid
	}
	return s.openSession(ctx, u)
}

// Refresh rotates a refresh token: the presented token is revoked and a new
// one from the same family is issued. Presenting an already-rotated (revoked)
// token revokes the whole family — the token was either replayed by an
// attacker or leaked.
func (s *Service) Refresh(ctx context.Context, plaintext string) (*Session, error) {
	invalid := apperror.Unauthorized("invalid refresh token")
	now := s.now()

	t, err := s.repo.GetByHash(ctx, HashToken(plaintext))
	if errors.Is(err, ErrTokenNotFound) {
		return nil, invalid
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	// The revocations below run outside the rotation transaction on purpose:
	// they must persist even though the request itself fails.
	if t.Revoked() {
		if err := s.repo.RevokeFamily(ctx, t.FamilyID, now); err != nil {
			return nil, apperror.Internal(err)
		}
		return nil, invalid
	}
	if t.Expired(now) {
		return nil, invalid
	}
	u, err := s.usersSvc.GetByID(ctx, t.UserID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == apperror.CodeNotFound {
			return nil, invalid
		}
		// Transient failures (e.g. pool exhaustion) must surface as 500, not
		// masquerade as 401 — a 401 here logs nothing and logs the user out.
		return nil, err
	}

	var sess *Session
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Revoke(ctx, t.ID, now); err != nil {
			return err
		}
		var err error
		sess, err = s.issueSession(ctx, u, t.FamilyID)
		return err
	})
	// Losing the Revoke race means a concurrent request rotated this same
	// token — that is reuse, so the family dies. Outside the transaction so
	// the revocation persists despite the request failing.
	if errors.Is(err, ErrTokenAlreadyRevoked) {
		if rerr := s.repo.RevokeFamily(ctx, t.FamilyID, now); rerr != nil {
			return nil, apperror.Internal(rerr)
		}
		return nil, invalid
	}
	if err != nil {
		var appErr *apperror.AppError
		if !errors.As(err, &appErr) {
			return nil, apperror.Internal(err)
		}
		return nil, err
	}
	return sess, nil
}

// Logout revokes the presented token's whole family. Unknown tokens succeed
// silently — logout must be idempotent.
func (s *Service) Logout(ctx context.Context, plaintext string) error {
	if plaintext == "" {
		return nil
	}
	t, err := s.repo.GetByHash(ctx, HashToken(plaintext))
	if errors.Is(err, ErrTokenNotFound) {
		return nil
	}
	if err != nil {
		return apperror.Internal(err)
	}
	if err := s.repo.RevokeFamily(ctx, t.FamilyID, s.now()); err != nil {
		return apperror.Internal(err)
	}
	return nil
}

// Me returns the authenticated user's profile.
func (s *Service) Me(ctx context.Context, userID uuid.UUID) (*users.User, error) {
	return s.usersSvc.GetByID(ctx, userID)
}

// openSession starts a brand-new token family for u.
func (s *Service) openSession(ctx context.Context, u *users.User) (*Session, error) {
	return s.issueSession(ctx, u, uuid.New())
}

// issueSession signs an access token and stores a refresh token in familyID.
func (s *Service) issueSession(ctx context.Context, u *users.User, familyID uuid.UUID) (*Session, error) {
	access, err := s.issuer.IssueAccess(u.ID, u.Role)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	plaintext, hash, err := s.issuer.NewRefreshToken()
	if err != nil {
		return nil, apperror.Internal(err)
	}
	t := &RefreshToken{
		UserID:    u.ID,
		TokenHash: hash,
		FamilyID:  familyID,
		ExpiresAt: s.now().Add(s.issuer.RefreshTTL()),
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, apperror.Internal(err)
	}
	return &Session{User: u, AccessToken: access, RefreshToken: plaintext}, nil
}
