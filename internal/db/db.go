package db

import (
	"context"
	"errors"

	"raid-website/internal/config"
)

// keep for later if needed
func Connect(ctx context.Context, cfg *config.Config) (*BunDB, error) {
	_ = ctx
	_ = cfg
	return nil, errors.New("db: Connect deprecated; use Open")
}
