package auth

import (
	"context"
	"errors"
	"testing"

	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/events"
)

// busRecorder captures every event the service publishes through a SyncBus.
type busRecorder struct {
	events []events.Event
}

func (r *busRecorder) bus() events.Bus {
	b := events.NewSync()
	b.Subscribe("test", 0, func(_ context.Context, e events.Event) {
		r.events = append(r.events, e)
	})
	return b
}

var testMeta = ClientMeta{IP: "10.0.0.7", UserAgent: "go-test"}

// TestLoginPublishesSucceeded proves a successful login emits exactly one
// LoginSucceeded carrying the actor and the client context.
func TestLoginPublishesSucceeded(t *testing.T) {
	rec := &busRecorder{}
	svc, accounts, _ := newTestAuthService(t)
	svc.bus = rec.bus()
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)

	if _, err := svc.Login(context.Background(), LoginRequest{Phone: "+84901234567", Password: "correct-password"}, testMeta); err != nil {
		t.Fatalf("login: %v", err)
	}

	if len(rec.events) != 1 {
		t.Fatalf("events = %d, want 1", len(rec.events))
	}
	e, ok := rec.events[0].(LoginSucceeded)
	if !ok {
		t.Fatalf("event type = %T, want LoginSucceeded", rec.events[0])
	}
	if e.UserID != p.Account.ID || e.IP != testMeta.IP || e.UserAgent != testMeta.UserAgent {
		t.Errorf("event = %+v", e)
	}
	if e.OccurredAt.IsZero() {
		t.Error("OccurredAt is zero")
	}
}

// TestLoginPublishesFailedWithMaskedPhone proves every credential rejection —
// wrong password, unknown phone, disabled account — emits LoginFailed with a
// masked phone and nothing else that could identify or expose the caller.
func TestLoginPublishesFailedWithMaskedPhone(t *testing.T) {
	rec := &busRecorder{}
	svc, accounts, _ := newTestAuthService(t)
	svc.bus = rec.bus()
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)
	accounts.add(t, "+84907777777", "correct-password", teachers.StatusDisabled)

	cases := []LoginRequest{
		{Phone: "+84901234567", Password: "wrong-password"},
		{Phone: "+84909999999", Password: "whatever-123"},
		{Phone: "+84907777777", Password: "correct-password"},
	}
	for _, req := range cases {
		if _, err := svc.Login(context.Background(), req, testMeta); err == nil {
			t.Fatalf("login %s must fail", req.Phone)
		}
	}

	if len(rec.events) != len(cases) {
		t.Fatalf("events = %d, want %d", len(rec.events), len(cases))
	}
	for i, ev := range rec.events {
		e, ok := ev.(LoginFailed)
		if !ok {
			t.Fatalf("event %d type = %T, want LoginFailed", i, ev)
		}
		if e.PhoneMasked == "" || e.PhoneMasked == cases[i].Phone {
			t.Errorf("phone must be masked, got %q", e.PhoneMasked)
		}
		if e.IP != testMeta.IP || e.UserAgent != testMeta.UserAgent || e.OccurredAt.IsZero() {
			t.Errorf("event %d missing client context: %+v", i, e)
		}
	}
}

// erroringAccounts fails GetByPhone with a non-NotFound error: the internal
// failure path, which must not publish (the caller's credentials were never
// judged).
type erroringAccounts struct{ *fakeAccountService }

func (e erroringAccounts) GetByPhone(context.Context, string) (*teachers.Profile, error) {
	return nil, errors.New("connection refused")
}

// TestLoginInternalErrorPublishesNothing proves infrastructure failures stay
// out of the audit trail — only judged credential rejections are login-fails.
func TestLoginInternalErrorPublishesNothing(t *testing.T) {
	rec := &busRecorder{}
	svc, accounts, _ := newTestAuthService(t)
	svc.bus = rec.bus()
	svc.accounts = erroringAccounts{accounts}

	if _, err := svc.Login(context.Background(), LoginRequest{Phone: "+84901234567", Password: "x1234567"}, testMeta); err == nil {
		t.Fatal("login must fail")
	}
	if len(rec.events) != 0 {
		t.Fatalf("events = %d, want 0 on internal error", len(rec.events))
	}
}

// TestLogoutPublishesLoggedOut proves a real logout emits LoggedOut with the
// token family's user, and an unknown/empty token (idempotent no-op) does not.
func TestLogoutPublishesLoggedOut(t *testing.T) {
	rec := &busRecorder{}
	svc, accounts, _ := newTestAuthService(t)
	svc.bus = rec.bus()
	p := accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)

	sess, err := svc.Login(context.Background(), LoginRequest{Phone: "+84901234567", Password: "correct-password"}, testMeta)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	rec.events = nil

	if err := svc.Logout(context.Background(), sess.RefreshToken, testMeta); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if len(rec.events) != 1 {
		t.Fatalf("events = %d, want 1", len(rec.events))
	}
	e, ok := rec.events[0].(LoggedOut)
	if !ok {
		t.Fatalf("event type = %T, want LoggedOut", rec.events[0])
	}
	if e.UserID != p.Account.ID || e.IP != testMeta.IP {
		t.Errorf("event = %+v", e)
	}

	rec.events = nil
	if err := svc.Logout(context.Background(), sess.RefreshToken, testMeta); err != nil {
		t.Fatalf("repeat logout: %v", err)
	}
	if err := svc.Logout(context.Background(), "", testMeta); err != nil {
		t.Fatalf("empty logout: %v", err)
	}
	if len(rec.events) != 0 {
		t.Fatalf("idempotent no-op logouts published %d events, want 0", len(rec.events))
	}
}

// TestNilBusIsSafe proves a service built without a bus (operator CLI paths)
// never panics on publish.
func TestNilBusIsSafe(t *testing.T) {
	svc, accounts, _ := newTestAuthService(t)
	accounts.add(t, "+84901234567", "correct-password", teachers.StatusActive)

	if _, err := svc.Login(context.Background(), LoginRequest{Phone: "+84901234567", Password: "correct-password"}, testMeta); err != nil {
		t.Fatalf("login with nil bus: %v", err)
	}
}

func TestMaskPhone(t *testing.T) {
	cases := map[string]string{
		"+84901234567": "+84***567",
		"0901234567":   "090***567",
		// Too short for head+tail to hide anything — mask everything.
		"090123": "***",
		"12345":  "***",
		"":       "***",
	}
	for in, want := range cases {
		if got := maskPhone(in); got != want {
			t.Errorf("maskPhone(%q) = %q, want %q", in, got, want)
		}
	}
}
