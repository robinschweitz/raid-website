package v1

import (
	"net/http"
)

func (a *API) listAuditLog(w http.ResponseWriter, r *http.Request) {
	entries, err := a.Audit.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list audit log")
		return
	}
	writeListJSON(w, entries)
}
