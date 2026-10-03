package main

import (
	"fmt"
	"log"
	"net/http"

	"raid-website/internal/config"
	"raid-website/internal/httpapi"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	h := httpapi.New()
	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
