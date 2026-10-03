package v1

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/raid"
	"raid-website/internal/raidcompletion"
	"raid-website/internal/tokens"
)

// API holds the dependencies shared by the v1 HTTP handlers.
type API struct {
	DB         *bun.DB
	Players    *player.Repo
	Raids      *raid.Repo
	Attendance *attendance.Repo
	Loot       *loot.Service
	LootRepo   *loot.Repo
	Points     *enhancepoints.Repo
	Tokens     *tokens.Repo
	Completion *raidcompletion.Service
}

func NewAPI(
	db *bun.DB,
	players *player.Repo,
	raids *raid.Repo,
	att *attendance.Repo,
	lootSvc *loot.Service,
	lootRepo *loot.Repo,
	points *enhancepoints.Repo,
	tokenRepo *tokens.Repo,
	completion *raidcompletion.Service,
) *API {
	return &API{
		DB:         db,
		Players:    players,
		Raids:      raids,
		Attendance: att,
		Loot:       lootSvc,
		LootRepo:   lootRepo,
		Points:     points,
		Tokens:     tokenRepo,
		Completion: completion,
	}
}

func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", Health)

	mux.HandleFunc("GET /api/v1/players", a.listPlayers)
	mux.HandleFunc("POST /api/v1/players", a.createPlayer)
	mux.HandleFunc("GET /api/v1/players/{id}", a.getPlayer)

	mux.HandleFunc("GET /api/v1/raids", a.listRaids)
	mux.HandleFunc("POST /api/v1/raids", a.createRaid)
	mux.HandleFunc("GET /api/v1/raids/{id}", a.getRaid)
	mux.HandleFunc("POST /api/v1/raids/{id}/complete", a.completeRaid)
	mux.HandleFunc("POST /api/v1/raids/{id}/cancel", a.cancelRaid)
	mux.HandleFunc("POST /api/v1/raids/{id}/archive", a.archiveRaid)

	mux.HandleFunc("GET /api/v1/raids/{id}/attendance", a.listAttendance)
	mux.HandleFunc("POST /api/v1/raids/{id}/attendance", a.createAttendance)

	mux.HandleFunc("GET /api/v1/raids/{id}/loot", a.listLoot)
	mux.HandleFunc("POST /api/v1/raids/{id}/loot", a.createLoot)

	mux.HandleFunc("GET /api/v1/players/{id}/points", a.getPoints)
	mux.HandleFunc("POST /api/v1/players/{id}/points/roll", a.rollPoints)
	mux.HandleFunc("GET /api/v1/players/{id}/tokens", a.getTokens)

	mux.HandleFunc("GET /api/v1/players/{id}/locks", a.listLocks)
	mux.HandleFunc("POST /api/v1/players/{id}/locks", a.createLock)
	mux.HandleFunc("POST /api/v1/players/{id}/locks/{lockID}/cancel", a.cancelLock)
}

func pathID(r *http.Request, name string) (int64, bool) {
	raw := r.PathValue(name)
	if raw == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
