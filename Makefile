.PHONY: fmt vet lint test frontend build all

fmt:
	gofmt -w cmd internal

vet:
	go vet ./...

lint:
	golangci-lint run

test:
	go test ./...

frontend:
	cd web && npm ci && npm run build

build: frontend
	go build -o bin/ ./cmd/server

all: fmt vet lint build test
