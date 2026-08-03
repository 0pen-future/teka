// Package seeds populates development data. Runs are idempotent: records are
// keyed by email and existing users are never modified, so reseeding a
// database with real data is safe.
package seeds

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/users"
)

const bcryptCost = 12

type seedUser struct {
	Email    string
	Password string
	Name     string
	Role     string
}

// Development credentials only — never used outside seeded local databases.
var seedUsers = []seedUser{
	{Email: "admin@teka.local", Password: "admin-password", Name: "Admin", Role: users.RoleAdmin},
	{Email: "alice@teka.local", Password: "alice-password", Name: "Alice Nguyen", Role: users.RoleUser},
	{Email: "bob@teka.local", Password: "bob-password", Name: "Bob Tran", Role: users.RoleUser},
	{Email: "carol@teka.local", Password: "carol-password", Name: "Carol Le", Role: users.RoleUser},
}

// Run inserts the seed users that do not exist yet and reports each outcome.
func Run(ctx context.Context, db *gorm.DB, log *slog.Logger) error {
	repo := users.NewRepository(db)
	for _, s := range seedUsers {
		if _, err := repo.GetByEmail(ctx, s.Email); err == nil {
			log.Info("seed: user exists, skipping", "email", s.Email)
			continue
		} else if !errors.Is(err, users.ErrNotFound) {
			return fmt.Errorf("seed: look up %s: %w", s.Email, err)
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(s.Password), bcryptCost)
		if err != nil {
			return fmt.Errorf("seed: hash password for %s: %w", s.Email, err)
		}
		u := &users.User{Email: s.Email, PasswordHash: string(hash), Name: s.Name, Role: s.Role}
		if err := repo.Create(ctx, u); err != nil {
			return fmt.Errorf("seed: create %s: %w", s.Email, err)
		}
		log.Info("seed: user created", "email", s.Email, "role", s.Role)
	}
	return nil
}
