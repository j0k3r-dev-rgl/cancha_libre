package schema

import "embed"

// Files holds the apply-on-reset SQL files for schema_identity.
//
//go:embed *.sql
var Files embed.FS
