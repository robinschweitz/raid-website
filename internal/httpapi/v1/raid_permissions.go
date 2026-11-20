package v1

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	rp "raid-website/internal/raidpermission"
	"raid-website/internal/validation"
)

type permissionKey struct{}

func WithRaidPermissions(repo rp.Repo, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), permissionKey{}, repo)))
	})
}
func permissionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, rp.ErrForbidden):
		WriteError(w, 403, "raid_permission_denied", "Für diese Aktion fehlt die Berechtigung.")
	case errors.Is(err, rp.ErrInvalid):
		BadRequest(w, "Ungültige Zuweisung oder inaktives Teammitglied.")
	case errors.Is(err, sql.ErrNoRows):
		WriteError(w, 404, "not_found", "Spieler oder Raid nicht gefunden.")
	default:
		WriteError(w, 500, "internal", "Berechtigungen konnten nicht geprüft werden.")
	}
}
func checkMask(w http.ResponseWriter, allowed, required rp.Actions, err error) bool {
	if err != nil {
		permissionError(w, err)
		return false
	}
	if required == 0 || allowed&required != required {
		permissionError(w, rp.ErrForbidden)
		return false
	}
	return true
}
func requireOccurrence(w http.ResponseWriter, r *http.Request, id int64, required rp.Actions) bool {
	repo, ok := r.Context().Value(permissionKey{}).(rp.Repo)
	if !ok {
		permissionError(w, rp.ErrForbidden)
		return false
	}
	p, _ := CurrentPlayer(r)
	effective, err := repo.Effective(r.Context(), p.ID, id, p.Role)
	return checkMask(w, effective.Allowed, required, err)
}

// The mux's registered pattern (never the untrusted URL spelling) identifies the
// staff action. New occurrence-scoped staff routes have no implicit permission.
func checkRaidAction(w http.ResponseWriter, r *http.Request) bool {
	_, path, _ := strings.Cut(r.Pattern, " ")
	id, _ := pathID(r, "id")
	p, _ := CurrentPlayer(r)
	const prefix = "/api/v1/raids/{id}"
	if !strings.HasPrefix(path, prefix) && path != "/api/v1/unclaimed-loot/{id}/remove" && path != "/api/v1/raid-series/{id}/plan" {
		return true
	}
	repo, ok := r.Context().Value(permissionKey{}).(rp.Repo)
	if !ok {
		permissionError(w, rp.ErrForbidden)
		return false
	}
	if path == "/api/v1/unclaimed-loot/{id}/remove" {
		raid, err := repo.UnclaimedRaid(r.Context(), id)
		if err != nil {
			permissionError(w, err)
			return false
		}
		return requireOccurrence(w, r, raid, rp.LootEP)
	}
	if path == "/api/v1/raid-series/{id}/plan" {
		return requireSeries(w, r, repo, p.ID, id)
	}
	suffix := strings.TrimPrefix(path, prefix)
	group, _, _ := strings.Cut(strings.TrimPrefix(suffix, "/"), "/")
	var action rp.Actions
	switch group {
	case "signups", "attendance", "quick-add", "shadowmourne", "addon-roster-import":
		action = rp.Roster
	case "kills", "loot", "unclaimed-loot", "wishlist", "addon-loot-import":
		action = rp.LootEP
	case "", "archive", "archive-preview", "cancel", "uncancel", "recruitment-template", "leaders":
		action = rp.Planning
	case "substitutes", "permissions":
		return true // these handlers check the grantor/target directly
	}
	if group == "leaders" {
		assigned, err := repo.HasAssignedPolicies(r.Context())
		if err != nil {
			permissionError(w, err)
			return false
		}
		if assigned {
			permissionError(w, rp.ErrForbidden)
			return false
		}
	}
	// A series recruitment template is shared by all its occurrences. A
	// substitute for a single evening may not change the whole series' text.
	if group == "recruitment-template" && r.Method == http.MethodPut {
		series, err := repo.RaidSeries(r.Context(), id)
		if err != nil {
			permissionError(w, err)
			return false
		}
		if series != 0 && !requireSeries(w, r, repo, p.ID, series) {
			return false
		}
	}
	return requireOccurrence(w, r, id, action)
}
func requireSeries(w http.ResponseWriter, r *http.Request, repo rp.Repo, actor, series int64) bool {
	p, err := repo.Policy(r.Context(), actor)
	if err != nil {
		permissionError(w, err)
		return false
	}
	if p == nil {
		return true
	}
	own := false
	if p.AssignedOnly && series > 0 {
		own, err = repo.SeriesLeader(r.Context(), actor, series)
	}
	return checkMask(w, rp.Base(validation.RoleRaidleader, p, own).Allowed, rp.Planning, err)
}

func (a *API) registerRaidPermissions(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/players/{id}/raid-permissions/overview", RequireRole(validation.RoleAdmin, a.playerRightsOverview))
	mux.HandleFunc("GET /api/v1/players/{id}/raid-permissions", RequireRole(validation.RoleAdmin, a.playerPolicy))
	mux.HandleFunc("PUT /api/v1/players/{id}/raid-permissions", RequireRole(validation.RoleAdmin, a.playerPolicy))
	mux.HandleFunc("GET /api/v1/raids/{id}/permissions", RequireRole(validation.RoleRaidassistent, a.raidRights))
	mux.HandleFunc("PUT /api/v1/raids/{id}/substitutes/{playerID}", RequireRole(validation.RoleRaidleader, a.setSubstitute))
}
func (a *API) playerRightsOverview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player")
		return
	}
	p, err := a.Players.Get(r.Context(), id)
	if err != nil {
		permissionError(w, err)
		return
	}
	raids, err := a.Raids.List(r.Context(), r.URL.Query().Get("include_archived") == "true")
	if err != nil {
		permissionError(w, err)
		return
	}
	type row struct {
		ID     int64        `json:"id"`
		Name   string       `json:"name"`
		Rights rp.Effective `json:"rights"`
	}
	list := []row{}
	repo := rp.Repo{DB: a.DB}
	for _, raid := range raids {
		rights := rp.Effective{Source: "inactive"}
		if p.Active {
			rights, err = repo.Effective(r.Context(), id, raid.ID, p.Role)
			if err != nil {
				permissionError(w, err)
				return
			}
		}
		list = append(list, row{raid.ID, raid.Name, rights})
	}
	writeListJSON(w, list)
}
func (a *API) playerPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player")
		return
	}
	repo := rp.Repo{DB: a.DB}
	if r.Method == "PUT" {
		var req struct {
			Allowed      *rp.Actions `json:"allowed"`
			AssignedOnly *bool       `json:"assigned_only"`
		}
		if DecodeJSON(r, &req) != nil || req.Allowed == nil || req.AssignedOnly == nil {
			BadRequest(w, "Vollständige Richtlinie erforderlich.")
			return
		}
		actor, _ := ActorID(r)
		if err := repo.SetPolicy(r.Context(), actor, id, rp.Policy{Allowed: *req.Allowed, AssignedOnly: *req.AssignedOnly}); err != nil {
			permissionError(w, err)
			return
		}
	}
	p, err := repo.Policy(r.Context(), id)
	if err != nil {
		permissionError(w, err)
		return
	}
	if p == nil {
		p = &rp.Policy{Allowed: rp.All}
	}
	WriteJSON(w, 200, map[string]any{"data": p})
}
func (a *API) raidRights(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid")
		return
	}
	p, _ := CurrentPlayer(r)
	repo := rp.Repo{DB: a.DB}
	rights, err := repo.Effective(r.Context(), p.ID, id, p.Role)
	if err != nil {
		permissionError(w, err)
		return
	}
	canGrant, err := repo.Delegable(r.Context(), p.ID, id)
	if err != nil {
		permissionError(w, err)
		return
	}
	grants := []rp.Grant{}
	if canGrant != 0 {
		grants, err = repo.Grants(r.Context(), id)
		if err != nil {
			permissionError(w, err)
			return
		}
	}
	type row struct {
		rp.Staff
		Effective rp.Effective `json:"effective"`
	}
	team := []row{}
	if canGrant != 0 || p.Role == validation.RoleAdmin {
		people, err := repo.Team(r.Context())
		if err != nil {
			permissionError(w, err)
			return
		}
		for _, s := range people {
			e, err := repo.Effective(r.Context(), s.ID, id, s.Role)
			if err != nil {
				permissionError(w, err)
				return
			}
			team = append(team, row{s, e})
		}
	}
	WriteJSON(w, 200, map[string]any{"data": map[string]any{"rights": rights, "delegable": canGrant, "substitutes": grants, "team": team}})
}
func (a *API) setSubstitute(w http.ResponseWriter, r *http.Request) {
	p, _ := CurrentPlayer(r)
	if p.Role == validation.RoleAdmin && !adminHasSecondFactor(r) {
		WriteError(w, 403, "mfa_setup_required", "admin requires a second factor")
		return
	}
	raid, ok := pathID(r, "id")
	target, ok2 := pathID(r, "playerID")
	var req struct {
		Allowed *rp.Actions `json:"allowed"`
	}
	if !ok || !ok2 || DecodeJSON(r, &req) != nil || req.Allowed == nil {
		BadRequest(w, "Ungültige Vertretung.")
		return
	}
	actor, _ := ActorID(r)
	if err := (rp.Repo{DB: a.DB}).SetGrant(r.Context(), actor, target, raid, *req.Allowed); err != nil {
		permissionError(w, err)
		return
	}
	w.WriteHeader(204)
}
