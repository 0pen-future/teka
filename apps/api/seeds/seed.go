// Package seeds populates development data. Runs are idempotent: records are
// keyed by phone and existing accounts are never modified, so reseeding a
// database with real data is safe.
package seeds

import (
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"teka/apps/api/internal/shared/id"
)

const (
	bcryptCost      = 12
	defaultTimezone = "Asia/Ho_Chi_Minh"
)

type seedTeacher struct {
	Phone    string
	Password string
	FullName string
}

// Development credentials only — never used outside seeded local databases.
var seedTeachers = []seedTeacher{
	{Phone: "+84901000001", Password: "lan-password", FullName: "Cô Lan"},
	{Phone: "+84901000002", Password: "minh-password", FullName: "Thầy Minh"},
}

// Run inserts the seed teachers that do not exist yet and reports each
// outcome. Each teacher is one user_accounts row plus one teachers row
// sharing the same id, created in a single transaction.
func Run(ctx context.Context, db *gorm.DB, log *slog.Logger) error {
	for _, s := range seedTeachers {
		var count int64
		err := db.WithContext(ctx).
			Raw("SELECT count(*) FROM user_accounts WHERE phone = ? AND deleted_at IS NULL", s.Phone).
			Scan(&count).Error
		if err != nil {
			return fmt.Errorf("seed: look up %s: %w", s.Phone, err)
		}
		if count > 0 {
			log.Info("seed: teacher exists, skipping", "phone", s.Phone)
			continue
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(s.Password), bcryptCost)
		if err != nil {
			return fmt.Errorf("seed: hash password for %s: %w", s.Phone, err)
		}
		accountID := id.New()
		err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(
				"INSERT INTO user_accounts (id, role, phone, password_hash, status) VALUES (?, 'teachers', ?, ?, 'active')",
				accountID, s.Phone, string(hash),
			).Error; err != nil {
				return err
			}
			return tx.Exec(
				"INSERT INTO teachers (id, full_name, timezone) VALUES (?, ?, ?)",
				accountID, s.FullName, defaultTimezone,
			).Error
		})
		if err != nil {
			return fmt.Errorf("seed: create %s: %w", s.Phone, err)
		}
		log.Info("seed: teacher created", "phone", s.Phone, "full_name", s.FullName)
	}
	return nil
}
