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
