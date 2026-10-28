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
	"raid-website/internal/auth"
	"raid-website/internal/config"
	"raid-website/internal/db"
	"raid-website/internal/httpapi"
	"raid-website/internal/migrations"
	"raid-website/internal/push"
	"raid-website/internal/raid"
	"raid-website/internal/raidseries"
	"raid-website/internal/recruitment"
	"raid-website/internal/registration"
	"raid-website/internal/webui"
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
	if err := checkWebBuild(cfg.Env, webui.CheckBuilt); err != nil {
		return err
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
	if err := auth.NewRepo(bdb.DB).PruneSecurityRecords(ctx, time.Now().UTC()); err != nil {
		log.Printf("authentication cleanup: %v", err)
	}
	if err := detectNamingPatterns(ctx, bdb.DB); err != nil {
		return fmt.Errorf("naming pattern detection: %w", err)
	}
	if err := reportInvalidRecruitmentSlots(ctx, bdb.DB); err != nil {
		return fmt.Errorf("recruitment slot report: %w", err)
	}
	if removed, err := push.NewService(push.NewRepo(bdb.DB), nil, log.Printf).DeleteUnclaimed(ctx); err != nil {
		return fmt.Errorf("push cleanup: %w", err)
	} else if removed > 0 {
		log.Printf("push: removed %d subscription(s) that never got a device", removed)
	}
	var vapidKey *push.VAPIDKey
	if cfg.VAPIDPrivateKey != "" {
		if vapidKey, err = push.ParseVAPIDKey(cfg.VAPIDPrivateKey, cfg.VAPIDSubject); err != nil {
			return fmt.Errorf("config: %w", err)
		}
	} else {
		log.Printf("web push off: VAPID_PRIVATE_KEY is not set")
	}
	risingGods := registration.RisingGodsClient{ProxyURL: cfg.RisingGodsProxyURL, ProxyToken: cfg.RisingGodsProxyToken}
	totpEncryption, err := auth.NewTOTPEncryptionWithPrevious(cfg.TOTPEncryptionKey, cfg.TOTPEncryptionKeysPrevious)
	if err != nil {
		return err
	}
	h, err := httpapi.New(bdb.DB, httpapi.Options{
		TOTPEncryption:    totpEncryption,
		CookieSecure:      cfg.CookieSecure,
		TrustProxyHeaders: cfg.TrustProxyHeaders,
		PublicOrigin:      cfg.PublicOrigin,
		AdminUsername:     cfg.AdminUsername,
		AdminPassword:     cfg.AdminPassword,
		RegistrationFeed:  risingGods,
		VAPIDKey:          vapidKey,
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

// checkWebBuild stops a production start without the embedded frontend
// (NFR-1.1, DR-13); development keeps working without a web build.
func checkWebBuild(env string, check func() error) error {
	if env != "production" {
		return nil
	}
	if err := check(); err != nil {
		return fmt.Errorf("web build: %w", err)
	}
	return nil
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

// reportInvalidRecruitmentSlots logs stored recruitment slots the allow-list no longer
// accepts. It changes no data; the owner decides what to do with them (FR-23.4).
func reportInvalidRecruitmentSlots(ctx context.Context, db bun.IDB) error {
	invalid, err := recruitment.NewService(db).InvalidStoredSlots(ctx)
	if err != nil {
		return err
	}
	for _, slot := range invalid {
		log.Printf("recruitment template %s=%d: group %d slot %d not accepted: %q", slot.Scope, slot.ID, slot.Group, slot.Slot, slot.Value)
	}
	if len(invalid) > 0 {
		log.Printf("recruitment: %d stored slot(s) not accepted; no data was changed", len(invalid))
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
