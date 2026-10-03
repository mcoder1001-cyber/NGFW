# OSPF observation source review

Immutable `ae28200981719ffb711b7b2dddeb767c22ca5d38`, feature scope vs b2c214b5.
Verdict: CHANGES REQUESTED for two P2 real-FRR mapping issues.

1. state.ts adjacency regex excludes `/-`. FRR stable/10.4
   [ospf_dump.c](https://github.com/FRRouting/frr/blob/stable/10.4/ospfd/ospf_dump.c#L153)
   formats point-to-point neighbor state as `%s/-` (lines 162–166).
   A valid `Full/-` therefore becomes Unknown. Direct Node evaluation of the exact
   regex returned false. Include the dash role and a point-to-point fixture.
2. state.ts rejects every non-IPv4 neighbor map key, invalidating the entire
   inventory. FRR's ordinary brief neighbor JSON uses the literal `neighbor` for
   NBMA Attempt with unknown router ID in
   [ospf_vty.c](https://github.com/FRRouting/frr/blob/stable/10.4/ospfd/ospf_vty.c#L4460).
   A valid such row mixed with healthy known neighbors becomes reader-invalid.
   Node isIP('neighbor') returned zero. Handle this precise documented shape without
   inventing an ID: nullable public router ID or explicit omitted/partial observation
   semantics, with a mixed normal/unknown-ID realistic fixture. Reject arbitrary
   malformed keys as before.

The existing Go OSPF reader registers the exact requested ospfNeighbors command
and documents FRR modern/legacy keys. AgentClient injects configured owner; the
controller does not expose reader/RIB selectors. Protected GET tests exercise
actual AuthGuard and readonly access. Agent RPC failures remain failures; stopped,
missing, malformed and limited reader cases are explicit. Raw diagnostics and
unrequested readers are excluded; byte/instance/inspection/output caps are bounded.
Tests cover those public behaviors, but existing valid-state fixtures all use
broadcast roles and IPv4 map keys, omitting the actual shapes above.

Only read-only immutable source and upstream public files were inspected. Tiny
Node predicates reproduced mapping rejection; no FRR/VPP/host operation, full test
or product change. Root owns feature registration, generated client and validation.
