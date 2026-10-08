# D1.3 IP unnumbered implementation checkpoint

Branch: `codex/unnumbered-complete-20261008`; base main `417e8fcd`.
Contract remote: `35ced80f39b4b351ac483eeeb665880007be3c8b` (local `de1883f5`).

Owned: new interface unnumbered descriptor/model/tests and desired unnumbered projection/tests;
small registration, domain membership, live managed-state, projection/assembly hooks.

Completed: generated internal model, generated VPP set/dump APIs, authoritative retrieval,
claim-before-write, donor/borrower dependencies and optional table binding ordering,
same IPv4/IPv6 table checks, foreign relationship protection and fresh identity deletion guard.
Projection rejects missing/self/nested donors, cross-VRF and incompatible address/DHCP configuration.
Agent restart uses live relationship dump and persisted physical-interface claims.

Focused tests: `go test -race ./internal/desired ./internal/descriptors/interface -run 'TestUnnumbered|TestRegister'`
PASS (desired 1.210s, interface 1.023s), no lab execution. Full CI deliberately deferred per owner.

Next: verify product registry/retrieve end-to-end fake, complete integration regression,
independent source review. Live VPP/API/browser/packet and simulated loss acceptance NOT RUN.
This checkpoint does not claim implementation complete or lab acceptance.
