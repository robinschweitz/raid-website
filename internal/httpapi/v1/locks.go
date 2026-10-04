package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/enhancepoints"
)

type createLockRequest struct {
	RaidID   int64  `json:"raid_id"`
	ItemID   string `json:"item_id"`
	ItemName string `json:"item_name"`
	Boss     string `json:"boss"`
	Notes    string `json:"notes"`
}

func (a *API) listLocks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	locks, err := a.Points.ListLocksByPlayer(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list locks")
		return
	}
	writeListJSON(w, locks)
}

func (a *API) createLock(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "authenticated session required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	var req createLockRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.RaidID <= 0 {
		BadRequest(w, "raid_id is required")
		return
	}
	lock := &enhancepoints.ItemLock{
		PlayerID: id,
		RaidID:   req.RaidID,
		ItemID:   req.ItemID,
		ItemName: req.ItemName,
		Boss:     req.Boss,
		Notes:    req.Notes,
	}
	if err := a.pointService.CreateLock(r.Context(), lock, actor); err != nil {
		if errors.Is(err, enhancepoints.ErrNotOwner) {
			WriteError(w, http.StatusForbidden, "forbidden", "only own locks may be created")
			return
		}
		if errors.Is(err, enhancepoints.ErrLockItemNameRequired) {
			BadRequest(w, "item_name darf nicht leer sein")
			return
		}
		if errors.Is(err, enhancepoints.ErrLockRaidNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "Raid nicht gefunden")
			return
		}
		if errors.Is(err, enhancepoints.ErrLockRaidClosed) {
			WriteError(w, http.StatusConflict, "lock_raid_closed", "Dieser Raid nimmt keine Locks mehr an")
			return
		}
		if errors.Is(err, enhancepoints.ErrLockConflict) {
			WriteError(w, http.StatusConflict, "lock_conflict", "Es gibt bereits einen aktiven Lock auf dieses Item")
			return
		}
		if errors.Is(err, enhancepoints.ErrLockBalanceTooLow) {
			WriteError(w, http.StatusUnprocessableEntity, "lock_not_allowed", "balance >= 150 required for a lock")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not create lock")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": lock})
}

func (a *API) cancelLock(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "authenticated session required")
		return
	}
	playerID, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	lockID, ok := pathID(r, "lockID")
	if !ok {
		BadRequest(w, "invalid lock id")
		return
	}
	lock, err := a.pointService.CancelLock(r.Context(), playerID, lockID, actor)
	if err != nil {
		if errors.Is(err, enhancepoints.ErrNotOwner) {
			WriteError(w, http.StatusForbidden, "forbidden", "only own locks may be cancelled")
			return
		}
		if errors.Is(err, enhancepoints.ErrLockNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "lock not found")
			return
		}
		if errors.Is(err, enhancepoints.ErrLockNotActive) {
			WriteError(w, http.StatusConflict, "lock_not_active", "only active locks can be cancelled")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not cancel lock")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": lock})
}
