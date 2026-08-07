package zalo

import (
	"encoding/base64"
	"time"

	"github.com/google/uuid"
)

// LinkStartRequest is the POST /me/zalo/link/start body. ConsentVersion is the
// consent text the teacher acknowledged; it is deliberately not enforced by a
// binding tag, so a blank value is refused by the same rule that refuses a
// blank one anywhere else — the service's.
type LinkStartRequest struct {
	ConsentVersion string `json:"consent_version"`
}

// LinkStartResponse hands back the id the client polls for progress.
type LinkStartResponse struct {
	LinkID uuid.UUID `json:"link_id"`
}

// LinkStatusResponse is one poll of a link attempt: where it is, the QR image
// to display while it waits, and — when it failed — a message written for the
// teacher rather than by Zalo.
//
// QRPNG is base64 so it can be dropped straight into an
// <img src="data:image/png;base64,…">. It is a login challenge, not a secret of
// the teacher's, and it is gone from every response once the attempt resolves.
type LinkStatusResponse struct {
	State        string `json:"state"`
	QRPNG        string `json:"qr_png,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// StatusResponse is what the profile card shows. Linked and Status are separate
// answers: an expired session is still a linked account, and the card says
// "reconnect" rather than "connect" because of it.
type StatusResponse struct {
	Linked      bool       `json:"linked"`
	DisplayName string     `json:"display_name,omitempty"`
	Status      string     `json:"status,omitempty"`
	LinkedAt    *time.Time `json:"linked_at,omitempty"`
}

func newStatusResponse(acc AccountStatus) StatusResponse {
	out := StatusResponse{
		Linked:      acc.Linked,
		DisplayName: acc.DisplayName,
		Status:      acc.Status,
	}
	if !acc.LinkedAt.IsZero() {
		linkedAt := acc.LinkedAt
		out.LinkedAt = &linkedAt
	}
	return out
}

func newLinkStatusResponse(snap LinkSnapshot) LinkStatusResponse {
	out := LinkStatusResponse{
		State:        string(snap.State),
		DisplayName:  snap.DisplayName,
		ErrorMessage: snap.Failure,
	}
	if len(snap.QRPNG) > 0 {
		out.QRPNG = base64.StdEncoding.EncodeToString(snap.QRPNG)
	}
	return out
}
