package migrations

import (
	"context"
	"embed"
	"errors"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

//go:embed *.sql
var MigrationFiles embed.FS

func NewMigrator(db *bun.DB) (*migrate.Migrator, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	ms := migrate.NewMigrations()
	if err := ms.Discover(MigrationFiles); err != nil {
		return nil, err
	}
	m := migrate.NewMigrator(db, ms)
	if err := m.Init(context.Background()); err != nil {
		return nil, err
	}
	return m, nil
}
