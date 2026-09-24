#!/usr/bin/env bash
# Regenerates vpn.pb.go from vpn.proto with the protoc-gen-go already pinned by packages/proto
# (buf.gen.yaml → protoc-gen-go v1.36.x, the version google.golang.org/protobuf in apps/agent/go.mod
# expects). Output is committed. Run from anywhere; needs protoc and protoc-gen-go in PATH.
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$PATH:$HOME/go/bin:/usr/local/go/bin"
protoc --proto_path=. --go_out=. --go_opt=paths=source_relative vpn.proto
gofmt -w vpn.pb.go
echo "generated $(pwd)/vpn.pb.go"
