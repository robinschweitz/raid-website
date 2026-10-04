package v1

import (
	"net/http"
	"time"

	"raid-website/internal/roster"
)

type guildRankRequest struct {
	Name      string `json:"name"`
	SortOrder *int   `json:"sort_order"`
	Active    *bool  `json:"active"`
}

type raidGroupRequest struct {
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder *int   `json:"sort_order"`
	Active    *bool  `json:"active"`
}

func (a *API) listGuildRanks(w http.ResponseWriter, r *http.Request) {
	gs, err := a.Roster.ListGuildRanks(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list guild ranks")
		return
	}
	writeListJSON(w, gs)
}

func (a *API) createGuildRank(w http.ResponseWriter, r *http.Request) {
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
		return
	}
	var req guildRankRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.Name == "" {
		BadRequest(w, "name is required")
		return
	}
	now := time.Now().UTC()
	g := &roster.GuildRank{Name: req.Name, Active: true, CreatedAt: now, UpdatedAt: now}
	if req.SortOrder != nil {
		g.SortOrder = *req.SortOrder
	}
	if req.Active != nil {
		g.Active = *req.Active
	}
	if err := a.Roster.CreateGuildRank(r.Context(), g); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not create guild rank")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": g})
}

func (a *API) updateGuildRank(w http.ResponseWriter, r *http.Request) {
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid guild rank id")
		return
	}
	var req guildRankRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	g, err := a.Roster.GetGuildRank(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "guild rank not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load guild rank")
		return
	}
	if req.Name != "" {
		g.Name = req.Name
	}
	if req.SortOrder != nil {
		g.SortOrder = *req.SortOrder
	}
	if req.Active != nil {
		g.Active = *req.Active
	}
	g.UpdatedAt = time.Now().UTC()
	if err := a.Roster.UpdateGuildRank(r.Context(), g); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not update guild rank")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": g})
}

func (a *API) listRaidGroups(w http.ResponseWriter, r *http.Request) {
	gs, err := a.Roster.ListRaidGroups(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list raid groups")
		return
	}
	writeListJSON(w, gs)
}

func (a *API) createRaidGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
		return
	}
	var req raidGroupRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.Name == "" {
		BadRequest(w, "name is required")
		return
	}
	now := time.Now().UTC()
	g := &roster.RaidGroup{Name: req.Name, Color: req.Color, Active: true, CreatedAt: now, UpdatedAt: now}
	if req.SortOrder != nil {
		g.SortOrder = *req.SortOrder
	}
	if req.Active != nil {
		g.Active = *req.Active
	}
	if err := a.Roster.CreateRaidGroup(r.Context(), g); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not create raid group")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": g})
}

func (a *API) updateRaidGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid group id")
		return
	}
	var req raidGroupRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	g, err := a.Roster.GetRaidGroup(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "raid group not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load raid group")
		return
	}
	if req.Name != "" {
		g.Name = req.Name
	}
	if req.Color != "" {
		g.Color = req.Color
	}
	if req.SortOrder != nil {
		g.SortOrder = *req.SortOrder
	}
	if req.Active != nil {
		g.Active = *req.Active
	}
	g.UpdatedAt = time.Now().UTC()
	if err := a.Roster.UpdateRaidGroup(r.Context(), g); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not update raid group")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": g})
}
