package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testSecretKey is a 16-byte AES key, base64-encoded the way a session stores
// the zpw_enk Zalo hands out at login.
var testSecretKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))

// sendSession builds a logged-in session whose chat service points at a test
// server, which is all SendMessage reads.
func sendSession(chatURL string) *Session {
	sess := NewSession()
	sess.UID = "self-uid"
	sess.IMEI = "test-imei"
	sess.SecretKey = testSecretKey
	sess.LoginInfo = &LoginInfo{
		ZpwServiceMapV3: ZpwServiceMapV3{Chat: []string{chatURL}},
	}
	return sess
}

// decryptRequestParams opens the encrypted params= field the way Zalo's server
// would, so a test can assert on the actual payload sent.
func decryptRequestParams(t *testing.T, enc string) map[string]any {
	t.Helper()
	key, err := base64.StdEncoding.DecodeString(testSecretKey)
	if err != nil {
		t.Fatalf("decode test key: %v", err)
	}
	plain, err := DecodeAESCBC(key, enc)
	if err != nil {
		t.Fatalf("decrypt request params: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatalf("parse request params: %v", err)
	}
	return payload
}

// encryptedEnvelope builds the outer response Zalo sends: the inner envelope
// {"error_code":0,"data":<data>} AES-encrypted under the session key, wrapped
// in a plaintext envelope.
func encryptedEnvelope(t *testing.T, innerData string) string {
	t.Helper()
	key, err := base64.StdEncoding.DecodeString(testSecretKey)
	if err != nil {
		t.Fatalf("decode test key: %v", err)
	}
	inner := `{"error_code":0,"error_message":"","data":` + innerData + `}`
	enc, err := EncodeAESCBC(key, inner, false)
	if err != nil {
		t.Fatalf("encrypt inner envelope: %v", err)
	}
	blob, err := json.Marshal(map[string]any{"error_code": 0, "data": enc})
	if err != nil {
		t.Fatalf("marshal outer envelope: %v", err)
	}
	return string(blob)
}

func TestSendMessage_SendsDMAndReturnsMsgID(t *testing.T) {
	var gotPath, gotMethod string
	var gotPayload map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotPayload = decryptRequestParams(t, r.PostForm.Get("params"))
		_, _ = w.Write([]byte(encryptedEnvelope(t, `{"msgId":991234}`)))
	}))
	defer srv.Close()

	sess := sendSession(srv.URL)
	msgID, err := SendMessage(context.Background(), sess, "friend-uid", "Học phí tháng 8")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if msgID != "991234" {
		t.Errorf("msgID = %q, want %q", msgID, "991234")
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/api/message/sms" {
		t.Errorf("path = %s, want /api/message/sms", gotPath)
	}
	// Payload fields must match the goclaw DM shape exactly.
	if gotPayload["message"] != "Học phí tháng 8" {
		t.Errorf("payload message = %v", gotPayload["message"])
	}
	if gotPayload["toid"] != "friend-uid" {
		t.Errorf("payload toid = %v", gotPayload["toid"])
	}
	if gotPayload["imei"] != "test-imei" {
		t.Errorf("payload imei = %v", gotPayload["imei"])
	}
	if gotPayload["ttl"] != float64(0) {
		t.Errorf("payload ttl = %v", gotPayload["ttl"])
	}
	if _, ok := gotPayload["clientId"]; !ok {
		t.Error("payload missing clientId")
	}
}

// A string msgId must survive too: Zalo has changed number/string encodings
// across endpoints before, and json.Number covers both.
func TestSendMessage_AcceptsStringMsgID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(encryptedEnvelope(t, `{"msgId":"88771"}`)))
	}))
	defer srv.Close()

	msgID, err := SendMessage(context.Background(), sendSession(srv.URL), "uid", "hi")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if msgID != "88771" {
		t.Errorf("msgID = %q, want %q", msgID, "88771")
	}
}

func TestSendMessage_RejectsEmptyText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request must be made for empty text")
	}))
	defer srv.Close()

	if _, err := SendMessage(context.Background(), sendSession(srv.URL), "uid", ""); err == nil {
		t.Fatal("want an error for empty text")
	}
}

func TestSendMessage_FailsWithoutChatServiceURL(t *testing.T) {
	sess := NewSession()
	sess.SecretKey = testSecretKey

	if _, err := SendMessage(context.Background(), sess, "uid", "hi"); err == nil {
		t.Fatal("want an error when the session has no chat service URL")
	}
}

func TestSendMessage_SurfacesZaloErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error_code":216,"error_message":"","data":null}`))
	}))
	defer srv.Close()

	_, err := SendMessage(context.Background(), sendSession(srv.URL), "uid", "hi")
	if err == nil {
		t.Fatal("want an error for a non-zero error_code")
	}
	if !strings.Contains(err.Error(), "216") {
		t.Errorf("error should name the code: %v", err)
	}
}

// Zalo can answer error_code 0 with a null data field. goclaw treats that as a
// sent message with no id, and this port keeps that meaning: callers must not
// read an empty msgId as a failed send.
func TestSendMessage_NullDataIsSuccessWithoutMsgID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"error_code":0,"error_message":"","data":null}`))
	}))
	defer srv.Close()

	msgID, err := SendMessage(context.Background(), sendSession(srv.URL), "uid", "hi")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if msgID != "" {
		t.Errorf("msgID = %q, want empty", msgID)
	}
}

// The decrypted data blob is itself a Zalo envelope; an error code buried in
// that inner layer must surface as an error too.
func TestSendMessage_SurfacesInnerEnvelopeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		key, err := base64.StdEncoding.DecodeString(testSecretKey)
		if err != nil {
			t.Errorf("decode test key: %v", err)
			return
		}
		inner := `{"error_code":-3,"error_message":"not logged in","data":null}`
		enc, err := EncodeAESCBC(key, inner, false)
		if err != nil {
			t.Errorf("encrypt inner envelope: %v", err)
			return
		}
		blob, _ := json.Marshal(map[string]any{"error_code": 0, "data": enc})
		_, _ = w.Write(blob)
	}))
	defer srv.Close()

	_, err := SendMessage(context.Background(), sendSession(srv.URL), "uid", "hi")
	if err == nil {
		t.Fatal("want an error for a non-zero inner error_code")
	}
	if !strings.Contains(err.Error(), "-3") {
		t.Errorf("error should name the inner code: %v", err)
	}
}

// A response whose encrypted data field arrives truncated must fail as an
// error, not crash the caller — the paced sender runs sends in a background
// goroutine with no recovery middleware underneath.
func TestSendMessage_TruncatedEncryptedDataIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		key, err := base64.StdEncoding.DecodeString(testSecretKey)
		if err != nil {
			t.Errorf("decode test key: %v", err)
			return
		}
		enc, err := EncodeAESCBC(key, `{"error_code":0,"data":{"msgId":1}}`, false)
		if err != nil {
			t.Errorf("encrypt inner envelope: %v", err)
			return
		}
		raw, _ := base64.StdEncoding.DecodeString(enc)
		cut := base64.StdEncoding.EncodeToString(raw[:len(raw)-5])
		blob, _ := json.Marshal(map[string]any{"error_code": 0, "data": cut})
		_, _ = w.Write(blob)
	}))
	defer srv.Close()

	_, err := SendMessage(context.Background(), sendSession(srv.URL), "uid", "hi")
	if err == nil {
		t.Fatal("want an error for a truncated encrypted data field")
	}
}

// The send path carries the IMEI in its encrypted payload and the URL query; a
// transport failure must not print either. Same standard as the auth fetchers
// (TestFetchers_TransportErrorOmitsCredentials).
func TestSendMessage_TransportErrorOmitsCredentials(t *testing.T) {
	const canaryIMEI = "canary-imei-0000-1111"

	sess := sendSession("https://chat.invalid")
	sess.IMEI = canaryIMEI
	sess.Client = &http.Client{Transport: failingTransport{}}

	_, err := SendMessage(context.Background(), sess, "uid", "hi")
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
	if !strings.Contains(msg, "connection refused") {
		t.Errorf("error dropped the underlying cause: %s", msg)
	}
}
