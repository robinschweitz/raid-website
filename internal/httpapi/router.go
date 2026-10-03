package httpapi

import (
	"net/http"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/enhancepoints"
	v1 "raid-website/internal/httpapi/v1"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidcompletion"
	"raid-website/internal/tokens"
)

// New constructs the HTTP handler tree with all v1 dependencies wired.
func New(db *bun.DB) http.Handler {
	players := player.NewRepo(db)
	raids := raid.NewRepo(db)
	att := attendance.NewRepo(db)
	lootRepo := loot.NewRepo(db)
	tokenRepo := tokens.NewRepo(db)
	points := enhancepoints.NewRepo(db)

	api := v1.NewAPI(
		db,
		players,
		raids,
		att,
		loot.NewService(db, lootRepo, tokenRepo),
		lootRepo,
		points,
		tokenRepo,
		raidcompletion.New(db, raids, att, lootRepo, points),
	)

	mux := http.NewServeMux()
	api.Register(mux)
	mux.HandleFunc("GET /health", v1.Health)

	return requestID(actor(logging(mux)))
}
