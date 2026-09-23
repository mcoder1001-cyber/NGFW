#!/usr/bin/env bash
# Regenerates Go and TS stubs. Output is committed; CI fails if it is dirty.
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
rm -rf gen/ts ../../apps/agent/gen
mkdir -p gen/ts ../../apps/agent/gen
buf generate
# generated Go imports grpc/protobuf — keep go.mod in sync so the gate never fails on a fresh checkout
(cd ../../apps/agent && GOFLAGS=-mod=mod go mod tidy)
echo "proto: generated Go → apps/agent/gen, TS → packages/proto/gen/ts"
