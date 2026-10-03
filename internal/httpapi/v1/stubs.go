package v1

import (
	"encoding/json"
	"net/http"
)

func RaidListStub(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
}
