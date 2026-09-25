# P12 topology test — BGP through linux-cp

`run.sh` runs `TestP12TopologyOnHost` (apps/agent/internal/agent/p12_topology_integration_test.go) for the slot in
`VRX_TEST_PREFIX`: the agent creates `host-<p>l0/w0` on the slot's veth rig with a linux-cp pair each (taps `<p>-l0`,
`<p>-w0` in `ns-<p>-frr`), the slot's FRR runs BGP there (AS 65080), two FRR peers run in `ns-<p>-lan` / `ns-<p>-wan`
(AS 65081/65082, 100 × /25 each in 10.<N>.64–163.0). Steps: commit → 200 routes · route map denying half → 100 ·
withdraw on a peer → gone < 5 s · agent restart with the pairs deleted behind its back → recovered · link down on a
VPP interface (observed) · rollback of the BGP config → 0 routes, sessions down.

The VPP-side count (linux-nl → FIB source `lcp-rt-dynamic`) is checked only with `VRX_P12_LINUXNL=1`, which must run in a
manager window (it sets the VPP-global lcp default netns for the first pair; docs/status/tasks/P12-questions.md Q1).
