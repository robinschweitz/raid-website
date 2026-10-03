package migrations

import (
	"context"
	"errors"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

func NewMigrator(db *bun.DB) (*migrate.Migrator, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	m := migrate.NewMigrator(db, migrate.NewMigrations())
	if err := m.Init(context.Background()); err != nil {
		return nil, err
	}
	return m, nil
}
