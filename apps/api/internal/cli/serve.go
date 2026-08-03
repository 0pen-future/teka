package cli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"teka/apps/api/internal/app"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the HTTP API server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		// Once shutdown starts, restore default signal handling so a second
		// Ctrl-C force-quits instead of being swallowed during the drain.
		go func() {
			<-ctx.Done()
			stop()
		}()
		return app.RunServer(ctx)
	},
}
