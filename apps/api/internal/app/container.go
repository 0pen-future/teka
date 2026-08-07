// Package app owns application bootstrap: dependency construction and
// lifecycle. Wiring is explicit constructor injection — no DI framework.
package app

import (
	"log/slog"

	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/shared/logger"
	"teka/apps/api/internal/shared/secrets"
)

// Container holds the app-wide dependencies shared by HTTP and CLI entrypoints.
type Container struct {
	Cfg *config.Config
	Log *slog.Logger
	DB  *gorm.DB
	// Zalo is the one feature service built here rather than in the router:
	// it owns background goroutines — link attempts and the session health
	// probe — whose lifetime is the process's, and Close is where they stop.
	Zalo *zalo.Service
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

	return &Container{Cfg: cfg, Log: log, DB: db, Zalo: zaloSvc}, nil
}

// Close releases held resources: background Zalo work first, so nothing is
// still using the database connection pool when it goes away.
func (c *Container) Close() {
	c.Zalo.Close()
	if err := database.Close(c.DB); err != nil {
		c.Log.Error("closing database", "error", err)
	}
}
