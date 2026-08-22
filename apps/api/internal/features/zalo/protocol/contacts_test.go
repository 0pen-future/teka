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

// findUserSession builds a logged-in session whose friend service points at a
// test server, which is all FindUser and SendFriendRequest read.
func findUserSession(friendURL string) *Session {
	sess := NewSession()
	sess.IMEI = "test-imei"
	sess.SecretKey = testSecretKey
	sess.LoginInfo = &LoginInfo{
		ZpwServiceMapV3: ZpwServiceMapV3{Friend: []string{friendURL}},
	}
	return sess
}

func TestFindUser_ResolvesPhonesToAccounts(t *testing.T) {
	var gotPath, gotMethod string
	var gotPayload map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotPayload = decryptRequestParams(t, r.URL.Query().Get("params"))
		_, _ = w.Write([]byte(encryptedEnvelope(t, `{
			"0901234567": {"uid":"111","display_name":"Mẹ bé An","zalo_name":"Lan Nguyễn","avatar":"https://a/1.jpg","extra_field":true}
		}`)))
	}))
	defer srv.Close()

	found, err := FindUser(context.Background(), findUserSession(srv.URL),
		[]string{"0901234567", "0907654321"})
	if err != nil {
		t.Fatalf("FindUser: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/api/friend/profile/multiget" {
		t.Errorf("path = %s", gotPath)
	}
	phones, ok := gotPayload["phones"].([]any)
	if !ok || len(phones) != 2 || phones[0] != "0901234567" || phones[1] != "0907654321" {
		t.Errorf("payload phones = %v", gotPayload["phones"])
	}
	if gotPayload["avatar_size"] != float64(240) {
		t.Errorf("payload avatar_size = %v", gotPayload["avatar_size"])
	}
	if gotPayload["language"] != DefaultLanguage {
		t.Errorf("payload language = %v", gotPayload["language"])
	}

	if len(found) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(found), found)
	}
	want := FoundUser{UID: "111", DisplayName: "Mẹ bé An", ZaloName: "Lan Nguyễn", Avatar: "https://a/1.jpg"}
	if found["0901234567"] != want {
		t.Errorf("found[0901234567] = %+v, want %+v", found["0901234567"], want)
	}
	if _, ok := found["0907654321"]; ok {
		t.Error("unresolved phone must be absent from the result")
	}
}

func TestFindUser_RejectsEmptyPhoneList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request must be made for an empty phone list")
	}))
	defer srv.Close()

	if _, err := FindUser(context.Background(), findUserSession(srv.URL), nil); err == nil {
		t.Fatal("want an error for an empty phone list")
	}
}

func TestFindUser_FailsWithoutFriendServiceURL(t *testing.T) {
	sess := NewSession()
	sess.SecretKey = testSecretKey

	if _, err := FindUser(context.Background(), sess, []string{"0901234567"}); err == nil {
		t.Fatal("want an error when the session has no friend service URL")
	}
}

// A refused lookup must surface as *APIError so the service layer can act on
// ErrCodeNotLoggedIn by name — and must NOT quote Zalo's error string, which
// can echo the request (phones, imei) back.
func TestFindUser_SurfacesZaloErrorCodeAsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error_code":-3,"error_message":"not logged in: 0901234567","data":null}`))
	}))
	defer srv.Close()

	_, err := FindUser(context.Background(), findUserSession(srv.URL), []string{"0901234567"})
	if err == nil {
		t.Fatal("want an error for a non-zero error_code")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != ErrCodeNotLoggedIn {
		t.Fatalf("want *APIError with code %d, got %v", ErrCodeNotLoggedIn, err)
	}
	if strings.Contains(err.Error(), "0901234567") {
		t.Errorf("error quotes Zalo's server-controlled message: %v", err)
	}
}

// The lookup payload and query carry the IMEI and phone numbers; a transport
// failure must not print either. Same standard as the other fetchers.
func TestFindUser_TransportErrorOmitsCredentials(t *testing.T) {
	const canaryIMEI = "canary-imei-0000-1111"
	const canaryPhone = "0900011122"

	sess := findUserSession("https://friend.invalid")
	sess.IMEI = canaryIMEI
	sess.Client = &http.Client{Transport: failingTransport{}}

	_, err := FindUser(context.Background(), sess, []string{canaryPhone})
	if err == nil {
		t.Fatal("want an error from a failing transport")
	}
	msg := err.Error()
	if strings.Contains(msg, canaryIMEI) {
		t.Errorf("error leaks the IMEI: %s", msg)
	}
	if strings.Contains(msg, canaryPhone) {
		t.Errorf("error leaks a phone number: %s", msg)
	}
	if strings.Contains(msg, "?") {
		t.Errorf("error carries a query string: %s", msg)
	}
}

func TestSendFriendRequest_PostsOneRequest(t *testing.T) {
	var gotPath, gotMethod string
	var gotPayload map[string]any
	var calls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotPath, gotMethod = r.URL.Path, r.Method
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotPayload = decryptRequestParams(t, r.PostForm.Get("params"))
		_, _ = w.Write([]byte(`{"error_code":0,"error_message":"","data":null}`))
	}))
	defer srv.Close()

	err := SendFriendRequest(context.Background(), findUserSession(srv.URL),
		"target-uid", "Chào chị, em là cô giáo của bé")
	if err != nil {
		t.Fatalf("SendFriendRequest: %v", err)
	}

	if calls != 1 {
		t.Errorf("HTTP calls = %d, want exactly 1", calls)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/api/friend/sendreq" {
		t.Errorf("path = %s", gotPath)
	}
	if gotPayload["toid"] != "target-uid" {
		t.Errorf("payload toid = %v", gotPayload["toid"])
	}
	if gotPayload["msg"] != "Chào chị, em là cô giáo của bé" {
		t.Errorf("payload msg = %v", gotPayload["msg"])
	}
	if gotPayload["reqsrc"] != float64(30) {
		t.Errorf("payload reqsrc = %v", gotPayload["reqsrc"])
	}
	if gotPayload["imei"] != "test-imei" {
		t.Errorf("payload imei = %v", gotPayload["imei"])
	}
	// srcParams is a JSON string, not a nested object — upstream wire shape.
	if gotPayload["srcParams"] != `{"uidTo":"target-uid"}` {
		t.Errorf("payload srcParams = %v", gotPayload["srcParams"])
	}
}

func TestSendFriendRequest_RejectsEmptyUserID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request must be made for an empty user id")
	}))
	defer srv.Close()

	if err := SendFriendRequest(context.Background(), findUserSession(srv.URL), "", "hi"); err == nil {
		t.Fatal("want an error for an empty user id")
	}
}

func TestSendFriendRequest_FailsWithoutFriendServiceURL(t *testing.T) {
	sess := NewSession()
	sess.SecretKey = testSecretKey

	if err := SendFriendRequest(context.Background(), sess, "uid", "hi"); err == nil {
		t.Fatal("want an error when the session has no friend service URL")
	}
}

// Refusal codes matter to callers by name (215 blocked, 222 already requested
// the other way, 225 already friends), so the error must be *APIError carrying
// the code — never Zalo's message string.
func TestSendFriendRequest_SurfacesZaloErrorCodeAsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error_code":225,"error_message":"already friends","data":null}`))
	}))
	defer srv.Close()

	err := SendFriendRequest(context.Background(), findUserSession(srv.URL), "uid", "hi")
	if err == nil {
		t.Fatal("want an error for a non-zero error_code")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 225 {
		t.Fatalf("want *APIError with code 225, got %v", err)
	}
	if strings.Contains(err.Error(), "already friends") {
		t.Errorf("error quotes Zalo's server-controlled message: %v", err)
	}
}

func TestSendFriendRequest_TransportErrorOmitsCredentials(t *testing.T) {
	const canaryIMEI = "canary-imei-0000-1111"

	sess := findUserSession("https://friend.invalid")
	sess.IMEI = canaryIMEI
	sess.Client = &http.Client{Transport: failingTransport{}}

	err := SendFriendRequest(context.Background(), sess, "uid", "hi")
	if err == nil {
		t.Fatal("want an error from a failing transport")
	}
	msg := err.Error()
	if strings.Contains(msg, canaryIMEI) {
		t.Errorf("error leaks the IMEI: %s", msg)
	}
	if strings.Contains(msg, "?") {
		t.Errorf("error carries a query string: %s", msg)
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
