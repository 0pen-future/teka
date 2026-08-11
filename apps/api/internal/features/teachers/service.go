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

// CreateRequest carries the fields needed to register a teacher; validation
// of shape happens at the HTTP boundary (features/auth), invariants here.
type CreateRequest struct {
	Phone    string
	Password string
	FullName string
}

// CenterProvisioner supplies the personal center a brand-new teacher starts
// in (consumer-defined interface; implemented by *centers.Service). It is
// two calls, not one, because of foreign-key ordering inside the registration
// transaction: the centers row must exist before the teachers row (immediate
// FK on teachers.center_id, while centers.owner_id is deferred), and the
// membership row can only follow the teachers row.
type CenterProvisioner interface {
	CreatePersonalCenter(ctx context.Context, ownerID uuid.UUID, name string) (uuid.UUID, error)
	OpenMembership(ctx context.Context, teacherID, centerID uuid.UUID) error
}

// Service implements teacher identity business logic over the repository.
type Service struct {
	repo    Repository
	centers CenterProvisioner
	now     func() time.Time
}

// NewService builds the teachers service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// SetCenterProvisioner wires the centers dependency after construction — a
// NewService parameter would cycle: centers needs this service for its
// owner-phone lookup (same pattern as attendance.SetReconciler).
func (s *Service) SetCenterProvisioner(p CenterProvisioner) {
	s.centers = p
}

// CreateTeacher registers a teacher: their personal centers row, one
// user_accounts row and one teachers row sharing a UUIDv7 id, and the live
// center_members row — all on the ambient transaction, which is REQUIRED
// here: centers.owner_id is a deferred FK pointing at a teachers row that
// only exists later in the same transaction. The unique phone index is the
// concurrency guard — no pre-check SELECT (TOCTOU).
func (s *Service) CreateTeacher(ctx context.Context, req CreateRequest) (*Profile, error) {
	// The binding max=72 counts runes but bcrypt rejects inputs over 72
	// BYTES, so a long multibyte password passes validation and would blow
	// up here as a 500 without this check.
	if len(req.Password) > 72 {
		return nil, apperror.Invalid("validation failed",
			map[string]string{"password": "must be at most 72 bytes"})
	}
	if s.centers == nil {
		return nil, apperror.Internal(errors.New("teachers: center provisioner not wired"))
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	hashStr := string(hash)

	accountID := id.New()
	centerID, err := s.centers.CreatePersonalCenter(ctx, accountID, req.FullName)
	if err != nil {
		return nil, err
	}
	acct := &Account{
		ID:           accountID,
		Role:         authctx.RoleTeacher,
		Phone:        validation.NormalizePhone(req.Phone),
		PasswordHash: &hashStr,
		Status:       StatusActive,
	}
	t := &Teacher{ID: accountID, FullName: req.FullName, Timezone: DefaultTimezone, CenterID: centerID}

	if err := s.repo.CreateAccountWithProfile(ctx, acct, t); err != nil {
		if errors.Is(err, ErrDuplicatePhone) {
			return nil, apperror.Conflict("phone already registered")
		}
		return nil, apperror.Internal(err)
	}
	if err := s.centers.OpenMembership(ctx, accountID, centerID); err != nil {
		return nil, err
	}
	return &Profile{Account: *acct, Teacher: *t}, nil
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
