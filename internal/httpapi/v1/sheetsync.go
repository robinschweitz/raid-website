package v1

import (
	"net/http"
	"raid-website/internal/sheetsync"
)

func (a *API) sheetSyncPreflight(w http.ResponseWriter, r *http.Request) {
	if a.SheetSync == nil {
		WriteError(w, http.StatusConflict, "sync_disabled", "Google-Sheets-Synchronisierung ist nicht aktiviert")
		return
	}
	report, err := a.SheetSync.Preflight(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "sync_preflight_failed", "Vorabprüfung konnte nicht abgeschlossen werden")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": report})
}

func (a *API) sheetSyncStatus(w http.ResponseWriter, r *http.Request) {
	if a.SheetSync == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"data": sheetsync.Status{Enabled: false}})
		return
	}
	status, err := a.SheetSync.GetStatus(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "sync_state_failed", "Synchronisierungsstatus konnte nicht geladen werden")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": status})
}

func (a *API) runSheetSync(w http.ResponseWriter, r *http.Request) {
	if a.SheetSync == nil {
		WriteError(w, http.StatusConflict, "sync_disabled", "Google-Sheets-Synchronisierung ist nicht aktiviert")
		return
	}
	status, err := a.SheetSync.RunOnce(r.Context())
	if err != nil {
		// The persisted status records the precise conflict or external error.
		// Clients can retrieve it without exposing credentials in this response.
		WriteJSON(w, http.StatusConflict, map[string]any{"data": status})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": status})
}
