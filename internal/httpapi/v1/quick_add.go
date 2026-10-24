package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/raidcorrection"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

func (a *API) listCharacters(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Characters.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "Charaktere konnten nicht geladen werden.")
		return
	}
	writeListJSON(w, rows)
}

func (a *API) characterOptions(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"data": validation.ClassSpecs})
}

func (a *API) quickAddCharacter(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	actor, _ := ActorID(r)
	var input raidcorrection.QuickAdd
	if err := DecodeJSON(r, &input); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	before := a.rosterSnapshot(r.Context(), id)
	row, err := a.Corrections.AddNewCharacter(r.Context(), id, actor, input)
	if errors.Is(err, raidcorrection.ErrPlayerExists) {
		WriteError(w, http.StatusConflict, "player_exists", "Ein Spieler mit diesem Namen existiert bereits. Bitte den vorhandenen Spieler auswählen.")
		return
	}
	if errors.Is(err, raidcorrection.ErrQuickAddRequestConflict) {
		WriteError(w, http.StatusConflict, "request_conflict", "Dieser Speicherschlüssel wurde bereits mit anderem Inhalt verwendet.")
		return
	}
	if errors.Is(err, raidcorrection.ErrQuickAddForbidden) {
		WriteError(w, http.StatusForbidden, "forbidden", "Raidpersonal erforderlich.")
		return
	}
	if errors.Is(err, registration.ErrProfileLinked) || errors.Is(err, registration.ErrProfileRequired) || errors.Is(err, registration.ErrProfileUnusable) {
		registrationError(w, err)
		return
	}
	if err != nil {
		writeCorrectionError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": row})
	a.notifyNewRosterPlayers(w, r, id, actor, before)
}
