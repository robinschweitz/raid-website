package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/enhancepoints"
)

func (a *API) getPoints(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	balance, err := a.Points.Balance(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load balance")
		return
	}
	txs, err := a.Points.ListByPlayer(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load transactions")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"player_id":    id,
			"balance":      balance,
			"transactions": nonNil(txs),
		},
	})
}

func (a *API) rollPoints(w http.ResponseWriter, r *http.Request) {
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
	result, err := a.pointService.Roll(r.Context(), id, actor)
	if err != nil {
		if errors.Is(err, enhancepoints.ErrNotOwner) {
			WriteError(w, http.StatusForbidden, "forbidden", "only own points may be rolled")
			return
		}
		if errors.Is(err, enhancepoints.ErrInsufficientBalance) {
			WriteError(w, http.StatusUnprocessableEntity, "insufficient_balance", "player has no points to roll")
			return
		}
		if errors.Is(err, enhancepoints.ErrInvalidCost) {
			WriteError(w, http.StatusUnprocessableEntity, "invalid_roll", "roll is not possible")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not record roll")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (a *API) getTokens(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	txs, err := a.Tokens.ListByPlayer(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load tokens")
		return
	}
	total := 0
	for _, t := range txs {
		total += t.Amount
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"player_id":    id,
			"total":        total,
			"transactions": nonNil(txs),
		},
	})
}
