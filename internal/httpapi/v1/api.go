package v1

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/uptrace/bun"

	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/legacyimport"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/poll"
	"raid-website/internal/raid"
	"raid-website/internal/raidcompletion"
	"raid-website/internal/roster"
	"raid-website/internal/tokens"
)

// Deps holds the dependencies shared by the v1 HTTP handlers.
type Deps struct {
	DB         *bun.DB
	Players    *player.Repo
	Characters *character.Repo
	Raids      *raid.Repo
	Attendance *attendance.Repo
	Loot       *loot.Service
	LootRepo   *loot.Repo
	Points     *enhancepoints.Repo
	Tokens     *tokens.Repo
	Polls      *poll.Repo
	Roster     *roster.Repo
	Audit      *audit.Repo
	Completion *raidcompletion.Service
	Imports    *legacyimport.Service
}

// API holds the dependencies shared by the v1 HTTP handlers.
type API struct {
	Deps
}

func NewAPI(d Deps) *API {
	return &API{Deps: d}
}

func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", Health)

	mux.HandleFunc("GET /api/v1/players", a.listPlayers)
	mux.HandleFunc("POST /api/v1/players", a.createPlayer)
	mux.HandleFunc("GET /api/v1/players/{id}", a.getPlayer)
	mux.HandleFunc("GET /api/v1/players/{id}/characters", a.listPlayerCharacters)
	mux.HandleFunc("POST /api/v1/players/{id}/characters", a.createCharacter)

	mux.HandleFunc("GET /api/v1/characters/{id}", a.getCharacter)
	mux.HandleFunc("PATCH /api/v1/characters/{id}", a.updateCharacter)
	mux.HandleFunc("GET /api/v1/characters/{id}/name-history", a.listCharacterNameHistory)

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

	mux.HandleFunc("GET /api/v1/guild-ranks", a.listGuildRanks)
	mux.HandleFunc("POST /api/v1/guild-ranks", a.createGuildRank)
	mux.HandleFunc("PATCH /api/v1/guild-ranks/{id}", a.updateGuildRank)

	mux.HandleFunc("GET /api/v1/raid-groups", a.listRaidGroups)
	mux.HandleFunc("POST /api/v1/raid-groups", a.createRaidGroup)
	mux.HandleFunc("PATCH /api/v1/raid-groups/{id}", a.updateRaidGroup)

	mux.HandleFunc("GET /api/v1/polls", a.listPolls)
	mux.HandleFunc("POST /api/v1/polls", a.createPoll)
	mux.HandleFunc("GET /api/v1/polls/{id}", a.getPoll)
	mux.HandleFunc("POST /api/v1/polls/{id}/vote", a.votePoll)
	mux.HandleFunc("GET /api/v1/polls/{id}/votes", a.listPollVotes)
	mux.HandleFunc("POST /api/v1/polls/{id}/close", a.closePoll)

	mux.HandleFunc("GET /api/v1/imports", a.listImports)
	mux.HandleFunc("POST /api/v1/imports/dry-run", a.dryRunImport)
	mux.HandleFunc("POST /api/v1/imports", a.mergeImport)
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
