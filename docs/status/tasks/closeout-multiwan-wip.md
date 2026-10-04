# Multi-WAN live acceptance WIP

Branch: `codex/closeout-multiwan-live`; base `d44d44eb6`.

Owned files: `test/topology/multiwan-host-acceptance/**`, `docs/status/tasks/closeout-multiwan*`. No product edits.

Real-stack fixture staged beside existing ACL harness. Real API outside, disposable VPP and real agent inside owned `ns-w7-mw-router`; LAN/two WAN peers isolated namespaces, slot7. Actual device-bound ICMP monitor and actual installed FIB route state, strict 3/3 packet gates for baseline/failover/restore/restart, rollback must remove default and produce 0/3. Linux LCP addresses and two endpoint-specific routes are fixture setup in the private router namespace; FRR is not driven. No root routes or physical NICs modified.

Current result: PASS32.73s, no skips, final fixture source `0b1343a4a1664f0c83e2dba6e763443997f900ef`. Actual static-gateway IPv4 failover/restore/restart/rollback evidence in closeout-multiwan.md and closeout-multiwan-evidence/final2.txt. Slot7 released; shared VPP MainPID1014/NRestarts0 unchanged. Wider balance/NAT/PBR/browser/IPv6 cases remain NOT RUN. No product changes. Source guard final PASS11s (`tools/ci.sh check --base d44d44eb6`); complete root quick/API gates are manager-owned. Local evidence checkpoint committed after this update; final manager publication/review handoff pending.

Next command: `NGFW_MULTIWAN_PRODUCT_ROOT=/root/.codex/worktrees/6189/NGFW NGFW_MULTIWAN_BIN_DIR=/root/ngfw-wt/codex-closeout-multiwan-live/.scratch/multiwan-bin python3 test/topology/multiwan-host-acceptance/run.py`.

Publication: manager published earlier fixture tree as remote `ead537cb1bf627034c089867b39065b6b4949fa2` with blob/tree verified; final source/evidence require updated manager publication. Local source commits alone are not durable remote publication.
