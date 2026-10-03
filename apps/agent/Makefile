BIN := bin/ngfw-agent
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
export PATH := $(PATH):$(HOME)/go/bin:/usr/local/go/bin

.PHONY: build lint test tidy run clean
build:
	@printf 'NGFW_TIMING BEGIN agent.build %s\n' "$$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/ngfw-agent
	@printf 'NGFW_TIMING END agent.build %s\n' "$$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
lint:
	@printf 'NGFW_TIMING BEGIN agent.vet %s\n' "$$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
	go vet ./...
	@printf 'NGFW_TIMING END agent.vet %s\n' "$$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
	@printf 'NGFW_TIMING BEGIN agent.lint-dispatch %s\n' "$$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run --allow-serial-runners ./...; else echo "golangci-lint not installed — go vet only"; fi
	@printf 'NGFW_TIMING END agent.lint-dispatch %s\n' "$$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
test:
	@printf 'NGFW_TIMING BEGIN agent.race-test %s\n' "$$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
	go test -race -count=1 ./...
	@printf 'NGFW_TIMING END agent.race-test %s\n' "$$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)"
tidy:
	go mod tidy
run: build
	$(BIN)
clean:
	rm -rf bin
