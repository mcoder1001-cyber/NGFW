BIN := bin/vrx-agent
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
export PATH := $(PATH):$(HOME)/go/bin:/usr/local/go/bin

.PHONY: build lint test tidy run clean
build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/vrx-agent
lint:
	go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; else echo "golangci-lint not installed — go vet only"; fi
test:
	go test -race -count=1 ./...
tidy:
	go mod tidy
run: build
	$(BIN)
clean:
	rm -rf bin
