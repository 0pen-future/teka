package zalo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/zalo/protocol"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/authctx"
)

const handlerTestSecret = "zalo-http-test-secret-0123456789ab"

// Canaries: values that only ever exist inside a credential. A response body
// containing one of these is the leak this feature must never have.
const (
	canaryIMEI      = "canary-imei-0000-1111"
	canaryCookie    = "canary-zpsid-cookie-value"
	canarySecretKey = "canary-zpw-enk-secret-key"
)

func newZaloHTTPTest(t *testing.T, opts ServiceOptions) (*gin.Engine, *fakeRepo, *Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := newFakeRepo()
	svc := newTestService(t, repo, opts)

	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc), middleware.RequireAuth(jwtCfg))
	return r, repo, svc
}

// mintToken signs an access token the way the auth issuer does, without
// importing features/auth.
func mintToken(t *testing.T, subject uuid.UUID) string {
	t.Helper()
	claims := authctx.AccessClaims{
		Role: authctx.RoleTeacher,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(handlerTestSecret))
	require.NoError(t, err)
	return signed
}

type httpEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func do(t *testing.T, r *gin.Engine, method, path, body, token string) (*httptest.ResponseRecorder, httpEnvelope) {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var env httpEnvelope
	if w.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "response is not an envelope: %s", w.Body.String())
	}
	return w, env
}

// scriptedLogin is a QR login that never touches the network: it publishes a QR
// image, waits to be released, and then reports a successful login.
type scriptedLogin struct {
	qr          []byte
	release     chan struct{}
	displayName string
	err         error
}

func newScriptedLogin(displayName string) *scriptedLogin {
	return &scriptedLogin{
		qr:          []byte("\x89PNG-not-a-real-image"),
		release:     make(chan struct{}),
		displayName: displayName,
	}
}

func (s *scriptedLogin) login(ctx context.Context, sess *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
	cb.OnQR(s.qr)
	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if s.err != nil {
		return nil, s.err
	}
	sess.UID = "zalo-uid"
	sess.DisplayName = s.displayName
	sess.IMEI = canaryIMEI
	sess.SecretKey = canarySecretKey
	cookies := protocol.NewHTTPCookies([]*http.Cookie{{Name: "zpsid", Value: canaryCookie}})
	return &protocol.Credentials{IMEI: canaryIMEI, Cookie: &cookies, UserAgent: "ua"}, nil
}

// waitForLinkState polls the status endpoint the way the browser will.
func waitForLinkState(t *testing.T, r *gin.Engine, token string, linkID uuid.UUID, want LinkState) LinkStatusResponse {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last LinkStatusResponse
	for time.Now().Before(deadline) {
		w, env := do(t, r, http.MethodGet, "/api/v1/me/zalo/link/status?id="+linkID.String(), "", token)
		require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
		// A fresh value each poll: unmarshalling into the previous one would
		// leave a field the new response omitted looking present.
		var got LinkStatusResponse
		require.NoError(t, json.Unmarshal(env.Data, &got))
		last = got
		if last.State == string(want) {
			return last
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("link never reached %q; last state was %q", want, last.State)
	return last
}

func linkedAccount(teacherID uuid.UUID, displayName string) *Account {
	name := displayName
	uid := "zalo-uid"
	return &Account{
		TeacherID:            teacherID,
		EncryptedCredentials: []byte("sealed"),
		ZaloUID:              &uid,
		DisplayName:          &name,
		Status:               StatusLinked,
		ConsentVersion:       testConsentVersion,
		ConsentAt:            time.Now(),
		LinkedAt:             time.Now(),
	}
}

func TestZaloEndpointsRejectAnUnauthenticatedCaller(t *testing.T) {
	t.Parallel()
	r, _, _ := newZaloHTTPTest(t, ServiceOptions{})

	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/me/zalo", ""},
		{http.MethodPost, "/api/v1/me/zalo/link/start", `{"consent_version":"` + testConsentVersion + `"}`},
		{http.MethodGet, "/api/v1/me/zalo/link/status?id=" + uuid.NewString(), ""},
		{http.MethodDelete, "/api/v1/me/zalo", ""},
	}
	for _, tc := range cases {
		w, _ := do(t, r, tc.method, tc.path, tc.body, "")
		require.Equal(t, http.StatusUnauthorized, w.Code, "%s %s", tc.method, tc.path)
	}
}

// Having no linked account is the normal state for every teacher who has not
// scanned a QR code, so it is an answer, not a 404.
func TestStatusOfAnUnlinkedTeacherReportsNotLinked(t *testing.T) {
	t.Parallel()
	r, _, _ := newZaloHTTPTest(t, ServiceOptions{})
	teacherID := uuid.New()

	w, env := do(t, r, http.MethodGet, "/api/v1/me/zalo", "", mintToken(t, teacherID))
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, env.Success)

	var body map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &body))
	require.Equal(t, map[string]any{"linked": false}, body,
		"an unlinked teacher's status carries nothing but the answer")
}

func TestStatusReportsTheLinkedAccount(t *testing.T) {
	t.Parallel()
	r, repo, _ := newZaloHTTPTest(t, ServiceOptions{})
	teacherID := uuid.New()
	repo.accounts[teacherID] = linkedAccount(teacherID, "Cô Lan")

	w, env := do(t, r, http.MethodGet, "/api/v1/me/zalo", "", mintToken(t, teacherID))
	require.Equal(t, http.StatusOK, w.Code)

	var got StatusResponse
	require.NoError(t, json.Unmarshal(env.Data, &got))
	require.True(t, got.Linked)
	require.Equal(t, "Cô Lan", got.DisplayName)
	require.Equal(t, StatusLinked, got.Status)
	require.NotNil(t, got.LinkedAt)
}

// An expired session is still a linked account — the profile card needs both
// facts to say "reconnect" instead of "connect".
func TestStatusReportsAnExpiredSession(t *testing.T) {
	t.Parallel()
	r, repo, _ := newZaloHTTPTest(t, ServiceOptions{})
	teacherID := uuid.New()
	acc := linkedAccount(teacherID, "Cô Lan")
	acc.Status = StatusExpired
	repo.accounts[teacherID] = acc

	_, env := do(t, r, http.MethodGet, "/api/v1/me/zalo", "", mintToken(t, teacherID))
	var got StatusResponse
	require.NoError(t, json.Unmarshal(env.Data, &got))
	require.True(t, got.Linked)
	require.Equal(t, StatusExpired, got.Status)
}

// Consent is the whole reason this row may exist, so nothing may reach Zalo
// before it is acknowledged.
func TestStartLinkWithoutConsentStartsNoAttempt(t *testing.T) {
	t.Parallel()

	bodies := []string{`{}`, `{"consent_version":""}`, ``}
	for _, body := range bodies {
		login := newScriptedLogin("Cô Lan")
		r, _, svc := newZaloHTTPTest(t, ServiceOptions{Login: login.login})
		teacherID := uuid.New()

		w, env := do(t, r, http.MethodPost, "/api/v1/me/zalo/link/start", body, mintToken(t, teacherID))
		require.Equal(t, http.StatusBadRequest, w.Code, "body %q", body)
		require.NotNil(t, env.Error)
		require.Equal(t, "BAD_REQUEST", env.Error.Code)

		svc.links.mu.Lock()
		_, running := svc.links.active[teacherID]
		svc.links.mu.Unlock()
		require.False(t, running, "no attempt may start without acknowledged consent")
	}
}

func TestLinkFlowServesTheQRThenReportsLinked(t *testing.T) {
	t.Parallel()
	login := newScriptedLogin("Cô Lan")
	r, repo, _ := newZaloHTTPTest(t, ServiceOptions{Login: login.login})
	teacherID := uuid.New()
	token := mintToken(t, teacherID)

	w, env := do(t, r, http.MethodPost, "/api/v1/me/zalo/link/start",
		`{"consent_version":"`+testConsentVersion+`"}`, token)
	require.Equal(t, http.StatusAccepted, w.Code)

	var started LinkStartResponse
	require.NoError(t, json.Unmarshal(env.Data, &started))
	require.NotEqual(t, uuid.Nil, started.LinkID)

	qrReady := waitForLinkState(t, r, token, started.LinkID, LinkStateQRReady)
	png, err := base64.StdEncoding.DecodeString(qrReady.QRPNG)
	require.NoError(t, err, "qr_png must be plain base64 an <img src=data:…> can render")
	require.Equal(t, login.qr, png)

	close(login.release)

	linked := waitForLinkState(t, r, token, started.LinkID, LinkStateLinked)
	require.Equal(t, "Cô Lan", linked.DisplayName)
	require.Empty(t, linked.QRPNG, "a spent challenge is not served again")
	require.Empty(t, linked.ErrorMessage)

	upserts, _, _ := repo.counts()
	require.Equal(t, 1, upserts)
}

// A failed attempt tells the teacher it failed and nothing more: Zalo's own
// error text can quote the request that produced it.
func TestLinkStatusReportsAFailureWithoutUpstreamDetail(t *testing.T) {
	t.Parallel()
	login := newScriptedLogin("Cô Lan")
	login.err = errStub("zalo_personal: login rejected for zpsid=" + canaryCookie)
	r, _, _ := newZaloHTTPTest(t, ServiceOptions{Login: login.login})
	teacherID := uuid.New()
	token := mintToken(t, teacherID)

	_, env := do(t, r, http.MethodPost, "/api/v1/me/zalo/link/start",
		`{"consent_version":"`+testConsentVersion+`"}`, token)
	var started LinkStartResponse
	require.NoError(t, json.Unmarshal(env.Data, &started))

	close(login.release)

	failed := waitForLinkState(t, r, token, started.LinkID, LinkStateError)
	require.NotEmpty(t, failed.ErrorMessage)
	require.NotContains(t, failed.ErrorMessage, canaryCookie)
	require.NotContains(t, failed.ErrorMessage, "zalo_personal:")
}

type errStub string

func (e errStub) Error() string { return string(e) }

// A link id is only meaningful to the teacher it was issued to; anyone else
// learns nothing, including whether it exists.
func TestLinkStatusHidesAnotherTeachersAttempt(t *testing.T) {
	t.Parallel()
	login := newScriptedLogin("Cô Lan")
	r, _, _ := newZaloHTTPTest(t, ServiceOptions{Login: login.login})
	t.Cleanup(func() { close(login.release) })

	owner := mintToken(t, uuid.New())
	_, env := do(t, r, http.MethodPost, "/api/v1/me/zalo/link/start",
		`{"consent_version":"`+testConsentVersion+`"}`, owner)
	var started LinkStartResponse
	require.NoError(t, json.Unmarshal(env.Data, &started))

	intruder := mintToken(t, uuid.New())
	for _, path := range []string{
		"/api/v1/me/zalo/link/status?id=" + started.LinkID.String(),
		"/api/v1/me/zalo/link/status?id=" + uuid.NewString(),
		"/api/v1/me/zalo/link/status?id=not-a-uuid",
		"/api/v1/me/zalo/link/status",
	} {
		w, env := do(t, r, http.MethodGet, path, "", intruder)
		require.Equal(t, http.StatusNotFound, w.Code, "path %s", path)
		require.NotNil(t, env.Error)
		require.Equal(t, "NOT_FOUND", env.Error.Code)
	}
}

func TestUnlinkRemovesTheAccountAndIsIdempotent(t *testing.T) {
	t.Parallel()
	r, repo, _ := newZaloHTTPTest(t, ServiceOptions{})
	teacherID := uuid.New()
	repo.accounts[teacherID] = linkedAccount(teacherID, "Cô Lan")
	token := mintToken(t, teacherID)

	w, _ := do(t, r, http.MethodDelete, "/api/v1/me/zalo", "", token)
	require.Equal(t, http.StatusNoContent, w.Code)
	require.Empty(t, w.Body.Bytes())

	w, _ = do(t, r, http.MethodGet, "/api/v1/me/zalo", "", token)
	require.Equal(t, http.StatusOK, w.Code)

	// A second DELETE — a double-click, or a retry after a dropped response —
	// answers exactly like the first: the account is gone either way.
	w, _ = do(t, r, http.MethodDelete, "/api/v1/me/zalo", "", token)
	require.Equal(t, http.StatusNoContent, w.Code)
	require.Empty(t, w.Body.Bytes())
}

// The security criterion of this feature, checked against real response bodies
// rather than by reading the DTOs.
func TestNoResponseCarriesCredentialMaterial(t *testing.T) {
	t.Parallel()
	login := newScriptedLogin("Cô Lan")
	r, _, _ := newZaloHTTPTest(t, ServiceOptions{Login: login.login})
	teacherID := uuid.New()
	token := mintToken(t, teacherID)

	_, env := do(t, r, http.MethodPost, "/api/v1/me/zalo/link/start",
		`{"consent_version":"`+testConsentVersion+`"}`, token)
	var started LinkStartResponse
	require.NoError(t, json.Unmarshal(env.Data, &started))

	bodies := []string{}
	record := func(w *httptest.ResponseRecorder) { bodies = append(bodies, w.Body.String()) }

	w, _ := do(t, r, http.MethodGet, "/api/v1/me/zalo/link/status?id="+started.LinkID.String(), "", token)
	record(w)

	close(login.release)
	waitForLinkState(t, r, token, started.LinkID, LinkStateLinked)

	w, _ = do(t, r, http.MethodGet, "/api/v1/me/zalo/link/status?id="+started.LinkID.String(), "", token)
	record(w)
	w, _ = do(t, r, http.MethodGet, "/api/v1/me/zalo", "", token)
	record(w)
	w, _ = do(t, r, http.MethodDelete, "/api/v1/me/zalo", "", token)
	record(w)

	for _, body := range bodies {
		for _, canary := range []string{canaryIMEI, canaryCookie, canarySecretKey} {
			require.NotContains(t, body, canary, "a response carried credential material")
		}
		lower := strings.ToLower(body)
		for _, field := range []string{"imei", "cookie", "secret", "credential"} {
			require.NotContains(t, lower, field, "a response names a credential field")
		}
	}
}

// The response types themselves must offer no place to put credentials, so a
// later edit cannot reintroduce the leak the test above rules out today.
func TestResponseTypesHaveNoCredentialFields(t *testing.T) {
	t.Parallel()

	forbidden := []string{"credential", "cookie", "imei", "secretkey", "session"}
	types := []reflect.Type{
		reflect.TypeOf(StatusResponse{}),
		reflect.TypeOf(LinkStartResponse{}),
		reflect.TypeOf(LinkStatusResponse{}),
	}
	for _, typ := range types {
		for i := range typ.NumField() {
			field := typ.Field(i)
			name := strings.ToLower(field.Name + " " + field.Tag.Get("json"))
			for _, bad := range forbidden {
				require.NotContains(t, name, bad, "%s.%s", typ.Name(), field.Name)
			}
			require.NotEqual(t, reflect.TypeOf(protocol.Credentials{}), field.Type,
				"%s.%s embeds protocol credentials", typ.Name(), field.Name)
		}
	}
}
