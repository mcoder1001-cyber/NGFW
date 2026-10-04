#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../../.."
: "${NGFW_ACCEPTANCE_API_MAIN:?set current-source built apps/api/dist/main.js}"
: "${NGFW_ACCEPTANCE_AGENT:?set current-source built ngfw-agent}"
export NGFW_SLOT=9 NGFW_TEST_PREFIX=w9 NGFW_HTTP_PORT=3900 NGFW_WEB_PORT=5900
export NGFW_METRICS_PORT=9191 NGFW_AGENT_SOCKET=/run/ngfw-test/w9/agent.sock
export NGFW_PG_DATABASE=ngfw_w9 NGFW_VALKEY_DB=9 NGFW_VPP_TABLE_BASE=9000
exec tools/heavy.sh python3 test/topology/hardware-smoke/isolated-vpp.py \
  python3 test/topology/management-dataplane/acceptance.py
