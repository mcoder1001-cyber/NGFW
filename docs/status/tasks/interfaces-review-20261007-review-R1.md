# R1 independent final inventory review

Source: 4292daa82 (source branch codex/interfaces-discovery-20261007; own cherry-pick22b3f9ceb). Earlier contract checkpoints859721ec8/c450739df and UI a3d2322f7 were also inspected.

No unresolved correctness finding. Fixtures independently prove automatic management/data/unclaimed discovery without Apply or datastore seeding; PCI/exact-name/unique physical MAC correlation; ambiguous MAC and virtual lookalike separation; distinct running/candidate physical marker fallback; both directions of partial observation failure. Engine rows preserve live state; inventory-only rows have null engine/config/counters and no invented VRF. Own Go fixture verification covers virtio PCI management/data detection and USB/mmio rejection. The unknown-link finding is fixed and frontend regression passes.

Earlier full InterfacesPage attempt was intentionally terminated before a default-reporter result after >3minutes, exit143. Cause was unverified; no infinite render-loop claim. Only selected new frontend tests were completed independently. Per manager envelope, do not duplicate the heavy complete gate; final unchanged quick gate and hosted gate remain required on final integration tree. No live-stack/hardware acceptance claim.
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
