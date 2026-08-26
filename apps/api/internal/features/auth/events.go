package auth

import (
	"time"

	"github.com/google/uuid"
)

// Auth events published on the event bus by the auth service — the service,
// not the handler, because only the service knows the real outcome and can
// resolve the acting user (logout resolves it from the refresh token). They
// live here, next to their publisher, so subscribers import auth and auth
// never imports them back.

// ClientMeta is the request context the handler forwards so events can carry
// where an auth action came from — the service itself never touches gin.
type ClientMeta struct {
	IP        string
	UserAgent string
}

// maskPhone hides the middle of a phone for the login-fail trail: first 3 and
// last 3 characters survive, the rest is "***". A phone too short for that to
// hide anything masks entirely.
func maskPhone(phone string) string {
	if len(phone) < 7 {
		return "***"
	}
	return phone[:3] + "***" + phone[len(phone)-3:]
}

// LoginSucceeded records a successful credential login.
type LoginSucceeded struct {
	OccurredAt time.Time
	UserID     uuid.UUID
	IP         string
	UserAgent  string
}

// EventName implements events.Event.
func (LoginSucceeded) EventName() string { return "auth.login_succeeded" }

// LoginFailed records a rejected login attempt. It carries only a masked
// phone (e.g. "090***123") — never the raw phone or password — enough for an
// owner to investigate brute-force patterns without exposing credentials.
type LoginFailed struct {
	OccurredAt  time.Time
	PhoneMasked string
	IP          string
	UserAgent   string
}

// EventName implements events.Event.
func (LoginFailed) EventName() string { return "auth.login_failed" }

// LoggedOut records an explicit logout; UserID is resolved from the refresh
// token inside the service.
type LoggedOut struct {
	OccurredAt time.Time
	UserID     uuid.UUID
	IP         string
	UserAgent  string
}

// EventName implements events.Event.
func (LoggedOut) EventName() string { return "auth.logged_out" }
