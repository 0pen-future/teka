package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/token"
	"teka/apps/api/internal/shared/validation"
)

// dummyBcryptHash is compared against on every login failure path so the
// response latency does not reveal whether the phone exists, the account is
// disabled, or the password was wrong.
const dummyBcryptHash = "$2a$12$000000000000000000000uGyHY.Pw6X8O0nMYcMHM1v1EYkg/aG6i"

// resetDMTimeout bounds the post-commit Zalo lookup+send for a forgot-password
// notification, the same guardrail invitations.dmTimeout applies to invite
// delivery — the request has already succeeded regardless of how this call
// turns out, so it must never run unbounded.
const resetDMTimeout = 10 * time.Second

// AccountService is the slice of the teachers feature this service consumes
// (consumer-defined interface; implemented by *teachers.Service). Account
// creation is invite-only now and lives on teachers.Service (CreateInCenter,
// Reactivate) — auth has no write path onto user_accounts/teachers beyond
// Disable and SetPassword.
type AccountService interface {
	GetByPhone(ctx context.Context, phone string) (*teachers.Profile, error)
	GetByID(ctx context.Context, id uuid.UUID) (*teachers.Profile, error)
	TouchLastLogin(ctx context.Context, id uuid.UUID) error
	// Disable flips the account to disabled status. Combined with
	// RevokeAllForUser below, this is the AccountDisabler facade centers
	// consumes to offboard a removed member.
	Disable(ctx context.Context, id uuid.UUID) error
	// SetPassword rewrites a currently-active account's password hash;
	// ResetPassword's write path onto user_accounts. ErrNotFound (the raw
	// teachers sentinel, not an apperror) means the account is not active —
	// ResetPassword folds that into its generic anti-enumeration rejection.
	SetPassword(ctx context.Context, accountID uuid.UUID, password string) error
}

// OwnerResolver is the consumer-defined seam into the centers feature:
// resolving the current center owner of a teacher, and whether that teacher
// IS the owner. Implemented by *centers.Service. ForgotPassword uses it to
// exclude center owners from self-service reset (owners recover via operator
// CLI only) and to anchor the reset-link DM on the owner's linked Zalo.
type OwnerResolver interface {
	// CenterOwner returns the owner teacher id of the teacher's current
	// center, and whether the teacher IS that owner.
	CenterOwner(ctx context.Context, teacherID uuid.UUID) (ownerID uuid.UUID, isOwner bool, err error)
}

// ResetDMSender is the consumer-defined seam into the Zalo feature — the same
// adapter shape invitations.ZaloSender uses. Implemented by *zalo.Service via
// its LookupPhone adapter.
type ResetDMSender interface {
	// LookupPhone reports the member's Zalo UID from the owner's friend list;
	// ok=false when the phone isn't a friend. err is zalo.ErrNotLinked when
	// the owner has no live Zalo session.
	LookupPhone(ctx context.Context, teacherID uuid.UUID, phone string) (uid string, ok bool, err error)
	SendDM(ctx context.Context, teacherID uuid.UUID, toUID, text string) (string, error)
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
	accounts      AccountService
	repo          Repository
	issuer        *TokenIssuer
	tx            database.TxManager
	owners        OwnerResolver
	dmSender      ResetDMSender
	resetTTL      time.Duration
	resetCooldown time.Duration
	publicBaseURL string
	now           func() time.Time
}

// NewService builds the auth service. owners and dmSender are constructor
// parameters, not setters: by the time router.go builds auth, centers.Service
// and zaloSvc already exist (auth is wired after centers), so there is no
// construction cycle to break here, unlike the auth<->teachers/centers
// AccountDisabler/TokenRevoker seams below. cfg supplies the reset TTL and
// cooldown; publicBaseURL is cfg.Statements.PublicBaseURL, reused rather than
// a new onboarding-specific env var (reset and invite links share one base).
func NewService(accounts AccountService, repo Repository, issuer *TokenIssuer, tx database.TxManager, owners OwnerResolver, dmSender ResetDMSender, cfg config.OnboardingConfig, publicBaseURL string) *Service {
	return &Service{
		accounts:      accounts,
		repo:          repo,
		issuer:        issuer,
		tx:            tx,
		owners:        owners,
		dmSender:      dmSender,
		resetTTL:      cfg.ResetTTL,
		resetCooldown: cfg.ResetCooldown,
		publicBaseURL: publicBaseURL,
		now:           time.Now,
	}
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

// Disable is the AccountDisabler facade centers.Service consumes to offboard
// a removed member: flip the account to disabled and revoke every refresh
// token it holds, atomically. Wrapped in its own WithinTx so it commits
// atomically whether called standalone or nested inside a caller's own
// transaction (WithinTx joins an already-open ambient transaction).
func (s *Service) Disable(ctx context.Context, accountID uuid.UUID) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.accounts.Disable(ctx, accountID); err != nil {
			return err
		}
		return s.repo.RevokeAllForUser(ctx, accountID, s.now())
	})
}

// RevokeAllForUser revokes every live refresh token for an account. Exposed
// as the teachers.TokenRevoker facade: teachers.Service.Reactivate calls this
// so a re-invited, previously-disabled account cannot be resumed from an old
// still-valid refresh token.
func (s *Service) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	return s.repo.RevokeAllForUser(ctx, userID, s.now())
}

// errResetRejected is the single anti-enumeration outcome ResetPassword
// returns for every rejection branch: unknown/used/expired/superseded token,
// or a token whose account is no longer active. Reused as one package-level
// value (not rebuilt per call) so every branch's response body is
// byte-for-byte identical.
var errResetRejected = apperror.BadRequest("unable to reset password")

// ForgotPassword is always a no-op from the caller's perspective: it never
// returns a business-rejection error, only a genuine (internal) failure. Side
// effects — minting a 48h reset token and attempting a best-effort Zalo DM to
// the center owner — happen only when the phone matches an active account
// that is a center MEMBER, never the owner (owners recover via operator CLI
// only). An unknown phone, a disabled account, and an owner's own phone are
// all indistinguishable from a member mid-cooldown or a successful send: the
// handler always renders the same generic response regardless of what
// happened here.
func (s *Service) ForgotPassword(ctx context.Context, req ForgotPasswordRequest) error {
	phone := validation.NormalizePhone(req.Phone)

	p, err := s.accounts.GetByPhone(ctx, phone)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == apperror.CodeNotFound {
			return nil
		}
		return err
	}
	if p.Account.Status != teachers.StatusActive {
		return nil
	}

	ownerID, isOwner, err := s.owners.CenterOwner(ctx, p.Account.ID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == apperror.CodeNotFound {
			return nil
		}
		return err
	}
	if isOwner {
		return nil
	}

	now := s.now()
	latest, err := s.repo.LatestResetToken(ctx, p.Account.ID)
	if err != nil && !errors.Is(err, ErrResetTokenNotFound) {
		return apperror.Internal(err)
	}
	if err == nil && now.Sub(latest.CreatedAt) < s.resetCooldown {
		return nil
	}

	plaintext, hash, err := token.New()
	if err != nil {
		return apperror.Internal(fmt.Errorf("mint reset token: %w", err))
	}
	rt := &PasswordResetToken{
		UserID:    p.Account.ID,
		TokenHash: hash,
		ExpiresAt: now.Add(s.resetTTL),
		CreatedAt: now,
	}
	txErr := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// Supersede any live token in the same transaction as the insert, or
		// uq_password_reset_active rejects this legitimate re-request.
		if err := s.repo.SupersedeResetTokens(ctx, p.Account.ID, now); err != nil {
			return err
		}
		return s.repo.CreateResetToken(ctx, rt)
	})
	if txErr != nil {
		return apperror.Internal(txErr)
	}

	// Best-effort DM strictly after the token is committed — delivery failure
	// must never invalidate an otherwise-valid token.
	s.attemptResetDM(ctx, ownerID, p.Account.Phone, s.buildResetLink(plaintext))
	return nil
}

// buildResetLink joins the configured public base URL with the reset route
// and plaintext token.
func (s *Service) buildResetLink(plaintext string) string {
	return strings.TrimRight(s.publicBaseURL, "/") + "/reset-password/" + plaintext
}

// attemptResetDM runs the best-effort Zalo delivery strictly after the reset
// token is already committed, under a bounded timeout detached from the
// request's own cancellation — a client disconnect must not race the DM
// against a canceled context. Unlike invitations.attemptDM this reports
// nothing back to the caller (log only): ForgotPassword's response never
// varies with delivery outcome.
func (s *Service) attemptResetDM(ctx context.Context, ownerID uuid.UUID, phone, link string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resetDMTimeout)
	defer cancel()

	uid, ok, err := s.dmSender.LookupPhone(ctx, ownerID, phone)
	if errors.Is(err, zalo.ErrNotLinked) || (err == nil && !ok) {
		return
	}
	if err != nil {
		slog.Warn("password reset dm lookup failed", "error", err)
		return
	}

	text := fmt.Sprintf("Yêu cầu đặt lại mật khẩu Teka. Bấm vào liên kết để đặt mật khẩu mới: %s", link)
	if _, err := s.dmSender.SendDM(ctx, ownerID, uid, text); err != nil {
		slog.Warn("password reset dm send failed", "error", err)
	}
}

// ResetPassword redeems a password-reset token: sets a new password and
// revokes every refresh token the account holds, so any session opened before
// the reset dies immediately. Every rejection reason — unknown/used/expired/
// superseded token, or a token whose account is no longer active — collapses
// to the same byte-identical errResetRejected. The whole flow runs in one
// WithinTx with a SELECT ... FOR UPDATE lock on the token row, so two
// concurrent redemptions of the same token serialize and only one can flip it
// to used.
func (s *Service) ResetPassword(ctx context.Context, req ResetPasswordRequest) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		t, err := s.repo.ConsumeResetTokenForUpdate(ctx, token.Hash(req.Token))
		if err != nil {
			if errors.Is(err, ErrResetTokenNotFound) {
				return errResetRejected
			}
			return apperror.Internal(err)
		}
		if !t.Live(s.now()) {
			return errResetRejected
		}

		if err := s.accounts.SetPassword(ctx, t.UserID, req.Password); err != nil {
			if errors.Is(err, teachers.ErrNotFound) {
				return errResetRejected
			}
			return err
		}
		if err := s.repo.MarkResetTokenUsed(ctx, t.ID, s.now()); err != nil {
			return apperror.Internal(err)
		}
		if err := s.repo.RevokeAllForUser(ctx, t.UserID, s.now()); err != nil {
			return apperror.Internal(err)
		}
		return nil
	})
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
