package v1

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"raid-website/internal/audit"
	"raid-website/internal/auth"
	"raid-website/internal/supportview"
	"raid-website/internal/validation"
)

const supportHeader = "X-Player-View"

func supportDenied(w http.ResponseWriter) {
	WriteError(w, 403, "support_forbidden", "Diese Aktion ist in der Spieleransicht nicht erlaubt.")
}
func supportError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		WriteError(w, 409, "support_expired", "Diese Spieleransicht ist beendet.")
		return
	}
	WriteError(w, 500, "internal", "Spieleransicht konnte nicht geladen werden.")
}
func (a *API) startSupport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target int64 `json:"player_id"`
	}
	actor, _ := ActorID(r)
	if DecodeJSON(r, &req) != nil || req.Target <= 0 || req.Target == actor {
		BadRequest(w, "Anderes aktives Spielerkonto auswählen.")
		return
	}
	p, err := a.Players.Get(r.Context(), req.Target)
	if err != nil || !p.Active || p.Username == nil {
		WriteError(w, 404, "not_found", "Aktives Spielerkonto nicht gefunden.")
		return
	}
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		supportDenied(w)
		return
	}
	v, err := (supportview.Repo{DB: a.DB}).Start(r.Context(), cookie.Value, actor, req.Target)
	if err != nil {
		supportError(w, err)
		return
	}
	WriteJSON(w, 201, map[string]any{"data": v})
}
func (a *API) changeSupport(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		supportDenied(w)
		return
	}
	repo := supportview.Repo{DB: a.DB}
	v, err := repo.Load(r.Context(), cookie.Value, r.Header.Get(supportHeader))
	if err != nil {
		supportError(w, err)
		return
	}
	actor, _ := ActorID(r)
	if err = repo.Change(r.Context(), cookie.Value, v, actor, r.URL.Path == "/api/v1/support/end"); err != nil {
		supportError(w, err)
		return
	}
	w.WriteHeader(204)
}

// Only existing, explicitly supported member mutations enter the domain handlers.
// Match the registered ServeMux pattern, never a client-supplied action name.
func supportWriteAllowed(pattern string) bool {
	switch pattern {
	case "PUT /api/v1/raids/{id}/signups", "POST /api/v1/raids/{id}/signups/{signupID}/withdraw", "PUT /api/v1/raids/{id}/absences/{playerID}", "DELETE /api/v1/raids/{id}/absences/{playerID}", "POST /api/v1/players/{id}/locks", "POST /api/v1/players/{id}/locks/{lockID}/cancel", "POST /api/v1/characters/{id}/wishlist/items", "PUT /api/v1/characters/{id}/wishlist/items/{itemID}", "DELETE /api/v1/characters/{id}/wishlist/items/{itemID}", "PUT /api/v1/characters/{id}/specs", "POST /api/v1/characters/{id}/reload-profile":
		return true
	}
	return false
}
func supportReadAllowed(pattern string) bool {
	switch pattern {
	case "GET /api/v1/auth/me", "GET /api/v1/auth/csrf", "GET /api/v1/players", "GET /api/v1/players/{id}", "GET /api/v1/players/{id}/characters", "GET /api/v1/players/{id}/points", "GET /api/v1/players/{id}/locks", "GET /api/v1/players/{id}/loot", "GET /api/v1/points", "GET /api/v1/ep-pools", "GET /api/v1/characters", "GET /api/v1/characters/{id}", "GET /api/v1/characters/{id}/name-history", "GET /api/v1/characters/{id}/wishlist", "GET /api/v1/characters/{id}/wishlist/addon-export", "GET /api/v1/character-options", "GET /api/v1/raid-series", "GET /api/v1/raid-news", "GET /api/v1/raids/current", "GET /api/v1/raids", "GET /api/v1/raids/{id}", "GET /api/v1/raids/{id}/attendance", "GET /api/v1/raids/{id}/leaders", "GET /api/v1/raids/{id}/signups", "GET /api/v1/raids/{id}/loot", "GET /api/v1/raids/{id}/kills", "GET /api/v1/raids/{id}/unclaimed-loot", "GET /api/v1/raids/{id}/shadowmourne", "GET /api/v1/shadowmourne", "GET /api/v1/guild-ranks", "GET /api/v1/raid-groups", "GET /api/v1/loot-catalog", "GET /api/v1/loot-catalog/instances", "GET /api/v1/loot-catalog/bosses", "GET /api/v1/loot-catalog/items", "GET /api/v1/polls", "GET /api/v1/polls/{id}", "GET /api/v1/polls/{id}/votes", "GET /api/v1/statistics":
		return true
	}
	return false
}

func (a *API) SupportView(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(supportHeader)
		if id == "" {
			mux.ServeHTTP(w, r)
			return
		}
		admin, ok := CurrentPlayer(r)
		if !ok || admin.Role != validation.RoleAdmin || !adminHasSecondFactor(r) {
			supportDenied(w)
			return
		}
		_, pattern := mux.Handler(r)
		if pattern == "POST /api/v1/support/end" || pattern == "POST /api/v1/support/write" {
			mux.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(auth.CookieName)
		if err != nil {
			supportDenied(w)
			return
		}
		v, err := (supportview.Repo{DB: a.DB}).Load(r.Context(), cookie.Value, id)
		if err != nil {
			supportError(w, err)
			return
		}
		p, err := a.Players.Get(r.Context(), v.Target)
		if err != nil || !p.Active || p.Username == nil {
			supportDenied(w)
			return
		}
		if !supportReadAllowed(pattern) {
			if !v.Writable || !supportWriteAllowed(pattern) {
				supportDenied(w)
				return
			}
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if strings.Contains(pattern, "/absences/") && parts[len(parts)-1] != strconv.FormatInt(p.ID, 10) {
				supportDenied(w)
				return
			}
			if strings.Contains(pattern, "/characters/") {
				cid, err := strconv.ParseInt(parts[3], 10, 64)
				if err != nil {
					supportDenied(w)
					return
				}
				ch, err := a.Characters.Get(r.Context(), cid)
				if err != nil || ch.PlayerID != p.ID {
					supportDenied(w)
					return
				}
			}
			if p.Role.AtLeast(validation.RoleRaidassistent) {
				p.Role = validation.RoleMember
			}
		}
		ctx := audit.WithDelegation(r.Context(), admin.ID, p.ID)
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r.WithContext(WithActor(WithAdminSecondFactor(ctx, false), p)))
	})
}
