# Automatic Interfaces discovery

PR: https://github.com/mcoder1001-cyber/NGFW/pull/199
Branch/worktree and owned files: task envelope. D-240 records additive read-only discovery policy.

Implemented: host physical PCI inventory merged into the existing state view; management and host-only rows appear automatically and host-only drawers expose no configuration controls. Configured PCI, exact live names and uniquely matching DPDK/native vmxnet3 identities avoid duplicates. Candidate-only physical markers are correlated without changing running ownership. Source-specific diagnostics preserve available rows during failed observations. False/unavailable host carrier is labelled down or unknown. Virtio-pci child netdevs resolve their physical PCI for inventory and management detection; USB/mmio devices remain excluded. Generated client updated through pnpm gen, EN/FA and user docs updated.

Actual completed verification so far:

```
pnpm gen: 13 successful, 13 total
pnpm --filter @ngfw/api exec vitest run src/state:
  Test Files 4 passed (4), Tests 17 passed (17)
pnpm --filter @ngfw/web exec vitest run ... -t 'unavailable host carrier|automatically displays':
  Test Files 1 passed (1), Tests 3 passed | 12 skipped (15)
  (selection only; no claim of full suite success)
API and web typecheck: exit 0
GOTOOLCHAIN=local go test ./internal/renderers/vppstartup:
  ok ngfw/agent/internal/renderers/vppstartup 0.679s
```

Independent applicable R1–R7 reviews APPROVE source4292daa826c5904aaf851f22f9f586f9c6112c57; report-only review commits cherry-picked to this branch. Remaining: unchanged complete quick gate (including full InterfacesPage suite) and manager sequential merge. Prior obsolete quick was stopped after source review changes; no quick pass claimed. Full old UI run showed completed cases passing with 11–59s durations before intentional cancellation; no proven infinite render loop. Latest full target still running.

Out of scope: Linux-only virtual and USB NIC discovery; new RPC/protocol shape; NIC claiming, host/engine ownership mutation, live host configuration changes. Host inventory contract exposes one row per PCI function, not one row per multiport Linux netdev. Ambiguous virtio engine/hardware identities remain separate until exact name or configured PCI identifies them. No live acceptance performed.

Real target appliance/browser screenshots and physical inventory verification NOT EXECUTED; explicitly deferred in docs/status/DEFERRED-ACCEPTANCE.md under owner instruction.

Actual gate failure and fix: short-TMPDIR complete quick reported i18next/no-literal-string on new JSX status="up". Replaced that JSX literal with typed HOST_LINK_UP constant; targeted web lint PASS, logical CSS455 files OK. Complete quick must rerun on this corrected source.

Before final scheduling retry, complete TS35/35 and full web106 files/622 tests passed, including InterfacesPage15/15. Go phase then reproduced existing HA resync50ms timing failure (0.36s, zero observations) already classified by manager as baseline scheduling flake. Full unchanged quick is retried with TMPDIR=/ift GOMAXPROCS=4; all checks and race remain enabled. No full gate pass claimed yet. Evidence: /root/ngfw-wt/logs/ci/interfaces-discovery-20261007-20261007-101441-2618508/10-agent.log.

## Final developer verification and handoff

The unchanged complete quick gate **PASSED**, exit0, on checkpoint `f4fde915037dfe6607fb939df4ec5125b9d0cf74` (product source unchanged from independently approved `e9e94d17e56470caf7526a61c59d915cc2c0866e`). The final commit after this checkpoint records results only; no further product changes or developer pushes follow handoff.

```
TMPDIR=/ift GOMAXPROCS=4 tools/ci.sh quick --base origin/main
Tasks: 35 successful, 35 total; Cached: 28 cached; Time: 1m6.593s
agent: lint, full race tests and build PASS
  ha-state-sync 1.190s; internal/agent 78.513s
CLI: lint, race tests and build PASS
all test/ Go modules: gofmt, vet and unit-mode tests PASS
apply-startup fake-host harness: all four shards PASS
mode quick · wall time 13m59s
CI GATE PASSED
```

Full output: `/root/ngfw-wt/logs/interfaces-discovery-quick-gomax4.log`; detailed logs: `/root/ngfw-wt/logs/ci/interfaces-discovery-20261007-20261007-102928-2714377`. Prior full web execution passed106 files/622 tests, including InterfacesPage15/15; the final cache replay uses that exact unchanged product source. API state17/17 and targeted newUI3/3 passed. All applicable independent R1–R7 reviews plus lint-correction addendum APPROVE.

Remaining manager work: preserve remote checkpoint history, D112/D114 integration onto actual post-wizard main, independently verify any integration consumer adjustment, complete exact combined-tree local and hosted quick gates, sequential merge and post-merge CI/board updates. Real appliance/browser inventory/screenshots remain explicitly deferred in `docs/status/DEFERRED-ACCEPTANCE.md`; no live host services/configuration changed and no live inventory acceptance claimed.
