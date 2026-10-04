package v1

import (
	"encoding/json"
	"net/http"
	"time"

	"raid-website/internal/loot"
	"raid-website/internal/tokens"
)

type createLootRequest struct {
	Boss                 string `json:"boss"`
	ItemID               string `json:"item_id"`
	ItemName             string `json:"item_name"`
	AwardedToPlayerID    int64  `json:"awarded_to_player_id"`
	AwardedToCharacterID *int64 `json:"awarded_to_character_id"`
	IsMainSpec           bool   `json:"is_main_spec"`
	ItemLockID           *int64 `json:"item_lock_id"`
	Notes                string `json:"notes"`
	Reason               string `json:"reason"`
}

type correctLootRequest struct {
	Boss                 *string         `json:"boss"`
	ItemID               *string         `json:"item_id"`
	ItemName             *string         `json:"item_name"`
	AwardedToPlayerID    *int64          `json:"awarded_to_player_id"`
	AwardedToCharacterID json.RawMessage `json:"awarded_to_character_id"`
	IsMainSpec           *bool           `json:"is_main_spec"`
	Notes                *string         `json:"notes"`
	Reason               string          `json:"reason"`
}

func (a *API) correctLoot(w http.ResponseWriter, r *http.Request) {
	actor, _ := ActorID(r)
	raidID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	id, ok := pathID(r, "recordID")
	if !ok {
		BadRequest(w, "invalid loot id")
		return
	}
	var req correctLootRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	var characterID *int64
	if req.AwardedToCharacterID != nil {
		if err := json.Unmarshal(req.AwardedToCharacterID, &characterID); err != nil {
			BadRequest(w, "invalid awarded_to_character_id")
			return
		}
	}
	row, err := a.Corrections.CorrectLoot(r.Context(), raidID, id, actor, req.Reason, func(row *loot.Award) {
		if req.Boss != nil {
			row.Boss = *req.Boss
		}
		if req.ItemID != nil {
			row.ItemID = *req.ItemID
		}
		if req.ItemName != nil {
			row.ItemName = *req.ItemName
		}
		if req.AwardedToPlayerID != nil {
			row.AwardedToPlayerID = *req.AwardedToPlayerID
		}
		if req.AwardedToCharacterID != nil {
			row.AwardedToCharacterID = characterID
		}
		if req.IsMainSpec != nil {
			row.IsMainSpec = *req.IsMainSpec
		}
		if req.Notes != nil {
			row.Notes = *req.Notes
		}
	})
	if err != nil {
		writeCorrectionError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": row})
}

func (a *API) listLoot(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid raid id")
		return
	}
	awards, err := a.LootRepo.ListByRaid(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list loot")
		return
	}
	writeListJSON(w, awards)
}

func (a *API) createLoot(w http.ResponseWriter, r *http.Request) {
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
	var req createLootRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.ItemName == "" {
		BadRequest(w, "item_name is required")
		return
	}
	if req.AwardedToPlayerID <= 0 {
		BadRequest(w, "awarded_to_player_id is required")
		return
	}
	now := time.Now().UTC()
	award := &loot.Award{
		RaidID:               raidID,
		Boss:                 req.Boss,
		ItemID:               req.ItemID,
		ItemName:             req.ItemName,
		AwardedToPlayerID:    req.AwardedToPlayerID,
		AwardedToCharacterID: req.AwardedToCharacterID,
		AwardedByPlayerID:    actor,
		Source:               "manual",
		IsMainSpec:           req.IsMainSpec,
		ItemLockID:           req.ItemLockID,
		Notes:                req.Notes,
		AwardedAt:            now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := a.Corrections.SaveLoot(r.Context(), award, actor, req.Reason); err != nil {
		writeCorrectionError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{
		"data":        award,
		"token_award": tokens.IsTokenAward(award.ItemName),
	})
}
