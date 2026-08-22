// Package zalo links a teacher's personal Zalo account to Teka and keeps the
// resulting session credentials usable across restarts. Those credentials are
// full account-takeover material: they exist in this package only as a sealed
// blob, are never logged, and are never returned by any API response.
package zalo

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Status values, matching zalo_accounts.status's CHECK constraint exactly.
// Every call site uses these constants, never a string literal.
const (
	// StatusLinked means the stored credentials were accepted by Zalo the last
	// time they were used.
	StatusLinked = "linked"
	// StatusExpired means Zalo rejected them; the teacher has to scan again.
	StatusExpired = "expired"
)

// Account is one row of zalo_accounts: a teacher's linked personal Zalo
// account.
//
// TeacherID is the primary key, not a surrogate id — one account per teacher
// is a product constraint, and making it the key means a second row cannot
// exist rather than merely should not.
//
// EncryptedCredentials holds the protocol credentials (IMEI + cookie jar)
// sealed by internal/shared/secrets. There is deliberately no field, column, or
// accessor for the plaintext: it is unsealed only in memory, only when a
// session is being restored.
type Account struct {
	TeacherID            uuid.UUID `gorm:"primaryKey"`
	EncryptedCredentials []byte
	ZaloUID              *string
	DisplayName          *string
	Status               string

	// ConsentVersion is the exact consent text the teacher acknowledged when
	// linking, and ConsentAt is when. The column is NOT NULL: a linked row
	// must always be traceable to an acknowledgement.
	ConsentVersion string
	ConsentAt      time.Time

	LinkedAt       time.Time
	LastVerifiedAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Account) TableName() string { return "zalo_accounts" }
