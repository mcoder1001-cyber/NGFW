BIN := bin/vrx-agent
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
export PATH := $(PATH):$(HOME)/go/bin:/usr/local/go/bin

.PHONY: build lint test tidy run clean
build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/vrx-agent
lint:
	go vet ./...
	@command -v golangci-lint >/dev/null && golangci-lint run ./... || echo "golangci-lint not installed — go vet only"
test:
	go test -race -count=1 ./...
tidy:
	go mod tidy
run: build
	$(BIN)
clean:
	rm -rf bin
