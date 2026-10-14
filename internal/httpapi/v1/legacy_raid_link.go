package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/legacyraidlink"
	"raid-website/internal/raid"
)

type createLegacyRaidLinkRequest struct {
	Source  string `json:"source"`
	SheetID string `json:"sheet_id"`
	RaidKey string `json:"raid_key"`
	Reason  string `json:"reason"`
}

func (a *API) getLegacyRaidLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "Ungueltige Raid-ID")
		return
	}
	link, err := a.LegacyRaidLinks.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, legacyraidlink.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "legacy_link_not_found", "Keine Legacy-Verknuepfung fuer diesen Raid vorhanden")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "Legacy-Verknuepfung konnte nicht geladen werden")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": link})
}

func (a *API) createLegacyRaidLink(w http.ResponseWriter, r *http.Request) {
	actorID, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "Anmeldung erforderlich")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "Ungueltige Raid-ID")
		return
	}
	var req createLegacyRaidLinkRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "Ungueltiger JSON-Text")
		return
	}
	link, err := a.LegacyRaidLinks.Create(r.Context(), id, actorID, legacyraidlink.CreateInput{Source: req.Source, SheetID: req.SheetID, RaidKey: req.RaidKey, Reason: req.Reason})
	if err != nil {
		switch {
		case errors.Is(err, raid.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "Raid nicht gefunden")
		case errors.Is(err, legacyraidlink.ErrSourceInvalid):
			WriteError(w, http.StatusBadRequest, "legacy_link_source_invalid", "Quelle muss google_sheets sein")
		case errors.Is(err, legacyraidlink.ErrSheetIDRequired):
			WriteError(w, http.StatusBadRequest, "legacy_link_sheet_id_required", "Sheet-ID ist erforderlich")
		case errors.Is(err, legacyraidlink.ErrRaidKeyRequired):
			WriteError(w, http.StatusBadRequest, "legacy_link_raid_key_required", "Raid-Schluessel ist erforderlich")
		case errors.Is(err, legacyraidlink.ErrReasonRequired):
			WriteError(w, http.StatusBadRequest, "legacy_link_reason_required", "Begruendung ist erforderlich")
		case errors.Is(err, legacyraidlink.ErrAlreadyLinked):
			WriteError(w, http.StatusConflict, "legacy_link_exists", "Raid oder Sheet-Schluessel ist bereits verknuepft")
		case errors.Is(err, legacyraidlink.ErrActorNotFound):
			WriteError(w, http.StatusUnprocessableEntity, "actor_not_found", "Verknuepfender Benutzer nicht gefunden")
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "Legacy-Verknuepfung konnte nicht gespeichert werden")
		}
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": link})
}
