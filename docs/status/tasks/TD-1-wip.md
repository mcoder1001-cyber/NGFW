# TD-1 WIP log

- 2026-09-24 start — read context, WORKER-OPS, D-063/D-071/D-076/D-080. Baseline `systemctl show vpp -p NRestarts` = NRestarts=3.
  Private boot-identity implementations found: dfkit/identity.go (strict triple, show_threads PID), df6/claims.go BootID
  (tolerant triple, control_ping vpe_pid), natcommon/config.go (VPPIdentity show_threads + tolerant triple BootIdentity),
  classify/table.go vppInstance (vpe_pid only, persisted as `vpp_instance`), acl/stats_enable.go vppIdentity (show_threads PID,
  in memory), interface/identity.go VPPIdentity + vppEpoch (show_threads PID, in memory). Callers: sr_mpls, pppoe, cnat, sflow.
- Next: apps/agent/internal/vpp/bootid + tests, then retrofit.
- 23:20 — bootid package + tests; retrofit dfkit, df6 (+sr_mpls, pppoe), natcommon (+cnat), classify store, acl stats flag,
  interface vppEpoch, sflow; fakes answer control_ping vpe_pid instead of show_threads. All agent unit tests green.
  Host run (slot 12) classify+acl: first run TestClassifyOnHost failed once with "loop1209 already has an output ACL
  (ip4-outacl) bound outside this agent's records" (transient host state; the base commit passed right after, and 3 reruns of
  the TD-1 code passed); final host run green. NRestarts 3 → 3. Docs updated. Next: CI, TD-1.md.
