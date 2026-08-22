package imports

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/authctx"
)

const handlerTestSecret = "imports-test-secret-0123456789abcdef"

// ownerID and memberID are fixed so the fake resolver can decide the caller's
// role from the token subject alone.
var (
	ownerID  = uuid.New()
	memberID = uuid.New()
	centerID = uuid.New()
)

type fakeScopeResolver struct{}

func (fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	return authctx.Scope{TeacherID: teacherID, CenterID: centerID, IsOwner: teacherID == ownerID}, nil
}

// countingDirectory records whether the member directory was consulted, which
// is how the tests below prove the owner gate runs before any work.
type countingDirectory struct {
	calls int
	dir   map[string]uuid.UUID
}

func (d *countingDirectory) MemberIDsByPhone(_ context.Context, _ authctx.Scope) (map[string]uuid.UUID, error) {
	d.calls++
	return d.dir, nil
}

func newHTTPTest(t *testing.T) (*gin.Engine, *countingDirectory) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := newTestDirectory()
	svc, _ := newTestService(dir, newFakeRoster())
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc),
		middleware.RequireAuth(jwtCfg), middleware.ResolveScope(fakeScopeResolver{}),
		// A permissive limiter: these tests exercise the routes, not the limit.
		middleware.RateLimit(middleware.TeacherKey(), 1000, time.Minute))
	return r, dir
}

// newTestDirectory returns a directory holding both teacher phones the sample
// workbook references.
func newTestDirectory() *countingDirectory {
	return &countingDirectory{dir: map[string]uuid.UUID{namPhone: uuid.New(), lanPhone: uuid.New()}}
}

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

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
	} `json:"error"`
}

// upload posts a multipart body. content is sent verbatim so a test can send
// something that is not a workbook at all.
func upload(t *testing.T, r *gin.Engine, token string, content []byte, dryRun string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "roster.xlsx")
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	if dryRun != "" {
		require.NoError(t, w.WriteField("dry_run", dryRun))
	}
	require.NoError(t, w.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/roster", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var env envelope
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
	}
	return rec, env
}

func getTemplate(t *testing.T, r *gin.Engine, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/imports/roster/template", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestRoutesRequireAuthentication(t *testing.T) {
	t.Parallel()
	r, _ := newHTTPTest(t)

	require.Equal(t, http.StatusUnauthorized, getTemplate(t, r, "").Code)

	rec, _ := upload(t, r, "", validWorkbook(t), "true")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRoutesRejectMembers(t *testing.T) {
	t.Parallel()
	r, dir := newHTTPTest(t)
	token := mintToken(t, memberID)

	require.Equal(t, http.StatusForbidden, getTemplate(t, r, token).Code)

	rec, env := upload(t, r, token, validWorkbook(t), "true")
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "FORBIDDEN", env.Error.Code)
	require.Zero(t, dir.calls,
		"the owner gate must run before any parsing or directory read, so the endpoint is not a workbook oracle for members")
}

func TestTemplateStreamsWorkbookOutsideEnvelope(t *testing.T) {
	t.Parallel()
	r, _ := newHTTPTest(t)
	rec := getTemplate(t, r, mintToken(t, ownerID))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, xlsxContentType, rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Header().Get("Content-Disposition"), ".xlsx")
	require.NotEmpty(t, rec.Body.Bytes())

	// A binary stream, not a JSON envelope — and it must be a real workbook.
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	require.ElementsMatch(t, []string{SheetClasses, SheetStudents}, f.GetSheetList())
}

func TestDryRunReportsCountsWithoutCommitting(t *testing.T) {
	t.Parallel()
	r, _ := newHTTPTest(t)
	rec, env := upload(t, r, mintToken(t, ownerID), validWorkbook(t), "true")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var rep Report
	require.NoError(t, json.Unmarshal(env.Data, &rep))
	require.False(t, rep.Committed)
	require.Equal(t, 2, rep.Classes.Created)
	require.Equal(t, 3, rep.Schedules.Created)
	require.Equal(t, 3, rep.Contacts.Created, "one parent under two teachers is two contacts")
	require.Equal(t, 3, rep.Students.Created)
	require.Equal(t, 3, rep.Enrollments.Created)
}

func TestDryRunDefaultsToTrueWhenFlagAbsent(t *testing.T) {
	t.Parallel()
	r, _ := newHTTPTest(t)
	// A missing or malformed flag must never be read as "write to the
	// database"; the safe reading is the checking one.
	for _, flag := range []string{"", "maybe", "TRUE"} {
		rec, _ := upload(t, r, mintToken(t, ownerID), validWorkbook(t), flag)
		require.Equal(t, http.StatusOK, rec.Code, "flag %q", flag)
	}
}

func TestInvalidRowsReturn422WithDetails(t *testing.T) {
	t.Parallel()
	r, _ := newHTTPTest(t)
	bad := buildWorkbook(t, map[string][][]string{
		SheetClasses: {
			classHeaders,
			exampleClassRow,
			{"", "0912345678", "31/02/2025", "150.000", "8", "18:00", "", ""},
		},
		SheetStudents: {studentHeaders, exampleStudentRow},
	})
	rec, env := upload(t, r, mintToken(t, ownerID), bad, "true")

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	require.Equal(t, "VALIDATION_ERROR", env.Error.Code)
	require.NotNil(t, env.Error.Details, "the row list rides in details; Fields cannot carry sheet+line+column")

	var payload ErrorsPayload
	require.NoError(t, json.Unmarshal(env.Error.Details, &payload))
	require.NotEmpty(t, payload.Errors)
	for _, e := range payload.Errors {
		require.NotEmpty(t, e.Sheet)
		require.NotZero(t, e.Line)
		require.NotEmpty(t, e.Code)
		require.NotEmpty(t, e.Message)
	}
}

func TestNonWorkbookUploadIsRejectedOnContent(t *testing.T) {
	t.Parallel()
	r, _ := newHTTPTest(t)
	// The filename says .xlsx; only the content decides.
	rec, env := upload(t, r, mintToken(t, ownerID), []byte("Tên lớp,SĐT\nToán 9A,0912345678\n"), "true")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "BAD_REQUEST", env.Error.Code)
}

func TestOversizeUploadIsRejected(t *testing.T) {
	t.Parallel()
	r, dir := newHTTPTest(t)
	rec, _ := upload(t, r, mintToken(t, ownerID), bytes.Repeat([]byte("A"), maxUploadBytes+1024), "true")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Zero(t, dir.calls, "an oversize body is refused before any work is done on it")
}

func TestMissingFileFieldIsRejected(t *testing.T) {
	t.Parallel()
	r, _ := newHTTPTest(t)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.WriteField("dry_run", "true"))
	require.NoError(t, w.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/roster", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+mintToken(t, ownerID))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCommitReportsWhatItWrote(t *testing.T) {
	t.Parallel()
	r, _ := newHTTPTest(t)
	rec, env := upload(t, r, mintToken(t, ownerID), validWorkbook(t), "false")
	require.Equal(t, http.StatusOK, rec.Code)

	var rep Report
	require.NoError(t, json.Unmarshal(env.Data, &rep))
	require.True(t, rep.Committed)
	require.Equal(t, 2, rep.Classes.Created)
	require.Equal(t, 3, rep.Schedules.Created)
	require.Equal(t, 3, rep.Students.Created)
	require.Equal(t, 3, rep.Enrollments.Created)
}

func TestRateLimitAppliesPerTeacher(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	svc, _ := newTestService(newTestDirectory(), newFakeRoster())
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc),
		middleware.RequireAuth(jwtCfg), middleware.ResolveScope(fakeScopeResolver{}),
		middleware.RateLimit(middleware.TeacherKey(), 1, time.Minute))

	token := mintToken(t, ownerID)
	rec, _ := upload(t, r, token, validWorkbook(t), "true")
	require.Equal(t, http.StatusOK, rec.Code)
	rec, _ = upload(t, r, token, validWorkbook(t), "true")
	require.Equal(t, http.StatusTooManyRequests, rec.Code,
		"the upload is the most expensive endpoint and the pool is shared across tenants")

	// The template route is cheap and deliberately unlimited.
	require.Equal(t, http.StatusOK, getTemplate(t, r, token).Code)
}
