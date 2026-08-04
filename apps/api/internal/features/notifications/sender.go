package notifications

import (
	"context"
	"errors"

	"teka/apps/api/internal/shared/apperror"
)

// ErrNotConfigured is returned by a sender whose provider integration is not
// yet wired — currently znsSender, until PRD Q1 (which ZNS provider/template)
// is answered. Callers translate it to a client-safe error rather than a
// generic 500, since it is an expected, actionable state, not a bug.
var ErrNotConfigured = errors.New("notification channel not configured")

// Sender delivers one queued notification over one channel. Send is
// responsible only for the delivery attempt; it never mutates n's status —
// the caller decides what a Send outcome means for the row (see
// manualSender's own doc comment for why even success does not imply
// "delivered").
type Sender interface {
	Channel() string
	Send(ctx context.Context, n *Notification) error
}

// manualSender backs ChannelZaloManual: the teacher copies the rendered text
// out of the API response and pastes it into Zalo themselves, then confirms
// via POST /notifications/mark-sent. Send is a deliberate no-op — there is
// nothing to transmit — leaving the row queued until the teacher's own
// mark-sent call moves it to sent. This is V1's default and only wired
// implementation.
type manualSender struct{}

func (manualSender) Channel() string { return ChannelZaloManual }

func (manualSender) Send(_ context.Context, _ *Notification) error { return nil }

// znsSender backs ChannelZaloZNS (Zalo Notification Service): a real
// provider integration, not yet implemented. Present so switching channels
// once PRD Q1 is answered is a config change, not a refactor.
type znsSender struct{}

func (znsSender) Channel() string { return ChannelZaloZNS }

func (znsSender) Send(_ context.Context, _ *Notification) error { return ErrNotConfigured }

// senderRegistry resolves a channel string to its Sender. sms appears in the
// schema's CHECK constraint (docs/schema_design.sql:438-439) with no
// implementation — it is deliberately absent from this map, so resolving it
// falls through to the "unknown channel" branch below.
func senderRegistry() map[string]Sender {
	return map[string]Sender{
		ChannelZaloManual: manualSender{},
		ChannelZaloZNS:    znsSender{},
	}
}

// resolveSender looks up channel in the registry. An unknown channel —
// including the schema-legal but unimplemented "sms" — is a client error,
// not a server one: the caller asked for something this deployment cannot
// do.
func resolveSender(channel string) (Sender, error) {
	sender, ok := senderRegistry()[channel]
	if !ok {
		return nil, apperror.BadRequest("unsupported notification channel: " + channel)
	}
	return sender, nil
}
