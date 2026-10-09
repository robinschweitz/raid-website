package v1

import (
	"net/http"
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
