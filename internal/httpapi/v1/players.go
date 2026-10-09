package v1

import (
	"net/http"
	"time"

	"raid-website/internal/player"
	"raid-website/internal/validation"
)

type playerListResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	GuildRankID *int64 `json:"guild_rank_id"`
	Active      bool   `json:"active"`
}

type playerResponse struct {
	ID          int64            `json:"id"`
	Name        string           `json:"name"`
	GuildRankID *int64           `json:"guild_rank_id"`
	Username    *string          `json:"username,omitempty"`
	Role        *validation.Role `json:"role,omitempty"`
	Active      bool             `json:"active"`
	CreatedAt   *time.Time       `json:"created_at,omitempty"`
	UpdatedAt   *time.Time       `json:"updated_at,omitempty"`
}

func listedPlayer(p player.Player) playerListResponse {
	return playerListResponse{ID: p.ID, Name: p.Name, GuildRankID: p.GuildRankID, Active: p.Active}
}

func visiblePlayer(actor *player.Player, p player.Player) playerResponse {
	username := p.Username
	role := &p.Role
	createdAt := &p.CreatedAt
	updatedAt := &p.UpdatedAt
	if actor == nil || (!actor.Role.AtLeast(validation.RoleAdmin) && actor.ID != p.ID) {
		username = nil
		role = nil
		createdAt = nil
		updatedAt = nil
	}
	return playerResponse{
		ID:          p.ID,
		Name:        p.Name,
		GuildRankID: p.GuildRankID,
		Username:    username,
		Role:        role,
		Active:      p.Active,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

func visiblePlayerPtr(actor *player.Player, p *player.Player) playerResponse {
	if p == nil {
		return playerResponse{}
	}
	return visiblePlayer(actor, *p)
}

type createPlayerRequest struct {
	Name        string `json:"name"`
	GuildRankID *int64 `json:"guild_rank_id"`
	Role        string `json:"role"`
	Active      *bool  `json:"active"`
}

func (a *API) listPlayers(w http.ResponseWriter, r *http.Request) {
	ps, err := a.Players.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list players")
		return
	}
	result := make([]playerListResponse, 0, len(ps))
	for _, p := range ps {
		result = append(result, listedPlayer(p))
	}
	writeListJSON(w, result)
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
	role := validation.RoleGuest
	if req.Role != "" {
		role = validation.Role(req.Role)
		if !role.Valid() {
			BadRequest(w, "invalid role")
			return
		}
	}
	now := time.Now().UTC()
	p := &player.Player{
		Name:        req.Name,
		GuildRankID: req.GuildRankID,
		Role:        role,
		Active:      active,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := a.Players.Create(r.Context(), p); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not create player")
		return
	}
	actor, _ := CurrentPlayer(r)
	WriteJSON(w, http.StatusCreated, map[string]any{"data": visiblePlayerPtr(actor, p)})
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
	actor, _ := CurrentPlayer(r)
	WriteJSON(w, http.StatusOK, map[string]any{"data": visiblePlayerPtr(actor, p)})
}
