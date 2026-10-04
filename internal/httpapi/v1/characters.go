package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"raid-website/internal/audit"
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

type updateCharacterRequest struct {
	Name       *string `json:"name"`
	Class      *string `json:"class"`
	MainSpec   *string `json:"main_spec"`
	SecondSpec *string `json:"second_spec"`
	IsMain     *bool   `json:"is_main"`
	Active     *bool   `json:"active"`
	Reason     string  `json:"reason"`
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
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid character id")
		return
	}
	var req updateCharacterRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
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
	before, _ := json.Marshal(c)
	oldName := c.Name
	now := time.Now().UTC()

	if req.Name != nil {
		if *req.Name == "" {
			BadRequest(w, "name cannot be empty")
			return
		}
		c.Name = *req.Name
	}
	if req.Class != nil {
		class := validation.NormalizeClassName(*req.Class)
		if !validClass(class) {
			BadRequest(w, "unknown class")
			return
		}
		c.Class = class
	}
	if req.MainSpec != nil {
		c.MainSpec = *req.MainSpec
	}
	if req.SecondSpec != nil {
		c.SecondSpec = *req.SecondSpec
	}
	if req.IsMain != nil {
		c.IsMain = *req.IsMain
	}
	if req.Active != nil {
		c.Active = *req.Active
	}
	c.UpdatedAt = now
	if c.Name != oldName {
		c.NameChangedAt = &now
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not begin transaction")
		return
	}
	defer func() { _ = tx.Rollback() }()

	chars := a.Characters.WithTx(tx)
	audits := a.Audit.WithTx(tx)
	if err := chars.Update(r.Context(), c); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not update character")
		return
	}
	if c.Name != oldName {
		if err := chars.InsertNameHistory(r.Context(), &character.NameHistory{
			CharacterID: c.ID,
			OldName:     oldName,
			NewName:     c.Name,
			ActorID:     actor,
			Reason:      req.Reason,
			CreatedAt:   now,
		}); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", "could not record name history")
			return
		}
	}
	after, _ := json.Marshal(c)
	action := "update"
	if c.Name != oldName {
		action = "rename"
	}
	if err := audits.Insert(r.Context(), &audit.Log{
		ActorID:    &actor,
		EntityType: "character",
		EntityID:   strconv.FormatInt(c.ID, 10),
		Action:     action,
		BeforeJSON: string(before),
		AfterJSON:  string(after),
		Reason:     req.Reason,
		CreatedAt:  now,
	}); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not write audit log")
		return
	}
	if err := tx.Commit(); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not commit transaction")
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
