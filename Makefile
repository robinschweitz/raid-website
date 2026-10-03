.PHONY: fmt vet lint test build all

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	@echo "golangci-lint not installed in PATH; skipping if missing"

test:
	go test ./...

build:
	go build ./...

all: fmt vet build test
