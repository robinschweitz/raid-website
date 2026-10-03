package httpapi

import (
	"net/http"

	v1 "raid-website/internal/httpapi/v1"
)

func New() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", v1.Health)
	mux.HandleFunc("/api/v1/raids", v1.RaidListStub)
	return mux
}
