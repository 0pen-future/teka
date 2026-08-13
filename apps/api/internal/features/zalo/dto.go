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

// FriendResponse is one row of the contact-mapping picker: exactly what the
// teacher needs to recognise a friend, hand-built so nothing from the protocol
// layer can ride along.
type FriendResponse struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar,omitempty"`
}

func newFriendResponses(friends []Friend) []FriendResponse {
	out := make([]FriendResponse, 0, len(friends))
	for _, f := range friends {
		name := f.DisplayName
		if name == "" {
			// A friend without an alias arrives with only the profile name;
			// an empty row would be impossible to recognise or map.
			name = f.ZaloName
		}
		out = append(out, FriendResponse{
			UserID:      f.UserID,
			DisplayName: name,
			Avatar:      f.Avatar,
		})
	}
	return out
}

// MatchFriendsRequest is the POST /me/zalo/friends/match body. The 1–200
// bound is enforced in the handler so the error can name the cap.
type MatchFriendsRequest struct {
	Phones []string `json:"phones"`
}

// FriendMatchResponse is one row of a match answer. Phone echoes the caller's
// input exactly — normalization stays server-side — so the client can join
// rows back to its contacts without re-implementing it. Hand-built from
// FriendMatch so nothing from the protocol layer can ride along.
type FriendMatchResponse struct {
	Phone       string `json:"phone"`
	Matched     bool   `json:"matched"`
	UserID      string `json:"user_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	ZaloName    string `json:"zalo_name,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
	IsFriend    bool   `json:"is_friend"`
}

func newFriendMatchResponses(rows []FriendMatch) []FriendMatchResponse {
	out := make([]FriendMatchResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, FriendMatchResponse(r))
	}
	return out
}

// FriendRequestRequest is the POST /me/zalo/friends/request body. Message is
// optional; a blank one falls back to a short Vietnamese greeting.
type FriendRequestRequest struct {
	UserID  string `json:"user_id"`
	Message string `json:"message"`
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
