// Package app owns application bootstrap: dependency construction and
// lifecycle. Wiring is explicit constructor injection — no DI framework.
package app

import (
	"context"
	"log/slog"

	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/audit"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/shared/logger"
	"teka/apps/api/internal/shared/secrets"
)

// Container holds the app-wide dependencies shared by HTTP and CLI entrypoints.
type Container struct {
	Cfg *config.Config
	Log *slog.Logger
	DB  *gorm.DB
	// Zalo, Statements, and Notifications are built here rather than in the
	// router because their lifetime is the process's, not a request's: zalo
	// owns link attempts and the session health probe, notifications owns the
	// background zalo_personal send runs (and consumes both of the others),
	// and Close is where that background work stops.
	Zalo          *zalo.Service
	Statements    *statements.Service
	Notifications *notifications.Service
	// Teachers, Centers, and Auth are built here — not left to
	// server.registerFeatures alone — because the operator CLI's onboarding
	// commands (create-center, reset-password) need the exact same identity
	// wiring the HTTP server uses, including the SetAccountDisabler/
	// SetTokenRevoker cross-wiring below; building it twice would risk the
	// two paths drifting apart. server.NewRouter takes these three as
	// parameters instead of constructing its own, the same way it already
	// receives Zalo/Statements/Notifications pre-built.
	Teachers  *teachers.Service
	Centers   *centers.Service
	Auth      *auth.Service
	TxManager database.TxManager
	// Bus is the in-process event bus mutating requests and auth flows
	// publish into; the audit subscriber below is its first consumer. Both
	// live here because their lifetime is the process's: Close must drain the
	// bus into the subscriber, then flush the subscriber, before the database
	// pool goes away.
	Bus   events.Bus
	Audit *audit.Subscriber
}

// NewContainer builds the dependency graph: logger first, then database.
func NewContainer(cfg *config.Config) (*Container, error) {
	log := logger.New(cfg.IsProduction(), cfg.SlogLevel())
	slog.SetDefault(log)

	// Before the database, because there is nothing to close yet if it fails:
	// a credential key this cipher rejects would make every linked Zalo
	// account unreadable, which is a reason not to start rather than a
	// runtime error to discover later.
	cipher, err := secrets.New(cfg.Zalo.CredKey)
	if err != nil {
		return nil, err
	}

	db, err := database.Open(cfg.Database)
	if err != nil {
		return nil, err
	}

	zaloSvc := zalo.NewService(zalo.NewRepository(db), cipher, zalo.ServiceOptions{Logger: log})

	txMgr := database.NewTxManager(db)

	bus := events.New(log)
	auditSub := audit.NewSubscriber(audit.NewRepository(db), log, cfg.Audit.BatchSize, cfg.Audit.FlushInterval)
	bus.Subscribe("audit", cfg.Audit.BufferSize, auditSub.Handle)

	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	centersSvc := centers.NewService(centers.NewRepository(db), txMgr)
	// centersSvc is auth's OwnerResolver (owner-exclusion + DM anchor for
	// forgot-password) and zaloSvc is its ResetDMSender — both already exist
	// by this point, so these are plain constructor parameters, not setters.
	// Reset links reuse Statements.PublicBaseURL rather than a second
	// base-URL env var.
	authSvc := auth.NewService(teachersSvc, auth.NewRepository(db), auth.NewTokenIssuer(cfg.JWT), txMgr,
		centersSvc, zaloSvc, cfg.Onboarding, cfg.Statements.PublicBaseURL, bus)
	// authSvc is the AccountDisabler centers consumes to offboard a removed
	// member (disable + revoke tokens) and the TokenRevoker teachers consumes
	// to invalidate old sessions on reactivate — setters, not constructor
	// parameters, because authSvc itself depends on teachersSvc as its
	// AccountService; a direct parameter here would cycle.
	centersSvc.SetAccountDisabler(authSvc)
	teachersSvc.SetTokenRevoker(authSvc)

	bankCfg := statements.BankConfig{
		BankCode:      cfg.Bank.BankCode,
		AccountNumber: cfg.Bank.AccountNumber,
		AccountName:   cfg.Bank.AccountName,
	}
	statementsSvc := statements.NewService(statements.NewRepository(db), txMgr, cfg.Statements, bankCfg, statements.NewQRBuilder())

	notificationsSvc := notifications.NewService(notifications.NewRepository(db), txMgr, statementsSvc, zaloSvc, log, cfg.Notifications)

	return &Container{
		Cfg:           cfg,
		Log:           log,
		DB:            db,
		Zalo:          zaloSvc,
		Statements:    statementsSvc,
		Notifications: notificationsSvc,
		Teachers:      teachersSvc,
		Centers:       centersSvc,
		Auth:          authSvc,
		TxManager:     txMgr,
		Bus:           bus,
		Audit:         auditSub,
	}, nil
}

// Close releases held resources: notification runs first (they send through
// zalo), then zalo's own background work; then the event bus drains its
// queues into the audit subscriber, whose own Close flushes the last batch —
// strictly in that order, and both before the database connection pool goes
// away, so nothing is still using it and no audit row is silently dropped.
func (c *Container) Close() {
	c.Notifications.Close()
	c.Zalo.Close()
	ctx, cancel := context.WithTimeout(context.Background(), c.Cfg.Audit.DrainTimeout)
	defer cancel()
	if err := c.Bus.Close(ctx); err != nil {
		c.Log.Error("draining event bus", "error", err)
	}
	c.Audit.Close()
	if err := database.Close(c.DB); err != nil {
		c.Log.Error("closing database", "error", err)
	}
}
