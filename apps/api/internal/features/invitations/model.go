// Package invitations lets a center owner onboard a teacher without public
// self-registration: the owner mints a single-use, hashed token for a phone
// number, delivers it as a link (best-effort over Zalo DM, always as a
// copy-link), and the invitee redeems it in Phase 3's accept flow. It sits
// beside centers rather than inside it — onboarding coordinates centers,
// teacher accounts, and Zalo, and folding that into centers would couple all
// three together.
package invitations

import (
	"time"

	"github.com/google/uuid"
)

// Status values for invitations.status, mirroring the migration 000008 CHECK
// constraint. "expired" is never stored — it is derived at read time from
// status="pending" && expires_at < now().
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRevoked  = "revoked"
)

// Invitation is one owner-issued, single-use onboarding token for a phone
// number. ID is a UUIDv7 generated in Go via id.New(); the column has no
// application-level default even though the migration sets one for direct
// SQL convenience.
type Invitation struct {
	ID         uuid.UUID `gorm:"primaryKey"`
	CenterID   uuid.UUID
	Phone      string
	TokenHash  string
	Status     string
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Invitation) TableName() string { return "invitations" }

// Expired reports the derived "expired" state: still pending, but past its
// deadline. It is never written to storage.
func (i Invitation) Expired(now time.Time) bool {
	return i.Status == StatusPending && i.ExpiresAt.Before(now)
}
