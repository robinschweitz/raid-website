.PHONY: fmt vet lint test frontend build all

fmt:
	gofmt -w cmd internal

vet:
	go vet ./...

lint:
	@echo "golangci-lint not installed in PATH; skipping if missing"

test:
	go test ./...

frontend:
	cd web && npm ci && npm run build

build: frontend
	go build -o bin/ ./cmd/server

all: fmt vet build test
