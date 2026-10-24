package v1

import (
	"net/http"

	"raid-website/internal/statistics"
)

// StatisticsHandlers serves the public statistics (FR-27, SC-004).
type StatisticsHandlers struct {
	Statistics *statistics.Service
}

type statusCountResponse struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

type classCountResponse struct {
	Class string `json:"class"`
	Count int64  `json:"count"`
}

type roleCountResponse struct {
	Role  string `json:"role"`
	Count int64  `json:"count"`
}

type playerCountResponse struct {
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	Count      int64  `json:"count"`
}

// statisticsResponse is the dedicated public DTO; it lists exactly the fields
// of ARCHITECTURE.md "Statistics".
type statisticsResponse struct {
	RaidsByStatus     []statusCountResponse `json:"raids_by_status"`
	ClassDistribution []classCountResponse  `json:"class_distribution"`
	RoleHistory       []roleCountResponse   `json:"role_history"`
	LootByClass       []classCountResponse  `json:"loot_by_class"`
	TopAttendance     []playerCountResponse `json:"top_attendance"`
	TopLoot           []playerCountResponse `json:"top_loot"`
}

func (h StatisticsHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/statistics", h.get)
}

func (h StatisticsHandlers) get(w http.ResponseWriter, r *http.Request) {
	s, err := h.Statistics.Get(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load statistics")
		return
	}
	out := statisticsResponse{
		RaidsByStatus:     []statusCountResponse{},
		ClassDistribution: []classCountResponse{},
		RoleHistory:       []roleCountResponse{},
		LootByClass:       []classCountResponse{},
		TopAttendance:     []playerCountResponse{},
		TopLoot:           []playerCountResponse{},
	}
	for _, row := range s.RaidsByStatus {
		out.RaidsByStatus = append(out.RaidsByStatus, statusCountResponse{Status: row.Status, Count: row.Count})
	}
	for _, row := range s.ClassDistrib {
		out.ClassDistribution = append(out.ClassDistribution, classCountResponse{Class: row.Class, Count: row.Count})
	}
	for _, row := range s.RoleHistory {
		out.RoleHistory = append(out.RoleHistory, roleCountResponse{Role: row.Role, Count: row.Count})
	}
	for _, row := range s.LootByClass {
		out.LootByClass = append(out.LootByClass, classCountResponse{Class: row.Class, Count: row.Count})
	}
	for _, row := range s.TopAttendance {
		out.TopAttendance = append(out.TopAttendance, playerCountResponse{PlayerID: row.PlayerID, PlayerName: row.PlayerName, Count: row.Count})
	}
	for _, row := range s.TopLoot {
		out.TopLoot = append(out.TopLoot, playerCountResponse{PlayerID: row.PlayerID, PlayerName: row.PlayerName, Count: row.Count})
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}
