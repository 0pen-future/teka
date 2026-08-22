package teachers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/validation"
)

// bcryptCost is fixed at 12 per the security baseline.
const bcryptCost = 12

// TokenRevoker lets Reactivate invalidate every refresh token a previously
// disabled account still holds, so a reactivated account cannot resume from
// an old still-valid session (consumer-defined interface; implemented by
// *auth.Service). A setter, not a NewService parameter, because auth.Service
// itself depends on this service as its AccountService — a direct parameter
// would cycle (same pattern as attendance.SetReconciler).
type TokenRevoker interface {
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
}

// Service implements teacher identity business logic over the repository.
type Service struct {
	repo    Repository
	revoker TokenRevoker
	now     func() time.Time
}

// NewService builds the teachers service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// SetTokenRevoker wires the auth dependency after construction.
func (s *Service) SetTokenRevoker(r TokenRevoker) {
	s.revoker = r
}

// CreateInCenter registers a teacher directly into an existing center: one
// user_accounts row and one teachers row sharing a UUIDv7 id, on the ambient
// transaction. Unlike the old self-registration path, the center already
// exists (the inviting center), so there is no personal-center provisioning
// step and no deferred-FK ordering concern; opening the center_members row is
// the caller's job (invitations.Service, via centers.Service.OpenMembership)
// once this call returns. The unique phone index is the concurrency guard —
// no pre-check SELECT (TOCTOU).
func (s *Service) CreateInCenter(ctx context.Context, phone, fullName, password string, centerID uuid.UUID) (uuid.UUID, error) {
	// The binding max=72 counts runes but bcrypt rejects inputs over 72
	// BYTES, so a long multibyte password passes validation and would blow
	// up here as a 500 without this check.
	if len(password) > 72 {
		return uuid.Nil, apperror.Invalid("validation failed",
			map[string]string{"password": "must be at most 72 bytes"})
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return uuid.Nil, apperror.Internal(err)
	}
	hashStr := string(hash)

	accountID := id.New()
	acct := &Account{
		ID:           accountID,
		Role:         authctx.RoleTeacher,
		Phone:        validation.NormalizePhone(phone),
		PasswordHash: &hashStr,
		Status:       StatusActive,
	}
	t := &Teacher{ID: accountID, FullName: fullName, Timezone: DefaultTimezone, CenterID: centerID}

	if err := s.repo.CreateAccountWithProfile(ctx, acct, t); err != nil {
		if errors.Is(err, ErrDuplicatePhone) {
			return uuid.Nil, apperror.Conflict("phone already registered")
		}
		return uuid.Nil, apperror.Internal(err)
	}
	return accountID, nil
}

// FindByPhone looks an account up by phone for the invitations accept flow to
// decide new-phone versus existing-account handling. Unlike GetByPhone it
// returns the bare Account and the raw ErrNotFound sentinel (not wrapped in
// apperror) so the caller can tell "no such account" — the expected new-phone
// case — apart from a real failure with errors.Is.
func (s *Service) FindByPhone(ctx context.Context, phone string) (Account, error) {
	p, err := s.repo.GetByPhone(ctx, validation.NormalizePhone(phone))
	if err != nil {
		return Account{}, err
	}
	return p.Account, nil
}

// Reactivate resumes a disabled account for the invitations accept flow: a
// new password and display name, status flips back to active, and every
// refresh token the old session held is revoked so it cannot be resumed
// alongside the new one. ErrNotDisabled guards the invariant that the caller
// (invitations.Service) has already confirmed disabled status.
func (s *Service) Reactivate(ctx context.Context, accountID uuid.UUID, fullName, password string) error {
	if len(password) > 72 {
		return apperror.Invalid("validation failed",
			map[string]string{"password": "must be at most 72 bytes"})
	}
	p, err := s.repo.GetByID(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("teacher")
	}
	if err != nil {
		return apperror.Internal(err)
	}
	if p.Account.Status != StatusDisabled {
		return ErrNotDisabled
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return apperror.Internal(err)
	}
	now := s.now()
	if err := s.repo.ReactivateAccount(ctx, accountID, string(hash), now); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotDisabled
		}
		return apperror.Internal(err)
	}
	p.Teacher.FullName = fullName
	if err := s.repo.Update(ctx, &p.Teacher); err != nil {
		return apperror.Internal(err)
	}
	if s.revoker != nil {
		if err := s.revoker.RevokeAllForUser(ctx, accountID); err != nil {
			return err
		}
	}
	return nil
}

// SetPassword rewrites the password hash of a currently-active account; it
// satisfies auth.AccountService so auth.Service.ResetPassword (the only
// caller) can complete a password-reset redemption. ErrNotFound propagates
// unwrapped (not apperror) — auth.Service.ResetPassword folds it into its own
// generic anti-enumeration rejection rather than leaking "account not active"
// to an unauthenticated caller. Revoking the account's refresh tokens is
// auth's own job, not this method's, mirroring Disable/RevokeAllForUser.
func (s *Service) SetPassword(ctx context.Context, accountID uuid.UUID, password string) error {
	if len(password) > 72 {
		return apperror.Invalid("validation failed",
			map[string]string{"password": "must be at most 72 bytes"})
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return apperror.Internal(err)
	}
	if err := s.repo.SetPasswordHash(ctx, accountID, string(hash), s.now()); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return apperror.Internal(err)
	}
	return nil
}

// SetPasswordForRecovery rewrites accountID's password hash regardless of
// active/disabled status and leaves status untouched — the operator
// `reset-password` CLI's write path (a locked-out or disabled owner has no
// self-service route). It deliberately does not satisfy auth.AccountService:
// that interface's SetPassword is the active-only self-service reset path;
// the CLI calls this method directly. Revoking the account's refresh tokens
// is the caller's job, same division as SetPassword.
func (s *Service) SetPasswordForRecovery(ctx context.Context, accountID uuid.UUID, password string) error {
	if len(password) > 72 {
		return apperror.Invalid("validation failed",
			map[string]string{"password": "must be at most 72 bytes"})
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return apperror.Internal(err)
	}
	if err := s.repo.SetPasswordHashForRecovery(ctx, accountID, string(hash), s.now()); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return apperror.Internal(err)
	}
	return nil
}

// Disable flips a live account to disabled status; it satisfies
// auth.AccountService so auth.Service.Disable (the AccountDisabler facade
// centers consumes) can offboard a removed member. Token revocation is auth's
// own job, not this method's.
func (s *Service) Disable(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.SetStatus(ctx, id, StatusDisabled); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperror.NotFound("teacher")
		}
		return apperror.Internal(err)
	}
	return nil
}

// GetByPhone looks an account up by phone (either accepted input form) with
// no caller authorization — it exists for the auth feature's credential
// check and must not back an endpoint directly.
func (s *Service) GetByPhone(ctx context.Context, phone string) (*Profile, error) {
	p, err := s.repo.GetByPhone(ctx, validation.NormalizePhone(phone))
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.NotFound("teacher")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return p, nil
}

// GetByID looks an account up by id with no caller authorization — for
// internal service-to-service use (auth refresh, /me).
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Profile, error) {
	p, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.NotFound("teacher")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return p, nil
}

// UpdateProfile changes the teacher's display name and timezone — nothing
// else; the fields are mapped explicitly so the request body can never touch
// role, status, or phone.
func (s *Service) UpdateProfile(ctx context.Context, teacherID uuid.UUID, req UpdateProfileRequest) (*Profile, error) {
	// The column holds an IANA zone name and the stdlib owns that
	// vocabulary; "Local" loads but is machine-relative, not a zone name.
	if _, err := time.LoadLocation(req.Timezone); err != nil || req.Timezone == "Local" {
		return nil, apperror.Invalid("validation failed",
			map[string]string{"timezone": "must be a valid IANA timezone"})
	}
	p, err := s.repo.GetByID(ctx, teacherID)
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.NotFound("teacher")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	p.Teacher.FullName = req.FullName
	p.Teacher.Timezone = req.Timezone
	if err := s.repo.Update(ctx, &p.Teacher); err != nil {
		return nil, apperror.Internal(err)
	}
	return p, nil
}

// TouchLastLogin stamps user_accounts.last_login_at with the current time.
func (s *Service) TouchLastLogin(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.TouchLastLogin(ctx, id, s.now()); err != nil {
		return apperror.Internal(err)
	}
	return nil
}
