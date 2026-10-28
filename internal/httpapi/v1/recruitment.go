package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/recruitment"
)

func writeRecruitmentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, recruitment.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "Raid nicht gefunden.")
	case errors.Is(err, recruitment.ErrInvalidSlot):
		WriteError(w, http.StatusUnprocessableEntity, "invalid_slot", "Ein Platz der Soll-Besetzung ist kein bekannter Spec und kein „Irgendein …“-Platzhalter.")
	case errors.Is(err, recruitment.ErrInvalid):
		WriteError(w, http.StatusBadRequest, "invalid_template", "Text (höchstens 2000 Zeichen) und eine Soll-Besetzung aus 5 Gruppen mit je 5 Plätzen sind erforderlich.")
	default:
		WriteError(w, http.StatusInternalServerError, "internal", "Gesuchtext konnte nicht verarbeitet werden.")
	}
}

func (a *API) getRecruitmentTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	t, err := a.Recruitment.Get(r.Context(), id)
	if err != nil {
		writeRecruitmentError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": t})
}

func (a *API) saveRecruitmentTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	var req struct {
		Text  string     `json:"text"`
		Setup [][]string `json:"setup"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	actor, _ := ActorID(r)
	t, err := a.Recruitment.Save(r.Context(), id, actor, req.Text, req.Setup)
	if err != nil {
		writeRecruitmentError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": t})
}
