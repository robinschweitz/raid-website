package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"raid-website/internal/config"
)

// Bun not yet wired; placeholder for later. Keep minimal.
type DB struct {
	// placeholder
}

func Connect(ctx context.Context, cfg *config.Config) (*sql.DB, error) {
	_ = cfg
	return nil, errors.New("db: connect not implemented yet")
}

func NowUTC() time.Time {
	return time.Now().UTC()
}
