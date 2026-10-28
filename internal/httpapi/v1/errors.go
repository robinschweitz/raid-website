package v1

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewError(code, msg string) []byte {
	b, _ := json.Marshal(ErrorResponse{Error: ErrorBody{Code: code, Message: msg}})
	return b
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func WriteError(w http.ResponseWriter, status int, code, msg string) {
	WriteJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: msg}})
}

// nonNil returns an empty slice when items is nil, so that lists are encoded as
// JSON arrays instead of null and clients can always iterate the response.
func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

// writeListJSON writes a list response with the items normalised to a non-nil
// slice. Every list endpoint must use it instead of WriteJSON.
func writeListJSON[T any](w http.ResponseWriter, items []T) {
	WriteJSON(w, http.StatusOK, map[string]any{"data": nonNil(items)})
}

const maxBodyBytes = 1 << 20

func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

var ErrInvalidBody = errors.New("invalid request body")

func BadRequest(w http.ResponseWriter, msg string) {
	WriteError(w, http.StatusBadRequest, "invalid_request", msg)
}

func writeInternalError(w http.ResponseWriter, err error) {
	log.Printf("API request failed: %v", err)
	WriteError(w, http.StatusInternalServerError, "internal", "Ein interner Fehler ist aufgetreten. Bitte später erneut versuchen.")
}

func stableErrorMessage(code string) string {
	switch code {
	case "not_found":
		return "Der angeforderte Eintrag wurde nicht gefunden."
	case "forbidden":
		return "Für diese Aktion fehlt die Berechtigung."
	case "merge_conflict":
		return "Die Einträge können wegen widersprüchlicher Daten nicht zusammengeführt werden."
	case "invalid_merge":
		return "Die Angaben zum Zusammenführen sind ungültig."
	case "source_inactive":
		return "Der zu übernehmende Spieler ist nicht aktiv."
	case "lock_conflict":
		return "Die Gegenstandssperren widersprechen sich."
	case "already_owned":
		return "Der Charakter gehört bereits diesem Spieler."
	case "transfer_conflict":
		return "Der Charakter kann wegen widersprüchlicher Teilnahmen nicht verschoben werden."
	case "roster_incomplete":
		return "Die Teilnahme ist unvollständig. Bitte Charaktername, Klasse, Spec und Rolle prüfen."
	case "catalog_invalid":
		return "Die Zuordnung zum Beutekatalog ist ungültig."
	case "occurrence_conflict":
		return "Für diesen Serientermin existiert bereits ein Raid."
	case "series_not_found":
		return "Die Raidserie wurde nicht gefunden."
	case "invalid_series":
		return "Die Angaben zur Raidserie sind ungültig."
	case "leader_not_found":
		return "Der Raidleiter wurde nicht gefunden."
	case "series_required":
		return "Für diese Aktion ist eine Raidserie erforderlich."
	case "scheduled_start_required":
		return "Für diese Aktion ist ein geplanter Beginn erforderlich."
	case "local_time_nonexistent":
		return "Diese Ortszeit existiert wegen der Zeitumstellung nicht."
	case "local_time_ambiguous":
		return "Diese Ortszeit ist wegen der Zeitumstellung mehrdeutig."
	case "plan_until_past":
		return "Das Planungsende darf nicht in der Vergangenheit liegen."
	case "plan_too_far":
		return "Das Planungsende liegt zu weit in der Zukunft."
	case "schedule_range":
		return "Der Termin liegt außerhalb des zulässigen Zeitraums."
	default:
		return "Die Anfrage konnte nicht verarbeitet werden."
	}
}
