# F-ha-state-sync independent R7 review

Reviewed repaired source48e5af12fd6a42323f3c83d04129eb6563cef650, 2026-10-05; /root/ngfw-wt/ha-acceptance-probes-20261005. Reviewer changes only this report.

MINOR: canonical report preserves 13/14-test checkpoint language while latest wip records the 16-case redirect closure. Prepend current exact-source summary and preserve prior failures as history. No false complete green gate appears: environment socket-path failure and pending retry are explicit.

Read-only commands actually executed: cat canonical/integration/wip and test/topology/ha-state-sync/README.md; git show --stat48e5af12f; inspected independent R1 repair/redirect report. Actual observed R1 latest evidence: sixteen Python cases PASS3.237s on48e5af12f, exact tracked source clean; real3.2-second single-socket delayed echo earlier PASS, no native two-node acceptance. This reviewer inspected recorded evidence, not reran those tests.

The prior generic argv-hook code gap is explicitly retired as acceptance proof. README and source status document concrete challenge-echo single TCP connection, server NAT identity, exact A/B EI tuple, VRRP roles/B counter continuity, bounded dumps and no reconnect. Priority mode documents lease/revision/candidate guard, unconfirmed60s transaction and automatic restoration; fault mode documents dedicated root-owned VM marker/boot/PID/handover nonce/strict SSH/pidfd and explicit D-012 post-handover restriction. No real shared-host signal/second appliance run is claimed. API redirect closure is recorded as actual security correction, not laboratory deferral. ED/ACL sync and native IPsec anti-replay state replication remain explicitly unsupported; IPsec requires rekey. New generated YANG correction is normal generator output, and failed prior gates are preserved. Lab-only two-node/fault/UDP capture may defer because concrete acceptance code now exists; this report does not treat missing code as deferred runtime.

No R7 BLOCKER/MAJOR. Exact48e5 complete quick, hosted gate, remaining independent applicable aspects and current-main integration remain mandatory; no pending gate is approved as passed.

Verdict: APPROVE.
