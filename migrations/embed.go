// Package migrations embeds the SQL migrations of all modules.
//
// File naming: <UTC timestamp YYYYMMDDHHMMSS>_<module>_<description>.sql.
// See CLAUDE.md, section "Миграции".
package migrations

import "embed"

// FS contains every *.sql migration in this directory.
//
//go:embed *.sql
var FS embed.FS
