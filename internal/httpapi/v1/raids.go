package v1

import (
	"encoding/json"
	"net/http"

	"raid-website/internal/raid"
)

type RaidHandler struct {
	repo *raid.Repo
}

func NewRaidHandler(r *raid.Repo) *RaidHandler {
	return &RaidHandler{repo: r}
}

func (h *RaidHandler) List(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
}
