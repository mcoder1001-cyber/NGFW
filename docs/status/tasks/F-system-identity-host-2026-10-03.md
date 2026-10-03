# F-system-identity-host acceptance, 2026-10-03

Fresh production API/schema dist and `.scratch/native-product-agent` exercised in slot 6 against disposable VPP. The system VPP remained PID 1014, NRestarts=0. Existing worktree `task/F-system-identity-host` was inspected read-only; its additional acceptance assertions were imported without merging its branch.

Command: `eval "$(tools/lab env 6)"; VRX_INTEGRATION=1 VRX_SYSID_AGENT_BIN=$PWD/.scratch/native-product-agent python3 test/topology/hardware-smoke/isolated-vpp.py tools/heavy.sh go -C test/topology/system-identity test -count=1 -v -timeout 5m .`

Result: **PASS, TestSystemIdentity 14.47 s**. [Full output](F-system-identity-host-2026-10-03-evidence/topology.txt).

- Real API commit/retrieve; hostname/timezone/banner/resolver files diff against all goldens, including issue.net; localtime symlink matches Asia/Tehran.
- Unknown timezone and ESC, CR, C1 CSI, bidi RLO login banners return HTTP400 application/problem+json with the expected pointers.
- Agent restart waits for reconciliation; all rendered mtimes remain equal, no system identity application or hostname setter logged.
- Kernel hostname, real hostname/localtime, banners, hosts and resolver identity remain unchanged. Disposable VPP stopped, test database/role dropped.
- P10 package requirement confirmed at `deploy/debian/vrx/debian/control`: Depends includes systemd-resolved and tzdata.

Recommendation: host acceptance complete, ready for review/integration. No merge performed. T4 screenshots remain assigned to integrated stack after merge; no screenshot or broad CI/lint pass claimed. Focused real acceptance is current evidence; old worktree go/vitest evidence remains historical.
