# Routing acceptance on 172.30.126.195 — 2026-10-10

Source base `4908716b4501312102382e6979b8fc1ded6f9311`; final tested product `391ec77bd24719eff4ac767f3105c100000adae0`, branch `codex/lab-routing-20261010`, PR218. Manager independently reviewed the namespace fixes, fixture changes and narrow PPPoE delegation fix. No shared VPP restart/config change, system FRR modification, real host NIC manipulation or throughput claim occurred. Path is **af_packet** in private outer network/mount namespaces and disposable real VPP26.06, with current FRR10.7.1.

## Actual complete host evidence

| Task/check | Actual command | Actual result |
|---|---|---|
| P12-fib-proof | `eval "$(tools/lab env 14)"; TMPDIR=/tmp/ngfw-lab-routing-tmp GOTMPDIR=/tmp/ngfw-lab-routing-tmp python3 test/topology/frr-linuxcp/private-fib.py` | `TestP12TopologyOnHost PASS89.86s`, package89.988s, noSKIP. Real FRR/kernel/VPP200→100→50→100; pair-loss plus agentrestart recovered100 in7.2s; postrestart50→100; rollback0 and owned-object cleanup. `run-p12-6.txt`. |
| F-ospf-host topology | Assigned6, same TMPDIR/GOTMPDIR, `flock -n /run/lock/ngfw-acceptance-slot6.lock python3 test/topology/ospf/private-fib.py` | `TestOSPFTopologyOnHost PASS64.13s`, package64.259s, noSKIP. BothFull adjacencies/real100FRR+VPP routes; withdrawal50/restoration100 each300ms; pair-loss plus agentrestart6.3s≤30s; postrestart50→100 each300ms; rollback0. `run-ospf5.txt`. |
| Current OSPF FRR golden/live state/poller/rollback | Assigned6, `python3 docs/status/tasks/lab-routing-20261010-evidence/run-frr-live.py` under exclusive slot6 lease | `TestOSPFLive PASS45.28s`, noSKIP:16golden/interface lines match, DryRunempty, Fullneighbor/50routes, default+connected redistribution, actual readers/poller and withdrawal/rollback. `run-frr-live2.txt`. |
| Current OSPF HTTP validation | Assigned6, fresh baseline490 API/agent binaries (exact hashes in `api-build-provenance.txt`), `NGFW_OSPF_EVIDENCE=.../api400 test/topology/ospf/api400.sh` | Undefinedarea51 validate400 and commit400 with `/routing/ospf/interfaces/host-w6l0/area`, runningOSPFnull; backbone stub400 with `/routing/ospf/areas/0/type`. `api400-run.txt`. |
| Production polling regression | `cd apps/agent; go test -race -count=1 -run TestPppoeDelegation ./internal/subsystems` | PASS1.713s, including existing lifecycle/ownership tests and deterministic exact emptyretry/idle/active/withdrawal/cancellation coverage. `pd-poll-regression.txt`. |

All paths above refer to `docs/status/tasks/lab-routing-20261010-evidence/` unless otherwise shown. Original test counts, deadlines, restart, postrestart withdrawal and rollback assertions are preserved. Real recovered routes and configured states were checked after restart; no unit-only replacement.

## Cleanup and boundaries

P12 guarded runner reports `PRIVATE_REMAINING={}`, `PRIVATE_PROCESSES_AFTER={}`, `OUTER_CLEANUP=PASS`; privateVPP3732947 stopped. Shared before/after **routes/LCP/MainPID1014/NRestarts0 exact equal**. OSPF privateVPP3742728 stopped and absent; all12 observed FRR PID/starttime/private-namespace identities absent afterwards; private netns/FRR handles empty (`ospf-owned-processes-before.json`, `ospf-cleanup.json`). OSPF service MainPID1014/NRestarts0 equal. OSPF lock named globals is taken inside private execution; all writes stay in disposable VPP/outer namespace. FRR-only namespace/FRR handles empty and shared service unchanged. API candidate discarded, own PIDs3703963/3703974 stopped, DBngfw_w6 dropped and run directory removed, sharedNRestarts0.

## Failures preserved and fixed

Earlier raw failures remain in numbered receipts: transient compiler exit made the strict process-inventory guard abort; immutable untouched GRE fallback links stopped OSPF preflight; host full filesystem prevented frr UID PID/log writes; repeated empty PPPoE delegation sync repeatedly retrieved unrelated FRR and kept Health busy past30s. Manager repaired temporary unreserved disk capacity; repository fixes preserve live PID-identity rejection and skip only vanished producers, strictly validate immutable empty fallback devices, supply the owned empty OSPFv3 fixture daemon required by the dual-version poller, and idle only successfully reconciled empty delegation snapshots. Active leases still reconcile everytick, failures retry, and active→empty withdraws once; global resync/config/reconnect remains unchanged. No deadline was increased.

Complete unchanged quick gate is in progress on exact product391 (`quick.txt`). Current-host acceptance for **both tasks is satisfied**; boardclosure/merge remains manager-owned and awaits mandatorygreen gate, final independent review and integration. This report does not claim a merge or completed CI.
