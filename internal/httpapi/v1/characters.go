package v1

import (
	"errors"
	"net/http"
	"time"

	"raid-website/internal/character"
	"raid-website/internal/validation"
)

type createCharacterRequest struct {
	Name       string `json:"name"`
	Class      string `json:"class"`
	MainSpec   string `json:"main_spec"`
	SecondSpec string `json:"second_spec"`
	IsMain     *bool  `json:"is_main"`
}

func validClass(class string) bool {
	_, ok := validation.ClassSpecs[class]
	return ok
}

func (a *API) listPlayerCharacters(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	cs, err := a.Characters.ListByPlayer(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list characters")
		return
	}
	writeListJSON(w, cs)
}

func (a *API) createCharacter(w http.ResponseWriter, r *http.Request) {
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
		return
	}
	playerID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	if _, err := a.Players.Get(r.Context(), playerID); err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "player not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load player")
		return
	}
	var req createCharacterRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.Name == "" {
		BadRequest(w, "name is required")
		return
	}
	class := validation.NormalizeClassName(req.Class)
	if class == "" || !validClass(class) {
		BadRequest(w, "unknown class")
		return
	}
	isMain := false
	if req.IsMain != nil {
		isMain = *req.IsMain
	}
	now := time.Now().UTC()
	c := &character.Character{
		PlayerID:   playerID,
		Name:       req.Name,
		Class:      class,
		MainSpec:   req.MainSpec,
		SecondSpec: req.SecondSpec,
		IsMain:     isMain,
		Active:     true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := a.Characters.Create(r.Context(), c); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not create character")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": c})
}

func (a *API) getCharacter(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid character id")
		return
	}
	c, err := a.Characters.Get(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "character not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load character")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": c})
}

func (a *API) updateCharacter(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "authentication required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid character id")
		return
	}
	var req character.Patch
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	c, err := character.NewService(a.Characters).Correct(r.Context(), actor, id, req)
	if err != nil {
		switch {
		case errors.Is(err, character.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "character not found")
		case errors.Is(err, character.ErrInvalidActor):
			WriteError(w, http.StatusUnauthorized, "invalid_actor", "actor does not exist")
		case errors.Is(err, character.ErrForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", "character correction requires admin")
		case errors.Is(err, character.ErrReasonRequired):
			WriteError(w, http.StatusBadRequest, "reason_required", "reason is required")
		case errors.Is(err, character.ErrInvalidName):
			WriteError(w, http.StatusBadRequest, "invalid_name", "name cannot be empty")
		case errors.Is(err, character.ErrInvalidClass):
			WriteError(w, http.StatusBadRequest, "invalid_class", "unknown class")
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "could not correct character")
		}
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": c})
}

func (a *API) listCharacterNameHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid character id")
		return
	}
	hs, err := a.Characters.ListNameHistory(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load name history")
		return
	}
	writeListJSON(w, hs)
}
