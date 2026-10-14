package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/legacyimport"
)

// DefaultSheetID is the guild spreadsheet imported by default.
const DefaultSheetID = "1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU"

type importRequest struct {
	Setup   *legacyimport.SetupConfig `json:"setup"`
	SheetID string                    `json:"sheet_id"`
	RawHash string                    `json:"raw_hash"`
	Reason  string                    `json:"reason"`
}

type syncRequest struct {
	SheetID string `json:"sheet_id"`
	RawHash string `json:"raw_hash"`
	Reason  string `json:"reason"`
}

type syncAdoptRequest struct {
	SheetID    string                                `json:"sheet_id"`
	Reason     string                                `json:"reason"`
	Players    []legacyimport.SyncPlayerSelection    `json:"players"`
	Characters []legacyimport.SyncCharacterSelection `json:"characters"`
}

func (a *API) dryRunSyncAdoption(w http.ResponseWriter, r *http.Request) {
	var req syncAdoptRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	sheetID := req.SheetID
	if sheetID == "" {
		sheetID = DefaultSheetID
	}
	result, err := a.Imports.DryRunSyncAdoption(r.Context(), "google_sheets", sheetID)
	if err != nil {
		WriteError(w, http.StatusBadGateway, "fetch_failed", "could not fetch the spreadsheet")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (a *API) adoptSync(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "Anmeldung erforderlich")
		return
	}
	var req syncAdoptRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	sheetID := req.SheetID
	if sheetID == "" {
		sheetID = DefaultSheetID
	}
	result, err := a.Imports.AdoptSync(r.Context(), actor, "google_sheets", sheetID, legacyimport.SyncAdoptionInput{Reason: req.Reason, Players: req.Players, Characters: req.Characters})
	if err != nil {
		switch {
		case errors.Is(err, legacyimport.ErrActorNotFound):
			WriteError(w, http.StatusUnprocessableEntity, "actor_not_found", "Import-Benutzer nicht gefunden")
		case errors.Is(err, legacyimport.ErrAdoptionReasonRequired):
			WriteError(w, http.StatusBadRequest, "sync_adoption_reason_required", "Begruendung ist erforderlich")
		case errors.Is(err, legacyimport.ErrAdoptionSelection):
			WriteError(w, http.StatusBadRequest, "sync_adoption_selection_invalid", "Ausgewaehlte Zuordnung ist ungueltig oder unvollstaendig")
		default:
			WriteError(w, http.StatusBadGateway, "fetch_failed", "could not fetch or adopt the spreadsheet")
		}
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (a *API) listImports(w http.ResponseWriter, r *http.Request) {
	imports, err := a.Imports.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list imports")
		return
	}
	writeListJSON(w, imports)
}

func (a *API) dryRunImport(w http.ResponseWriter, r *http.Request) {
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "Anmeldung erforderlich")
		return
	}
	var req importRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	sheetID := req.SheetID
	if sheetID == "" {
		sheetID = DefaultSheetID
	}
	rep, err := a.Imports.DryRunWithSetup(r.Context(), "google_sheets", sheetID, req.Setup)
	if err != nil {
		WriteError(w, http.StatusBadGateway, "fetch_failed", "could not fetch or parse the spreadsheet")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": rep})
}

func (a *API) mergeImport(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "Anmeldung erforderlich")
		return
	}
	var req importRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	sheetID := req.SheetID
	if sheetID == "" {
		sheetID = DefaultSheetID
	}
	rep, err := a.Imports.MergeWithSetup(r.Context(), actor, "google_sheets", sheetID, req.RawHash, req.Reason, req.Setup)
	if err != nil {
		if errors.Is(err, legacyimport.ErrPreviewRequired) {
			WriteError(w, http.StatusConflict, "preview_required", "Bitte zuerst eine Vorschau erstellen")
			return
		}
		if errors.Is(err, legacyimport.ErrSourceChanged) {
			WriteError(w, http.StatusConflict, "source_changed", "Die Quelle hat sich seit der Vorschau geaendert")
			return
		}
		if errors.Is(err, legacyimport.ErrActorNotFound) {
			WriteError(w, http.StatusUnprocessableEntity, "actor_not_found", "Import-Benutzer nicht gefunden")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not merge the spreadsheet")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": rep})
}

func (a *API) dryRunSync(w http.ResponseWriter, r *http.Request) {
	var req syncRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	sheetID := req.SheetID
	if sheetID == "" {
		sheetID = DefaultSheetID
	}
	rep, err := a.Imports.DryRunSync(r.Context(), "google_sheets", sheetID)
	if err != nil {
		WriteError(w, http.StatusBadGateway, "fetch_failed", "could not fetch or parse the spreadsheet")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": rep})
}

func (a *API) mergeSync(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "Anmeldung erforderlich")
		return
	}
	var req syncRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	sheetID := req.SheetID
	if sheetID == "" {
		sheetID = DefaultSheetID
	}
	rep, err := a.Imports.MergeSync(r.Context(), actor, "google_sheets", sheetID, req.RawHash, req.Reason)
	if err != nil {
		if errors.Is(err, legacyimport.ErrPreviewRequired) {
			WriteError(w, http.StatusConflict, "preview_required", "Bitte zuerst eine Vorschau erstellen")
			return
		}
		if errors.Is(err, legacyimport.ErrSourceChanged) {
			WriteError(w, http.StatusConflict, "source_changed", "Die Quelle hat sich seit der Vorschau geaendert")
			return
		}
		if errors.Is(err, legacyimport.ErrActorNotFound) {
			WriteError(w, http.StatusUnprocessableEntity, "actor_not_found", "Import-Benutzer nicht gefunden")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not synchronize the spreadsheet")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": rep})
}
