package v1

import (
	"log/slog"
	"net/http"
)

func (a *API) createResetCode(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		WriteError(w, http.StatusNotFound, "not_found", "Spieler nicht gefunden.")
		return
	}
	actor, _ := ActorID(r)
	result, err := a.Registration.CreateResetCode(r.Context(), actor, id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "Spieler nicht gefunden.")
		} else {
			registrationError(w, err)
		}
		return
	}
	a.securityEvent(w, r, "reset_code_created", slog.Int64("actor_id", actor), slog.Int64("player_id", id))
	w.Header().Set("Cache-Control", "no-store")
	WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}
