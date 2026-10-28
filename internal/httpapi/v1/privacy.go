package v1

import "net/http"

// PrivacyContact is the responsible person of the privacy page (FR-30.1). The
// values come from configuration (SC-033).
type PrivacyContact struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// PrivacyHandlers serves the contact of the privacy page without a session
// (FR-1.2). Only the dedicated public DTO leaves the server (SC-004).
type PrivacyHandlers struct{ Contact PrivacyContact }

// Register mounts the public privacy-contact read.
func (h PrivacyHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/privacy-contact", h.get)
}

func (h PrivacyHandlers) get(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{"data": h.Contact})
}
