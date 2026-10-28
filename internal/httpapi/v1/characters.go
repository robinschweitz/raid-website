package v1

import (
	"errors"
	"log"
	"net/http"

	"raid-website/internal/character"
	"raid-website/internal/registration"
)

type createCharacterRequest struct {
	ProfileID string `json:"profile_id"`
	IsMain    *bool  `json:"is_main"`
}

type transferCharacterRequest struct {
	TargetPlayerID int64 `json:"target_player_id"`
}

func (a *API) listPlayerCharacters(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	cs, err := a.Characters.ListByPlayer(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list characters")
		return
	}
	writeListJSON(w, cs)
}

func (a *API) createCharacter(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "authentication required")
		return
	}
	playerID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	if _, err := a.Players.Get(r.Context(), playerID); err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "player not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load player")
		return
	}
	var req createCharacterRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.ProfileID == "" {
		BadRequest(w, "profile_id is required")
		return
	}
	isMain := false
	if req.IsMain != nil {
		isMain = *req.IsMain
	}
	c, err := a.Registration.CreateLinkedCharacter(r.Context(), registration.LinkedCharacterInput{PlayerID: playerID, ProfileID: req.ProfileID, IsMain: isMain, ActorID: &actor, Reason: "admin linked Rising-Gods character"})
	if err != nil {
		registrationError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": c})
}

func (a *API) getCharacter(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid character id")
		return
	}
	c, err := a.Characters.Get(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "character not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load character")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": c})
}

func (a *API) updateCharacter(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "authentication required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid character id")
		return
	}
	var req character.Patch
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	c, err := character.NewService(a.Characters).Correct(r.Context(), actor, id, req)
	if err != nil {
		switch {
		case errors.Is(err, character.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "character not found")
		case errors.Is(err, character.ErrInvalidActor):
			WriteError(w, http.StatusUnauthorized, "invalid_actor", "actor does not exist")
		case errors.Is(err, character.ErrForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", "character correction requires admin")
		case errors.Is(err, character.ErrReasonRequired):
			WriteError(w, http.StatusBadRequest, "reason_required", "reason is required")
		case errors.Is(err, character.ErrInvalidName):
			WriteError(w, http.StatusBadRequest, "invalid_name", "name cannot be empty")
		case errors.Is(err, character.ErrInvalidClass):
			WriteError(w, http.StatusBadRequest, "invalid_class", "unknown class")
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "could not correct character")
		}
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": c})
}

func (a *API) transferCharacter(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "authentication required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid character id")
		return
	}
	var req transferCharacterRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	c, err := character.NewService(a.Characters).Transfer(r.Context(), actor, id, req.TargetPlayerID)
	if err != nil {
		switch {
		case errors.Is(err, character.ErrNotFound), errors.Is(err, character.ErrPlayerNotFound):
			WriteError(w, http.StatusNotFound, "not_found", stableErrorMessage("not_found"))
		case errors.Is(err, character.ErrInvalidActor):
			WriteError(w, http.StatusUnauthorized, "invalid_actor", "actor does not exist")
		case errors.Is(err, character.ErrForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", "character transfer requires admin")
		case errors.Is(err, character.ErrSameOwner):
			WriteError(w, http.StatusConflict, "already_owned", stableErrorMessage("already_owned"))
		case errors.Is(err, character.ErrAttendanceConflict):
			WriteError(w, http.StatusConflict, "transfer_conflict", stableErrorMessage("transfer_conflict"))
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "could not transfer character")
		}
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": c})
}

func (a *API) retryCharacterLink(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "authentication required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid character id")
		return
	}
	c, err := a.Registration.RetryCharacterLink(r.Context(), actor, id)
	if err != nil {
		switch {
		case errors.Is(err, character.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "character not found")
		case errors.Is(err, character.ErrInvalidActor):
			WriteError(w, http.StatusUnauthorized, "invalid_actor", "actor does not exist")
		case errors.Is(err, character.ErrForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", "character link requires admin")
		case errors.Is(err, registration.ErrProfileLinked):
			WriteError(w, http.StatusConflict, "profile_already_linked", "Rising-Gods-Profil ist bereits verknüpft.")
		case errors.Is(err, registration.ErrProfileAmbiguous):
			WriteError(w, http.StatusConflict, "profile_ambiguous", "Mehrere Rising-Gods-Profile mit diesem Namen gefunden.")
		case errors.Is(err, registration.ErrProfileRequired):
			WriteError(w, http.StatusNotFound, "profile_not_found", "Kein passendes Rising-Gods-Profil gefunden.")
		case errors.Is(err, registration.ErrExternalUnavailable):
			log.Printf("rising-gods retry-link unavailable character_id=%d: %v", id, err)
			WriteError(w, http.StatusBadGateway, "rising_gods_unavailable", "Rising Gods ist momentan nicht erreichbar.")
		case errors.Is(err, registration.ErrProfileUnusable):
			WriteError(w, http.StatusBadRequest, "profile_unusable", "Rising-Gods-Profil konnte nicht verwendet werden.")
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "Rising-Gods-Verknüpfung fehlgeschlagen.")
		}
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": c})
}

func (a *API) reloadCharacterProfile(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "authentication required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid character id")
		return
	}
	c, err := a.Registration.ReloadLinkedCharacter(r.Context(), actor, id)
	if err != nil {
		switch {
		case errors.Is(err, character.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "character not found")
		case errors.Is(err, character.ErrInvalidActor):
			WriteError(w, http.StatusUnauthorized, "invalid_actor", "actor does not exist")
		case errors.Is(err, character.ErrForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", "character profile reload requires admin")
		case errors.Is(err, registration.ErrProfileMissing):
			WriteError(w, http.StatusBadRequest, "profile_missing", "character has no Rising-Gods profile")
		case errors.Is(err, registration.ErrProfileRequired), errors.Is(err, registration.ErrProfileUnusable):
			WriteError(w, http.StatusBadRequest, "profile_unusable", "Rising-Gods profile could not be loaded")
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "could not reload character profile")
		}
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": c})
}

func (a *API) listCharacterNameHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid character id")
		return
	}
	hs, err := a.Characters.ListNameHistory(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load name history")
		return
	}
	writeListJSON(w, hs)
}
