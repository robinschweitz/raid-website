package httpapi

import (
	"net/http"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
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
)

// New constructs the HTTP handler tree with all v1 dependencies wired.
func New(db *bun.DB) http.Handler {
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

	api := v1.NewAPI(v1.Deps{
		DB:         db,
		Players:    players,
		Characters: chars,
		Raids:      raids,
		Attendance: att,
		Loot:       loot.NewService(db, lootRepo, tokenRepo),
		LootRepo:   lootRepo,
		Points:     points,
		Tokens:     tokenRepo,
		Polls:      polls,
		Roster:     rosterRepo,
		Audit:      audits,
		Completion: raidcompletion.New(db, raids, att, lootRepo, points),
		Imports:    importsSvc,
	})

	mux := http.NewServeMux()
	api.Register(mux)
	mux.HandleFunc("GET /health", v1.Health)

	return requestID(actor(logging(mux)))
}
