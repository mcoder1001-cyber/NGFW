# Final64 API acceptance envelope and checkpoint

Branch `codex/final64-api-acceptance`, isolated worktree `/root/ngfw-wt/codex-final64-api-acceptance`, frozen product source `64fffb9dda441f169834ecb1dfc06865a1c6d37d`. Own evidence/status only, no product updates. Slot9; disk TMPDIR/GOTMPDIR, bounded heavy scheduler. No shared VPP restart or host service mutation.

Main PR164 changed real RIP authentication/IGP secret delivery code/contracts. Prior ac12 receipts remain preserved but do not certify this new product source. Prepare current own frozen dependency install and schema/proto/YANG/API builds, independently build production agent with source-diff checks before/after, record artifact hashes with neutral labels. Then run unchanged complete API integration config against real PostgreSQL/Valkey/fake agent; preserve exact default-missing-binary skips and run those three real-agent tests separately with certified binary/disposable VPP. No skips count as PASS.

Own frozen install, four TS component builds and production agent build passed, with exact product source diff empty before/after. Agent SHA256 `397de0aa478627c833212d0ba40830795c7cbb04a9833dc7fc61a6d4fe6791de`. Main entry digest unchanged from ac12 because source changes are imported modules; whole compiled API/schema/proto/YANG artifact-tree digests are separately recorded to bind those changed modules. Full API suite exited0:60files289tests PASS,1file/exact3 default-missing-binary skips (292total),825.12seconds. Cleaned slot9 DB/role and2097 prefixed Valkey keys. Exact3 real-agent tests then PASS/no skips,36.08seconds, own certified64 binary/private VPP. Combined fresh292 API tests PASS; initial3skip receipt remains preserved. Source/history guard PASS10seconds, gitleaks clean. Final64 post-run receipt verifies agent hash, whole compiled API/schema/proto/YANG tree digests unchanged and zero product-source diff. Management/TLS/dataplane preview repeat also PASS on same exact64 source: same API PID across certificate hot swap, TLS1.2 refusal/TLS1.3 acceptance for minimum1.3, exact invalid-certificate pointers, zero PEM leaks, unchanged startup/sharedVPP1014/NRestarts0. Never invoked apply/restart. Owned VPPs stopped; slot9 DB/role removed after each suite. Publication/final gates are manager-owned.

Exact real-agent command:

```
env $(tools/lab env 9 | sed 's/^export //') TMPDIR="$PWD/.scratch/tmp" NGFW_INTEGRATION=1 NGFW_ISOLATED_TEST_RUN=1 NGFW_AGENT_BIN="$PWD/.scratch/ngfw-agent" tools/heavy.sh python3 test/topology/hardware-smoke/isolated-vpp.py pnpm --filter @ngfw/api exec vitest run -c vitest.e2e.config.ts test/integration/agent.int.test.ts
```

Management uses existing `test/topology/management-dataplane/run.sh` with owned current API main and certified agent paths, NGFW_ISOLATED_TEST_RUN1 and disk TMPDIR. No product or fixture changes; evidence-only commits. Complete final integration/hosted gates and browser/appliance acceptance remain separate.

Final `tools/ci.sh check --base 64fffb9d`: PASS; gitleaks scanned ~100.59KB with no leaks. Log `closeout-final64-api-evidence/final-check.log`. Product source remains untouched and own worktree is clean after this evidence checkpoint.
