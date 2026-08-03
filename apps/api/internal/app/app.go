package app

import (
	"context"

	"teka/apps/api/internal/config"
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

	router := server.NewRouter(c.Cfg, c.Log, c.DB)
	return server.Run(ctx, c.Cfg, c.Log, router)
}
