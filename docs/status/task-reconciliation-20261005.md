# Task reconciliation — 2026-10-05

Image PR173 merged at e5605d9c, but its board closeout was bundled into upgrade PR174. Main now contains the correct image and upgrade closures; preserve their exact receipts. Hardening PR172 was already correctly merged. Upgrade PR174 merged at31ad9213. IPsec PR176 subsequently merged atf445f57c; close its stale P11-host running row independently of the next feature PR.

P12-fib-proof historical8084739c PASS does not establish current acceptance. PR180 withdrew runner cleanup approval; fresh proof failed at mgmtd startup before the200-route criteria. Remove obsolete LAB-vpp-per-slot dependency/blocker and record actual failure. Keep parked, awaiting resume. No failure is waived.

## Open PR ownership snapshot

The seven-task parent chat is active: PR175 BFD and PR178 HA Draft remain in final implementation/review/current-main gates; VPN access still requires VPP/ACL integration. Preserve main's newer detailed worker-status evidence. Parent activity does not prove each individual worker alive. PR174 and PR176 are now merged.

Acceptance chat stopped: PR179 Draft HOLD (historical MPLS recommit504 cause unresolved, fresh replay incomplete); PR180 Draft HOLD (current FRR startup failure); PR181 Draft on PR180 branch (AutoBlock correction requires corrected dependency and final gates). These are recovery checkpoints, not duplicates. Closing or merging would hide unfinished work. Their existing HOLD bodies document the failures. Awaiting resume, not active development.

No product source, timeout, acceptance criterion or mandatory gate changed. Historical merged host evidence does not imply full acceptance. Further acceptance rows remain parked until missing proof is reconciled. Merge closures should be recorded with the corresponding source merge, not bundled into the next unrelated feature.
