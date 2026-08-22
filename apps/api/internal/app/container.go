// Package app owns application bootstrap: dependency construction and
// lifecycle. Wiring is explicit constructor injection — no DI framework.
package app

import (
	"log/slog"

	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/zalo"
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

	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	centersSvc := centers.NewService(centers.NewRepository(db), txMgr)
	// centersSvc is auth's OwnerResolver (owner-exclusion + DM anchor for
	// forgot-password) and zaloSvc is its ResetDMSender — both already exist
	// by this point, so these are plain constructor parameters, not setters.
	// Reset links reuse Statements.PublicBaseURL rather than a second
	// base-URL env var.
	authSvc := auth.NewService(teachersSvc, auth.NewRepository(db), auth.NewTokenIssuer(cfg.JWT), txMgr,
		centersSvc, zaloSvc, cfg.Onboarding, cfg.Statements.PublicBaseURL)
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
	}, nil
}

// Close releases held resources: notification runs first (they send through
// zalo), then zalo's own background work, so nothing is still using the
// database connection pool when it goes away.
func (c *Container) Close() {
	c.Notifications.Close()
	c.Zalo.Close()
	if err := database.Close(c.DB); err != nil {
		c.Log.Error("closing database", "error", err)
	}
}
