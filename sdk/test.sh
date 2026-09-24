#!/usr/bin/env bash
# sdk/test.sh — the unit gate of the automation clients (not part of tools/ci.sh yet; ~15 s):
#   Python SDK: venv from the hash-pinned lock, pytest (mocked HTTP; the live test skips without VRX_INTEGRATION)
#   Terraform provider: gofmt, go vet, golangci-lint (when installed), go test (httptest fake API + protocol harness)
# Live runs against a real API/agent/VPP: test/topology/sdk-terraform-ansible/live.sh run … (see that script).
set -euo pipefail
SDK="$(cd "$(dirname "$0")" && pwd)"
export GOTOOLCHAIN=local
echo "== python sdk"
"$SDK/python/lock.sh" venv >/dev/null
(cd "$SDK/python" && .venv/bin/pytest -p no:cacheprovider)
echo "== terraform provider"
bad=$(gofmt -l "$SDK/terraform")
[[ -z $bad ]] || { echo "gofmt: $bad" >&2; exit 1; }
go -C "$SDK/terraform" vet ./...
if command -v golangci-lint >/dev/null 2>&1; then (cd "$SDK/terraform" && golangci-lint run ./...); else echo "golangci-lint not installed — go vet only"; fi
go -C "$SDK/terraform" test -count=1 ./...
echo "sdk: OK"
