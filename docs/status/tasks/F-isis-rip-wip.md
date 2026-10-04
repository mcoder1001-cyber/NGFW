# F-isis-rip completion WIP

Branch: codex/next-isis-rip-20261004; base dfd001be4. Remote checkpoint: pending first contract publication. Owned files/scope: envelope alongside this file.

Recovered existing IS-IS/RIPv2 renderer, FRR desired forwarding, adjacency poller and combined configuration UI. Initial additive schema/proto source introduces RIPng, IS-IS family switches/password references, explicit RIPv2 version and Event21. Contracts generated; schema contract 2 tests and Go contracttest PASS. First contract checkpoint publishing.

Remaining: contract validation/generation, compatible-level and RIPng references, family/auth renderer, RIPng renderer, strict observed-state readers/API/live UI, globals-only OSI descriptor, docs and focused tests. Production password delivery remains PENDING-secret-channel; irreversible OSI enable and all live-host acceptance remain deferred. Current failure: none. Next command: publish exact contract tree via connector, then implement semantic/renderer consumers.


2026-10-04 consumer checkpoint (unfinished): previous remote ebafcb1c99239934d7ef28aa31eb1412d83c7870, tree e9c418ebcb26cdb2bb818235304f4bf3fce1e75b. Added RIPng section/projection, IS-IS family switches/auth via resolver, semantic circuit/family/RIPng references, globals-only explicitly opted-in OSI descriptor/getter, optional fixed-command text parser seam and Event21 mapping. Corrected prerequisite Event20 production EventOf drop with v2/v3 regression. IS-IS/RIP existing tests PASS; first aggregate tests hit /tmp inode exhaustion (100% inodes); rerunning with TMPDIR=/root/.cache/ngfw-isis-test-tmp. No OSI enable or daemon mutation executed. Still remaining text parsers/API/live UI/user docs/new consumer tests; not complete. Next command: finish focused consumer tests, publish checkpoint, then implement observed state/API/UI.

2026-10-04 root integration: consumer checkpoint c4533e5cf recovered onto current
main and combined with tunnels. Text state/API/live grids/docs completed; sealed
API-to-agent routing-password delivery and production FRR resolver now implemented.
Focused schema4/API12/UI2 checks pass; FRR protocol/OSI/contract packages pass.
Source review/integration pending; laboratory packet/FIB/restart/browser acceptance
remains explicitly deferred, not a passing result. Earlier WIP paragraphs are historical.
