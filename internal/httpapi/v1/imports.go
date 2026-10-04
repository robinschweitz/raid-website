package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/legacyimport"
)

// DefaultSheetID is the guild spreadsheet imported by default.
const DefaultSheetID = "1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU"

type importRequest struct {
	SheetID string `json:"sheet_id"`
	RawHash string `json:"raw_hash"`
	Reason  string `json:"reason"`
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
	rep, err := a.Imports.DryRun(r.Context(), "google_sheets", sheetID)
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
	rep, err := a.Imports.Merge(r.Context(), actor, "google_sheets", sheetID, req.RawHash, req.Reason)
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
