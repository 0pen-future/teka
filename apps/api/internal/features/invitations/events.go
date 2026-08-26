package invitations

import (
	"time"

	"github.com/google/uuid"
)

// ClientMeta is the request context the handler forwards so events can carry
// where the accept came from — the service itself never touches gin. It is
// deliberately invitations-local (auth has its own): features do not import
// each other for a two-field struct.
type ClientMeta struct {
	IP        string
	UserAgent string
}

// MemberJoined records a successfully redeemed invitation — the only
// public-accept outcome worth an audit row. Published by the service strictly
// after the accept transaction commits, because it is also the only place
// that knows the center and the joining account (the anonymous HTTP request
// carries neither). It lives next to its publisher so subscribers import
// invitations and invitations never imports them back.
type MemberJoined struct {
	OccurredAt   time.Time
	CenterID     uuid.UUID
	UserID       uuid.UUID
	InvitationID uuid.UUID
	IP           string
	UserAgent    string
}

// EventName implements events.Event.
func (MemberJoined) EventName() string { return "invitations.member_joined" }
