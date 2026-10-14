# Appliance half-resource startup defaults

## Current recovery status — 2026-10-14

Recovery branch: `codex/finish-resource-records-20261014`, based on main `4f70b3315b9641738f9d193bbad6fb271c9a3d3f`. This recovers documentation from `codex/appliance-half-resource-defaults` at `fc1fcaa4ae0a78b61b3a04442af466deabb1ebb4`, whose base was `8eb2f11c4`. That source branch contains eight new documentation/artifact files and no product code changes. Its older base predates the Ethernet feature now on main; only its unique records are recovered here.

Owned files and boundaries are in [the envelope](appliance-half-resource-defaults-envelope.md). The six source artifacts retain their exact original bytes as `.py.txt` / `.mjs.txt`; [the archive index](appliance-half-resource-defaults/README.md) provides hashes and replay limitations. These are historical operation records, not installed product tools or a general auto-tuner. Existing branches are preserved.

All appliance observations below come from the published source checkpoint. This recovery worker did not contact either appliance or repeat its runtime checks; the records do not prove current host state. GitHub main quick run `38590496159` and packaging run `38590496148` were separately read through the GitHub connector: both are `completed/success` for exactly main `4f70b3315b9641738f9d193bbad6fb271c9a3d3f`. Links and timestamps are in [the Ethernet closure record](restore-ethernet-display-dual-host-wip.md).

## Historical allocation, formatted summary

| Property | Appliance `172.30.110.211` | Appliance `172.30.126.37` |
| --- | --- | --- |
| Observed CPUs | 20 logical / 12 physical | 44 logical / 22 physical |
| Observed RAM | 31,863 MiB | 15,874 MiB |
| DATA NICs | 17: twelve X710 10G, five I226 2.5G | Seven I226 |
| Management | `enp4s0`, PCI `0000:04:00.0`, driver `igc` | `enp12s0`, PCI `0000:0c:00.0`, driver `igc` |
| Main core | 16 | 1 |
| Worker cores | 2–9 and 17: nine workers | 2–11 and 23–33: 21 workers |
| Total allocation including main | Ten logical / six physical CPUs | 22 logical / 11 physical CPUs |
| Hugepages | 7,680 × 2 MiB = 15 GiB | 3,584 × 2 MiB = 7 GiB |
| Buffers per NUMA node | 1,048,576 | 524,288 |
| RX/TX queues | Eight on X710; four on I226 | Four per DATA NIC |
| RX/TX descriptors | 2,048 | 2,048 |
| Running revision after apply | 3 | 2 |

## Historical source record (retained verbatim)

The following text preserves the original narrative, including its historical next-step and review statements. Those statements describe the earlier execution, not this recovery. In particular, the archived `verify.mjs.txt` alone does not prove the expected DATA port count; its limitations are documented in the archive index.


Owner authorized live VPP resource configuration, all DATA NIC VFIO binding and startup/restart on 172.30.110.211 and172.30.126.37. Protect management; no shared dev-host changes. Branch codex/appliance-half-resource-defaults; base8eb2f11c4. Owned files docs/status/tasks/appliance-half-resource-defaults*.md and operation artifacts in sibling directory. No product source edits or universal performance claims.

Hardware verified:21120logical/12physical CPUs,31863MiB RAM,17data NICs (12X71010G,5I2262.5G), managementenp4s0PCI04:00.0. 37:44logical/22physical CPUs,15874MiB RAM,7I226data,managementenp12s0PCI0c:00.0. IOMMU groups exist;37 data already VFIO. Both VPP no workers;211 startup no-pci,37 seven devs1queue1024rings.

Plan211:main16,workers2-9,17 (10logical and6physical total),15GiB2MiBhugepages,1048576buffers/node,8RX/TXqueuesX710 and4IGC,2048RX/TXdesc. Plan37:main1,workers2-11,23-33(22logical/11physical),7GiBpages,524288buffers/node,4queues/2048desc. Round RAM down within schema integerGiB; keep OS remainder. Existing configured logical names and all non-dataplane domains preserved.

Snapshot both running revisions via installed DatastoreService, root-protected on-box backups only (not published). Legitimate scoped persistence uses installed CommitService.systemCommit with PostgreSQL advisory locks, existing validation/agent/promote and fail-closed audit begin/finish. Root guard before using existing service UID for peer DB auth. No JWT creation, SQL mutation, credentials output, login change or HTTP listener. Generator --check with planned hugepage host overrides passes17/7devices and excludes verified management. Reviewer pending. Next: reserve actual pages and verify counts, systemCommit scoped tuning, dry-run product apply, bind211 isolated DATA groups, apply with exact old/new hashes and deadman, verify runtime/API/reconcile and durable boot settings. Roll back host binding/pages if startup apply rolls back. No throughput benchmark performed.

## Live outcome,2026-10-14 15:17UTC

Runtime reservation PASS:2117680x2MiB=15GiB,373584x2MiB=7GiB. Legitimate audited system commits promoted running revisions2113/372, dataplane startup domain correctly reports not-applied until manager startup step. No non-dataplane changes. Exact generated product startup application:
-211 old367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184,new889f9a6fea6c2ec0e29088d4a816441bfe8c349f6913c64e06e3e05eff0ccc32,work/var/lib/ngfw/startup-apply/20261014-151047-62477,COMMITTED healthy60s7identityreads,deadman canceled.
-37 oldeae58f26b4bf2aa36be5468d5810fd5b52c48397aa654a2d911c99195c3d202c,new585fedd51cf19d91ca0708a6a6387a034a0b0076f5d6e12c367b57983802e7dc,work/var/lib/ngfw/startup-apply/20261014-151017-1744388,COMMITTED healthy60s6reads,deadman canceled.

VPPshowthreads:9/21workers+1main pinned asplanned; showhardware all17/7ports,2048RX/TXdescriptors,8queuesX710/4I226, RSS active. AgentresyncAPPLIED17/7interfaces after restart. Final installed StateController interfaces via local service container:dataplane+hostInventoryavailable,observationErrors[],17/7data rowsstatepresent/awaitingfalse/VFIOtrue; managementenp4s0/enp12s0igc/VFIOfalse. Brief startup observation delay on211 cleared on fresh query. Startup generator semanticdiff against storedrunning exits0 both. VPP/API/agent/nginxactive,NRestarts0,managementaddresses/default routes intact.

Persistence:hugepages in/etc/sysctl.d/80-vpp.conf,CPUperformance oneshot dependencyBeforeVPP only policieswhoserelated_cpus subset of budget,LimitMEMLOCKinfinity dropin andliveprlimitunlimited.211 per-device bootbinder dependency installed andsystemdverifyclean;37 preexisting reviewed bootbinder preserved. Initial211driverctlbinding added dynamicvendor/device VFIO matches; removed introduced1572/125c IDs via remove_id withoutdetach and archived17driverctlpersistfiles outside/etc. Finalbootuses onlyper-deviceoverride/probe, no globalIDs/nounsafeIOMMU. Binder--checkPASS17 singletongroups+protectedmanagement,all17currentoverridesvfio verified. Independentreview idempotencyfindingfixed by settingoverride onalreadybounddevice; performance policy scopeguardadded. Fullmachine reboot/trafficbenchmark notexecuted; no maximumthroughput claim. Recoveryfiles protectedroot /root/ngfw-half-resources-20261014,productapply rollbackbackups retained.

Remotecheckpointcf0c4bb10; nextcommand:publish finaloperationscheckpoint and completePR236 hostedgreen expectedheadmerge. This branch records task-specific operational defaults; does not install a universal unreviewed auto-tuner or modify sharedlab host.

Final independent operational review: corrected per-device override idempotency and shared frequency-policy scope; no blocking findings. Root-authored operation scripts are recovery artifacts, not a shipped authentication route. Published checkpoint 7fd2d4297.

Additional boot repair on 211: only loopback was known to ifquery; /etc/network/interfaces.d was empty while a manually started dhclient held enp4s0's current lease. Persisted the same existing DHCP method in /etc/network/interfaces.d/ngfw-management (`auto enp4s0`, `iface enp4s0 inet dhcp`), without restarting DHCP/networking. `ifup --no-act --verbose enp4s0` parses the expected dhclient invocation; current 172.30.110.211/24 and default gateway remain intact. The DHCP server remains responsible for lease assignment. Both VPP instances and APIs remain healthy. Full operating-system reboot acceptance and packet throughput measurement are not claimed.


## Remaining acceptance and next action

Full operating-system reboot acceptance on both appliances, authenticated browser rendering, packet traffic and throughput measurement remain NOT RUN. No maximum-throughput claim is made. Current host reachability, installed source and configuration need fresh verification before a later authorized acceptance campaign. This recovery does not restart VPP or networking.

Recovery verification actually performed in the isolated worktree:

- Six archived sources: exact bytes equal their named source-commit blobs, SHA-256 values match the archive index, and mode is non-executable `0644`.
- Four Python sources: `ast.parse` passed without executing code. Two JavaScript modules: `node --input-type=module --check` passed without importing or running the installed service.
- Local Markdown links resolve; `git diff --check` passed. Searches under `deploy`, `.github`, `tools`, and root `package.json` found no reference to these archival scripts.
- `gitleaks dir docs/status/tasks/appliance-half-resource-defaults --no-banner --redact --config .github/gitleaks.toml`: scanned 20.03 KB; no leaks found.
- `PATH=/workspace/scratch/e4f791ef53f7/ci-tools:$PATH tools/ci.sh check --base origin/main`: `check PASSED (0m03s)`, including forbidden patterns, board validation (212 tasks) and slot checks. The pre-commit history scan covered zero new commits; a post-commit history scan remains the next step.

Local working parent and recovery base: `4f70b3315b9641738f9d193bbad6fb271c9a3d3f`. Remote recovery publication is pending. Next local command after commit: `PATH=/workspace/scratch/e4f791ef53f7/ci-tools:$PATH tools/ci.sh check --base origin/main`; then publish the reviewed recovery tree through GitHub. Independent review and the unchanged complete hosted quick gate remain integration requirements. Historical main CI receipts do not cover a later recovery-branch integration.
