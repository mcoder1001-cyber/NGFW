# R4 independent final inventory review

Source: 4292daa82 (source branch codex/interfaces-discovery-20261007; own cherry-pick22b3f9ceb). Earlier contract checkpoints859721ec8/c450739df and UI a3d2322f7 were also inspected.

No unresolved data-plane/shared-host safety finding. Added Go helper only parses existing readlink strings; accepts direct PCI or exactly one canonical numeric virtioN child, never walks arbitrary ancestors to misidentify USB controller/MMIO as NIC. HostNICs and management ifacePCIs reuse identical resolver. No binapi/proto changes, host writes, device binds, process restart, daemon/object/DB creation or privilege changes. Existing agent HostNics remains read-only. Management protection is improved for virtio PCI child layouts.

No integration slot/lab lock needed for these pure temporary fake-sysfs tests. No live mutation/packet/restart proof asserted; no new forwarding object exists.
```text
TMPDIR=/root/ngfw-review-tmp/interfaces-review pnpm --filter @ngfw/api exec vitest run src/state/interface-discovery.test.ts
Test Files1 passed; Tests8 passed; tests49ms; duration43.62s
TMPDIR=/root/ngfw-review-tmp/interfaces-review pnpm --filter @ngfw/web exec vitest run src/domains/interfaces/InterfacesPage.test.tsx -t 'automatically displays|bound PCI host NIC'
Test Files1 passed; Tests2 passed/13 excluded; EN3404ms; FA2666ms
TMPDIR=/root/ngfw-review-tmp/interfaces-review pnpm --filter @ngfw/web exec vitest run src/domains/interfaces/InterfacesPage.test.tsx -t 'does not report unavailable host carrier'
Test Files1 passed; Tests1 passed/14 excluded; tests3036ms; duration50.08s
TMPDIR=/root/ngfw-review-tmp/interfaces-review go test ./internal/renderers/vppstartup -run 'TestHostNICs|TestNetdevPCI|TestReadHost' -count=1
ok ngfw/agent/internal/renderers/vppstartup0.112s
```

Findings:0 BLOCKER;0 MAJOR;0 MINOR.
Verdict: APPROVE (source review; integration gates still required).
