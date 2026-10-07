# Current final integration checkpoint

Manager-owned codex/integrate-hardening-20261005, RAMworktree /dev/shm/ngfw-integrate-hardening-20261005, pinnedparent492c0156. Reviewed source726d9214 and actualindependent fullquick PASS36m12 preserved (remote88f63715). All selected R1/R2/R4/R7/R8 APPROVE and T3fixture10PASS, combinedreport adjacent. Productsource carried unchanged; packagingmain hunks merge cleanly. Only own reports/currentstate and deferredruntime record added. Singlecommit/current-main and hosted exact-head quick required beforemerge; no finalmerge/gate PASS claimed. Root ownsintegration and publication; no active sourceauthor presumed. Shared daemon/package/config unchanged. Historical evidence follows.

# F-hardening-lite source and verification

Branch codex/ready-hardening-20261005, PR172. Source implementation complete;
mandatory independent panel, final integration quick and merge remain pending.

The opt-in offline stager supplies a conservative OS baseline, optional
key-only SSH and explicit management-interface reverse-path control, thirteen
systemd profiles, a read-only exact-content compliance checker and pinned APT
Release verification/key rotation tooling. It never activates controls on the
shared host. Packaging ships these assets without installing active drop-ins.
VPP and the retired strongSwan unit are excluded; nftables remains the existing
host-policy renderer's responsibility.

Ten fixture tests cover unsafe root/ancestor paths, management-interface and
SSH selection, repeated staging/manifest drift, exact compliance, signed,
unsigned/tampered/wrong-key repositories, rotation and concurrent replacement
of metadata during verification. The verifier authenticates bounded immutable
copies and parses exactly those signed bytes. The unchanged quick discovers
them through test/topology/hardening-lite.

Actual commands/results:

```
deploy/hardening/tests/run.sh: Ran 9 tests; OK (before explicit rotation case)
GOFLAGS=-p=2 go -C test/hardening-lite test -race -count=1 ./...:
  ok ngfw/test/hardening-lite 4.997s (before wrapper path alignment)
TMPDIR=/root/.cache/ngfw-hardening-tmp GOMAXPROCS=2 GOFLAGS=-p=2 \
  NGFW_CI_TASK_CONCURRENCY=2 tools/heavy.sh tools/ci.sh quick --base origin/main:
  CI GATE PASSED; wall time 17m58s; source73b6c32f
```

Quick logs: /root/ngfw-wt/logs/ci/ready-hardening-20261005-20261005-062246-1309117.
The existing gate reran scenario15 serially after a parallel timing failure;
the unchanged assertion passed. Wrapper addition follows that run, so the final
independent T1 gate must include the wrapper rather than inherit this old pass.
Fresh independent R2 reran all nine tests and approved the security fixes.

Shared hunks: deploy/debian/ngfw/prepare.sh ships the hardening assets;
ngfw-meta.install adds their package path; test_prepare.py copies the hardening
fixture and checks shipped-but-inactive controls. No tools/ci.sh gate change.

Offline systemd-analyze fixture scores satisfy the stated API/agent targets;
see docs/install/hardening.md for control mapping and exceptions. Actual
appliance daemon operation under these profiles, signed package install and
full boot smoke are NOT RUN: the required signed pool/appliance environment
is unavailable. This is deferred runtime acceptance, not CIS certification.
No shared host configuration/service, reference VPP or production signing key
was modified. Remaining lab smoke is recorded by the manager before closure.

Final acceptance addition exercises throwaway old/new signing keys, overlap trust,
new-signature switch and old-key retirement rejection. The final test count is10;
source implementation of the verifier is unchanged.
