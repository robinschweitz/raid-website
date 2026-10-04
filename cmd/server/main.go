package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"raid-website/internal/config"
	"raid-website/internal/db"
	"raid-website/internal/httpapi"
	"raid-website/internal/migrations"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (err error) {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	bdb, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer func() { err = errors.Join(err, bdb.Close()) }()
	m, err := migrations.NewMigrator(bdb.DB)
	if err != nil {
		return fmt.Errorf("migrator: %w", err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	h, err := httpapi.New(bdb.DB, httpapi.Options{
		CookieSecure:  cfg.CookieSecure,
		AdminUsername: cfg.AdminUsername,
		AdminPassword: cfg.AdminPassword,
	})
	if err != nil {
		return fmt.Errorf("http initialization: %w", err)
	}
	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
