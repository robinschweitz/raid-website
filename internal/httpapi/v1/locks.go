package v1

import (
	"net/http"
	"time"

	"raid-website/internal/enhancepoints"
	"raid-website/internal/validation"
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
	WriteJSON(w, http.StatusOK, map[string]any{"data": locks})
}

func (a *API) createLock(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
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
	balance, err := a.Points.Balance(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load balance")
		return
	}
	if !enhancepoints.CanLock(balance) {
		WriteError(w, http.StatusUnprocessableEntity, "lock_not_allowed", "balance >= 150 required for a lock")
		return
	}
	now := time.Now().UTC()
	lock := &enhancepoints.ItemLock{
		PlayerID:  id,
		RaidID:    req.RaidID,
		ItemID:    req.ItemID,
		ItemName:  req.ItemName,
		Boss:      req.Boss,
		Status:    validation.LockStatusActive,
		LockedAt:  now,
		ActorID:   actor,
		Notes:     req.Notes,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := a.Points.CreateLock(r.Context(), lock); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not create lock")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": lock})
}

func (a *API) cancelLock(w http.ResponseWriter, r *http.Request) {
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
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
	lock, err := a.Points.GetLock(r.Context(), lockID)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "lock not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load lock")
		return
	}
	if lock.PlayerID != playerID {
		WriteError(w, http.StatusNotFound, "not_found", "lock not found")
		return
	}
	if lock.Status != validation.LockStatusActive {
		WriteError(w, http.StatusConflict, "lock_not_active", "only active locks can be cancelled")
		return
	}
	now := time.Now().UTC()
	lock.Status = validation.LockStatusCancelled
	lock.CancelledAt = &now
	lock.UpdatedAt = now
	if err := a.Points.UpdateLock(r.Context(), lock); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not cancel lock")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": lock})
}
