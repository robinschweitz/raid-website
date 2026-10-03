package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"raid-website/internal/config"
	"raid-website/internal/db"
	"raid-website/internal/httpapi"
	"raid-website/internal/migrations"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	bdb, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer bdb.Close()
	m, err := migrations.NewMigrator(bdb.DB)
	if err != nil {
		log.Fatalf("migrator: %v", err)
	}
	if _, err := m.Migrate(ctx); err != nil {
		log.Printf("migrate: %v", err)
	}
	h := httpapi.New(bdb.DB)
	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
