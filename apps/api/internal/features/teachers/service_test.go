package teachers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/validation"
)

// fakeRepository implements Repository in memory.
type fakeRepository struct {
	profiles map[uuid.UUID]*Profile
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{profiles: map[uuid.UUID]*Profile{}}
}

func (r *fakeRepository) GetByPhone(_ context.Context, phone string) (*Profile, error) {
	for _, p := range r.profiles {
		if p.Account.Phone == phone {
			cp := *p
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeRepository) GetByID(_ context.Context, id uuid.UUID) (*Profile, error) {
	p, ok := r.profiles[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *fakeRepository) CreateAccountWithProfile(_ context.Context, acct *Account, t *Teacher) error {
	for _, p := range r.profiles {
		if p.Account.Phone == acct.Phone {
			return ErrDuplicatePhone
		}
	}
	r.profiles[acct.ID] = &Profile{Account: *acct, Teacher: *t}
	return nil
}

func (r *fakeRepository) Update(_ context.Context, t *Teacher) error {
	p, ok := r.profiles[t.ID]
	if !ok {
		return ErrNotFound
	}
	p.Teacher = *t
	return nil
}

func (r *fakeRepository) TouchLastLogin(_ context.Context, id uuid.UUID, at time.Time) error {
	p, ok := r.profiles[id]
	if !ok {
		return ErrNotFound
	}
	stamp := at
	p.Account.LastLoginAt = &stamp
	return nil
}

func (r *fakeRepository) SetStatus(_ context.Context, id uuid.UUID, status string) error {
	p, ok := r.profiles[id]
	if !ok {
		return ErrNotFound
	}
	p.Account.Status = status
	return nil
}

func (r *fakeRepository) SetPasswordHash(_ context.Context, id uuid.UUID, passwordHash string, _ time.Time) error {
	p, ok := r.profiles[id]
	if !ok || p.Account.Status != StatusActive {
		return ErrNotFound
	}
	p.Account.PasswordHash = &passwordHash
	return nil
}

func (r *fakeRepository) SetPasswordHashForRecovery(_ context.Context, id uuid.UUID, passwordHash string, _ time.Time) error {
	p, ok := r.profiles[id]
	if !ok {
		return ErrNotFound
	}
	p.Account.PasswordHash = &passwordHash
	return nil
}

func (r *fakeRepository) ReactivateAccount(_ context.Context, id uuid.UUID, passwordHash string, _ time.Time) error {
	p, ok := r.profiles[id]
	if !ok || p.Account.Status != StatusDisabled {
		return ErrNotFound
	}
	p.Account.PasswordHash = &passwordHash
	p.Account.Status = StatusActive
	return nil
}

// fakeTokenRevoker implements TokenRevoker in memory, recording every account
// id Reactivate asked it to revoke.
type fakeTokenRevoker struct {
	revoked []uuid.UUID
	err     error
}

func (r *fakeTokenRevoker) RevokeAllForUser(_ context.Context, userID uuid.UUID) error {
	if r.err != nil {
		return r.err
	}
	r.revoked = append(r.revoked, userID)
	return nil
}

func seedProfile(repo *fakeRepository, phone string) *Profile {
	accountID := uuid.New()
	p := &Profile{
		Account: Account{ID: accountID, Phone: validation.NormalizePhone(phone), Status: StatusActive},
		Teacher: Teacher{ID: accountID, FullName: "Before", Timezone: DefaultTimezone},
	}
	repo.profiles[accountID] = p
	return p
}

func wantFieldError(t *testing.T, err error, field string) {
	t.Helper()
	appErr := apperror.From(err)
	if appErr.Code != apperror.CodeValidation || appErr.Fields[field] == "" {
		t.Fatalf("want VALIDATION_ERROR with %q field message, got %v", field, err)
	}
}

func TestCreateInCenterNormalizesAndPairsRows(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	centerID := uuid.New()

	accountID, err := svc.CreateInCenter(context.Background(), "0901234567", "Cô Lan", "password-123", centerID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	p := repo.profiles[accountID]
	if p.Account.Phone != "+84901234567" {
		t.Fatalf("phone must normalize to E.164, got %q", p.Account.Phone)
	}
	if p.Account.ID != p.Teacher.ID {
		t.Fatal("account and teacher must share one id")
	}
	if p.Account.PasswordHash == nil || *p.Account.PasswordHash == "password-123" {
		t.Fatal("password must be stored hashed")
	}
	if p.Teacher.CenterID != centerID {
		t.Fatalf("teacher must land in the given center, got %s", p.Teacher.CenterID)
	}
	if p.Account.Status != StatusActive {
		t.Fatalf("new account must be active, got %q", p.Account.Status)
	}

	_, err = svc.CreateInCenter(context.Background(), "+84901234567", "Trùng", "password-123", centerID)
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("duplicate phone must conflict, got %v", err)
	}
}

func TestCreateInCenterRejectsOver72BytePassword(t *testing.T) {
	svc := NewService(newFakeRepository())

	// 25 three-byte runes = 25 characters (passes binding max=72 runes) but 75
	// bytes (exceeds bcrypt's input limit).
	_, err := svc.CreateInCenter(context.Background(), "0901234567", "X", strings.Repeat("ầ", 25), uuid.New())
	wantFieldError(t, err, "password")
}

func TestFindByPhoneReturnsAccountOrRawNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	p := seedProfile(repo, "+84901234567")

	acct, err := svc.FindByPhone(context.Background(), "0901234567")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if acct.ID != p.Account.ID {
		t.Fatalf("want account %s, got %s", p.Account.ID, acct.ID)
	}

	_, err = svc.FindByPhone(context.Background(), "0909999999")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown phone must return the raw ErrNotFound sentinel, got %v", err)
	}
}

func TestReactivateFlipsStatusSetsPasswordAndRevokesTokens(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	revoker := &fakeTokenRevoker{}
	svc.SetTokenRevoker(revoker)
	p := seedProfile(repo, "+84901234567")
	p.Account.Status = StatusDisabled

	if err := svc.Reactivate(context.Background(), p.Account.ID, "New Name", "new-password"); err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	stored := repo.profiles[p.Account.ID]
	if stored.Account.Status != StatusActive {
		t.Fatalf("account must be active again, got %q", stored.Account.Status)
	}
	if stored.Account.PasswordHash == nil || *stored.Account.PasswordHash == "new-password" {
		t.Fatal("password must be stored hashed")
	}
	if stored.Teacher.FullName != "New Name" {
		t.Fatalf("full name must update, got %q", stored.Teacher.FullName)
	}
	if len(revoker.revoked) != 1 || revoker.revoked[0] != p.Account.ID {
		t.Fatalf("reactivate must revoke every token for the account, got %v", revoker.revoked)
	}
}

func TestReactivateRejectsAlreadyActiveAccount(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	p := seedProfile(repo, "+84901234567") // StatusActive by default

	err := svc.Reactivate(context.Background(), p.Account.ID, "New Name", "new-password")
	if !errors.Is(err, ErrNotDisabled) {
		t.Fatalf("want ErrNotDisabled, got %v", err)
	}
}

func TestDisableFlipsStatusToDisabled(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	p := seedProfile(repo, "+84901234567")

	if err := svc.Disable(context.Background(), p.Account.ID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if repo.profiles[p.Account.ID].Account.Status != StatusDisabled {
		t.Fatalf("account must be disabled, got %q", repo.profiles[p.Account.ID].Account.Status)
	}
}

func TestSetPasswordForRecoveryUpdatesDisabledAccountWithoutReactivating(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	p := seedProfile(repo, "+84901234567")
	p.Account.Status = StatusDisabled

	if err := svc.SetPasswordForRecovery(context.Background(), p.Account.ID, "new-password"); err != nil {
		t.Fatalf("set password for recovery: %v", err)
	}
	stored := repo.profiles[p.Account.ID]
	if stored.Account.Status != StatusDisabled {
		t.Fatalf("status must stay disabled, got %q", stored.Account.Status)
	}
	if stored.Account.PasswordHash == nil || *stored.Account.PasswordHash == "new-password" {
		t.Fatal("password must be stored hashed")
	}
}

func TestSetPasswordForRecoveryUpdatesActiveAccount(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	p := seedProfile(repo, "+84901234567") // StatusActive by default

	if err := svc.SetPasswordForRecovery(context.Background(), p.Account.ID, "new-password"); err != nil {
		t.Fatalf("set password for recovery: %v", err)
	}
	stored := repo.profiles[p.Account.ID]
	if stored.Account.PasswordHash == nil || *stored.Account.PasswordHash == "new-password" {
		t.Fatal("password must be stored hashed")
	}
}

func TestSetPasswordForRecoveryRejectsOver72BytePassword(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	p := seedProfile(repo, "+84901234567")

	err := svc.SetPasswordForRecovery(context.Background(), p.Account.ID, strings.Repeat("ầ", 25))
	wantFieldError(t, err, "password")
}

func TestSetPasswordForRecoveryUnknownAccount(t *testing.T) {
	svc := NewService(newFakeRepository())

	err := svc.SetPasswordForRecovery(context.Background(), uuid.New(), "new-password")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpdateProfileChangesOnlyNameAndTimezone(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	p := seedProfile(repo, "+84901234567")

	updated, err := svc.UpdateProfile(context.Background(), p.Account.ID, UpdateProfileRequest{
		FullName: "After", Timezone: "Asia/Bangkok",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Teacher.FullName != "After" || updated.Teacher.Timezone != "Asia/Bangkok" {
		t.Fatalf("update not applied: %+v", updated.Teacher)
	}
	stored := repo.profiles[p.Account.ID]
	if stored.Teacher.FullName != "After" || stored.Teacher.Timezone != "Asia/Bangkok" {
		t.Fatalf("update not persisted: %+v", stored.Teacher)
	}
	if stored.Account.Status != StatusActive || stored.Account.Phone != "+84901234567" {
		t.Fatalf("update must not touch the account row: %+v", stored.Account)
	}
}

func TestUpdateProfileRejectsInvalidTimezone(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)
	p := seedProfile(repo, "+84901234567")

	for _, tz := range []string{"Mars/Olympus", "Local", "GMT+7"} {
		_, err := svc.UpdateProfile(context.Background(), p.Account.ID, UpdateProfileRequest{
			FullName: "After", Timezone: tz,
		})
		wantFieldError(t, err, "timezone")
	}
	if repo.profiles[p.Account.ID].Teacher.FullName != "Before" {
		t.Fatal("rejected update must not persist anything")
	}
}

func TestUpdateProfileUnknownTeacher(t *testing.T) {
	svc := NewService(newFakeRepository())

	_, err := svc.UpdateProfile(context.Background(), uuid.New(), UpdateProfileRequest{
		FullName: "Ghost", Timezone: DefaultTimezone,
	})
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("unknown teacher must be NOT_FOUND, got %v", err)
	}
}
