# Current final integration checkpoint

Manager-owned codex/integrate-hardening-20261005, RAMworktree /dev/shm/ngfw-integrate-hardening-20261005, pinnedparent492c0156. Reviewed source726d9214 and actualindependent fullquick PASS36m12 preserved (remote88f63715). All selected R1/R2/R4/R7/R8 APPROVE and T3fixture10PASS, combinedreport adjacent. Productsource carried unchanged; packagingmain hunks merge cleanly. Only own reports/currentstate and deferredruntime record added. Singlecommit/current-main and hosted exact-head quick required beforemerge; no finalmerge/gate PASS claimed. Root ownsintegration and publication; no active sourceauthor presumed. Shared daemon/package/config unchanged. Historical evidence follows.

# F-hardening-lite recovery

Branch `codex/ready-hardening-20261005`, worktree `/root/ngfw-wt/ready-hardening-20261005`.
Developer root slot17, daemon owner none. Own hardening sources, docs, minimal
prepare.sh and ngfw-meta install hunks. Initial published source c889740598c6e1f6195fb34256178a5871ee9e72;
local checkpoints6a206af4 andbc0f5f20 (Python build artifacts removed).

Implemented offline baseline staging, conditional SSH key-only activation,
explicit management rp_filter, per-daemon isolation profiles, pinned repository
signature/index verifier, readonly drift inspection and opt-in packaged tools.
No host boot/service/config mutation. All six focused tests passed (0.431s).
Systemd259 copied offline unit fixture scores API3.0→1.7, agent5.0→3.8;
other profile scores nginx3.0 PostgreSQL2.5 Valkey1.3 FRR4.7 Kea4/6 2.5
Unbound2.9 chrony2.8 snmpd3.1 keepalived3.1 rsyslog3.0.
No live daemon compatibility claim; source opt-in until appliance smoke passes.
Remaining: packaging tests, complete quick gate, independent review, PR.
Next: `deploy/hardening/tests/run.sh` then `tools/ci.sh check --base origin/main`.

Independent image-agent review BLOCK found missing optional-control inspection
and stale repeated-stage manifest. Fixed c899a56f: inspect selected management
file exactly; undeclared optional files FAIL; repeated stage retains prior SSH
and management selections and revalidates keys. Eight focused tests PASS0.520s.
Reviewer upgrade-agent re-review pending. Source remains opt-in for appliance
smoke. Initial local complete quick FAIL in unrelated Go timing tests plus
/tmp ENOSPC (logs/ci/ready-hardening-20261005-20261005-055644-1073447/10-agent.log).
Final unchanged complete quick will run with bounded concurrency and task TMPDIR;
no failed test or scanner rule will be weakened. Hosted PR172 gate pending.

Final security fixes: a70c88e6/73b6c32f verify immutable bounded snapshots of
Release, signature and key material, then parse exactly the authenticated bytes.
Nine focused tests passed, including concurrent signed-source replacement.
Fresh independent R2 reran all nine successfully. Root complete unchanged quick
on73b6c32f PASS17m58s, logs/ci/ready-hardening-20261005-20261005-062246-1309117;
scenario15 required the gate's existing one serial rerun and passed unchanged.
No assertion or scanner rule changed. Add test/topology/hardening-lite Go wrapper so the
unchanged complete quick discovers and executes the nine Python security/control
tests automatically. Final independent T1 must run this new integration tree.
Remote previous source60e9c4b1 on PR172. Mandatory panel remains pending.
