package classprogram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/library"
	"teka/apps/api/internal/features/teaching"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

const handlerTestSecret = "classprogram-test-secret-0123456789abcdef"

// The two apply request cases under test never reach the service (binding
// fails in the handler first), so every collaborator is a bare stub that
// only needs to satisfy its interface.

type stubClassSource struct{}

func (stubClassSource) GetReadable(_ context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error) {
	return &classes.Class{ID: classID, TeacherID: sc.TeacherID, CenterID: sc.CenterID}, nil
}

type stubCurriculumStore struct{}

func (stubCurriculumStore) GetCurriculum(_ context.Context, _ authctx.Scope, _ uuid.UUID) (*teaching.CurriculumResponse, error) {
	return &teaching.CurriculumResponse{}, nil
}

func (stubCurriculumStore) LockCurriculum(_ context.Context, _ authctx.Scope, _ uuid.UUID) error {
	return nil
}

func (stubCurriculumStore) PutCurriculum(_ context.Context, _ authctx.Scope, _ uuid.UUID, _ teaching.PutCurriculumRequest) (*teaching.CurriculumResponse, error) {
	return &teaching.CurriculumResponse{}, nil
}

type stubLibrarySource struct{}

func (stubLibrarySource) PublishedVersion(_ context.Context, _ authctx.Scope, _ uuid.UUID) (*library.VersionResponse, []library.LessonDetailResponse, error) {
	return &library.VersionResponse{}, nil, nil
}

func (stubLibrarySource) ReleasedVersion(_ context.Context, _ authctx.Scope, _ uuid.UUID) (*library.VersionResponse, []library.LessonDetailResponse, error) {
	return &library.VersionResponse{}, nil, nil
}

func (stubLibrarySource) LockTemplateForVersion(_ context.Context, _ authctx.Scope, _ uuid.UUID) error {
	return nil
}

type stubRepository struct{}

func (stubRepository) Get(_ context.Context, _ authctx.Scope, _ uuid.UUID) (*ProgramRow, error) {
	return nil, nil
}

func (stubRepository) Upsert(_ context.Context, _ *Program) error { return nil }

func (stubRepository) Delete(_ context.Context, _ authctx.Scope, _ uuid.UUID) (bool, error) {
	return false, nil
}

type stubTxManager struct{}

func (stubTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// fakeScopeResolver resolves every teacher as the owner of their own center —
// enough for handler-level binding cases, which never reach an authz check.
type fakeScopeResolver struct{}

func (fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	return authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}, nil
}

func newClassProgramHTTPTest(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc := NewService(stubRepository{}, stubClassSource{}, stubCurriculumStore{}, stubLibrarySource{}, stubTxManager{})
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc), middleware.RequireAuth(jwtCfg), middleware.ResolveScope(fakeScopeResolver{}))
	return r
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
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return signed
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields"`
	} `json:"error"`
}

func do(t *testing.T, r *gin.Engine, method, path, body, token string) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env envelope
	if len(w.Body.Bytes()) > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatalf("response is not an envelope: %v\nbody: %s", err, w.Body.String())
		}
	}
	return w, env
}

// TestApplyMissingTemplateVersionIdOverHTTP proves a body without
// template_version_id never reaches the service: gin's binding:"required"
// on the zero-value uuid.UUID fails first, with a field-scoped 422.
func TestApplyMissingTemplateVersionIdOverHTTP(t *testing.T) {
	r := newClassProgramHTTPTest(t)
	token := mintToken(t, uuid.New())
	path := "/api/v1/classes/" + uuid.NewString() + "/program"

	w, env := do(t, r, http.MethodPut, path, `{}`, token)

	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != apperror.CodeValidation {
		t.Fatalf("want 422 %s, got %d %+v", apperror.CodeValidation, w.Code, env)
	}
	if env.Error.Fields["template_version_id"] == "" {
		t.Fatalf("want a template_version_id field error, got %+v", env.Error.Fields)
	}
}

// TestApplyMalformedTemplateVersionIdOverHTTP proves a malformed uuid fails
// at JSON decoding (uuid.UUID's UnmarshalText), before gin's validator even
// runs, and surfaces as a plain bad request rather than a field error.
func TestApplyMalformedTemplateVersionIdOverHTTP(t *testing.T) {
	r := newClassProgramHTTPTest(t)
	token := mintToken(t, uuid.New())
	path := "/api/v1/classes/" + uuid.NewString() + "/program"

	w, env := do(t, r, http.MethodPut, path, `{"template_version_id":"not-a-uuid"}`, token)

	if w.Code != http.StatusBadRequest || env.Error == nil || env.Error.Code != apperror.CodeBadRequest {
		t.Fatalf("want 400 %s, got %d %+v", apperror.CodeBadRequest, w.Code, env)
	}
}

// TestApplyMalformedClassIdIs404OverHTTP proves a malformed class id in the
// path never reaches binding: classID's uuid.Parse fails first.
func TestApplyMalformedClassIdIs404OverHTTP(t *testing.T) {
	r := newClassProgramHTTPTest(t)
	token := mintToken(t, uuid.New())

	w, env := do(t, r, http.MethodPut, "/api/v1/classes/not-a-uuid/program", `{}`, token)

	if w.Code != http.StatusNotFound || env.Error == nil || env.Error.Code != apperror.CodeNotFound {
		t.Fatalf("want 404 %s, got %d %+v", apperror.CodeNotFound, w.Code, env)
	}
}
