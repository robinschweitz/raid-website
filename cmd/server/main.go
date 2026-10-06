package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/uptrace/bun"
	"raid-website/internal/config"
	"raid-website/internal/db"
	"raid-website/internal/httpapi"
	"raid-website/internal/migrations"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
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
	if err := detectNamingPatterns(ctx, bdb.DB); err != nil {
		return fmt.Errorf("naming pattern detection: %w", err)
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

func detectNamingPatterns(ctx context.Context, db bun.IDB) error {
	seriesRepo := raidseries.NewRepo(db)
	seriesList, err := seriesRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("list series: %w", err)
	}
	raidRepo := raid.NewRepo(db)
	for _, s := range seriesList {
		raids, err := raidRepo.ListBySeries(ctx, s.ID)
		if err != nil {
			return fmt.Errorf("list raids for series %d: %w", s.ID, err)
		}
		if detectPattern(&s, raids) {
			if err := seriesRepo.Save(ctx, &s); err != nil {
				return fmt.Errorf("save series %d: %w", s.ID, err)
			}
			log.Printf("detected naming pattern for series %d: %s (next: %d)", s.ID, s.NameTemplate, s.NextRaidNumber)
		}
	}
	return nil
}

func detectPattern(series *raidseries.Series, raids []raid.Raid) bool {
	if len(raids) == 0 {
		return false
	}
	if series.NameTemplate != "" && series.NameTemplate != series.Name {
		return false
	}
	prefix := ""
	maxNum := 0
	for _, r := range raids {
		name := r.Name
		idx := strings.LastIndex(name, "-")
		if idx <= 0 || idx == len(name)-1 {
			return false
		}
		numStr := name[idx+1:]
		num, err := strconv.Atoi(numStr)
		if err != nil || num <= 0 {
			return false
		}
		p := name[:idx+1]
		if prefix == "" {
			prefix = p
		} else if p != prefix {
			return false
		}
		if num > maxNum {
			maxNum = num
		}
	}
	series.NameTemplate = prefix + "{number}"
	series.NextRaidNumber = maxNum + 1
	return true
}
