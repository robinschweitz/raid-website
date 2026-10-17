package v1

import (
	"errors"
	"net/http"
	"strings"

	"raid-website/internal/auth"
	"raid-website/internal/registration"
)

type profileRequest struct {
	ProfileID string `json:"profile_id"`
}

type redeemRequest struct {
	Code     string `json:"code"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type claimCodeRequest struct {
	Code string `json:"code"`
}

func registrationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, registration.ErrInvalidCode):
		WriteError(w, http.StatusBadRequest, "registration_code_invalid", "Code ungültig oder abgelaufen.")
	case errors.Is(err, registration.ErrUsernameTaken):
		WriteError(w, http.StatusConflict, "username_taken", "Benutzername ist bereits vergeben.")
	case errors.Is(err, registration.ErrForbidden):
		WriteError(w, http.StatusForbidden, "registration_forbidden", "Dieser Code passt nicht zu dieser Aktion.")
	case errors.Is(err, registration.ErrProfileLinked):
		WriteError(w, http.StatusConflict, "profile_already_linked", "Dieser Rising-Gods-Charakter ist bereits verknüpft.")
	case errors.Is(err, registration.ErrRequestExists):
		WriteError(w, http.StatusConflict, "registration_request_exists", "Für diesen Rising-Gods-Charakter besteht bereits eine offene Anfrage oder ein Konto.")
	case errors.Is(err, registration.ErrRequestNotFound), errors.Is(err, registration.ErrCodeNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "Eintrag nicht gefunden.")
	case errors.Is(err, registration.ErrExternalUnavailable):
		WriteError(w, http.StatusBadGateway, "rising_gods_unavailable", "Die Rising-Gods-Charaktersuche ist vorübergehend nicht erreichbar. Bitte versuche es später erneut.")
	case errors.Is(err, registration.ErrInvalid), errors.Is(err, registration.ErrProfileRequired), errors.Is(err, registration.ErrProfileUnusable):
		WriteError(w, http.StatusBadRequest, "registration_invalid", "Bitte einen aktiven Rising-Gods-Charakter auswählen.")
	case errors.Is(err, auth.ErrWeakPassword):
		WriteError(w, http.StatusUnprocessableEntity, "password_too_short", "Das Passwort muss mindestens 12 Zeichen lang sein.")
	default:
		WriteError(w, http.StatusInternalServerError, "internal", "Registrierung konnte nicht verarbeitet werden.")
	}
}

func (a *API) searchRisingGodsCharacters(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < 3 {
		BadRequest(w, "query must contain at least 3 characters")
		return
	}
	rows, err := a.Registration.SearchCharacters(r.Context(), query)
	if err != nil {
		registrationError(w, err)
		return
	}
	writeListJSON(w, rows)
}

func (a *API) createRegistrationRequest(w http.ResponseWriter, r *http.Request) {
	var req profileRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	created, err := a.Registration.CreateRequest(r.Context(), nil, req.ProfileID)
	if err != nil {
		registrationError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (a *API) createCharacterClaimRequest(w http.ResponseWriter, r *http.Request) {
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	var req profileRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	actorID := actor.ID
	created, err := a.Registration.CreateRequest(r.Context(), &actorID, req.ProfileID)
	if err != nil {
		registrationError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (a *API) listRegistrationRequests(w http.ResponseWriter, r *http.Request) {
	statuses := make([]string, 0, 2)
	for _, status := range strings.Split(r.URL.Query().Get("status"), ",") {
		switch status {
		case registration.StatusPending, registration.StatusApproved, registration.StatusRejected, registration.StatusConsumed:
			statuses = append(statuses, status)
		case "":
		default:
			BadRequest(w, "invalid registration request status")
			return
		}
	}
	rows, err := a.Registration.ListRequests(r.Context(), statuses)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list registration requests")
		return
	}
	writeListJSON(w, rows)
}

func (a *API) createRegistrationCode(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid request id")
		return
	}
	code, err := a.Registration.CreateCode(r.Context(), actor, id)
	if err != nil {
		registrationError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": code})
}

func (a *API) listRegistrationCodes(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid request id")
		return
	}
	codes, err := a.Registration.ListCodesByRequest(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list registration codes")
		return
	}
	writeListJSON(w, codes)
}

func (a *API) rejectRegistrationRequest(w http.ResponseWriter, r *http.Request) {
	actor, _ := ActorID(r)
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid request id")
		return
	}
	req, err := a.Registration.RejectRequest(r.Context(), actor, id)
	if err != nil {
		registrationError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": req})
}

func (a *API) revokeRegistrationCode(w http.ResponseWriter, r *http.Request) {
	actor, _ := ActorID(r)
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid code id")
		return
	}
	code, err := a.Registration.RevokeCode(r.Context(), actor, id)
	if err != nil {
		registrationError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": code})
}

func (a *API) markRegistrationCodeSent(w http.ResponseWriter, r *http.Request) {
	actor, _ := ActorID(r)
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid code id")
		return
	}
	code, err := a.Registration.MarkCodeSent(r.Context(), actor, id)
	if err != nil {
		registrationError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": code})
}

func (a *API) registerWithCode(w http.ResponseWriter, r *http.Request) {
	var req redeemRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	token, p, err := a.Registration.RedeemNewAccount(r.Context(), req.Code, req.Username, req.Password)
	if err != nil {
		registrationError(w, err)
		return
	}
	a.setSessionCookie(w, token, sessionMaxAgeSeconds)
	WriteJSON(w, http.StatusOK, map[string]any{"data": visiblePlayerPtr(p, p)})
}

func (a *API) claimCharacterWithCode(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	var req claimCodeRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	claimed, err := a.Registration.RedeemClaim(r.Context(), actor, req.Code)
	if err != nil {
		registrationError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": claimed})
}
