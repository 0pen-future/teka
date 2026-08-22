package protocol

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// failingTransport fails every request the way a real outage does, so the error
// under test is the one net/http itself builds.
type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("dial tcp 1.2.3.4:443: connect: connection refused")
}

// A transport failure must not carry the account's credentials out with it.
// These requests put the IMEI in the query string in the clear, and the ZCID
// beside it is that same IMEI encrypted under a key that ships in this package.
// net/http returns a *url.Error holding the whole URL, and callers log it, so
// one network outage would otherwise print every linked account's IMEI.
func TestFetchers_TransportErrorOmitsCredentials(t *testing.T) {
	const canaryIMEI = "canary-imei-0000-1111"

	newSession := func() *Session {
		sess := NewSession()
		sess.IMEI = canaryIMEI
		sess.Client = &http.Client{Transport: failingTransport{}}
		return sess
	}

	fetchers := map[string]func(context.Context, *Session) error{
		"fetchServerInfo": func(ctx context.Context, s *Session) error {
			_, err := fetchServerInfo(ctx, s)
			return err
		},
		"fetchLoginInfo": func(ctx context.Context, s *Session) error {
			_, err := fetchLoginInfo(ctx, s)
			return err
		},
	}

	for name, fetch := range fetchers {
		t.Run(name, func(t *testing.T) {
			sess := newSession()
			err := fetch(context.Background(), sess)
			if err == nil {
				t.Fatal("want an error from a failing transport, got nil")
			}

			msg := err.Error()
			if strings.Contains(msg, canaryIMEI) {
				t.Errorf("error leaks the IMEI: %s", msg)
			}
			if strings.Contains(msg, "?") {
				t.Errorf("error carries a query string, which holds zcid: %s", msg)
			}
			// The cause still has to survive, or the log says nothing useful.
			if !strings.Contains(msg, "connection refused") {
				t.Errorf("error dropped the underlying cause: %s", msg)
			}
		})
	}
}

// A zero QRCallbacks must be usable: LoginQR reports through it unconditionally,
// so nil hooks have to degrade to no-ops rather than panic.
func TestQRCallbacks_ZeroValueIsSilent(_ *testing.T) {
	var cb QRCallbacks
	cb.qr([]byte("png"))
	cb.progress(QRStateScanned)
}

func TestQRCallbacks_ForwardsToHooks(t *testing.T) {
	var gotPNG []byte
	var gotStates []QRState

	cb := QRCallbacks{
		OnQR:       func(png []byte) { gotPNG = png },
		OnProgress: func(s QRState) { gotStates = append(gotStates, s) },
	}

	cb.qr([]byte{0x89, 0x50, 0x4e, 0x47})
	cb.progress(QRStateScanned)
	cb.progress(QRStateConfirmed)

	if !bytes.Equal(gotPNG, []byte{0x89, 0x50, 0x4e, 0x47}) {
		t.Errorf("OnQR got %v", gotPNG)
	}
	want := []QRState{QRStateScanned, QRStateConfirmed}
	if len(gotStates) != len(want) {
		t.Fatalf("got %d states, want %d", len(gotStates), len(want))
	}
	for i := range want {
		if gotStates[i] != want[i] {
			t.Errorf("state[%d] = %q, want %q", i, gotStates[i], want[i])
		}
	}
}
