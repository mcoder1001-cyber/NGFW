# F-mpls-ldp-host checkpoint
Branch: task/F-mpls-ldp-host-20261004
Publication: local checkpoint; manager publishes through connector.
Implemented: FRRDoc LDP preservation, named MPLS route record isolation, bounded
FRR LIB/neighbor/discovery/RIB adapter, active next-hop joining, S1 dynamic source,
60-second failed-read hold-down, successful withdrawal, neighbor events and live
cached state RPC. Added read-only slot-isolated live acceptance probe.

Actual tests:
`go test -race ./internal/frrsync/ldp ./internal/renderers/frr/ldp ./internal/descriptors/mpls`
PASS: ldp 1.188s, renderer 1.038s, mpls 1.131s.
Focused compile/test of desired+agent passed (selected filter had no matching tests).
Lab FRR/VPP acceptance NOT RUN. Aggregate quick CI not yet run.
Next: finish production wiring tests and named ownership/collision test, run
focused tests/vet and tools/ci.sh check --base main; independent manager review.
Known remaining: source-derived fixtures require pinned lab confirmation; live
renderer canonical comparison, ECMP/PHP/withdrawal/restart evidence missing.

Review corrections: dirty retry after failed apply; current LCP identity filtering;
4 MiB/10k feature-local bounds, indexed joins, cancellation; wrong-schema read
rejection; static IP-binding takeover rejected. New named-route tests prove
source/static isolation in table 0 and private tables, persisted restart retrieval,
collision refusal and stale-route deletion. New subsystem tests prove isolated
registration, configuration removal, interface removal/remap and FRR projection.
Actual final scoped race tests PASS: frrsync/ldp 1.177s, descriptors/mpls 1.106s,
subsystems TestLdp 1.151s. Python acceptance driver compiles (not a live pass).
First remote checkpoint SHA: 39efeb18c8f63df8bbc5a28541ef4ffb4deca5a1
(manager connector publication corresponding to local 8cdd9543).
Final supported dynamic cap: 256 distinct routes, with oversized-read preservation
regression. Scale follow-up owned by Codex manager, due 2026-10-11. Exact next
command after handoff: independently review current committed head and publish
through the authorized GitHub connector; lab acceptance remains deferred.

Final R8 correction: Installed is derived by the state RPC from current named
MPLS descriptor Retrieve (10-second deadline), not cached observation length.
Configuration withdrawals and failed applies therefore report actual surviving
owned routes. Retrieval/disconnection returns unavailable. Added regression for
cached-but-suppressed routes and surviving routes after failed apply.
User documentation updated for current implementation, supported 256-route EOS
IPv4 limit, explicit-null/non-EOS exclusions, LCP Linux CLI names and deferred lab.
