# F-dataplane-ui source completion — 2026-10-04

Branch: codex/complete-dataplane-ui-20261004; worktree /tmp/ngfw-complete-dataplane.
Base: origin/main 121c09747. Contract local 9aecc792d; published remote dd2d62edf51334b621d4cff84e87a685e0d1142a (exact same tree).
Owned: dataplane proto additive fields and generated Go/TS bindings; agent rpc_dataplane_startup*.go and read-only runtime integration test; API features/dataplane; DataplanePage and tests; en/fa dataplane strings; user dataplane documentation. API client generation belongs to manager integration.

Completed source: actual VPP thread CPU/core/NUMA readback via generated ShowThreads; fixed show-only cli_inband probes for loaded plugins, RX placement, hardware NIC details and verbose memory/page sizes. Failures explicit, never replaced with installed settings. Installed startup settings and host hugepage pool retain their separate meanings. UI renders observed runtime separately from candidate/running document comparison and installed file; preview now displays rendered source and unified diff. Apply remains disabled; no restart or file write path added.

Verification:
- go test ./internal/agent -run 'Startup|DataplaneRuntime' -count=1: PASS; includes disconnected and installed/runtime divergence tests.
- NGFW_INTEGRATION=1 go test ./internal/agent -run '^TestDataplaneRuntimeOnHost$' -v -count=1: PASS on actual /run/vpp/api.sock; threads=1 plugins_bytes=9578 queues_bytes=334 memory_bytes=2116, zero probe failures. Read-only under shared lab lock, no objects created.
- API focused Vitest: 3 tests PASS.
- Web DataplanePage + locale focused Vitest: 5 tests PASS (first runs needed dependency builds, then passed).
- Web typecheck: PASS.
- API build and proto/schema/client generation via workspace scheduler: PASS. Generated API-client changes deliberately excluded from this branch ownership.
- Full CI not run per user's existing waiver.

Remaining acceptance: independent review pending; browser screenshot against deployed real REST endpoint and complete transactional commit/rollback/restart acceptance remain lab/deployment work. No claim of full product acceptance. VPP runtime memory output reports observed memory/page sizes, host hugepage totals are not claimed as VPP usage.
Current failure: none in focused completed checks. Exact next command: git diff origin/main -- apps/agent/internal/agent/rpc_dataplane_startup.go apps/api/src/features/dataplane apps/web/src/domains/system/dataplane.
Publication of final source checkpoint: pending below; manager must verify expected tree before merge.
