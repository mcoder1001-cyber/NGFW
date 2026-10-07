# Published host prerequisite audit — 2026-10-07

Audited source: origin/main `3ddb1680e475e94d43e8036cd3776bc60c87208b`, fetched before creating the authorized isolated branch/worktree. Main was green: complete hosted CI run [37590128647](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37590128647). PR179 is merged at that SHA. Open PR inventory at start: 180, 193, 196; none is this audit. Verified remote task checkpoint: `2fe5e6422a510232e0693c4b9dcb80857c11cdef`. No board or product edits and no lab execution in this task. Live worker inventory: unverifiable; historical owner/slot labels do not establish active workers.

## TD-19 — resume source gaps, do not repeat the trust decision

Recovered implementation: `04dbab69d4a4c01da9e8b99e84af7996cccff4fc` installs D-238's exact owner-authorized primaries; corrective `5e2d08c7f4bfdbd330ed0ffcde2bc231aafe8d87` uses canonical NodeSource selection and the shared identity parser. Both are published in audited main. `git merge-base --is-ancestor 5e2d08c7f4bfdbd330ed0ffcde2bc231aafe8d87 origin/main` exits0. DEC-238-td19-repository-trust.md supersedes the board's unanswered PENDING and older trust-material report/DEFERRED-ACCEPTANCE wording. Exact set checks, changed-identity refusal, private GPG selection and artifact/transfer guards already exist; recover them, do not replace them. Trust authorization does not establish Ubuntu26.04 install/boot compatibility.

Remaining source failures/gaps, distinct from lab deferral:

- scripts/20-install-build.sh still installs docker.io/docker-compose-v2; scripts/40-install-lab.sh still installs Docker, libvirt/qemu-kvm and pinned containerlab. The board historically requests containerlab but the original TD19 prompt explicitly removes these under the higher-precedence VMware/no-container rules. A source worker should remove forbidden platform dependencies; no installation is needed to do that.
- Script40 has floating pip upgrade and unversioned robotframework/SSHLibrary/scapy/pytest/requests; script20 falls back to unpinned `npm install -g pnpm`. Resolve reviewed exact versions/lock inputs consistent with existing runtime contracts, not guessed pins.
- Original NGFW_INSTALL_ROOT/--dry-run and scripts/tests/td19.sh acceptance seam is absent. Existing --check-config and redirected temporary fixture tests are narrower. Do not run production installers bare or call --dry-run until the safe interface is implemented and tested.
- test/topology/ngfw-b.yml and ngfw-c.yml still specify /root/vpp/build-root rather than the published /srv/ngfw-artifacts/vpp/<version> directory. Local tools/lab verification still uses 26.06* rather than exact deploy/vpp/VERSION. Remote transfer verifies all seven package identities/digests and installed versions already; preserve it.
- Current no-apply provisioning plans print `ssh root@tbd` and exit3 because both inventories are planned. This is an inventory prerequisite, not an SSH failure. /srv/ngfw-artifacts/vpp and /srv/ngfw-artifacts/apt are absent on this host. Their absence does not disprove a remote publisher's artifacts; obtain an exact accessible artifact location.

Executable safe next task (source-only, in a manager-assigned TD19 implementation worktree, after ownership transfer):

```bash
python3 -B docs/status/tasks/TD-19-run-fixtures.py
bash scripts/20-install-build.sh --check-config
bash scripts/40-install-lab.sh --check-config
shellcheck -x -P SCRIPTDIR scripts/00-add-repos.sh scripts/20-install-build.sh scripts/40-install-lab.sh tools/lab
tools/lab provision ngfw-b
tools/lab provision ngfw-c
```

These commands validate current source or print plans; planned-inventory exit3 is expected and must remain visible. Continue the integrated source above in a new implementation branch; own scripts00/20/40, TD19 fixture files, tools/lab provision hunks and b/c vpp.source only as separately assigned. Implement the enumerated gaps, retain strict refusal fixtures, review independently and run unchanged complete hosted quick plus provisioning fixtures before merge. This audit owns none of those edits.

Concrete human/manager request for later target acceptance: supply disposable Ubuntu26.04 ngfw-b/c management IPs, exact non-management PCI/port-group mapping and target owner; publish the genuine seven shipping runtimes with manifest/SHA256SUMS at an accessible versioned artifact directory; supply reviewed root-owned no-start policy and explicit target ownership handover. Approve target installation separately after source gaps are closed. No shared-host packages, units, sysctl or security changes are requested. D238 needs no further answer unless the identity set rotates.

## F-nat46-host — bounded return support and proof are already integrated

Published main commit `e3517a98353dd081558465adcf2bac04eaae3aad` contains the restored fallback/projection/driver and closeout records; ancestry check exits0. Current nat46.go emits embedded /64 domains when bits64..95 are zero and low32bits encode the service IPv4. desired/nat46.go retains exact masked MAP-E/T IPv6-key collision refusal. Schema, API and NAT46 UI are present. Historical closeout WIP's instruction to cherry-pick `760696e35`/`3d001ee60` is superseded by this integration; do not cherry-pick or rebuild these fixes.

Recovered evidence: closeout-nat46.md, closeout-nat46-evidence/review-final-live.txt and closeout-api-nat46-independent-review.md record named TestNat46OnHost with no skip, real3/3 ICMP, exact TCP payload, own-agent restart/recreate within1.00s, duplicate refusal and rollback removing domain/features/route in disposable VPP, shared NRestarts unchanged. This is historical scoped evidence on its recorded source/binary, not fresh current-main packet acceptance by this audit. closeout-api-wip.md separately records four real PostgreSQL/Valkey validation e2e cases using a fake agent; that is not actual API-to-VPP/browser acceptance.

Remaining implementation: arbitrary/nonembedded IPv6 servers retain forward-only /128 behavior; general bidirectional SIIT/EAM is not implemented. A ready source task can document/test the existing supported boundary and prepare the design spike without any packets; a VPP C change requires the separate owner-approved code track. Do not label this gap a lab-only NOT RUN or request a shared VPP restart to solve it.

Executable safe next task (read source/proof, then assign actual API/browser acceptance):

```bash
bash -n docs/status/tasks/F-nat46-host-evidence/host.sh
sed -n '1,45p' docs/status/tasks/closeout-nat46.md
sed -n '1,45p' docs/status/tasks/closeout-api-wip.md
rg -n 'embedded|forward|return|64|128' apps/agent/internal/descriptors/nat46/nat46.go
```

Concrete manager request: reserve an idle owned slot, build and prove artifacts against an exact current source SHA, supply a disposable mount-isolated VPP with its root-owned isolation marker, private AF_PACKET rig and PostgreSQL/Valkey/API/browser stack, and assign an acceptance worker. The existing host.sh requires the disposable marker and different mount namespace; tools/lab per-slot VPP existence alone does not establish that guard. Reuse host.sh through test/topology/hardware-smoke/isolated-vpp.py only after reviewing its contract. Historical slot17 is not an active lease; slot8 used by later proof is released. Remaining deployed en/fa browser/screenshots and real API-to-agent commit/rollback require that owned stack. All execution is outside this read-only task. No human shared-instance restart is necessary based on the recovered old hung-VPP report.

## F-global-blocking-host — real functional evidence recovered; wider scope explicit

Published main `e3517a98353dd081558465adcf2bac04eaae3aad` includes test/topology/security-host-acceptance/run.py, closeout-security-host.md and raw closeout-security-host-evidence/netns-product-final.txt. Independent scoped review: closeout-agent-fixtures-independent-reviews.md. The report records TestGlobalBlockingRealAPI PASS32.54s using real API/PostgreSQL/Valkey/product agent/disposable VPP: listed forward and reverse traffic0/3, unlisted3/3, unselectedLAN3/3, actual private nft local-input drop counter3packets252bytes, deleted-owned-ACL agent replay, rollback restoring3/3 and cleanup. Combined expiry/global run was2PASS/noSKIP68.242s. Existing broad board/deferred wording must be reconciled against this scoped proof; it is not all NOT RUN and not whole-task DONE.

Remaining acceptance: browser/screenshots; real URL scheduled refresh and failure/empty/garbage last-good retention; IPv6/global anti-lockout interactions; historical200k scale/lookup/commit measurements. Performance execution is explicitly forbidden by this owner envelope and remains a separate human hardware campaign, not a prerequisite to resume functional work. Existing source uses protectHost across host interfaces because mapping is unavailable (original report); wider selected-interface local-in semantics must be checked explicitly rather than inferred from the single fixture.

Executable safe next task:

```bash
python3 -B test/topology/security-host-acceptance/run.py --dry-run
sed -n '1,70p' docs/status/tasks/closeout-security-host.md
rg -n 'last good|refresh|protectHost|Not done' docs/status/tasks/F-global-blocking.md
```

Concrete manager request: reserve slot8 for this existing hard-coded runner or assign a separate source worker to parameterize its lease safely; provide source-identical API/agent/ctl/preflight artifacts, disposable mount-isolated VPP, private nft host namespace, PostgreSQL/Valkey and browser endpoint. Then assign feed-failure/IPv6/anti-lockout/browser acceptance separately, retaining source SHA, revision/slot ownership and cleanup proof. No new implementation of existing blocking/forwarding/replay code is warranted by the stale parked row. No host nft policy, units or shared VPP changes are authorized here.

## Fresh checks and limits

- tools/ci.sh check --base origin/main: PASS14s before initial checkpoint (contract/forbidden patterns/gitleaks/packet-trace/board212/slot guards). This is only check mode, not full quick.
- bash -n scripts00/20/40 and tools/lab: PASS. ShellCheck initial plain/-x invocations reported SC1091 from source-path lookup; correct `shellcheck -x -P SCRIPTDIR` for scripts00/20/40 passed. Preserve the diagnostic rather than call the initial command green.
- --check-config20 reports Go1.26.0 and generators1.36.12/1.6.2/govpp0.13.0; --check-config40 reports pinned containerlab0.79.0. These read-only outputs do not approve platform scope or prove installation.
- Provision plans b/c: exit3, planned, mgmt/PCI=tbd and old artifact source; no remote operation or apply.
- Security runner --dry-run: exit0; plan only, no packets.
- Read-only systemctl: active/MainPID1014/NRestarts0; both shared sockets exist. Socket presence/service state does not certify responsiveness or plugin/readback correctness. No VPP CLI command issued. Handover remains pending in docs/lab/host-ngfw-a.md.
- Existing strict TD19 fixtures: PASS44/44 in102.475s, exit0; failures/errors/skips/expectedFailures/unexpectedSuccesses all0. Actual private GPG/signature fixtures and fake/blocked package/remote commands were used; no target package installation occurred. Full output summary is in resume-host-prereqs-20261007-checks.md.
- No new diagnostic script was necessary: existing pinned config/strict-fixture/plan entry points cover the safe checks. No installs, provisioning apply, packet/performance work or product changes performed.
- Independent review and exact final-branch complete hosted quick remain manager merge prerequisites. Main's green run does not certify this new documentation branch; this worker does not merge.
