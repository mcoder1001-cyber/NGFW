# TD-5 — WIP

started 2026-09-24T14:17, slot 2 (w2), branch task/TD-5, base main@bccb9e4 (main merged at 14:37 and 14:40)

- [x] quiesce helper (`af_packet/quiesce.go` logic + `quiesce_linux.go` netlink) behind `Links`
- [x] Delete: quiesce → BeforeDelete → af_packet_delete; Create rollback: quiesce → af_packet_delete (fail closed, orphan named)
- [x] unit tests with a fake link controller (order, EPERM, timeout, ENODEV, already-down, still-up, settle cancelled)
- [x] guard `TestEveryAfPacketDeleteIsQuiesced` + planted temp tree
- [x] integration_test.go: disable_ipv6 before up, explicit Delete, netdev reads down, vppctl check, per-call timings
- [x] ifsanitize cap: holes + 2×FreshRun ≤ 64 (D-105); holes = every freed index proven by the pop order (D1 in questions)
- [x] D-108 rings: TX 2048×256, RX 2048×8 (unit test + host `show hardware-interfaces`)
- [x] docs: af_packet.md "Delete ordering (D-101/V24)" + rings, interface.md cap paragraph, V24 row agent-side status
- [x] host runs 14:32 (capped → D1), 14:35, 14:41 (final): NRestarts 0 → 0 each
- [x] make lint test green (88 packages)
- [x] tools/ci.sh --base main: CI GATE PASSED on 3381e3e (14:50)
- [x] TD-5.md (done 14:5x)

Fix round 1 (15:14–, `TD-5.fix1.md`):
- [x] main merged (7f90854), apps/agent/bin + apps/cli/bin deleted
- [x] H1 TX 67584 × 16 (D-113) · M1 reviewer's fix + probe test · L2 WithoutCancel + 30 s · L3 veth-only · L1 restore · N1–N4
- [x] unit + guard + lint green; host run 15:36 NRestarts 0 → 0 (create 4.6/4.4 s, delete 0.33/0.31 s)
- [x] tools/ci.sh --base main: CI GATE PASSED on 5c1148f (15:43, wall 5m36s)
- [x] TD-5.md "Fix round 1", questions F1–F3

Manager notes: D-107 (quiesce lowers, does not remove the V24 double close — stated in docs), D-108 (rings), merge main before gate.
