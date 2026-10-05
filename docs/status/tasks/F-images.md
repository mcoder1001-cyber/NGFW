# Current integration readiness

Root-owned codex/integrate-images-final-20261005 in /dev/shm/ngfw-integrate-images-final-20261005, pinned parent b8c6c5478858691ca28e790d12b5f9dd32cdf981. Reviewed guard6bd unchanged; hardening PR172 verified merged with identical tested tree. SelectedfreshR1/R2/R4/R7/R8 allAPPROVE; independent exact6bd fullquick PASS22m44,13imagecases+149actualfakehostchecks, T3guard13+5adversarialPASS. Sourceproductunchanged, thischeckpoint integrates review receipts/currentstate only. Priorbf9 and6bdreviewedhistory archivedlocally/remotely beforeD112finalrewrite. Current-main rebase/singlecommit finalexacttree/local+hosted gates remainrequired; no mergeclaimed. Fullsignedappliancebuild/boot/cloudimport remainsNOTRUN missingpackagepool/fulltargetbuildcapacity. Historicalchronology below.

# F-images source implementation and acceptance

Branch: `codex/ready-images-20261005`; worktree `/root/ngfw-wt/ready-images-20261005`;
base `origin/main@7b507db5`; slot 16. PR: https://github.com/mcoder1001-cyber/NGFW/pull/173.
Initial durable checkpoint `faaa8dfa7f2c10bfabc277ec3a0a327481055bbb`;
source checkpoint `446bcac78e82840e577268f19ee9805cabdc5658` published via the
GitHub connector after CLI push returned HTTP 403. Remote SHA verification is
required for every later checkpoint; local commits alone are not publication.

## Built

Signed dependency pool/VPP producer verification; private-mount loop image
builder; exact P14 GPT labels and rootB reservation; target-only BIOS/UEFI GRUB;
serial console; explicit management NIC/netplan/base-firewall policy; shared P14
bootstrap and password banner; cloud-init VM/AWS/Azure/GCP datasources and early
drivers; qcow2/streamOptimized VMDK/OVF/OVA/VHDX, fixed AWS/Azure VHD and GCP raw
tar; offline target/package/security inspection; package versions, VPP provenance
and SHA256SUMS. No host install, host boot write, VM boot or cloud write performed.

P14 common is actually `deploy/image/iso/common`; reused read-only. Current P10
publishes `firstboot-complete`, while P14 reads `firstboot.done`. A relative image
symlink bridges the exact completion state; tests prove the bridge is absent until
P10 completion and then observes the same contents. No shared source fork.
Explicit management-interface input agrees with current packaging firewall input.
All clones get their own bootstrap API password, machine identity and SSH keys;
no Unix password login or default user is shipped. Cloud DPDK stays `no-pci`.

## Actual verification

```
python3 -m unittest discover -s test/topology/images -p 'test_*.py' -v
Ran 10 tests in 0.943s
OK
shellcheck -x deploy/image/vm/*.sh
(no findings; exit 0)
tools/ci.sh check --base origin/main
check PASSED (0m27s)
```

The 10 tests include real 16 MiB qemu-img conversions and byte comparisons for
qcow2, streamOptimized VMDK, VHDX and fixed VHD, root escape refusal, missing
package/EFI/GRUB/initramfs rejection, safe datasources, secret/identity rejection,
OVF hardware references, the opt-in root guard and the P10/P14 marker bridge.
A Go module wrapper makes these run in the unchanged quick gate. Appliance
integration inspection is skipped honestly without a prepared image root.

Complete quick gate: RUNNING at source checkpoint, log `/tmp/F-images-quick.log`
and `/root/ngfw-wt/logs/ci/ready-images-20261005-20261005-060153-1096102`.
No green quick-gate claim yet. No gate step changed or disabled.

Owned loops and mounts: `losetup -a | rg 'ready-images|w16-images'` and
`findmnt -rn -o TARGET | rg 'ready-images|w16-images'` returned no matches.
Host `/boot/grub/grubenv` was not written; its inspection hash is recorded in the
next checkpoint. Full before/after build hash acceptance is NOT RUN because no
mounted image build was launched.

## Deferred acceptance and real blockers

Full production artifact build NOT RUN: `/srv/ngfw-artifacts/apt` and matching
verified VPP manifest are absent in this environment. `df -h /` reported 29 GiB
free, below the builder's 40 GiB gate; manager explicitly prohibited large mounted
builds/conversions pending headroom. Production `ls -l`, qemu-img inspection,
partition mounts/package acceptance, SHA256SUMS and manifest are therefore not
available. Fixture conversions are not evidence of an appliance artifact.

Provide a signed complete pool, matching producer manifest and sufficient
headroom on an authorized build host; run the documented build for each profile,
record full offline inspection plus cleanup evidence. Then provision disposable
KVM/VMware/Hyper-V/Proxmox VMs and approved cloud accounts. Execute the exact
BIOS/UEFI, filesystem, DHCP/explicit NIC, console/bootstrap/login cleanup,
clone-identity, firstboot/package/nftables/API/VPP, reboot persistence and cloud
metadata/driver/import checks in `docs/install/images.md`. Record code failures
for repair; unavailable VM/account execution alone may remain deferred.

Out of scope: installer ISO, A/B logic, marketplace, cloud route-table HA,
image signing/Secure Boot, VPP compilation, SR-IOV automation and enabling an
untested hardening baseline. Release-blocking formats remain a release decision.
Shared hunks: none. Independent review: manager-assigned, pending.

Additional focused evidence: `go vet ./... && go test -count=1 ./...` in
`test/topology/images`: `ok ngfw/test/topology/images 2.233s`.
Read-only host grubenv inspection: `f64122858064885ef0733e42c6a3d2d3fd642671f714db0d974b880c0f087430`.
Independent F-hardening source review evidence is preserved alongside this task;
reviewer made no hardening product edits.

First quick gate finished FAILED-ENV at Go lint: repeated `no space left on device`
in `/tmp/g-w16/go-build2571229008`. Shared `/tmp` had 28 GiB free when rechecked
(after failed processes cleaned their temporary files). Complete final-source
gate is retried with root-disk TMPDIR `/root/.cache/g-w16`; log
`/tmp/F-images-quick-final.log`. No merge-ready/green claim until that succeeds.

## Complete unchanged quick-gate result
Retry completed with `CI GATE PASSED`, wall time **16m18s**;
log `/tmp/F-images-quick-final.log`, step logs
`/root/ngfw-wt/logs/ci/ready-images-20261005-20261005-060915-1162923`.
Actual summary includes:
```
Tasks: 35 successful, 35 total Cached: 28 cached, 35 total
apps/agent: make lint test build (passed)
apps/cli: make lint test build (passed)
test/topology/images: gofmt ok · go vet ok · ok ngfw/test/topology/images 1.902s
CI GATE PASSED
```
The independent-review path-preflight fix was applied during this run before the
image Go-module/Python suite executed. Source suite separately passed 11 tests
in 0.819s. All eleven image source tests were exercised by the module after the
fix. The manager must still gate the exact final integration tree before merge;
this log is working-tree evidence and does not claim a start-SHA-only gate.

## R4 repair — current integration source

R4 independently reproduced grub_config accepting an aliased image root and
writing outside its intended root (private fixtures only). Common mutation_root
validation now rejects host root, aliased roots/ancestors and symlink mutation
paths for configure, put and grub. GRUB validates before inspecting kernels or
writing anything.13 Python image regressions PASS, including root/alias/ancestor
refusal and preservation of external boot/grub/config sentinels. Existing real
16MiB format and valid-image GRUB fixtures continue passing. Prior bf9 reviews
and T1 remain archived provenance; the repair requires affected verification and
unchanged full integration gate. No full image/boot/cloud acceptance claimed.
