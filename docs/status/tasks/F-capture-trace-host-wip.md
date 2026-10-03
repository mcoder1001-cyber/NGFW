# F-capture-trace-host recovery checkpoint

Branch: `codex/capture-host-20261003`; PR: https://github.com/mcoder1001-cyber/NGFW/pull/137.
Published checkpoint: `881c1b5f0293bfc2bb5868283f6baab58b7d2ff8` (same source tree as local `552f7201`). First local checkpoint `67d78104c3690e517fa911b48de948334b65551e`.
Owned files: `test/topology/capture-trace/**`, API capture e2e, agent capture integration test and these task docs.
Completed source: authenticated lifecycle/download/audit/admin guard/busy/BPF e2e; same-boot persisted recovery; dedicated-VPP rig runner with safety gates and 9 Python tests. Missing dedicated VPP explicitly skips in generic full integration, while an explicitly configured shared socket fails.
Actual checks: Python9 PASS; focused Go TestCapture PASS (host skipped); API ESLint + complete typecheck PASS; forbidden/contract/gitleaks check PASS. Actual API e2e setup blocked by absent PostgreSQL; no assertions executed.
Mandatory quick with verified pinned toolchain passed generation/guards, then encountered 11 baseline API suites failing Unix-socket EPERM; remaining Turbo still running. Log `/tmp/capture-ci4.log`, CI step logs `/tmp/ngfw-ci/task-capture-20261003-203532-5/`. Gate not passed and no merge authorized by this evidence.
Remaining actual acceptance: dedicated-VPP recovery, rig packets/BPF and actual UI screenshot. No VPP/services available. Source report and README distinguish NOT RUN from source completion.
Next action: execute the unchanged complete quick in the hosted runner with socket privileges; run actual API e2e in provisioned PostgreSQL/Valkey slot, then dedicated-VPP acceptance. No checks disabled.
