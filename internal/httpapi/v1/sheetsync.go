package v1

import (
	"errors"
	"log"
	"net/http"
	"raid-website/internal/sheetsync"
)

func (a *API) sheetSyncPreflight(w http.ResponseWriter, r *http.Request) {
	if a.SheetSync == nil {
		WriteError(w, http.StatusConflict, "sync_disabled", "Google-Sheets-Synchronisierung ist nicht aktiviert")
		return
	}
	report, err := a.SheetSync.Preflight(r.Context())
	if errors.Is(err, sheetsync.ErrDisabled) {
		WriteError(w, http.StatusConflict, "sync_disabled", "Synchronisierung ist nicht aktiviert oder vollständig konfiguriert")
		return
	}
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
	if errors.Is(err, sheetsync.ErrDisabled) {
		WriteError(w, http.StatusConflict, "sync_disabled", "Synchronisierung ist nicht aktiviert oder vollständig konfiguriert")
		return
	}
	if err != nil {
		// The persisted status records the precise conflict or external error.
		// Clients can retrieve it without exposing credentials in this response.
		WriteJSON(w, http.StatusConflict, map[string]any{"data": status})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": status})
}

func (a *API) sheetSyncConfig(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"data": a.SheetSync.Configuration()})
}

func (a *API) updateSheetSyncConfig(w http.ResponseWriter, r *http.Request) {
	var req sheetsync.SettingsUpdate
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "Ungültige Konfiguration")
		return
	}
	saved, err := a.SheetSync.Save(r.Context(), req)
	if errors.Is(err, sheetsync.ErrInvalidSettings) {
		WriteError(w, http.StatusBadRequest, "invalid_sync_config", err.Error())
		return
	}
	if err != nil {
		// The browser receives no filesystem paths or credential details.
		// Keep the underlying error in the server log for actionable support.
		log.Printf("sheet sync config save failed: %v", err)
		WriteError(w, http.StatusInternalServerError, "sync_config_failed", "Konfiguration konnte nicht gespeichert werden (Details im Serverlog)")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": saved})
}
