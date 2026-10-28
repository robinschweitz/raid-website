package v1

import (
	"errors"
	"net/http"
	"strings"

	"raid-website/internal/character"
)

type mergeCharacterRequest struct {
	SourceCharacterID int64  `json:"source_character_id"`
	Reason            string `json:"reason"`
}

func (a *API) mergeCharacter(w http.ResponseWriter, r *http.Request) {
	targetID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid target character id")
		return
	}
	var req mergeCharacterRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.SourceCharacterID <= 0 || strings.TrimSpace(req.Reason) == "" {
		BadRequest(w, "source_character_id and reason are required")
		return
	}
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "authentication required")
		return
	}
	err := character.NewService(a.Characters).MergeDuplicate(r.Context(), actor.ID, targetID, req.SourceCharacterID, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, character.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", stableErrorMessage("not_found"))
		case errors.Is(err, character.ErrForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", stableErrorMessage("forbidden"))
		case errors.Is(err, character.ErrMergeDifferentOwner), errors.Is(err, character.ErrMergeConflict), errors.Is(err, character.ErrMergeInactive):
			WriteError(w, http.StatusConflict, "merge_conflict", stableErrorMessage("merge_conflict"))
		case errors.Is(err, character.ErrMergeSame), errors.Is(err, character.ErrReasonRequired):
			WriteError(w, http.StatusUnprocessableEntity, "invalid_merge", stableErrorMessage("invalid_merge"))
		default:
			WriteError(w, http.StatusInternalServerError, "internal", "could not merge characters")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
