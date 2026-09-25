package server

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/events"
)

// TestRegisterFeaturesWiresTaskHandover proves registerFeatures itself calls
// centersSvc.SetTaskHandover(tasksSvc) right after constructing tasksSvc —
// the wiring the architecture relies on to hand a departing member's tasks
// over during centers.Service.RemoveMember (see
// centers/rbac_integration_test.go's TestRemoveMemberHandsOverTasks for the
// end-to-end, DB-backed proof of the handover itself). This test needs no
// database: it calls registerFeatures directly, mirroring
// newTestRouterWith's construction, and only inspects the resulting wiring.
func TestRegisterFeaturesWiresTaskHandover(t *testing.T) {
	cfg := &config.Config{
		Env:      config.EnvTest,
		LogLevel: "info",
		HTTP:     config.HTTPConfig{Port: 0, MaxBodyBytes: 1 << 20},
		Database: config.DatabaseConfig{ConnMaxLifetime: time.Minute},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	zaloSvc := newTestZaloService(t)
	statementsSvc := statements.NewService(statements.NewRepository(nil), database.NewTxManager(nil),
		cfg.Statements, statements.BankConfig{}, statements.NewQRBuilder())
	notificationsSvc := notifications.NewService(notifications.NewRepository(nil), database.NewTxManager(nil),
		statementsSvc, zaloSvc, log, cfg.Notifications)
	t.Cleanup(notificationsSvc.Close)

	txMgr := database.NewTxManager(nil)
	teachersSvc := teachers.NewService(teachers.NewRepository(nil))
	centersSvc := centers.NewService(centers.NewRepository(nil), txMgr, nil)
	authSvc := auth.NewService(teachersSvc, auth.NewRepository(nil), auth.NewTokenIssuer(cfg.JWT), txMgr,
		centersSvc, zaloSvc, cfg.Onboarding, cfg.Statements.PublicBaseURL, nil)
	centersSvc.SetAccountDisabler(authSvc)
	teachersSvc.SetTokenRevoker(authSvc)

	if centersSvc.TaskHandoverWired() {
		t.Fatal("centersSvc must start with no TaskHandover wired")
	}

	v1 := gin.New().Group("/api/v1")
	registerFeatures(v1, cfg, log, nil, zaloSvc, statementsSvc, notificationsSvc, teachersSvc, centersSvc, authSvc, events.NewSync())

	if !centersSvc.TaskHandoverWired() {
		t.Error("registerFeatures must wire tasksSvc into centersSvc via SetTaskHandover")
	}
	if !centersSvc.ClassInviteCancellerWired() {
		t.Error("registerFeatures must wire classInvitesSvc into centersSvc via SetClassInviteCanceller")
	}
}
