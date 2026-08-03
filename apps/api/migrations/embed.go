// Package migrations embeds the versioned SQL schema migrations so the
// compiled binary can migrate without external files.
package migrations

import "embed"

// FS holds every *.sql migration, consumed via the iofs source driver.
//
//go:embed *.sql
var FS embed.FS
