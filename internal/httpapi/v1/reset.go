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

func (a *API) redeemResetCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "Ungültige Anfrage.")
		return
	}
	if err := a.Registration.RedeemResetCode(r.Context(), req.Code, req.Password, clientAddress(r, a.TrustProxyHeaders)); err != nil {
		if !a.rateLimitEvent(w, r, err) {
			a.securityEvent(w, r, "password_reset_failed")
		}
		registrationError(w, err)
		return
	}
	a.securityEvent(w, r, "password_reset_redeemed")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
