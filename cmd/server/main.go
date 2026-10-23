package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/uptrace/bun"
	"raid-website/internal/config"
	"raid-website/internal/db"
	"raid-website/internal/httpapi"
	"raid-website/internal/migrations"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/registration"
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
	risingGods := registration.RisingGodsClient{ProxyURL: cfg.RisingGodsProxyURL, ProxyToken: cfg.RisingGodsProxyToken}
	h, err := httpapi.New(bdb.DB, httpapi.Options{
		CookieSecure:          cfg.CookieSecure,
		TrustProxyHeaders:     cfg.TrustProxyHeaders,
		PublicOrigin:          cfg.PublicOrigin,
		AdminUsername:         cfg.AdminUsername,
		AdminPassword:         cfg.AdminPassword,
		PrivacyContactName:    cfg.PrivacyContactName,
		PrivacyContactAddress: cfg.PrivacyContactAddress,
		RegistrationFeed:      risingGods,
	})
	if err != nil {
		return fmt.Errorf("http initialization: %w", err)
	}
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	stopCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()
	log.Printf("listening on %s", cfg.ListenAddr)
	return serve(stopCtx, newHTTPServer(h), ln, shutdownTimeout)
}

// Server limits from SECURITY.md SC-026. systemd stops the service after 30 s,
// so the shutdown grace period stays below that.
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownTimeout   = 25 * time.Second
)

func newHTTPServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// serve runs srv on ln until ctx is cancelled, then stops accepting
// connections and gives running requests up to grace to finish.
func serve(ctx context.Context, srv *http.Server, ln net.Listener, grace time.Duration) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	// The parent context is already cancelled, so the grace period needs its own.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
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
