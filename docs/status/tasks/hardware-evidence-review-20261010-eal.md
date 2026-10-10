# Independent EAL failure review — same hardware campaign

Scope: docs/source/private receipt inspection only. Reviewer performs no target or
product mutation. Current physical acceptance verdict: BLOCK pending a corrected
renderer/native artifact and actual guarded physical result. This is a real runtime
failure, not a deferred laboratory-only test.

Actual root-owned failure receipt f64ad15bd241d4bbed2e3219900b6836138dcaf13be17ce2f916df2bc9559de1
is18019B0600. Independent selected JSON assertions confirm no COMMITTED, rolled-back
marker present, no console/deadman marker; five distinct VPP starts failed EINVAL22.
Native log reports old configuration restored, VPP/management healthy at14:30:10
and locks released at14:30:11. The inactive/dead native unit Result=success reflects
successful rollback. Full private journal/config/network remain off Git.

Independent source command/output:

```text
Python urllib read exact FDio VPP c3200b88dc46bd380f00a49ca3392a102cc1980b files
src/plugins/dpdk/device/init.c SHA256=6e29b55f722f348f62b8aecf3dc543a4e381475410586bd51a296aab6cbff660
build/external/packages/dpdk.mk SHA256=0590711efcc03255442fbb826e29d2ada8acf6f7dc819d4dcf583da13b030c93
dpdk.mk default version=26.03; tarball SHA pinned
literal PCI blacklist => EAL -b at init.c1303–1310
configured nonblacklisted devices => EAL -a at init.c1485–1500
DPDK26.03 eal_common_options.c215–218 rejects nonempty allow and block lists
DPDK26.03 Linux eal.c556–560 maps that parser failure to EINVAL
actual failed rendered startup:17 dev rows + management04 blacklist
```

Pinned [VPP parser](https://github.com/FDio/vpp/blob/c3200b88dc46bd380f00a49ca3392a102cc1980b/src/plugins/dpdk/device/init.c#L1303)
turns these clauses into incompatible EAL options. Its [DPDK build pin](https://github.com/FDio/vpp/blob/c3200b88dc46bd380f00a49ca3392a102cc1980b/build/external/packages/dpdk.mk#L24)
matches the independently read [DPDK conflict check](https://github.com/DPDK/dpdk/blob/v26.03/lib/eal/common/eal_common_options.c#L215)
and [Linux EINVAL path](https://github.com/DPDK/dpdk/blob/v26.03/lib/eal/linux/eal.c#L556).
This is a source-derived cause tied to the actual configuration and failure;
full live expanded EAL argv was not captured. No diagnostic VPP relaunch is required.

Appropriate narrow correction: when explicit physical dev rows exist, emit only
their closed allowlist; exclude protected management and blacklisted PCIs from
that allowlist through the existing rejecting model. Do not also emit literal
EAL blacklist clauses. With no physical dev rows, retain existing noPCI/blacklist
output unchanged. Canonical apply separately snapshots protected PCI identities
through management interface discovery; this must remain independent of rendered
blacklist syntax. Independent final product diff/regressions, full hosted quick,
corrected native helper identity and exact fresh render/dryrun precede any retry.

Root owns the isolated mandatory correction. Worker source diagnosis was published
at3363c2293510a907184b176e0d1ed44150faf690; reviewer independently used the primary
sources above and actual receipt, not just that worker conclusion. Current .211
VPP is rolled back to noPCI; buffers65536 remains stored but not enforced. Physical
enumeration/forwarding/reboot acceptance remains incomplete.

## Independent exact correction source review

PR226 candidate97ae88ee5b6aaf304f78547abed39e80bbeac5e1 is one commit on
d1f3f19d4837de3f7a36bfffcbd3c70593bea307, tree449ea85e5111ad946245200dbf8f6a3490c5353a.
Actual remote readback matches. All11 declared files read; product correction
source applicability APPROVE. The template branches on a nonempty validated
device list, emits the closed explicit data allowlist plus management-exclusion
comments, and uses literal blacklist/no-pci only without devices. buildDevices
still rejects every protected management PCI in whitelist/devices. Canonical apply
still independently discovers management interfaces/PCI and snapshots their drivers
at deploy/vpp/apply-startup.sh939–943. No contract, privilege or VPP-version change.

Exact independent focused command/output, executed read-only against root's
isolated correction source checkout, not presented as this reviewer's product edit:

```text
CWD=/dev/shm/ngfw-hardware-firstboot-integration-20261010/apps/agent
go test ./internal/renderers/vppstartup -run 'Test(DPDKDeviceSelectionDoesNotMixEALAllowAndBlockLists|ManagementFromHost|ManagementPaths|SixNICSample|Golden)$' -count=1
ok  ngfw/agent/internal/renderers/vppstartup 0.092s
git status --short
<empty after root's coherent commit>
git ls-remote origin refs/heads/codex/hardware-eal-fix-20261010 refs/heads/main
97ae88ee5b6aaf304f78547abed39e80bbeac5e1 refs/heads/codex/hardware-eal-fix-20261010
d1f3f19d4837de3f7a36bfffcbd3c70593bea307 refs/heads/main
gh run view 38061027648 --json status,conclusion,headSha
{"conclusion":"","headSha":"97ae88ee5b6aaf304f78547abed39e80bbeac5e1","status":"in_progress"}
```

R7-1 BLOCKER: eal-fix-wip.md10 claims the root's Go package tests passed0.435/0.274
without exact command/pasted stdout; the completed check14s also lacks pasted
output in the candidate. Mandatory R7 requires command/output evidence, not only
prose. Root accepted the finding and will append existing output once immutable
97ae native preparation completes; no new test is requested. R7 current verdict
BLOCK for this documentation item, separate from the source applicability APPROVE.
Pending complete hosted quick/native/runtime results are correctly kept pending.
Decision D246 records closed allowlist, rejected mixing and retained management
validation. Scope stays within the owner's existing hardware correction campaign.
