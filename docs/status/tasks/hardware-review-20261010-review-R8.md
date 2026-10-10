# Independent hardware installation and recovery review

Reviewed source: `d2d55984d74fa1d06c32e8271886f11f16375407`. This is a review of the current deployment plan and observed target prerequisites, not a product merge or release approval.

## BLOCKER: both mounted root filesystems have confirmed structural corruption

Independently inspected both targets over SSH using `findmnt`, `journalctl`, `stat`, `lsblk`, `ip -j address`, `ip -j route show table all`, `systemctl is-active ssh.service` and PCI sysfs symlinks. No package installation, filesystem repair, interface change, service mutation or reboot was performed.

Both targets returned:

```text
/dev/sda2 ext4 rw,relatime
/dev/sda2 contains a file system with errors, check forced.
/dev/sda2: Inode 259596 has an invalid extent node (blk 15505493, lblk 0)
/dev/sda2: UNEXPECTED INCONSISTENCY; RUN fsck MANUALLY.
(i.e., without -a or -p options)
EXT4-fs error (device sda2): __ext4_find_entry:1624: inode #259599:
    comm sshd-session: checksumming directory block 0
EXT4-fs warning (device sda2): ext4_dirblock_csum_verify:375: inode #259599:
    No space for directory leaf checksum. Please run e2fsck -D.
EXT4-fs error (device sda2): ext4_find_extent:939: inode #259597:
    comm journalctl: pblk 15503361 bad header/extent: invalid magic
An error was encountered while opening journal file or directory:
    Input/output error
ssh.service: active
```

The 126.37 target also exposes invalid journal extent inode 259598 at block 15503362. Both root UUIDs are `343f6a24-1c79-4177-b7d0-52c8582914fb`, EFI UUIDs `9804-C768`, and archived journal machine-id paths match. The shared identity and identical structural failures suggest a cloned corrupt source image; this is an inference, not proof of disk hardware failure or the original cause. Host clocks reported April 18, 2026 despite the October 10 review session; their timestamps are not reliable session timestamps.

Installing databases/daemons or rebooting without a reachable console would create material data and access risk. Package writes cannot be treated as safe because `/` is presently mounted read/write and SSH succeeds. An online `e2fsck -n` would not establish a clean filesystem or clear this blocker, so it was not run.

Required recovery prerequisite: verified console/rescue access, export recoverable management network config and SSH access material to an appropriate trusted off-host location without committing secrets, boot rescue with `/dev/sda2` unmounted, obtain available disk-health evidence and a recoverable backup/image before repairs, diagnose with `e2fsck -fn /dev/sda2`, then review interactive repair with `e2fsck -fD /dev/sda2`. Do not run repair on mounted `/`, delete corrupt directories to hide the issue, or blindly answer all repair questions with `-y`. Repeat the offline check and require clean state, a normal boot, unchanged management routes, and a fresh SSH connection before deployment resumes. Repair actions remain with the manager/host operator.

## Verified management path and permitted interface scope

| Target | Management interface | Management PCI / driver | Required unchanged address / default |
| --- | --- | --- | --- |
| 172.30.126.37 | enp12s0 | 0000:0c:00.0 / igc | 172.30.126.37/24; via 172.30.126.1 dev enp12s0 |
| 172.30.110.211 | enp4s0 | 0000:04:00.0 / igc | 172.30.110.211/24; via 172.30.110.1 dev enp4s0 |

Both have `00-installer-config.yaml` and `90-ngfw-management.yaml` under `/etc/netplan`. Preserve their exact content and Linux management drivers. Current route inventory has no additional IPv4 routing interface beyond management; preserve any subsequently discovered default, control-peer, or configured production route interface as well.

126.37 has eight physical kernel NICs, seven available data NICs: enp10s0, enp11s0, enp13s0, enp14s0, enp15s0, enp16s0, enp17s0. 110.211 has eighteen physical kernel NICs, seventeen data NICs: enp5s0 through enp9s0 plus enp1s0f0np0 through enp1s0f3np3, enp2s0f0np0 through enp2s0f3np3 and enp3s0f0np0 through enp3s0f3np3. All data links presently lack carrier. Several 110.211 data ports remain enslaved to empty Linux bridges. Import physical PCI NICs; these bridge/loopback rows are not additional physical hardware. Preserve baseline bridge config off-host and deliberately detach only authorized data ports when the healthy-host deployment resumes. Without cable/peer links, NIC enumeration/startup acceptance does not prove packet forwarding.

## Deployment plan constraints after recovery

1. Use packages built from the reviewed current source; the older October 4 delivery's management binaries are stale. The manager reported reuse verification of its VPP `26.06-release+ngfw3` payload against current VERSION/patches/lock/manifest/control fields and hashes, then the full verifier passing all 72 tests during fresh package preparation. These VPP results were reported by the manager, not independently executed by this reviewer. `prepare.sh` builds fresh API/web/Go artifacts and always runs the verifier with tests enabled.
2. Package install must be staged with service-start suppression for dependency packages. `debian/rules:21` uses `dh_installsystemd --no-enable --no-start`; `ngfw-meta.postinst` nevertheless enables future product boot units. Dependencies' own maintainer scripts remain outside that guarantee. Check the final built maintainer scripts and resolver simulation; don't leave enabled incomplete firstboot units for an accidental reboot. Remove temporary suppression only after explicit bootstrap inputs and recovery are ready.
3. Package firstboot runs `ngfw-startupgen` at `assets/firstboot.sh:72` and overwrites `/etc/vpp/startup.conf`. Back up an existing file, render and inspect it before initial startup. Its empty initial dataplane is intentionally `no-pci` and host-derived management blacklisting, not physical NIC handover. No product daemon should run on the development host with pending VPP handover.
4. Configure explicit `NGFW_BOOTSTRAP_MGMT_IF`, `NGFW_MGMT_IF` and `NGFW_MGMT_PCI` with the independently verified values above. Exclude management PCI from both `dataplane.devices` and `pciWhitelist`. Exclude management interface from punt interfaces. The renderer rejects a host management PCI even when the document mislabels it (`vppstartup/model.go:598`); use the guarded `apply-startup.sh` for later changes so snapshots, deadlines, rollback and a successful preflight management probe protect handover.
5. **Routing risk requiring plan-specific verification:** `render-base-policy.py:31-38` installs an input chain with policy drop, permits loopback/established traffic, TCP 22/443 on management and selected IPv6 neighbor traffic. New inbound ICMP, DHCP and routing-protocol traffic on management are not generally admitted. Snapshot the existing nftables state, inspect required local-in protocols and any additional table drops, and establish appropriate retained admission before loading this base table. An accept in another nftables base chain does not override a subsequent drop here. Current static default-route ARP and established outbound ICMP responses are not themselves blocked by this input policy, but an existing SSH session alone is insufficient verification: require a fresh connection and management routing/probe checks.
6. API default NIC seeding is opt-in (`NGFW_SEED_DEFAULT_NICS=1`) and absent from the shipped API unit. Firstboot's canonical `api.env` rejects added initial overrides. Perform controlled import through candidate/validation/commit, or add the opt-in only after successful firstboot. Verify all seven/seventeen data NICs in PostgreSQL running config and live VPP state, preserve management exclusions and confirm guarded startup/handover before calling them operational.
7. Existing root SSH access is not modified by packaging: hardening assets are copied only, no `sshd` activation is part of the reviewed firstboot/postinst path. Don't run hardening activation incidentally. Test root key login afresh after activation and reboot.
8. Require migration/bootstrap/secret permissions, TLS UI/API, agent-to-VPP connection, candidate/commit/rollback, data-NIC live retrieval, service restart and healthy-host reboot persistence. Record link/peer constraints explicitly; don't claim packet-level acceptance for no-carrier links. No licensing/release/whole-appliance acceptance approval is implied by these lab preparations.

## Dependency compatibility observed read-only

Both targets identify Ubuntu 26.04 LTS (resolute). Cached `apt-cache policy` independently returned:

```text
nodejs: Installed (none); Candidate 22.22.1+dfsg+~cs22.19.15-1ubuntu1
postgresql: Installed (none); Candidate 18+290ubuntu1
valkey-server: Installed (none); Candidate 9.0.4-0ubuntu0.1
frr: Installed (none); Candidate 10.5.1-1ubuntu4.1
nftables: Installed (none); Candidate 1.1.6-1
```

Node satisfies `ngfw-api`'s `>=22,<23` dependency. PostgreSQL 18 satisfies `ngfw-meta`'s package `>=16` constraint and `docs/09-os-packages.md:25` explicitly recognizes resolute's PostgreSQL 18. Actual bootstrap/migration compatibility must still be established on the selected version, not reported as a PostgreSQL 16 result. Cache evidence is not a fresh package download/install result. `dpkg --audit` emitted no text; this does not override the filesystem errors.

## Tests independently executed in the reviewer worktree

```text
python3 deploy/debian/ngfw/tests/test_packaging.py
Ran 16 tests in 1.391s
OK

python3 deploy/debian/ngfw/tests/test_firstboot.py
Ran 6 tests in 21.400s
OK

python3 deploy/debian/ngfw/tests/test_base_policy.py
Ran 3 tests in 0.002s
OK

go test ./internal/renderers/vppstartup -run 'TestHostNICs|TestHostFacts|TestManagement' -count=1
ok ngfw/agent/internal/renderers/vppstartup 0.138s
```

The selected Go tests use isolated filesystem fixtures and generated config; they are not real hardware forwarding tests. The Go test subprocess printed the successful result above, but the parent Go command remained alive in post-test processing with no child test process: its final exit is not verified and the result is explicitly output-only. The three Python commands exited zero. Full hosted quick and target installation tests have not been independently executed by this reviewer; no merge is proposed.

`tools/ci.sh check --base origin/main` independently exited zero with `check PASSED (0m21s)`: no new forbidden patterns, secrets, contract changes or board/slot validation failures.

**Verdict: BLOCK hardware installation/reboot until both root filesystems are repaired offline and clean boot/network prerequisites are verified. APPROVE the decision to keep targets unchanged and prepare current-source artifacts on the separate build host. The post-recovery deployment plan requires the explicit safety checks above and a final applicable review of artifacts/configuration/results.**
