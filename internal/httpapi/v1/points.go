package v1

import (
	"errors"
	"math/rand/v2"
	"net/http"
	"time"

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
			"transactions": txs,
		},
	})
}

func (a *API) rollPoints(w http.ResponseWriter, r *http.Request) {
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
	balance, err := a.Points.Balance(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load balance")
		return
	}
	cost, err := enhancepoints.RollCost(balance)
	if err != nil {
		if errors.Is(err, enhancepoints.ErrInsufficientBalance) {
			WriteError(w, http.StatusUnprocessableEntity, "insufficient_balance", "player has no points to roll")
			return
		}
		WriteError(w, http.StatusUnprocessableEntity, "invalid_roll", "roll is not possible")
		return
	}
	now := time.Now().UTC()
	tx := &enhancepoints.Transaction{
		PlayerID:  id,
		Amount:    -cost,
		Type:      "enhance_roll",
		Reason:    "enhance roll cost",
		ActorID:   actor,
		CreatedAt: now,
	}
	if err := a.Points.InsertTransaction(r.Context(), tx); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not record roll cost")
		return
	}
	minRoll, maxRoll := enhancepoints.RollRange(cost)
	roll := minRoll + rand.IntN(maxRoll-minRoll+1)
	WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"player_id":     id,
			"cost":          cost,
			"roll":          roll,
			"min":           minRoll,
			"max":           maxRoll,
			"balance_after": balance - cost,
		},
	})
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
			"transactions": txs,
		},
	})
}
