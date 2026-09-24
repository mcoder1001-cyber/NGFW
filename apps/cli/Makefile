# vrx CLI (P13). Not a pnpm workspace package: run `make -C apps/cli lint test build` (tools/ci.sh does not know
# apps/cli yet — P13-questions #1).
BIN := bin/vrx
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
export PATH := $(PATH):$(HOME)/go/bin:/usr/local/go/bin
# the acceptance grep, spelled so that this file does not match it itself
NO_VPP_TOOL := vpp""ctl

.PHONY: all build lint test check gen docs e2e clean
all: lint test build

build:
	go build -trimpath -ldflags "-s -w -X ngfw/cli/internal/cli.Version=$(VERSION)" -o $(BIN) ./cmd/vrx

lint: check
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: files above need formatting"; exit 1; }
	go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run --allow-serial-runners ./...; else echo "golangci-lint not installed — go vet only"; fi

# unit tests only; the e2e skips without VRX_INTEGRATION=1
test:
	go test -race -count=1 ./...

# P13 acceptance: the CLI never reaches the agent or VPP directly
check:
	@if grep -rn "$(NO_VPP_TOOL)\|/run/vpp\|govpp\|agent\.sock\|vrx\.v1\.Dataplane" --include='*.go' . ; then echo "direct agent/VPP access in apps/cli"; exit 1; fi
	@echo "ok: apps/cli talks only to the REST API"

# regenerate the operation table from packages/api-client/openapi.json (written by `pnpm gen`)
gen:
	go run ./cmd/vrx-opgen -in ../../packages/api-client/openapi.json -out internal/api/operations_gen.go

docs:
	go run ./cmd/vrx-docgen -out ../../docs/user/cli/reference.md

# needs the slot dev stack: eval "$$(tools/lab env <N>)"; tools/lab lock shared apps/cli/test/devstack.sh start
e2e:
	VRX_INTEGRATION=1 go test -count=1 -v ./test/e2e/

clean:
	rm -rf bin
