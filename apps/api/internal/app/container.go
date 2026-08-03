// Package app owns application bootstrap: dependency construction and
// lifecycle. Wiring is explicit constructor injection — no DI framework.
package app

import (
	"log/slog"

	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/logger"
)

// Container holds the app-wide dependencies shared by HTTP and CLI entrypoints.
type Container struct {
	Cfg *config.Config
	Log *slog.Logger
	DB  *gorm.DB
}

// NewContainer builds the dependency graph: logger first, then database.
func NewContainer(cfg *config.Config) (*Container, error) {
	log := logger.New(cfg.IsProduction(), cfg.SlogLevel())
	slog.SetDefault(log)

	db, err := database.Open(cfg.Database)
	if err != nil {
		return nil, err
	}

	return &Container{Cfg: cfg, Log: log, DB: db}, nil
}

// Close releases held resources, currently the database connection pool.
func (c *Container) Close() {
	if err := database.Close(c.DB); err != nil {
		c.Log.Error("closing database", "error", err)
	}
}
