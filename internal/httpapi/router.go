package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

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
	"raid-website/internal/roster"
	"raid-website/internal/tokens"
	"raid-website/internal/validation"
	"raid-website/internal/webui"
)

// Options controls optional runtime behaviour of the HTTP layer.
type Options struct {
	CookieSecure  bool
	AdminUsername string
	AdminPassword string
}

// New constructs the HTTP handler tree with all v1 dependencies wired.
func New(db *bun.DB, opts Options) http.Handler {
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

	authSvc := auth.NewService(db, players, auth.NewRepo(db))

	importsSvc := legacyimport.NewService(
		db,
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

	bootstrapAdmin(authSvc, players, opts)

	api := v1.NewAPI(v1.Deps{
		DB:           db,
		Auth:         authSvc,
		CookieSecure: opts.CookieSecure,
		Players:      players,
		Characters:   chars,
		Raids:        raids,
		Attendance:   att,
		Loot:         loot.NewService(db, lootRepo, tokenRepo),
		LootRepo:     lootRepo,
		Points:       points,
		Tokens:       tokenRepo,
		Polls:        polls,
		Roster:       rosterRepo,
		Audit:        audits,
		Completion:   raidcompletion.New(db, raids, att, lootRepo, points),
		Imports:      importsSvc,
	})

	mux := http.NewServeMux()
	api.Register(mux)
	mux.HandleFunc("GET /health", v1.Health)
	if spa, err := webui.Handler(); err == nil {
		mux.Handle("/", spa)
	}

	return requestID(logging(authenticate(authSvc)(mux)))
}

// bootstrapAdmin creates the initial admin account from configuration when the
// configured username does not exist yet. It is a no-op without configuration.
func bootstrapAdmin(authSvc *auth.Service, players *player.Repo, opts Options) {
	if opts.AdminUsername == "" || opts.AdminPassword == "" {
		return
	}
	ctx := context.Background()
	username := strings.ToLower(strings.TrimSpace(opts.AdminUsername))
	if _, err := players.FindByUsername(ctx, username); err == nil {
		return
	}
	now := time.Now().UTC()
	p := &player.Player{
		Name:      opts.AdminUsername,
		Active:    true,
		Role:      validation.RoleAdmin,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := players.Create(ctx, p); err != nil {
		return
	}
	_, _ = authSvc.SetCredentials(ctx, p.ID, username, opts.AdminPassword, validation.RoleAdmin)
}
