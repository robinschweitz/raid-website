package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/auth"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	v1 "raid-website/internal/httpapi/v1"
	"raid-website/internal/legacyimport"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/poll"
	"raid-website/internal/raid"
	"raid-website/internal/raidcompletion"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/roster"
	"raid-website/internal/tokens"
	"raid-website/internal/webui"
)

// Options controls optional runtime behaviour of the HTTP layer.
type Options struct {
	CookieSecure  bool
	AdminUsername string
	AdminPassword string
}

// New constructs the HTTP handler tree with all v1 dependencies wired.
func New(db *bun.DB, opts Options) (http.Handler, error) {
	players := player.NewRepo(db)
	chars := character.NewRepo(db)
	raids := raid.NewRepo(db)
	att := attendance.NewRepo(db)
	lootRepo := loot.NewRepo(db)
	tokenRepo := tokens.NewRepo(db)
	points := enhancepoints.NewRepo(db)
	rosterRepo := roster.NewRepo(db)
	polls := poll.NewRepo(db)
	audits := audit.NewRepo(db)

	authSvc := auth.NewService(players, auth.NewRepo(db))

	importsSvc := legacyimport.NewService(
		legacyimport.NewRepo(db),
		players,
		chars,
		raids,
		att,
		lootRepo,
		points,
		tokenRepo,
		legacyimport.GoogleFetcher{},
	)

	if err := authSvc.BootstrapAdmin(context.Background(), opts.AdminUsername, opts.AdminPassword); err != nil {
		return nil, fmt.Errorf("bootstrap admin: %w", err)
	}

	api := v1.NewAPI(v1.Deps{
		DB:           db,
		Auth:         authSvc,
		CookieSecure: opts.CookieSecure,
		Players:      players,
		Characters:   chars,
		Raids:        raids,
		Attendance:   att,
		Loot:         loot.NewService(lootRepo),
		LootRepo:     lootRepo,
		Points:       points,
		Tokens:       tokenRepo,
		Polls:        polls,
		Roster:       rosterRepo,
		Audit:        audits,
		Completion:   raidcompletion.NewService(raidcompletion.NewRepo(db)),
		Corrections:  raidcorrection.NewService(raidcorrection.NewRepo(db)),
		Imports:      importsSvc,
	})

	mux := http.NewServeMux()
	api.Register(mux)
	mux.HandleFunc("GET /health", v1.Health)
	if spa, err := webui.Handler(); err == nil {
		mux.Handle("/", spa)
	}

	return requestID(logging(authenticate(authSvc)(mux))), nil
}
