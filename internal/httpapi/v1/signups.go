package v1

import (
	"errors"
	"net/http"
	"raid-website/internal/raidsignup"
)

func writeSignupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, raidsignup.ErrForbidden):
		WriteError(w, http.StatusForbidden, "signup_forbidden", "Nur eigene Charaktere können angemeldet werden; die Aktion erfordert die passende Berechtigung.")
	case errors.Is(err, raidsignup.ErrClosed):
		WriteError(w, http.StatusConflict, "signups_closed", "Anmeldungen sind geschlossen. Zum Öffnen ist ein zukünftiger geplanter Start erforderlich.")
	case errors.Is(err, raidsignup.ErrTooLate):
		WriteError(w, http.StatusConflict, "raid_started", "Der Raid hat bereits begonnen. Eine Absage ist nur noch über das Raid-Team möglich.")
	case errors.Is(err, raidsignup.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "signup_invalid", "Bitte eigene Charaktere mit einem gültigen Spec ihrer Klasse auswählen.")
	case errors.Is(err, raidsignup.ErrConflict):
		WriteError(w, http.StatusConflict, "signup_conflict", "Der Charakter ist bereits bestätigt, zurückgezogen oder die Aufstellung wurde geändert. Bitte neu laden; ein Wechsel muss ausdrücklich bestätigt werden.")
	case errors.Is(err, raidsignup.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "Anmeldung, Charakter oder Raid nicht gefunden.")
	case errors.Is(err, raidsignup.ErrRaidState):
		WriteError(w, http.StatusConflict, "raid_state", "Anmeldungen können nur für aktive Raids verwaltet werden.")
	default:
		writeCorrectionError(w, err)
	}
}

func (a *API) getSignups(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	result, err := a.Signups.Overview(r.Context(), id)
	if err != nil {
		writeSignupError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (a *API) setSignupAvailability(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	var req struct {
		IsEnabled *bool `json:"is_enabled"`
	}
	if err := DecodeJSON(r, &req); err != nil || req.IsEnabled == nil {
		BadRequest(w, "is_enabled is required")
		return
	}
	actor, _ := ActorID(r)
	if err := a.Signups.SetEnabled(r.Context(), id, actor, *req.IsEnabled); err != nil {
		writeSignupError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) saveSignups(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	var req struct {
		Choices []raidsignup.Choice `json:"choices"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	actor, _ := ActorID(r)
	if err := a.Signups.Save(r.Context(), id, actor, req.Choices); err != nil {
		writeSignupError(w, err)
		return
	}
	// The response reports all persisted choices, including preserved alternatives.
	a.getSignups(w, r)
}

func (a *API) withdrawSignup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	signupID, ok := pathID(r, "signupID")
	if !ok {
		BadRequest(w, "invalid signup id")
		return
	}
	actor, _ := ActorID(r)
	if err := a.Signups.Withdraw(r.Context(), id, signupID, actor); err != nil {
		writeSignupError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) acceptSignup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	signupID, ok := pathID(r, "signupID")
	if !ok {
		BadRequest(w, "invalid signup id")
		return
	}
	var req struct {
		ReplaceAttendanceID int64 `json:"replace_attendance_id"`
		ExpectedCharacterID int64 `json:"expected_character_id"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	actor, _ := ActorID(r)
	if err := a.Signups.Accept(r.Context(), id, signupID, actor, req.ReplaceAttendanceID, req.ExpectedCharacterID); err != nil {
		writeSignupError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) declineAbsence(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	target, ok := pathID(r, "playerID")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	actor, _ := ActorID(r)
	if err := a.Signups.Decline(r.Context(), id, actor, target, req.Reason); err != nil {
		writeSignupError(w, err)
		return
	}
	a.getSignups(w, r)
}

func (a *API) revokeAbsence(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	target, ok := pathID(r, "playerID")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	actor, _ := ActorID(r)
	if err := a.Signups.Revoke(r.Context(), id, actor, target); err != nil {
		writeSignupError(w, err)
		return
	}
	a.getSignups(w, r)
}
