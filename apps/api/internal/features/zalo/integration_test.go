//go:build integration

package zalo_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/features/zalo/protocol"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/secrets"
	"teka/apps/api/internal/testutil"
)

// testCredKey is a fixed, non-secret key for these tests only — it protects
// nothing real, so a literal here carries none of the risk a production key
// would.
var testCredKey = []byte("integration-test-zalo-cred-key-32b")

const testConsentVersion = "2026-08-06"

func newRepo(t *testing.T) (zalo.Repository, *gorm.DB, uuid.UUID) {
	t.Helper()
	db := testutil.StartPostgres(t)
	_, teacher := testutil.Teacher(t, db)
	return zalo.NewRepository(db), db, teacher.ID
}

func sealed(t *testing.T, plaintext string) []byte {
	t.Helper()
	c, err := secrets.New(testCredKey)
	require.NoError(t, err)
	out, err := c.Seal([]byte(plaintext))
	require.NoError(t, err)
	return out
}

func account(t *testing.T, teacherID uuid.UUID, plaintext string) *zalo.Account {
	t.Helper()
	uid, name := "zalo-uid-1", "Cô Lan"
	return &zalo.Account{
		TeacherID:            teacherID,
		EncryptedCredentials: sealed(t, plaintext),
		ZaloUID:              &uid,
		DisplayName:          &name,
		Status:               zalo.StatusLinked,
		ConsentVersion:       testConsentVersion,
	}
}

func TestUpsertStoresSealedCredentialsAndStampsConsent(t *testing.T) {
	t.Parallel()
	repo, _, teacherID := newRepo(t)
	ctx := context.Background()

	acc := account(t, teacherID, `{"imei":"abc","userAgent":"ua"}`)
	require.NoError(t, repo.Upsert(ctx, acc))

	got, err := repo.GetByTeacher(ctx, teacherID)
	require.NoError(t, err)
	require.Equal(t, teacherID, got.TeacherID)
	require.Equal(t, acc.EncryptedCredentials, got.EncryptedCredentials, "BYTEA must round-trip byte for byte")
	require.Equal(t, testConsentVersion, got.ConsentVersion)
	require.False(t, got.ConsentAt.IsZero(), "a linked row must record when consent was given")
	require.False(t, got.LinkedAt.IsZero())
	require.Equal(t, zalo.StatusLinked, got.Status)

	// The stored blob is only readable through the credential key.
	c, err := secrets.New(testCredKey)
	require.NoError(t, err)
	plain, err := c.Open(got.EncryptedCredentials)
	require.NoError(t, err)
	require.JSONEq(t, `{"imei":"abc","userAgent":"ua"}`, string(plain))
}

// One account per teacher is a product constraint expressed as the primary
// key: a second link replaces the first rather than adding a row.
func TestUpsertTwiceKeepsExactlyOneRow(t *testing.T) {
	t.Parallel()
	repo, db, teacherID := newRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, account(t, teacherID, "first")))

	second := account(t, teacherID, "second")
	newName := "Cô Lan (mới)"
	second.DisplayName = &newName
	require.NoError(t, repo.Upsert(ctx, second))

	var count int64
	require.NoError(t, db.Table("zalo_accounts").Where("teacher_id = ?", teacherID).Count(&count).Error)
	require.EqualValues(t, 1, count)

	got, err := repo.GetByTeacher(ctx, teacherID)
	require.NoError(t, err)
	require.Equal(t, second.EncryptedCredentials, got.EncryptedCredentials)
	require.Equal(t, newName, *got.DisplayName)
}

// Linking hands a third party's session to this system, so a stored row
// without an acknowledged consent version must be impossible to create.
func TestUpsertRejectsMissingConsentVersion(t *testing.T) {
	t.Parallel()
	repo, db, teacherID := newRepo(t)
	ctx := context.Background()

	acc := account(t, teacherID, "creds")
	acc.ConsentVersion = ""
	require.ErrorIs(t, repo.Upsert(ctx, acc), zalo.ErrConsentVersionRequired)

	var count int64
	require.NoError(t, db.Table("zalo_accounts").Where("teacher_id = ?", teacherID).Count(&count).Error)
	require.EqualValues(t, 0, count, "a rejected upsert must not leave a row behind")
}

func TestGetByTeacherReportsMissingAccount(t *testing.T) {
	t.Parallel()
	repo, _, _ := newRepo(t)

	_, err := repo.GetByTeacher(context.Background(), uuid.New())
	require.ErrorIs(t, err, zalo.ErrAccountNotFound)
}

func TestDeleteHidesAccountAndRelinkRevivesIt(t *testing.T) {
	t.Parallel()
	repo, db, teacherID := newRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, account(t, teacherID, "creds")))
	require.NoError(t, repo.Delete(ctx, teacherID))

	_, err := repo.GetByTeacher(ctx, teacherID)
	require.ErrorIs(t, err, zalo.ErrAccountNotFound)

	// Soft delete: the row is still there, just stamped.
	var deletedAt *time.Time
	require.NoError(t, db.Table("zalo_accounts").
		Where("teacher_id = ?", teacherID).
		Select("deleted_at").Scan(&deletedAt).Error)
	require.NotNil(t, deletedAt)

	// Re-linking the same teacher must work despite the primary-key collision.
	require.NoError(t, repo.Upsert(ctx, account(t, teacherID, "relinked")))
	got, err := repo.GetByTeacher(ctx, teacherID)
	require.NoError(t, err)
	require.Equal(t, zalo.StatusLinked, got.Status)
}

func TestDeleteReportsMissingAccount(t *testing.T) {
	t.Parallel()
	repo, _, _ := newRepo(t)

	require.ErrorIs(t, repo.Delete(context.Background(), uuid.New()), zalo.ErrAccountNotFound)
}

func TestUpdateStatus(t *testing.T) {
	t.Parallel()
	repo, _, teacherID := newRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, account(t, teacherID, "creds")))
	require.NoError(t, repo.UpdateStatus(ctx, teacherID, zalo.StatusExpired))

	got, err := repo.GetByTeacher(ctx, teacherID)
	require.NoError(t, err)
	require.Equal(t, zalo.StatusExpired, got.Status)

	require.ErrorIs(t, repo.UpdateStatus(ctx, uuid.New(), zalo.StatusExpired), zalo.ErrAccountNotFound)
}

func TestMarkVerified(t *testing.T) {
	t.Parallel()
	repo, _, teacherID := newRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, account(t, teacherID, "creds")))
	before, err := repo.GetByTeacher(ctx, teacherID)
	require.NoError(t, err)
	require.Nil(t, before.LastVerifiedAt, "a fresh row has never been verified")

	require.NoError(t, repo.MarkVerified(ctx, teacherID))

	got, err := repo.GetByTeacher(ctx, teacherID)
	require.NoError(t, err)
	require.NotNil(t, got.LastVerifiedAt)
	require.WithinDuration(t, time.Now(), *got.LastVerifiedAt, time.Minute)

	require.ErrorIs(t, repo.MarkVerified(ctx, uuid.New()), zalo.ErrAccountNotFound)
}

// The health probe sweeps this list, so it must contain exactly the accounts
// worth spending a Zalo login on.
func TestListLinkedReturnsOnlyLiveLinkedAccounts(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	repo := zalo.NewRepository(db)
	ctx := context.Background()

	_, linkedTeacher := testutil.Teacher(t, db)
	_, expiredTeacher := testutil.Teacher(t, db)
	_, unlinkedTeacher := testutil.Teacher(t, db)

	require.NoError(t, repo.Upsert(ctx, account(t, linkedTeacher.ID, "creds")))
	require.NoError(t, repo.Upsert(ctx, account(t, expiredTeacher.ID, "creds")))
	require.NoError(t, repo.UpdateStatus(ctx, expiredTeacher.ID, zalo.StatusExpired))
	require.NoError(t, repo.Upsert(ctx, account(t, unlinkedTeacher.ID, "creds")))
	require.NoError(t, repo.Delete(ctx, unlinkedTeacher.ID))

	ids, err := repo.ListLinked(ctx)
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{linkedTeacher.ID}, ids)
}

// The schema must offer no place to put credentials in the clear.
func TestSchemaHasNoPlaintextCredentialColumn(t *testing.T) {
	t.Parallel()
	_, db, _ := newRepo(t)

	var cols []struct {
		ColumnName string
		DataType   string
	}
	require.NoError(t, db.Raw(`
		SELECT column_name, data_type FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'zalo_accounts'`).Scan(&cols).Error)

	credentialCols := 0
	for _, c := range cols {
		switch c.ColumnName {
		case "encrypted_credentials":
			credentialCols++
			require.Equal(t, "bytea", c.DataType)
		case "credentials", "imei", "cookie", "cookies", "session":
			t.Errorf("zalo_accounts exposes a plaintext credential column %q", c.ColumnName)
		}
	}
	require.Equal(t, 1, credentialCols, "encrypted_credentials must be the only credential column")
}

const httpTestSecret = "zalo-integration-secret-0123456789"

// scriptedQRLogin stands in for Zalo: it publishes a QR image, waits to be
// released, and reports a successful login. Nothing here touches the network.
type scriptedQRLogin struct {
	qr      []byte
	release chan struct{}
}

func (s *scriptedQRLogin) login(ctx context.Context, sess *protocol.Session, cb protocol.QRCallbacks) (*protocol.Credentials, error) {
	cb.OnQR(s.qr)
	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	sess.UID = "zalo-uid-1"
	sess.DisplayName = "Cô Lan"
	cookies := protocol.NewHTTPCookies([]*http.Cookie{{Name: "zpsid", Value: "integration-cookie"}})
	return &protocol.Credentials{IMEI: "integration-imei", Cookie: &cookies, UserAgent: "ua"}, nil
}

// fakeScopeResolver resolves every teacher id to a scope where it owns its
// own center — like the real resolver does for a teacher who never joined
// anyone else's center. The zalo routes only ever read TeacherID from it.
type fakeScopeResolver struct{}

func (fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	return authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}, nil
}

func mintToken(t *testing.T, teacherID uuid.UUID) string {
	t.Helper()
	claims := authctx.AccessClaims{
		Role: authctx.RoleTeacher,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   teacherID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(httpTestSecret))
	require.NoError(t, err)
	return signed
}

func call(t *testing.T, r *gin.Engine, method, path, body, token string) (int, json.RawMessage) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Body.Len() == 0 {
		return w.Code, nil
	}
	var env struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "body: %s", w.Body.String())
	if env.Error != nil {
		return w.Code, json.RawMessage(`"` + env.Error.Code + `"`)
	}
	return w.Code, env.Data
}

// The whole feature over HTTP against real SQL: link, read, and unlink, with
// the credential never leaving the database in the clear.
func TestZaloHTTPLinkLifecycleAgainstARealDatabase(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	_, owner := testutil.Teacher(t, db)
	_, other := testutil.Teacher(t, db)

	cipher, err := secrets.New(testCredKey)
	require.NoError(t, err)
	login := &scriptedQRLogin{qr: []byte("\x89PNG-not-a-real-image"), release: make(chan struct{})}
	// Linking proves the sealed credentials with a cookie login before it
	// stores them; stub that login too or the test would call Zalo for real.
	relogin := func(_ context.Context, sess *protocol.Session, _ protocol.Credentials) error {
		sess.UID = "zalo-uid-1"
		sess.LoginInfo = &protocol.LoginInfo{ZpwServiceMapV3: protocol.ZpwServiceMapV3{
			Chat:    []string{"https://chat.example"},
			Profile: []string{"https://profile.example"},
		}}
		return nil
	}
	svc := zalo.NewService(zalo.NewRepository(db), cipher, zalo.ServiceOptions{Login: login.login, Relogin: relogin})
	t.Cleanup(svc.Close)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	zalo.RegisterRoutes(
		r.Group("/api/v1"),
		zalo.NewHandler(svc),
		middleware.RequireAuth(config.JWTConfig{Secret: httpTestSecret, AccessTTL: 15 * time.Minute}),
		middleware.ResolveScope(fakeScopeResolver{}),
	)
	ownerToken, otherToken := mintToken(t, owner.ID), mintToken(t, other.ID)

	code, data := call(t, r, http.MethodGet, "/api/v1/me/zalo", "", ownerToken)
	require.Equal(t, http.StatusOK, code)
	require.JSONEq(t, `{"linked":false}`, string(data))

	code, data = call(t, r, http.MethodPost, "/api/v1/me/zalo/link/start",
		`{"consent_version":"`+testConsentVersion+`"}`, ownerToken)
	require.Equal(t, http.StatusAccepted, code)
	var started struct {
		LinkID uuid.UUID `json:"link_id"`
	}
	require.NoError(t, json.Unmarshal(data, &started))

	statusPath := "/api/v1/me/zalo/link/status?id=" + started.LinkID.String()
	waitForState := func(want string) map[string]any {
		var last map[string]any
		require.Eventually(t, func() bool {
			code, data := call(t, r, http.MethodGet, statusPath, "", ownerToken)
			require.Equal(t, http.StatusOK, code)
			last = map[string]any{}
			require.NoError(t, json.Unmarshal(data, &last))
			return last["state"] == want
		}, 5*time.Second, 10*time.Millisecond, "link never reached %q (last: %v)", want, last)
		return last
	}

	qrReady := waitForState("qr_ready")
	png, err := base64.StdEncoding.DecodeString(qrReady["qr_png"].(string))
	require.NoError(t, err)
	require.Equal(t, login.qr, png)

	// Another teacher holding the same link id learns nothing.
	code, data = call(t, r, http.MethodGet, statusPath, "", otherToken)
	require.Equal(t, http.StatusNotFound, code)
	require.JSONEq(t, `"NOT_FOUND"`, string(data))

	close(login.release)
	linked := waitForState("linked")
	require.Equal(t, "Cô Lan", linked["display_name"])
	require.NotContains(t, linked, "qr_png")

	// The row that link wrote is sealed, consented, and decryptable only here.
	stored, err := zalo.NewRepository(db).GetByTeacher(context.Background(), owner.ID)
	require.NoError(t, err)
	require.Equal(t, zalo.StatusLinked, stored.Status)
	require.Equal(t, testConsentVersion, stored.ConsentVersion)
	require.NotContains(t, string(stored.EncryptedCredentials), "integration-imei")
	plain, err := cipher.Open(stored.EncryptedCredentials)
	require.NoError(t, err)
	require.Contains(t, string(plain), "integration-imei")

	code, data = call(t, r, http.MethodGet, "/api/v1/me/zalo", "", ownerToken)
	require.Equal(t, http.StatusOK, code)
	var status map[string]any
	require.NoError(t, json.Unmarshal(data, &status))
	require.Equal(t, true, status["linked"])
	require.Equal(t, "Cô Lan", status["display_name"])
	require.Equal(t, zalo.StatusLinked, status["status"])
	require.NotEmpty(t, status["linked_at"])

	code, _ = call(t, r, http.MethodDelete, "/api/v1/me/zalo", "", ownerToken)
	require.Equal(t, http.StatusNoContent, code)

	code, data = call(t, r, http.MethodGet, "/api/v1/me/zalo", "", ownerToken)
	require.Equal(t, http.StatusOK, code)
	require.JSONEq(t, `{"linked":false}`, string(data))

	// Unlinking again is not an error: the account is already gone.
	code, _ = call(t, r, http.MethodDelete, "/api/v1/me/zalo", "", ownerToken)
	require.Equal(t, http.StatusNoContent, code)
}
