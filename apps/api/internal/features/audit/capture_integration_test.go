//go:build integration

package audit_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/audit"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/testutil"
)

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// newCapture wires the audit subscriber to a SyncBus on a real database:
// events publish synchronously, and Close flushes the buffer, so a test can
// assert rows without sleeping.
func newCapture(t *testing.T, batchSize int) (*gorm.DB, events.Bus, *audit.Subscriber) {
	t.Helper()
	db := testutil.StartPostgres(t)
	sub := audit.NewSubscriber(audit.NewRepository(db), discardLogger(), batchSize, time.Hour)
	bus := events.NewSync()
	bus.Subscribe("audit", 0, sub.Handle)
	return db, bus, sub
}

func auditRows(t *testing.T, db *gorm.DB) []audit.Log {
	t.Helper()
	var rows []audit.Log
	require.NoError(t, db.Order("occurred_at, action").Find(&rows).Error)
	return rows
}

func rowByAction(t *testing.T, rows []audit.Log, action string) audit.Log {
	t.Helper()
	for _, r := range rows {
		if r.Action == action {
			return r
		}
	}
	t.Fatalf("no row with action %q in %d rows", action, len(rows))
	return audit.Log{}
}

// TestAuthFlowsWriteAuditRows drives the real auth service through login
// success, login failure, and logout, and asserts each lands as the correct
// audit row — with the failure row carrying only a masked phone.
func TestAuthFlowsWriteAuditRows(t *testing.T) {
	db, bus, sub := newCapture(t, 100)
	ctx := context.Background()

	txMgr := database.NewTxManager(db)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	centersSvc := centers.NewService(centers.NewRepository(db), txMgr, nil)
	issuer := auth.NewTokenIssuer(config.JWTConfig{
		Secret:     testutil.JWTSecret,
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 720 * time.Hour,
	})
	onboarding := config.OnboardingConfig{ResetTTL: 48 * time.Hour, ResetCooldown: 15 * time.Minute}
	svc := auth.NewService(teachersSvc, auth.NewRepository(db), issuer, txMgr,
		centersSvc, nil, onboarding, "https://app.example.com", bus)

	const password = "s3cret-pass-123"
	acct, _ := testutil.Teacher(t, db, testutil.WithPassword(password))
	meta := auth.ClientMeta{IP: "203.0.113.9", UserAgent: "audit-itest"}

	sess, err := svc.Login(ctx, auth.LoginRequest{Phone: acct.Phone, Password: password}, meta)
	require.NoError(t, err)
	_, err = svc.Login(ctx, auth.LoginRequest{Phone: acct.Phone, Password: "wrong-password"}, meta)
	require.Error(t, err)
	require.NoError(t, svc.Logout(ctx, sess.RefreshToken, meta))

	sub.Close()

	rows := auditRows(t, db)
	require.Len(t, rows, 3)

	login := rowByAction(t, rows, "auth.login")
	require.NotNil(t, login.ActorUserID)
	require.Equal(t, acct.ID, *login.ActorUserID)
	require.Nil(t, login.CenterID, "auth events carry no center scope")
	require.Equal(t, meta.IP, login.IP)
	require.Equal(t, meta.UserAgent, login.UserAgent)
	require.False(t, login.OccurredAt.IsZero())

	fail := rowByAction(t, rows, "auth.login_fail")
	require.Nil(t, fail.ActorUserID, "failed login has no verified actor")
	masked := acct.Phone[:3] + "***" + acct.Phone[len(acct.Phone)-3:]
	require.Equal(t, audit.Metadata{"phone_masked": masked}, fail.Metadata)
	require.NotContains(t, fail.Metadata["phone_masked"], acct.Phone[3:len(acct.Phone)-3],
		"metadata must never carry the raw phone")

	logout := rowByAction(t, rows, "auth.logout")
	require.NotNil(t, logout.ActorUserID)
	require.Equal(t, acct.ID, *logout.ActorUserID)
}

// TestMutationRequestWritesOneAuditRow drives one authenticated mutating HTTP
// request through the real middleware chain and asserts exactly one mapped
// row lands, scoped to the caller's center.
func TestMutationRequestWritesOneAuditRow(t *testing.T) {
	db, bus, _ := newCapture(t, 1) // batch of 1: every event flushes on arrival

	actor := uuid.New()
	center := uuid.New()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	v1 := r.Group("/api/v1")
	v1.Use(middleware.RequestEvents(bus))
	// Stands in for RequireAuth+ResolveScope: the middleware reads identity
	// after c.Next(), exactly as in the real router where per-route auth runs
	// inside the group.
	v1.POST("/classes", func(c *gin.Context) {
		authctx.Set(c, authctx.Principal{UserID: actor, Role: "teachers"})
		authctx.SetScope(c, authctx.Scope{TeacherID: actor, CenterID: center, IsOwner: true})
		c.Next()
	}, func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/classes", nil)
	req.Header.Set("User-Agent", "audit-itest")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	rows := auditRows(t, db)
	require.Len(t, rows, 1, "one mutation must produce exactly one row")

	row := rows[0]
	require.Equal(t, "class.create", row.Action)
	require.Equal(t, "class", row.EntityType)
	require.NotNil(t, row.CenterID)
	require.Equal(t, center, *row.CenterID)
	require.NotNil(t, row.ActorUserID)
	require.Equal(t, actor, *row.ActorUserID)
	require.Equal(t, "owner", row.ActorRole)
	require.Equal(t, http.MethodPost, row.Method)
	require.Equal(t, "/api/v1/classes", row.Path)
	require.Equal(t, http.StatusCreated, row.StatusCode)
	require.NotEmpty(t, row.RequestID)
	require.Equal(t, "audit-itest", row.UserAgent)
}

// Enrollment create is audited by the service's StudentEnrolled event alone —
// the richer row carrying class and student ids. The request middleware must
// stay silent for that route, or the same request lands twice in the trail.
func TestEnrollmentCreateWritesExactlyOneAuditRow(t *testing.T) {
	db, bus, _ := newCapture(t, 1)

	actor := uuid.New()
	center := uuid.New()
	enrollmentID := uuid.New()
	classID := uuid.New()
	studentID := uuid.New()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	v1 := r.Group("/api/v1")
	v1.Use(middleware.RequestEvents(bus))
	v1.POST("/enrollments", func(c *gin.Context) {
		authctx.Set(c, authctx.Principal{UserID: actor, Role: "teachers"})
		authctx.SetScope(c, authctx.Scope{TeacherID: actor, CenterID: center, IsOwner: true})
		c.Next()
	}, func(c *gin.Context) {
		// The real handler's service publishes this event on success.
		bus.Publish(enrollments.StudentEnrolled{
			OccurredAt:   time.Now(),
			CenterID:     center,
			ActorID:      actor,
			EnrollmentID: enrollmentID,
			ClassID:      classID,
			StudentID:    studentID,
		})
		c.JSON(http.StatusCreated, gin.H{})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enrollments", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	rows := auditRows(t, db)
	require.Len(t, rows, 1, "one enrollment create must land as exactly one audit row")
	row := rows[0]
	require.Equal(t, "enrollment.create", row.Action)
	require.Equal(t, "enrollment", row.EntityType)
	require.Equal(t, enrollmentID.String(), row.EntityID)
	require.Equal(t, audit.Metadata{
		"class_id":   classID.String(),
		"student_id": studentID.String(),
	}, row.Metadata)
}

// TestShutdownDrainsAsyncBus publishes through the real AsyncBus and closes
// in the exact Container.Close order — bus.Close(ctx) drains the queue into
// the subscriber, then Subscriber.Close flushes the final batch — proving no
// row is lost at graceful shutdown. The full app.Container is not
// constructible here (testutil.StartPostgres owns its DSN), but the drain
// semantics live entirely in these two components, wired identically.
func TestShutdownDrainsAsyncBus(t *testing.T) {
	db := testutil.StartPostgres(t)
	log := discardLogger()
	sub := audit.NewSubscriber(audit.NewRepository(db), log, 100, time.Hour)
	bus := events.New(log)
	bus.Subscribe("audit", 1024, sub.Handle)

	const n = 250 // spans multiple batches, well under the buffer
	userID := uuid.New()
	for i := 0; i < n; i++ {
		bus.Publish(auth.LoggedOut{
			OccurredAt: time.Now(),
			UserID:     userID,
			IP:         "203.0.113.9",
			UserAgent:  "drain-itest",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, bus.Close(ctx))
	sub.Close()

	var count int64
	require.NoError(t, db.Model(&audit.Log{}).Count(&count).Error)
	require.EqualValues(t, n, count, "every published event must survive shutdown")
}
