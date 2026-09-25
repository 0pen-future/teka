package classinvites

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
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

const handlerTestSecret = "classinvites-test-secret-0123456789abcdef"

// fakeScopeResolver resolves the owner id as the owner of the shared center
// and everyone else as a plain member of it.
type fakeScopeResolver struct {
	center  uuid.UUID
	ownerID uuid.UUID
}

func (f fakeScopeResolver) ResolveScope(_ context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	return authctx.Scope{TeacherID: teacherID, CenterID: f.center, IsOwner: teacherID == f.ownerID}, nil
}

func newHTTPTest(t *testing.T) (*gin.Engine, *testDeps) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	svc, deps := newTestService()
	r := gin.New()
	jwtCfg := config.JWTConfig{Secret: handlerTestSecret, AccessTTL: 15 * time.Minute}
	RegisterRoutes(r.Group("/api/v1"), NewHandler(svc),
		middleware.RequireAuth(jwtCfg),
		middleware.ResolveScope(fakeScopeResolver{center: deps.center, ownerID: deps.ownerID}))
	return r, deps
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

func TestAllRoutesRequireAuth(t *testing.T) {
	r, _ := newHTTPTest(t)
	someID := uuid.NewString()
	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/classes/" + someID + "/invitations"},
		{http.MethodGet, "/api/v1/class-invitations"},
		{http.MethodPost, "/api/v1/class-invitations/" + someID + "/accept"},
		{http.MethodPost, "/api/v1/class-invitations/" + someID + "/decline"},
		{http.MethodPost, "/api/v1/class-invitations/" + someID + "/cancel"},
		{http.MethodPost, "/api/v1/class-invitations/" + someID + "/remind"},
		{http.MethodPost, "/api/v1/class-invitations/" + someID + "/confirm"},
	}
	for _, route := range routes {
		w, env := do(t, r, route.method, route.path, "", "")
		if w.Code != http.StatusUnauthorized || env.Error == nil || env.Error.Code != apperror.CodeUnauthorized {
			t.Fatalf("%s %s: want 401, got %d %+v", route.method, route.path, w.Code, env)
		}
	}
}

func TestSendAcceptConfirmFlow(t *testing.T) {
	r, d := newHTTPTest(t)
	owner := mintToken(t, d.ownerID)
	member := mintToken(t, d.memberID)

	body := `{"teacher_id":"` + d.memberID.String() + `","role_key":"tro_giang","message":"Nhờ em hỗ trợ"}`
	w, env := do(t, r, http.MethodPost, "/api/v1/classes/"+d.classID.String()+"/invitations", body, owner)
	if w.Code != http.StatusCreated {
		t.Fatalf("send: want 201, got %d %+v", w.Code, env)
	}
	var created InvitationResponse
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusPending || created.Message == nil || *created.Message != "Nhờ em hỗ trợ" {
		t.Fatalf("unexpected created %+v", created)
	}

	w, env = do(t, r, http.MethodGet, "/api/v1/class-invitations?status=pending", "", member)
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d %+v", w.Code, env)
	}
	var listed []InvitationResponse
	if err := json.Unmarshal(env.Data, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("member must see the invitation, got %+v", listed)
	}

	w, env = do(t, r, http.MethodPost, "/api/v1/class-invitations/"+created.ID.String()+"/accept", "", member)
	if w.Code != http.StatusOK {
		t.Fatalf("accept: want 200, got %d %+v", w.Code, env)
	}

	w, env = do(t, r, http.MethodPost, "/api/v1/class-invitations/"+created.ID.String()+"/confirm", "", owner)
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: want 200, got %d %+v", w.Code, env)
	}
	var confirmed ConfirmResponse
	if err := json.Unmarshal(env.Data, &confirmed); err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != StatusAssigned || len(d.assigner.assigns) != 1 {
		t.Fatalf("confirm must assign the stint, got %+v", confirmed)
	}
}

func TestSendValidationAndPathErrors(t *testing.T) {
	r, d := newHTTPTest(t)
	owner := mintToken(t, d.ownerID)

	w, env := do(t, r, http.MethodPost, "/api/v1/classes/"+d.classID.String()+"/invitations", `{"role_key":"hoc_vu"}`, owner)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Fields["teacher_id"] == "" {
		t.Fatalf("missing teacher_id: want 422 with field error, got %d %+v", w.Code, env)
	}

	w, _ = do(t, r, http.MethodPost, "/api/v1/classes/not-a-uuid/invitations", `{"teacher_id":"`+d.memberID.String()+`","role_key":"hoc_vu"}`, owner)
	if w.Code != http.StatusNotFound {
		t.Fatalf("bad class id: want 404, got %d", w.Code)
	}

	w, env = do(t, r, http.MethodPost, "/api/v1/classes/"+d.classID.String()+"/invitations", `{"teacher_id":"`+d.ownerID.String()+`","role_key":"hoc_vu"}`, owner)
	if w.Code != http.StatusUnprocessableEntity || env.Error == nil || env.Error.Code != CodeSelfInvite {
		t.Fatalf("self invite: want 422 %s, got %d %+v", CodeSelfInvite, w.Code, env)
	}

	w, _ = do(t, r, http.MethodGet, "/api/v1/class-invitations?class_id=abc", "", owner)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad class_id filter: want 422, got %d", w.Code)
	}

	w, _ = do(t, r, http.MethodPost, "/api/v1/class-invitations/nope/cancel", "", owner)
	if w.Code != http.StatusNotFound {
		t.Fatalf("bad invitation id: want 404, got %d", w.Code)
	}
}

func TestOwnerOnlyRoutesForbidMember(t *testing.T) {
	r, d := newHTTPTest(t)
	member := mintToken(t, d.memberID)
	inv := d.pending(d.memberID, authctx.StaffRoleTroGiang)
	for _, action := range []string{"cancel", "remind", "confirm"} {
		w, _ := do(t, r, http.MethodPost, "/api/v1/class-invitations/"+inv.ID.String()+"/"+action, "", member)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s by member: want 403, got %d", action, w.Code)
		}
	}
	w, _ := do(t, r, http.MethodPost, "/api/v1/classes/"+d.classID.String()+"/invitations", `{"teacher_id":"`+d.ownerID.String()+`","role_key":"hoc_vu"}`, member)
	if w.Code != http.StatusForbidden {
		t.Fatalf("send by member: want 403, got %d", w.Code)
	}
}
