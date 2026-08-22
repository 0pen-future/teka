package invitations

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// CreateRequest creates an invitation for one phone number.
type CreateRequest struct {
	Phone string `json:"phone" binding:"required,vnphone"`
}

// CreateResponse is returned from POST /centers/me/invitations. Link and
// DMStatus are always present: the link is built before any Zalo work runs,
// so it never depends on delivery succeeding.
type CreateResponse struct {
	ID        uuid.UUID `json:"id"`
	Phone     string    `json:"phone"`
	ExpiresAt time.Time `json:"expires_at"`
	Link      string    `json:"link"`
	DMStatus  string    `json:"dm_status"`
}

// InvitationResponse is one row of GET /centers/me/invitations. Status
// reflects the derived "expired" state — never the raw stored column — so a
// pending invite past its deadline reads as expired without a background job.
type InvitationResponse struct {
	ID        uuid.UUID `json:"id"`
	Phone     string    `json:"phone"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// FromModel maps a stored row onto the list response shape, deriving the
// expired status at read time.
func FromModel(inv Invitation, now time.Time) InvitationResponse {
	status := inv.Status
	if inv.Expired(now) {
		status = "expired"
	}
	return InvitationResponse{
		ID:        inv.ID,
		Phone:     inv.Phone,
		Status:    status,
		ExpiresAt: inv.ExpiresAt,
		CreatedAt: inv.CreatedAt,
	}
}

// PreviewRequest carries the invite token to preview. The token travels in
// the body on every public onboarding route, never the path, so it stays out
// of access logs.
type PreviewRequest struct {
	Token string `json:"token" binding:"required"`
}

// PreviewResponse is the public, pre-authentication preview of a valid
// pending invitation: enough for the invitee to recognize it as theirs
// without exposing the full phone number to anyone who merely holds the link.
type PreviewResponse struct {
	CenterName  string `json:"center_name"`
	PhoneMasked string `json:"phone_masked"`
}

// AcceptRequest redeems an invitation: creates a new account, or reactivates
// a previously-removed one, in the inviting center.
type AcceptRequest struct {
	Token    string `json:"token" binding:"required"`
	FullName string `json:"full_name" binding:"required,min=1,max=100"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

// maskPhone hides every digit but the country code and the last 3, e.g.
// "+84901234567" becomes "+84******567" — enough for the invitee to recognize
// their own number, not enough to leak it to whoever merely holds the link.
// Invitation phones are always stored E.164-normalized (validation.NormalizePhone
// at Create time), so the "+84" prefix and 8-digit body are a safe assumption.
func maskPhone(phone string) string {
	const prefixLen, visibleSuffix = len("+84"), 3
	if len(phone) <= prefixLen+visibleSuffix {
		return phone
	}
	maskedLen := len(phone) - prefixLen - visibleSuffix
	return phone[:prefixLen] + strings.Repeat("*", maskedLen) + phone[len(phone)-visibleSuffix:]
}
