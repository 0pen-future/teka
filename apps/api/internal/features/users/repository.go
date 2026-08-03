package users

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/pagination"
)

// ListFilter narrows List results; zero values mean "no filter".
type ListFilter struct {
	// Query matches name or email case-insensitively (substring).
	Query string
	Role  string
}

// Repository is the persistence contract for users; the service depends on
// this interface, tests supply a fake.
type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context, f ListFilter, p pagination.Params) ([]User, int64, error)
	Update(ctx context.Context, u *User) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

// ErrNotFound is returned when no matching user exists; the service maps it
// onto the API error contract.
var ErrNotFound = errors.New("user not found")

// ErrDuplicateEmail is returned when the partial unique email index rejects a
// write.
var ErrDuplicateEmail = errors.New("email already in use")

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, u *User) error {
	err := database.FromContext(ctx, r.db).Create(u).Error
	return translateError(err)
}

func (r *gormRepository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var u User
	err := database.FromContext(ctx, r.db).First(&u, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *gormRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	err := database.FromContext(ctx, r.db).First(&u, "email = ?", email).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *gormRepository) List(ctx context.Context, f ListFilter, p pagination.Params) ([]User, int64, error) {
	q := database.FromContext(ctx, r.db).Model(&User{})
	if f.Query != "" {
		like := "%" + f.Query + "%"
		q = q.Where("name ILIKE ? OR email ILIKE ?", like, like)
	}
	if f.Role != "" {
		q = q.Where("role = ?", f.Role)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []User
	if err := q.Scopes(p.Scope).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) Update(ctx context.Context, u *User) error {
	err := database.FromContext(ctx, r.db).Save(u).Error
	return translateError(err)
}

func (r *gormRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	res := database.FromContext(ctx, r.db).Delete(&User{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// translateError maps the partial unique index violation onto
// ErrDuplicateEmail so callers stay driver-agnostic.
func translateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicateEmail
	}
	return err
}
