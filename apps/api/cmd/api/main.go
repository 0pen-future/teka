// The api binary is the single entrypoint for the backend: HTTP server,
// migrations, seeding, and admin operations, dispatched via Cobra subcommands.
package main

import "teka/apps/api/internal/cli"

func main() {
	cli.Execute()
}
