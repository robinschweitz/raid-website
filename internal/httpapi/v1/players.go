package v1

import (
	"net/http"
	"time"

	"raid-website/internal/player"
)

type createPlayerRequest struct {
	Name        string `json:"name"`
	GuildRankID *int64 `json:"guild_rank_id"`
	Active      *bool  `json:"active"`
}

func (a *API) listPlayers(w http.ResponseWriter, r *http.Request) {
	ps, err := a.Players.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list players")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": ps})
}

func (a *API) createPlayer(w http.ResponseWriter, r *http.Request) {
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
		return
	}
	var req createPlayerRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.Name == "" {
		BadRequest(w, "name is required")
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	now := time.Now().UTC()
	p := &player.Player{
		Name:        req.Name,
		GuildRankID: req.GuildRankID,
		Active:      active,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := a.Players.Create(r.Context(), p); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not create player")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": p})
}

func (a *API) getPlayer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	p, err := a.Players.Get(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "player not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load player")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": p})
}
