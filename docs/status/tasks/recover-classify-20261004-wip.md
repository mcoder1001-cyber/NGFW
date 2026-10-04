# Classify recovery — 2026-10-04

Branch: `codex/recover-classify-20261004`; remote publication: pending root manager connector.
Base: `980d54be5`; checkpoint SHA is the commit containing this file (`git rev-parse HEAD`).
Recovered reset helper, fixture resets, rig repair-before-delete, static Go/shell guards, classify table-0 sentinel and globals-owner reconnect wiring from reviewed historical merges `ada90234` and `75d12c1f`. Preserved current LCP sanitizer and IGMP reconnect behavior. Imports/env/lock paths follow NGFW; legacy fixed 16-byte sentinel signature is retained to recognize existing sentinels.

Actual checks:
- `git diff --check` and staged equivalent: pass.
- `bash -n tools/ci.sh tools/lab test/topology/wireguard/stack.sh`: pass.
- `tools/ci.sh check --base origin/main`: `check PASSED (0m12s)` including shell reset guard, secrets and slots. This is static check mode, not full quick CI.
- `go test ./internal/vpp/ifsanitize/... ./internal/subsystems/...`: pass; ifsanitize 3.131s, subsystems 12.741s, ruleexpiry 0.322s. Integration cases skipped without NGFW_INTEGRATION.
- `go test` affected descriptors acl, df2, df6, df7, dfkit, gso, interface, lcp, natcommon, vpn, vxlan and agent: pass; agent 35.361s.
- `go test ./...` in test/topology/bonding and loopback-bvi-gso-lldp-span: pass (0.012s, 0.031s); live-host cases skipped.

No remaining product code in these two recovered tasks. Current failure: none. Full hosted CI waived by owner; fresh live VPP acceptance remains deferred to manager laboratory window. Historical acceptance is evidence only, not fresh validation.
Next command: `git show --stat HEAD` for independent review; manager publishes and integrates sequentially against current main.
