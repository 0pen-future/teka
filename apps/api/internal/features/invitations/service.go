package invitations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/token"
	"teka/apps/api/internal/shared/validation"
)

// dmTimeout bounds the post-commit Zalo lookup+send. MatchFriends alone can
// take seconds against an unofficial endpoint; the invitation is already
// valid regardless of how this call turns out, so it must never be allowed
// to run unbounded.
const dmTimeout = 10 * time.Second

// DM status values reported in CreateResponse.DMStatus. None of them is an
// HTTP error — a create always succeeds once the row is committed.
const (
	DMStatusSkipped = "skipped"
	DMStatusFailed  = "failed"
	DMStatusSent    = "sent"
)

// ZaloSender is the consumer-defined seam into the Zalo feature: resolving an
// invitee's UID from the owner's friend list and sending the invite DM.
// Implemented by *zalo.Service via its LookupPhone adapter.
type ZaloSender interface {
	// LookupPhone reports the invitee's Zalo UID from the owner's friend
	// list; ok=false when the phone isn't a friend. err is zalo.ErrNotLinked
	// when the owner has no live Zalo session.
	LookupPhone(ctx context.Context, teacherID uuid.UUID, phone string) (uid string, ok bool, err error)
	SendDM(ctx context.Context, teacherID uuid.UUID, toUID, text string) (string, error)
}

// AccountOnboarder is the slice of account lifecycle Accept consumes to
// create or resume a teacher account (consumer-defined interface; implemented
// by *teachers.Service, which already owns the write path onto
// user_accounts/teachers and the bcrypt cost).
type AccountOnboarder interface {
	// CreateInCenter creates account+teacher rows (role teachers, status
	// active) inside the ambient tx, with teachers.center_id already pointing
	// at centerID. Opening the center_members row is Accept's own job via
	// MembershipOpener.OpenMembership, once this call returns.
	CreateInCenter(ctx context.Context, phone, fullName, password string, centerID uuid.UUID) (uuid.UUID, error)
	// Reactivate sets password+name, flips status to active, and revokes
	// every refresh token the account still holds. Returns
	// teachers.ErrNotDisabled when the account is not currently disabled — a
	// defensive check Accept never expects to hit, since it only calls this
	// after confirming disabled status itself.
	Reactivate(ctx context.Context, accountID uuid.UUID, fullName, password string) error
	// FindByPhone reports the account for a phone, or teachers.ErrNotFound
	// (the raw sentinel, not an apperror) for the expected new-phone case.
	FindByPhone(ctx context.Context, phone string) (teachers.Account, error)
}

// MembershipOpener is the slice of center membership Accept consumes
// (consumer-defined interface; implemented by *centers.Service, over the
// existing close-before-open tx helpers and WasEverMember).
type MembershipOpener interface {
	OpenMembership(ctx context.Context, teacherID, centerID uuid.UUID) error
	// SwitchTeacherCenter moves teachers.center_id to centerID; only the
	// reactivate branch calls this — CreateInCenter already points a new
	// account's center_id at the inviting center.
	SwitchTeacherCenter(ctx context.Context, teacherID, centerID uuid.UUID) error
	// WasEverMember gates the disabled->active branch on real prior
	// membership in the inviting center, instead of trusting the token alone.
	WasEverMember(ctx context.Context, teacherID, centerID uuid.UUID) (bool, error)
}

// Service owns the owner-only invitation lifecycle (create, list, revoke) and
// the public accept flow (preview, accept).
type Service struct {
	repo          Repository
	zalo          ZaloSender
	onboarder     AccountOnboarder
	opener        MembershipOpener
	tx            database.TxManager
	inviteTTL     time.Duration
	publicBaseURL string
	// bus receives MemberJoined events for the audit trail. Nil is a
	// supported state — publish goes through the nil-safe helper below.
	bus events.Bus
	now func() time.Time
}

// NewService builds the invitations service. cfg supplies the invite TTL;
// publicBaseURL is cfg.Statements.PublicBaseURL, reused rather than a new
// onboarding-specific env var (invite and statement links share one base).
// onboarder and opener are constructor parameters, not setters: by the time
// router.go builds invitations, teachers.Service and centers.Service already
// exist (invitations is wired last), so there is no construction cycle to
// break here, unlike the auth<->teachers/centers seams.
func NewService(repo Repository, zaloSender ZaloSender, onboarder AccountOnboarder, opener MembershipOpener, tx database.TxManager, cfg config.OnboardingConfig, publicBaseURL string, bus events.Bus) *Service {
	return &Service{
		repo:          repo,
		zalo:          zaloSender,
		onboarder:     onboarder,
		opener:        opener,
		tx:            tx,
		inviteTTL:     cfg.InviteTTL,
		publicBaseURL: publicBaseURL,
		bus:           bus,
		now:           time.Now,
	}
}

// publish emits e when a bus is wired; a nil bus makes every publish a no-op.
func (s *Service) publish(e events.Event) {
	if s.bus != nil {
		s.bus.Publish(e)
	}
}

// requireOwner gates every invitation write: members do not manage
// onboarding, only the center owner does.
func requireOwner(sc authctx.Scope) error {
	if !sc.IsOwner {
		return apperror.Forbidden("only the center owner can manage invitations")
	}
	return nil
}

// Create makes a new pending invitation for a phone in the caller's center,
// superseding (revoking) any pending invite already outstanding for that
// phone. The response — including the copy-link — is built and returned
// before any Zalo work: DM delivery is best-effort and must never delay or
// fail the create. The response is identical whether or not the phone
// belongs to an existing account (anti-enumeration): the service never looks
// that up.
func (s *Service) Create(ctx context.Context, sc authctx.Scope, req CreateRequest) (*CreateResponse, error) {
	if err := requireOwner(sc); err != nil {
		return nil, err
	}
	phone := validation.NormalizePhone(req.Phone)

	plaintext, hash, err := token.New()
	if err != nil {
		return nil, apperror.Internal(fmt.Errorf("mint invitation token: %w", err))
	}
	inv := &Invitation{
		ID:        id.New(),
		CenterID:  sc.CenterID,
		Phone:     phone,
		TokenHash: hash,
		Status:    StatusPending,
		ExpiresAt: s.now().Add(s.inviteTTL),
	}

	txErr := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.RevokePendingForPhone(ctx, sc.CenterID, phone); err != nil {
			return err
		}
		return s.repo.Create(ctx, inv)
	})
	switch {
	case errors.Is(txErr, ErrPendingExists):
		// A concurrent request's insert won the (center, phone) slot between
		// this request's revoke and its own insert. Rather than surface that
		// as an error (F15b), reload the survivor and rotate its token hash
		// to the one this request just minted: the row identity stays
		// exactly one pending invite for the phone (no duplicate created),
		// while this response still carries a link whose plaintext this
		// caller actually holds.
		existing, gerr := s.repo.GetPendingByPhone(ctx, sc.CenterID, phone)
		if gerr != nil {
			return nil, apperror.Internal(fmt.Errorf("reload invitation after pending-slot race: %w", gerr))
		}
		if err := s.repo.SetTokenHash(ctx, existing.ID, hash); err != nil {
			return nil, apperror.Internal(fmt.Errorf("rotate invitation token after pending-slot race: %w", err))
		}
		inv = existing
	case txErr != nil:
		return nil, apperror.Internal(txErr)
	}

	link := s.buildLink(plaintext)
	resp := &CreateResponse{
		ID:        inv.ID,
		Phone:     inv.Phone,
		ExpiresAt: inv.ExpiresAt,
		Link:      link,
	}
	resp.DMStatus = s.attemptDM(ctx, sc.TeacherID, phone, link)
	return resp, nil
}

// buildLink joins the configured public base URL with the accept route and
// plaintext token. The token travels in the URL only here, for this
// copy/paste link — Phase 3's actual accept POST takes it in the body.
func (s *Service) buildLink(plaintext string) string {
	return strings.TrimRight(s.publicBaseURL, "/") + "/invite/" + plaintext
}

// attemptDM runs the best-effort Zalo delivery strictly after the invitation
// is already committed, under a bounded timeout detached from the request's
// own cancellation — a client disconnect must not race the DM against a
// canceled context and misreport it as failed.
func (s *Service) attemptDM(ctx context.Context, ownerID uuid.UUID, phone, link string) string {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dmTimeout)
	defer cancel()

	uid, ok, err := s.zalo.LookupPhone(ctx, ownerID, phone)
	if errors.Is(err, zalo.ErrNotLinked) || (err == nil && !ok) {
		return DMStatusSkipped
	}
	if err != nil {
		return DMStatusFailed
	}

	text := fmt.Sprintf("Bạn được mời tham gia trung tâm trên Teka. Bấm vào liên kết để tạo tài khoản: %s", link)
	if _, err := s.zalo.SendDM(ctx, ownerID, uid, text); err != nil {
		return DMStatusFailed
	}
	return DMStatusSent
}

// List returns every invitation in the caller's center, pending first, with
// the expired status derived at read time.
func (s *Service) List(ctx context.Context, sc authctx.Scope) ([]InvitationResponse, error) {
	if err := requireOwner(sc); err != nil {
		return nil, err
	}
	rows, err := s.repo.List(ctx, sc.CenterID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	now := s.now()
	out := make([]InvitationResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, FromModel(r, now))
	}
	return out, nil
}

// Revoke cancels a pending invitation. It is idempotent on an
// already-revoked (or otherwise non-pending) invite, and reports NotFound
// for an id belonging to another center — the same denial a missing id gets,
// so cross-center existence is never distinguishable.
func (s *Service) Revoke(ctx context.Context, sc authctx.Scope, invID uuid.UUID) error {
	if err := requireOwner(sc); err != nil {
		return err
	}
	if err := s.repo.Revoke(ctx, sc.CenterID, invID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperror.NotFound("invitation")
		}
		return apperror.Internal(err)
	}
	return nil
}

// Preview is the public, pre-authentication read an invitee sees before
// accepting: the center name and a masked phone, so they can recognize the
// invite as theirs. Anything but a live pending invitation — unknown token,
// already accepted/revoked, or past its derived expiry — answers the same
// generic 404 (anti-enumeration): a prober can never learn a token used to
// exist versus never existed.
func (s *Service) Preview(ctx context.Context, req PreviewRequest) (*PreviewResponse, error) {
	inv, err := s.repo.GetByTokenHash(ctx, token.Hash(req.Token))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apperror.NotFound("invitation")
		}
		return nil, apperror.Internal(err)
	}
	if inv.Status != StatusPending || inv.Expired(s.now()) {
		return nil, apperror.NotFound("invitation")
	}
	centerName, err := s.repo.GetCenterName(ctx, inv.CenterID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return &PreviewResponse{CenterName: centerName, PhoneMasked: maskPhone(inv.Phone)}, nil
}

// Accept redeems a pending invitation: a phone with no account creates one
// (AccountOnboarder.CreateInCenter); a disabled account that was ever a
// member of the inviting center (MembershipOpener.WasEverMember) reactivates
// into it instead. Every other outcome — bad/used/expired/revoked token, an
// active account, or a disabled account that never belonged to this center —
// collapses to the same byte-identical errAcceptRejected: the caller can
// never distinguish "no such token" from "right token, wrong account state".
// The whole flow runs in one WithinTx with a SELECT ... FOR UPDATE lock on
// the invitation row, so two concurrent accepts of the same token serialize
// and only one can flip pending->accepted. A successful accept publishes
// MemberJoined strictly after the transaction commits — a rolled-back
// membership must never reach the audit trail.
func (s *Service) Accept(ctx context.Context, req AcceptRequest, meta ClientMeta) error {
	var joined MemberJoined
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		inv, err := s.repo.GetByTokenHashForUpdate(ctx, token.Hash(req.Token))
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return errAcceptRejected
			}
			return apperror.Internal(err)
		}
		if inv.Status != StatusPending || inv.Expired(s.now()) {
			return errAcceptRejected
		}

		joined = MemberJoined{
			CenterID:     inv.CenterID,
			InvitationID: inv.ID,
			IP:           meta.IP,
			UserAgent:    meta.UserAgent,
		}

		acct, err := s.onboarder.FindByPhone(ctx, inv.Phone)
		switch {
		case errors.Is(err, teachers.ErrNotFound):
			accountID, cerr := s.onboarder.CreateInCenter(ctx, inv.Phone, req.FullName, req.Password, inv.CenterID)
			if cerr != nil {
				return cerr
			}
			if oerr := s.opener.OpenMembership(ctx, accountID, inv.CenterID); oerr != nil {
				return oerr
			}
			joined.UserID = accountID
		case err != nil:
			return apperror.Internal(err)
		case acct.Status == teachers.StatusActive:
			return errAcceptRejected
		default:
			// Neither active nor "no account" — a disabled account is the
			// only remaining status, and reactivation is gated on real prior
			// membership in this exact center, not on the token alone.
			ok, werr := s.opener.WasEverMember(ctx, acct.ID, inv.CenterID)
			if werr != nil {
				return apperror.Internal(werr)
			}
			if !ok {
				return errAcceptRejected
			}
			if rerr := s.onboarder.Reactivate(ctx, acct.ID, req.FullName, req.Password); rerr != nil {
				if errors.Is(rerr, teachers.ErrNotDisabled) {
					// Raced concurrently active between FindByPhone and here.
					return errAcceptRejected
				}
				return rerr
			}
			if oerr := s.opener.OpenMembership(ctx, acct.ID, inv.CenterID); oerr != nil {
				return oerr
			}
			if serr := s.opener.SwitchTeacherCenter(ctx, acct.ID, inv.CenterID); serr != nil {
				return serr
			}
			joined.UserID = acct.ID
		}

		if merr := s.repo.MarkAccepted(ctx, inv.ID, s.now()); merr != nil {
			return apperror.Internal(merr)
		}
		return nil
	})
	if err != nil {
		return err
	}
	joined.OccurredAt = s.now()
	s.publish(joined)
	return nil
}
