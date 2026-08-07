package app

import (
	"context"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/server"
)

// RunServer is the serve entrypoint: config → container → router → HTTP
// server, blocking until ctx is cancelled.
func RunServer(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c, err := NewContainer(cfg)
	if err != nil {
		return err
	}
	defer c.Close()

	// The health probe runs for as long as the server does; c.Close stops it.
	c.Zalo.StartHealthProbe(ctx, zalo.ProbeOptions{})

	// A "running" run can only be true while this process is sending it, so
	// any found at boot belongs to a previous process that died mid-run — mark
	// them interrupted (resumable) before requests can observe them.
	if err := c.Notifications.ReconcileInterrupted(ctx); err != nil {
		return err
	}

	router := server.NewRouter(c.Cfg, c.Log, c.DB, c.Zalo, c.Statements, c.Notifications)
	return server.Run(ctx, c.Cfg, c.Log, router)
}
