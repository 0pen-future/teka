package protocol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// friendsSession builds a logged-in session whose profile service points at a
// test server, which is all FetchFriends reads.
func friendsSession(profileURL string) *Session {
	sess := NewSession()
	sess.IMEI = "test-imei"
	sess.SecretKey = testSecretKey
	sess.LoginInfo = &LoginInfo{
		ZpwServiceMapV3: ZpwServiceMapV3{Profile: []string{profileURL}},
	}
	return sess
}

func TestFetchFriends_ReturnsFriendList(t *testing.T) {
	var gotPath, gotMethod string
	var gotPayload map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotPayload = decryptRequestParams(t, r.URL.Query().Get("params"))
		_, _ = w.Write([]byte(encryptedEnvelope(t, `[
			{"userId":"111","displayName":"Mẹ bé An","zaloName":"Lan Nguyễn","avatar":"https://a/1.jpg"},
			{"userId":"222","displayName":"Bố bé Bình"}
		]`)))
	}))
	defer srv.Close()

	friends, err := FetchFriends(context.Background(), friendsSession(srv.URL))
	if err != nil {
		t.Fatalf("FetchFriends: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/api/social/friend/getfriends" {
		t.Errorf("path = %s", gotPath)
	}
	if gotPayload["imei"] != "test-imei" {
		t.Errorf("payload imei = %v", gotPayload["imei"])
	}

	want := []FriendInfo{
		{UserID: "111", DisplayName: "Mẹ bé An", ZaloName: "Lan Nguyễn", Avatar: "https://a/1.jpg"},
		{UserID: "222", DisplayName: "Bố bé Bình"},
	}
	if len(friends) != len(want) {
		t.Fatalf("got %d friends, want %d", len(friends), len(want))
	}
	for i := range want {
		if friends[i] != want[i] {
			t.Errorf("friend[%d] = %+v, want %+v", i, friends[i], want[i])
		}
	}
}

func TestFetchFriends_FailsWithoutProfileServiceURL(t *testing.T) {
	sess := NewSession()
	sess.SecretKey = testSecretKey

	if _, err := FetchFriends(context.Background(), sess); err == nil {
		t.Fatal("want an error when the session has no profile service URL")
	}
}

// A refused friend list must surface as *APIError so the service layer can
// tell a dead session (-3) from any other refusal — and because Zalo's error
// strings can quote the request that produced them, which here carries the
// IMEI in the query.
func TestFetchFriends_SurfacesZaloErrorCodeAsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error_code":-3,"error_message":"not logged in: test-imei","data":null}`))
	}))
	defer srv.Close()

	_, err := FetchFriends(context.Background(), friendsSession(srv.URL))
	if err == nil {
		t.Fatal("want an error for a non-zero error_code")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != ErrCodeNotLoggedIn {
		t.Fatalf("want *APIError with code %d, got %v", ErrCodeNotLoggedIn, err)
	}
	if strings.Contains(err.Error(), "test-imei") {
		t.Errorf("the server's message must never be quoted: %v", err)
	}
}

// Same credential-stripping standard as the send path and the auth fetchers.
func TestFetchFriends_TransportErrorOmitsCredentials(t *testing.T) {
	const canaryIMEI = "canary-imei-0000-1111"

	sess := friendsSession("https://profile.invalid")
	sess.IMEI = canaryIMEI
	sess.Client = &http.Client{Transport: failingTransport{}}

	_, err := FetchFriends(context.Background(), sess)
	if err == nil {
		t.Fatal("want an error from a failing transport")
	}
	msg := err.Error()
	if strings.Contains(msg, canaryIMEI) {
		t.Errorf("error leaks the IMEI: %s", msg)
	}
	if strings.Contains(msg, "?") {
		t.Errorf("error carries a query string, which holds the encrypted imei: %s", msg)
	}
	if !strings.Contains(msg, "connection refused") {
		t.Errorf("error dropped the underlying cause: %s", msg)
	}
}
