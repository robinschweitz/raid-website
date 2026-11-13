package v1

import (
	"errors"
	"net/http"
	"strconv"

	"raid-website/internal/enhancepoints"
)

// publicPool is the only pool data visitors see: ID and name (FR-16.9).
type publicPool struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type poolRequest struct {
	Name string `json:"name"`
}

// queryPoolID reads the optional pool_id query value; 0 means "the default
// pool". A malformed value answers 400 and returns false.
func queryPoolID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.URL.Query().Get("pool_id")
	if raw == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		BadRequest(w, "invalid pool id")
		return 0, false
	}
	return id, true
}

// resolvePool turns the pool_id query into an existing pool ID, defaulting to
// the first pool (ADR 113).
func (a *API) resolvePool(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := queryPoolID(w, r)
	if !ok {
		return 0, false
	}
	var err error
	if id == 0 {
		id, err = a.Points.DefaultPoolID(r.Context())
	} else {
		_, err = a.Points.GetPool(r.Context(), id)
	}
	if errors.Is(err, enhancepoints.ErrPoolNotFound) {
		WriteError(w, http.StatusNotFound, "ep_pool_not_found", "EP-Topf nicht gefunden")
		return 0, false
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load EP pool")
		return 0, false
	}
	return id, true
}

// listPools serves the public pool list, oldest first (FR-16.9).
func (a *API) listPools(w http.ResponseWriter, r *http.Request) {
	pools, err := a.Points.ListPools(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load EP pools")
		return
	}
	out := make([]publicPool, 0, len(pools))
	for _, pool := range pools {
		out = append(out, publicPool{ID: pool.ID, Name: pool.Name})
	}
	writeListJSON(w, out)
}

func (a *API) createPool(w http.ResponseWriter, r *http.Request) {
	a.savePool(w, r, 0)
}

func (a *API) updatePool(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid pool id")
		return
	}
	a.savePool(w, r, id)
}

// savePool creates or renames a pool; only an ADMIN reaches it (FR-16.10).
func (a *API) savePool(w http.ResponseWriter, r *http.Request, id int64) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "authenticated session required")
		return
	}
	var req poolRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	pool := &enhancepoints.Pool{ID: id, Name: req.Name}
	err := a.pointService.SavePool(r.Context(), actor, pool)
	switch {
	case errors.Is(err, enhancepoints.ErrPoolNameInvalid):
		WriteError(w, http.StatusUnprocessableEntity, "ep_pool_name_invalid", "Der Name des EP-Topfs fehlt oder ist zu lang.")
	case errors.Is(err, enhancepoints.ErrPoolNameTaken):
		WriteError(w, http.StatusConflict, "ep_pool_name_taken", "Es gibt schon einen EP-Topf mit diesem Namen.")
	case errors.Is(err, enhancepoints.ErrPoolNotFound):
		WriteError(w, http.StatusNotFound, "ep_pool_not_found", "EP-Topf nicht gefunden")
	case err != nil:
		WriteError(w, http.StatusInternalServerError, "internal", "EP-Topf konnte nicht gespeichert werden.")
	default:
		status := http.StatusOK
		if id == 0 {
			status = http.StatusCreated
		}
		WriteJSON(w, status, map[string]any{"data": publicPool{ID: pool.ID, Name: pool.Name}})
	}
}

// listPublicPoints serves the public EP standing of one pool: balances only,
// no transactions (FR-16.9).
func (a *API) listPublicPoints(w http.ResponseWriter, r *http.Request) {
	poolID, ok := a.resolvePool(w, r)
	if !ok {
		return
	}
	rows, err := a.Points.ActiveBalances(r.Context(), poolID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load balances")
		return
	}
	writeListJSON(w, rows)
}

// getPoints serves a player's balance and history in one pool (FR-16.12).
func (a *API) getPoints(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	poolID, ok := a.resolvePool(w, r)
	if !ok {
		return
	}
	balance, err := a.Points.Balance(r.Context(), id, poolID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load balance")
		return
	}
	txs, err := a.Points.ListByPlayerInPool(r.Context(), id, poolID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load transactions")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"player_id":    id,
			"pool_id":      poolID,
			"balance":      balance,
			"transactions": nonNil(txs),
		},
	})
}
