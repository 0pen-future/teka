// The api binary is the single entrypoint for the backend: HTTP server,
// migrations, seeding, and admin operations, dispatched via Cobra subcommands.
package main

import "teka/apps/api/internal/cli"

// Stamped by production builds: -ldflags "-X main.version=<git sha>".
var version = "dev"

// OpenAPI root metadata (rendered at /swagger outside production).
//
//	@title						Teka API
//	@version					1.0
//	@description				Backend API for Teka. Every response uses the {success, data, meta, error} envelope.
//	@BasePath					/api/v1
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer" followed by a space and the access token.
func main() {
	cli.Execute(version)
}
