#!/usr/bin/env bash
# Regenerates Go and TS stubs. Output is committed; CI fails if it is dirty.
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
# Both output trees are 100% generated (the contract tests live in apps/agent/internal/contracttest
# and packages/proto/test): wipe and regenerate.
rm -rf gen/ts ../../apps/agent/gen
mkdir -p gen/ts ../../apps/agent/gen
buf generate                                  # vrx/v1: Go + gRPC + TS (buf.gen.yaml)
buf generate --template buf.gen.model.yaml    # vrx/model: Go only (agent-internal object model, D-055)
# generated Go imports grpc/protobuf — keep go.mod in sync so the gate never fails on a fresh checkout
(cd ../../apps/agent && GOFLAGS=-mod=mod go mod tidy)
echo "proto: generated Go → apps/agent/gen (vrx/v1 + vrx/model), TS → packages/proto/gen/ts (vrx/v1)"
