package teachers

import (
	"context"
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

func TestCreateTeacherNormalizesAndPairsRows(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo)

	p, err := svc.CreateTeacher(context.Background(), CreateRequest{
		Phone: "0901234567", Password: "password-123", FullName: "Cô Lan",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Account.Phone != "+84901234567" {
		t.Fatalf("phone must normalize to E.164, got %q", p.Account.Phone)
	}
	if p.Account.ID != p.Teacher.ID {
		t.Fatal("account and teacher must share one id")
	}
	if p.Account.PasswordHash == nil || *p.Account.PasswordHash == "password-123" {
		t.Fatal("password must be stored hashed")
	}

	_, err = svc.CreateTeacher(context.Background(), CreateRequest{
		Phone: "+84901234567", Password: "password-123", FullName: "Trùng",
	})
	if apperror.From(err).Code != apperror.CodeConflict {
		t.Fatalf("duplicate phone must conflict, got %v", err)
	}
}

func TestCreateTeacherRejectsOver72BytePassword(t *testing.T) {
	svc := NewService(newFakeRepository())

	// 25 three-byte runes = 25 characters (passes binding max=72 runes) but 75
	// bytes (exceeds bcrypt's input limit).
	_, err := svc.CreateTeacher(context.Background(), CreateRequest{
		Phone: "0901234567", Password: strings.Repeat("ầ", 25), FullName: "X",
	})
	wantFieldError(t, err, "password")
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
