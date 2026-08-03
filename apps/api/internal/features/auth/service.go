package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
)

// dummyBcryptHash is compared against on every login failure path so the
// response latency does not reveal whether the phone exists, the account is
// disabled, or the password was wrong.
const dummyBcryptHash = "$2a$12$000000000000000000000uGyHY.Pw6X8O0nMYcMHM1v1EYkg/aG6i"

// AccountService is the slice of the teachers feature this service consumes
// (consumer-defined interface; implemented by *teachers.Service).
type AccountService interface {
	CreateTeacher(ctx context.Context, req teachers.CreateRequest) (*teachers.Profile, error)
	GetByPhone(ctx context.Context, phone string) (*teachers.Profile, error)
	GetByID(ctx context.Context, id uuid.UUID) (*teachers.Profile, error)
	TouchLastLogin(ctx context.Context, id uuid.UUID) error
}

// Session is the outcome of register/login/refresh: the teacher profile, a
// signed access token, and the refresh-token plaintext destined for the
// cookie.
type Session struct {
	Teacher      *teachers.Profile
	AccessToken  string
	RefreshToken string
}

// Service implements the authentication flows.
type Service struct {
	accounts AccountService
	repo     Repository
	issuer   *TokenIssuer
	tx       database.TxManager
	now      func() time.Time
}

// NewService builds the auth service.
func NewService(accounts AccountService, repo Repository, issuer *TokenIssuer, tx database.TxManager) *Service {
	return &Service{accounts: accounts, repo: repo, issuer: issuer, tx: tx, now: time.Now}
}

// Register creates a teacher (account + profile rows) and opens a session;
// all three inserts happen in one transaction.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*Session, error) {
	var sess *Session
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := s.accounts.CreateTeacher(ctx, teachers.CreateRequest{
			Phone:    req.Phone,
			Password: req.Password,
			FullName: req.FullName,
		})
		if err != nil {
			return err
		}
		sess, err = s.openSession(ctx, p)
		return err
	})
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// Login verifies credentials and opens a new session (new token family).
// Unknown phone, disabled account, passwordless account, and wrong password
// are indistinguishable to the caller — same message, comparable latency.
func (s *Service) Login(ctx context.Context, req LoginRequest) (*Session, error) {
	invalid := apperror.Unauthorized("invalid phone or password")

	p, err := s.accounts.GetByPhone(ctx, req.Phone)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == apperror.CodeNotFound {
			burnPassword(req.Password)
			return nil, invalid
		}
		return nil, err
	}
	if p.Account.Status != teachers.StatusActive {
		burnPassword(req.Password)
		return nil, invalid
	}
	if p.Account.PasswordHash == nil {
		burnPassword(req.Password)
		return nil, invalid
	}
	if bcrypt.CompareHashAndPassword([]byte(*p.Account.PasswordHash), []byte(req.Password)) != nil {
		return nil, invalid
	}
	if err := s.accounts.TouchLastLogin(ctx, p.Account.ID); err != nil {
		return nil, err
	}
	return s.openSession(ctx, p)
}

// burnPassword runs a bcrypt comparison against a fixed dummy hash so every
// login failure path costs roughly the same time regardless of why it failed.
func burnPassword(password string) {
	_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
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
	p, err := s.accounts.GetByID(ctx, t.UserID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == apperror.CodeNotFound {
			return nil, invalid
		}
		// Transient failures (e.g. pool exhaustion) must surface as 500, not
		// masquerade as 401 — a 401 here logs nothing and logs the user out.
		return nil, err
	}
	// A disabled account keeps its unexpired refresh tokens; they must stop
	// working the moment the account is disabled.
	if p.Account.Status != teachers.StatusActive {
		return nil, invalid
	}

	var sess *Session
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Revoke(ctx, t.ID, now); err != nil {
			return err
		}
		var err error
		sess, err = s.issueSession(ctx, p, t.FamilyID)
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

// openSession starts a brand-new token family for the teacher.
func (s *Service) openSession(ctx context.Context, p *teachers.Profile) (*Session, error) {
	return s.issueSession(ctx, p, uuid.New())
}

// issueSession signs an access token and stores a refresh token in familyID.
func (s *Service) issueSession(ctx context.Context, p *teachers.Profile, familyID uuid.UUID) (*Session, error) {
	access, err := s.issuer.IssueAccess(p.Account.ID, p.Account.Role)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	plaintext, hash, err := s.issuer.NewRefreshToken()
	if err != nil {
		return nil, apperror.Internal(err)
	}
	t := &RefreshToken{
		UserID:    p.Account.ID,
		TokenHash: hash,
		FamilyID:  familyID,
		ExpiresAt: s.now().Add(s.issuer.RefreshTTL()),
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, apperror.Internal(err)
	}
	return &Session{Teacher: p, AccessToken: access, RefreshToken: plaintext}, nil
}
