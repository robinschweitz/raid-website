package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/addonsync"
	"raid-website/internal/loot"
	"raid-website/internal/validation"
)

// AddonSyncHandlers serve the copy-and-paste exchange with the raid lead's WoW
// addon (RaidLedger; formerly RaidSync).
type AddonSyncHandlers struct {
	AddonSync *addonsync.Service
}

// Register adds the addon exchange routes.
func (h AddonSyncHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/raids/{id}/addon-invite-export", RequireAuth(h.inviteExport))
	mux.HandleFunc("POST /api/v1/raids/{id}/addon-loot-import/preview", RequireRole(validation.RoleRaidassistent, h.previewLootImport))
	mux.HandleFunc("POST /api/v1/raids/{id}/addon-loot-import", RequireRole(validation.RoleRaidassistent, h.applyLootImport))
	mux.HandleFunc("POST /api/v1/raids/{id}/addon-roster-import/preview", RequireRole(validation.RoleRaidassistent, h.previewRosterImport))
	mux.HandleFunc("POST /api/v1/raids/{id}/addon-roster-import", RequireRole(validation.RoleRaidassistent, h.applyRosterImport))
}

type addonInviteExportResponse struct {
	Text string `json:"text"`
}

type addonPreviewRequest struct {
	Payload string `json:"payload"`
}

type addonLootImportRequest struct {
	Payload    string                    `json:"payload"`
	Selections []addonsync.LootSelection `json:"selections"`
}

type addonRosterImportRequest struct {
	Payload    string                      `json:"payload"`
	Selections []addonsync.RosterSelection `json:"selections"`
}

func (h AddonSyncHandlers) inviteExport(w http.ResponseWriter, r *http.Request) {
	raidID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	text, err := h.AddonSync.InviteExport(r.Context(), raidID, r.URL.Query().Get("bench") == "1")
	if err != nil {
		writeAddonSyncError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": addonInviteExportResponse{Text: text}})
}

func (h AddonSyncHandlers) previewLootImport(w http.ResponseWriter, r *http.Request) {
	raidID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	var req addonPreviewRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	preview, err := h.AddonSync.PreviewLootImport(r.Context(), raidID, req.Payload)
	if err != nil {
		writeAddonSyncError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": preview})
}

func (h AddonSyncHandlers) applyLootImport(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "session required")
		return
	}
	raidID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	var req addonLootImportRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	results, err := h.AddonSync.ApplyLootImport(r.Context(), raidID, actor, req.Payload, req.Selections)
	if err != nil {
		writeAddonSyncError(w, err)
		return
	}
	writeListJSON(w, results)
}

func (h AddonSyncHandlers) previewRosterImport(w http.ResponseWriter, r *http.Request) {
	raidID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	var req addonPreviewRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	preview, err := h.AddonSync.PreviewRosterImport(r.Context(), raidID, req.Payload)
	if err != nil {
		writeAddonSyncError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": preview})
}

func (h AddonSyncHandlers) applyRosterImport(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "session required")
		return
	}
	raidID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	var req addonRosterImportRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	results, err := h.AddonSync.ApplyRosterImport(r.Context(), raidID, actor, req.Payload, req.Selections)
	if err != nil {
		writeAddonSyncError(w, err)
		return
	}
	writeListJSON(w, results)
}

func writeAddonSyncError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, addonsync.ErrRaidNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "Raid nicht gefunden")
	case errors.Is(err, addonsync.ErrInvalidPayload):
		WriteError(w, http.StatusBadRequest, "invalid_addon_export", "Der Text ist kein passender Export von RaidLedger")
	case errors.Is(err, addonsync.ErrRaidState):
		WriteError(w, http.StatusConflict, "raid_state", "Addon-Daten können nur in geplante oder offene, nicht archivierte Raids importiert werden")
	case errors.Is(err, loot.ErrCatalogContextRequired):
		WriteError(w, http.StatusConflict, "catalog_context_required", "Raidinstanz und Raidgroesse muessen vor dem Lootimport konfiguriert werden")
	default:
		WriteError(w, http.StatusInternalServerError, "internal", "Addon-Daten konnten nicht verarbeitet werden")
	}
}
