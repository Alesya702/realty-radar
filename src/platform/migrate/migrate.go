// Package migrate runs the SQL migrations embedded from the top-level
// migrations directory using goose.
package migrate

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"realtyradar/migrations"
)

// NewProvider returns a goose provider bound to db and the embedded migrations.
func NewProvider(db *sql.DB) (*goose.Provider, error) {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return nil, fmt.Errorf("create migration provider: %w", err)
	}
	return provider, nil
}
