// Package migrations embeds the versioned SQL migrations so the service and tests
// apply exactly the files committed to the repository.
package migrations

import "embed"

// FS holds every *.sql migration file.
//
//go:embed *.sql
var FS embed.FS
