package users

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/pagination"
)

// fakeRepository is an in-memory Repository for service tests.
type fakeRepository struct {
	users map[uuid.UUID]*User
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{users: map[uuid.UUID]*User{}}
}

func (r *fakeRepository) Create(_ context.Context, u *User) error {
	for _, existing := range r.users {
		if existing.Email == u.Email {
			return ErrDuplicateEmail
		}
	}
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	cp := *u
	r.users[u.ID] = &cp
	return nil
}

func (r *fakeRepository) GetByID(_ context.Context, id uuid.UUID) (*User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (r *fakeRepository) GetByEmail(_ context.Context, email string) (*User, error) {
	for _, u := range r.users {
		if u.Email == email {
			cp := *u
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeRepository) List(_ context.Context, f ListFilter, _ pagination.Params) ([]User, int64, error) {
	var out []User
	for _, u := range r.users {
		if f.Role != "" && u.Role != f.Role {
			continue
		}
		out = append(out, *u)
	}
	return out, int64(len(out)), nil
}

func (r *fakeRepository) Update(_ context.Context, u *User) error {
	if _, ok := r.users[u.ID]; !ok {
		return ErrNotFound
	}
	cp := *u
	r.users[u.ID] = &cp
	return nil
}

func (r *fakeRepository) SoftDelete(_ context.Context, id uuid.UUID) error {
	if _, ok := r.users[id]; !ok {
		return ErrNotFound
	}
	delete(r.users, id)
	return nil
}

func newTestService(t *testing.T) (*Service, *fakeRepository) {
	t.Helper()
	repo := newFakeRepository()
	return NewService(repo), repo
}

func mustCreate(t *testing.T, svc *Service, email, role string) *User {
	t.Helper()
	u, err := svc.Create(context.Background(), CreateRequest{
		Email:    email,
		Password: "password-123",
		Name:     "Test User",
		Role:     role,
	})
	if err != nil {
		t.Fatalf("create %s: %v", email, err)
	}
	return u
}

func wantAppErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("want *apperror.AppError with code %s, got %v", code, err)
	}
	if appErr.Code != code {
		t.Fatalf("want code %s, got %s", code, appErr.Code)
	}
}

func TestCreateHashesPasswordAndDefaultsRole(t *testing.T) {
	svc, repo := newTestService(t)

	u := mustCreate(t, svc, "a@example.com", "")

	if u.Role != RoleUser {
		t.Fatalf("want default role %q, got %q", RoleUser, u.Role)
	}
	stored := repo.users[u.ID]
	if stored.PasswordHash == "password-123" {
		t.Fatal("password stored in plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("password-123")); err != nil {
		t.Fatalf("stored hash does not match password: %v", err)
	}
}

func TestCreateRejectsPasswordOver72Bytes(t *testing.T) {
	svc, _ := newTestService(t)

	// 40 runes but 120 bytes: passes the rune-counting binding tag yet
	// exceeds bcrypt's 72-byte limit — must be 422, not 500.
	multibyte := strings.Repeat("ế", 40)
	_, err := svc.Create(context.Background(), CreateRequest{
		Email: "vn@example.com", Password: multibyte, Name: "VN",
	})
	wantAppErrorCode(t, err, apperror.CodeValidation)
}

func TestCreateDuplicateEmailIsConflict(t *testing.T) {
	svc, _ := newTestService(t)
	mustCreate(t, svc, "a@example.com", "")

	_, err := svc.Create(context.Background(), CreateRequest{
		Email: "a@example.com", Password: "password-123", Name: "Other",
	})
	wantAppErrorCode(t, err, apperror.CodeConflict)
}

func TestGetAuthorization(t *testing.T) {
	svc, _ := newTestService(t)
	admin := mustCreate(t, svc, "admin@example.com", RoleAdmin)
	user := mustCreate(t, svc, "user@example.com", RoleUser)
	other := mustCreate(t, svc, "other@example.com", RoleUser)

	ctx := context.Background()
	adminP := Principal{UserID: admin.ID, Role: RoleAdmin}
	userP := Principal{UserID: user.ID, Role: RoleUser}

	if _, err := svc.Get(ctx, adminP, other.ID); err != nil {
		t.Fatalf("admin reading another user: %v", err)
	}
	if _, err := svc.Get(ctx, userP, user.ID); err != nil {
		t.Fatalf("user reading self: %v", err)
	}
	_, err := svc.Get(ctx, userP, other.ID)
	wantAppErrorCode(t, err, apperror.CodeForbidden)
}

func TestListIsAdminOnly(t *testing.T) {
	svc, _ := newTestService(t)
	user := mustCreate(t, svc, "user@example.com", RoleUser)

	_, _, err := svc.List(context.Background(), Principal{UserID: user.ID, Role: RoleUser}, ListFilter{}, pagination.Params{})
	wantAppErrorCode(t, err, apperror.CodeForbidden)
}

func TestUpdateRoleRules(t *testing.T) {
	svc, _ := newTestService(t)
	admin := mustCreate(t, svc, "admin@example.com", RoleAdmin)
	user := mustCreate(t, svc, "user@example.com", RoleUser)

	ctx := context.Background()
	adminRole := RoleAdmin
	newName := "Renamed"

	// A user renaming themselves is allowed.
	updated, err := svc.Update(ctx, Principal{UserID: user.ID, Role: RoleUser}, user.ID, UpdateRequest{Name: &newName})
	if err != nil {
		t.Fatalf("self rename: %v", err)
	}
	if updated.Name != newName {
		t.Fatalf("want name %q, got %q", newName, updated.Name)
	}

	// A user escalating their own role is forbidden.
	_, err = svc.Update(ctx, Principal{UserID: user.ID, Role: RoleUser}, user.ID, UpdateRequest{Role: &adminRole})
	wantAppErrorCode(t, err, apperror.CodeForbidden)

	// A user modifying someone else is forbidden.
	_, err = svc.Update(ctx, Principal{UserID: user.ID, Role: RoleUser}, admin.ID, UpdateRequest{Name: &newName})
	wantAppErrorCode(t, err, apperror.CodeForbidden)

	// An admin changing another user's role is allowed.
	updated, err = svc.Update(ctx, Principal{UserID: admin.ID, Role: RoleAdmin}, user.ID, UpdateRequest{Role: &adminRole})
	if err != nil {
		t.Fatalf("admin role change: %v", err)
	}
	if updated.Role != RoleAdmin {
		t.Fatalf("want role %q, got %q", RoleAdmin, updated.Role)
	}
}

func TestDeleteRules(t *testing.T) {
	svc, _ := newTestService(t)
	admin := mustCreate(t, svc, "admin@example.com", RoleAdmin)
	user := mustCreate(t, svc, "user@example.com", RoleUser)

	ctx := context.Background()
	adminP := Principal{UserID: admin.ID, Role: RoleAdmin}

	err := svc.Delete(ctx, Principal{UserID: user.ID, Role: RoleUser}, admin.ID)
	wantAppErrorCode(t, err, apperror.CodeForbidden)

	err = svc.Delete(ctx, adminP, admin.ID)
	wantAppErrorCode(t, err, apperror.CodeConflict)

	if err := svc.Delete(ctx, adminP, user.ID); err != nil {
		t.Fatalf("admin delete user: %v", err)
	}
	err = svc.Delete(ctx, adminP, user.ID)
	wantAppErrorCode(t, err, apperror.CodeNotFound)
}
