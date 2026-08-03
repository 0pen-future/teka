package users

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
)

// bcryptCost is fixed at 12 per the security baseline.
const bcryptCost = 12

// Principal aliases the shared authenticated-caller type.
type Principal = authctx.Principal

// Service implements user business logic over the repository interface.
type Service struct {
	repo Repository
}

// NewService builds the users service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create registers a new user with a bcrypt-hashed password. Role defaults
// to "user" when empty.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*User, error) {
	// The binding max=72 counts runes but bcrypt rejects inputs over 72
	// BYTES, so a long multibyte password passes validation and would blow
	// up here as a 500 without this check.
	if len(req.Password) > 72 {
		return nil, apperror.Invalid("validation failed",
			map[string]string{"password": "must be at most 72 bytes"})
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	role := req.Role
	if role == "" {
		role = RoleUser
	}
	u := &User{Email: req.Email, PasswordHash: string(hash), Name: req.Name, Role: role}
	if err := s.repo.Create(ctx, u); err != nil {
		if errors.Is(err, ErrDuplicateEmail) {
			return nil, apperror.Conflict("email already in use")
		}
		return nil, apperror.Internal(err)
	}
	return u, nil
}

// GetByEmail looks a user up by email with no caller authorization — it
// exists for the auth feature's credential check and must not back an
// endpoint directly.
func (s *Service) GetByEmail(ctx context.Context, email string) (*User, error) {
	u, err := s.repo.GetByEmail(ctx, email)
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.NotFound("user")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return u, nil
}

// GetByID looks a user up by id with no caller authorization — for internal
// service-to-service use (e.g. auth "me").
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.NotFound("user")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return u, nil
}

// Get returns a user readable by the caller: admins read anyone, users read
// themselves.
func (s *Service) Get(ctx context.Context, caller Principal, id uuid.UUID) (*User, error) {
	if !caller.IsAdmin() && caller.UserID != id {
		return nil, apperror.Forbidden("cannot access another user")
	}
	u, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.NotFound("user")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return u, nil
}

// List returns a page of users; admin-only (enforced in routes, re-checked
// here as defense in depth).
func (s *Service) List(ctx context.Context, caller Principal, f ListFilter, p pagination.Params) ([]User, int64, error) {
	if !caller.IsAdmin() {
		return nil, 0, apperror.Forbidden("admin only")
	}
	rows, total, err := s.repo.List(ctx, f, p)
	if err != nil {
		return nil, 0, apperror.Internal(err)
	}
	return rows, total, nil
}

// Update applies a partial update. Admins update anyone including role;
// users update only themselves and never their role.
func (s *Service) Update(ctx context.Context, caller Principal, id uuid.UUID, req UpdateRequest) (*User, error) {
	if !caller.IsAdmin() {
		if caller.UserID != id {
			return nil, apperror.Forbidden("cannot modify another user")
		}
		if req.Role != nil {
			return nil, apperror.Forbidden("cannot change own role")
		}
	}

	u, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.NotFound("user")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}

	if req.Name != nil {
		u.Name = *req.Name
	}
	if req.Role != nil {
		u.Role = *req.Role
	}
	if err := s.repo.Update(ctx, u); err != nil {
		return nil, apperror.Internal(err)
	}
	return u, nil
}

// Delete soft-deletes a user; admin-only, and admins cannot delete themselves
// (avoids locking the last admin out).
func (s *Service) Delete(ctx context.Context, caller Principal, id uuid.UUID) error {
	if !caller.IsAdmin() {
		return apperror.Forbidden("admin only")
	}
	if caller.UserID == id {
		return apperror.Conflict("cannot delete your own account")
	}
	err := s.repo.SoftDelete(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("user")
	}
	if err != nil {
		return apperror.Internal(err)
	}
	return nil
}
