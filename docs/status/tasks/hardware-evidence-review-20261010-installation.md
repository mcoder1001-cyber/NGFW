# Independent installation evidence review

Owned reviewer branch codex/hardware-evidence-review-20261010. Product source approval
and actual filesystem recovery are separate from installation/activation acceptance.
Reviewer has performed no target operation. Private safeguard snapshots and marker
contents are not committed here.

## .211 guard source and actual preparation

Operator source checkpoint6bdf80b843d827cf52e12c9bc94ef8a0369d4003:
start-guards.py SHA2562530bc534fabd6d83712388fc7aad0eb4cf1a71f9a5a02215d2c8c08aeb81fe6
and package-input.py SHA256e5c7a1c927b03d0298ae122acc7f765edbd5f23b12cc2d8cea03500a91e6b346.
Full actual source read, actual bytes hashed, outer/embedded Python AST2+3 PASS.

Inspect4905 B/0600 SHA256c45e00dfcdbf10618b06634808eba3445dba088817d413988cb4c6a60de01d82
independently parsed: original policy and VPP mask both absent, target_mutated=false.
Prepare source persists a private root-owned0700/0600 original-root8:2 marker with
original states, baseline digest and exact owned guards before the first safeguard
rename; marker file and directories fsynced, collisions refused. Exact policy101,
persistent /dev/null VPP mask, daemon-reload and controller wallclock are the changes.
Restore accepts only each original or exact guard state, handles partial preparation,
restores exact originals, checks L3, then removes only owned marker. No RTC/NTP/network
configuration change, package install, service activation or NIC binding in source.

Actual prepare10002 B/0600
SHA25649b105010c0329471c62a56cd05cfb4bde9ff330dd909664712e7cac468a316a
independently parsed: network_before==network_afterTrue, root-owned policy19 B/0755
SHA256c2bcd9decf63ff2c0d9f473f38bc3607900530aad80f99139855d56678456230,
root-owned /dev/null symlink and marker present. Fresh readback474 B/0600
SHA25680f0d37b2c93d3a72da65e496887ba8ccf0672c39c00418590dbf91a80c45fbf
exit0/empty stderr: marker443 B/0600/dev8:2 under0700,
SHA256146f46835119cb7341fce3974a40378bf5feab8159e419f962ded1a28d4ec792;
exact101, VPP masked/inactive, counter6. No marker contents are disclosed.

Verdict APPROVE source applicability and actual guarded preparation/simulation
only under parent phase release. Installation/activation requires actual plan/source
review and subsequent management evidence.

## Actual dependency simulation

Full solver21634 B/0600
SHA25607d88eb379ed4a9a813ac8c5cf6d0be9b31e0446ac9cbd5563afdbc49af035d8
independently read: exit0/empty stderr, package_install=false. Exact11 local archives
plus named actual-missing jq,pciutils,driverctl,curl,nftables are simulated with
apt-get -s --no-remove --no-install-recommends and disabled package-cache outputs.
All114 Inst lines examined, zero Remv lines,3 upgrades/111 new/33 not upgraded.
Only upgrades are perl-base and OpenSSL legacy provider/libssl3t64 same-ABI3.5.5
security updates3.5.5-1ubuntu3 to3.5.5-1ubuntu3.2. No systemd/sshd/netplan/iproute/kernel/
bootloader replacement. Critical regex is not an exhaustive plan review.

New service dependencies include chrony, keepalived, FRR, snmpd, nginx, PostgreSQL,
valkey, Kea4/6, unbound and rsyslog plus NGFW/VPP. Exact101 suppresses standard
invoke-rc.d/deb-systemd-invoke starts; persistent VPP mask independently suppresses
VPP. Routing/DHCP/DNS configuration and service intent need assessment before
activation or removal of safeguards. Exact planned OpenSSL update motivated an
automatic-service-restart hook check to protect SSH.

Actual needrestart-readonly-preflight.json486 B/0600
SHA256f92b1e0bd1d279b94b3ff5d8b6493d8e08ff3361859a2a619d8bac406c99dd8a
independently parsed: SSH0/empty stderr; dpkg-query1 confirms absent package, five
known hook/executable/config paths absent, hook_inventory[]. Needrestart is absent
from all114 planned Inst lines. Therefore no unsupported needrestart environment
variable or hypothetical restart blocker is imposed. Operator intends to refuse
new unknown apt-conf references in the installation source and record actual start
suppression, fresh authenticated22 and complete L3 proof after installation.

At this checkpoint installation/activation/binding has not been executed or
accepted. Preserve actual failure receipts and safeguards if any subsequent phase
fails; do not label simulation as hardware acceptance.


## Exact installation source applicability

Final operator checkpoint936ec02c52982b21ad0a14bfe9de8c8be6ee5a33 independently
remote-read back via git ls-remote. Exact install-reviewed.py actual bytes
SHA256d47b7759eead7459ade5bdc889f7bec4f4ea839a5613d28f66814f5422de2c9d,
outer/REMOTE AST2 PASS; full source and focused delta inspected. Immediately
re-simulates and requires byte-exact ordered114 Inst lines/zero removals matching
immutable solver07d88eb3, verifies11 archive hashes, original cleanroot8:2,
protected PCI/driver/IOMMU28 and complete L3, trusted time, exact101policy, masked
inactive VPP and durable marker146f4683. Refuses actual new needrestart hooks/tool.

APT installation command uses --no-remove/--no-install-recommends, --force-confold,
DEBIAN_FRONTEND=noninteractive and named archives/dependencies only. No timeout
kills dpkg. Private target RAM logs and controller fsynced receipts retain outputs;
partial failure must retain marker/guards and must not be blindly rerun over
existing logs. No firstboot/service activation/NIC binding/hugepage/reboot action
is requested by this script.

Review found a concrete completeness omission in the initial15-unit postinstall
assertion: planned routing daemon keepalived, enabled firewall-bootstrap and
privileged namespace-broker socket were not included. Corrected before execution.
Final21-unit list explicitly asserts those plus snmpd, rsyslog, PostgreSQL cluster
and existing product/runtime services/socket suppression. Exact11 control archives
were independently streamed and inspected: product postinst enables/updates helper
state without direct start or NIC/network commands; VPP uses standard policy-aware
deb-systemd-invoke/invoke-rc.d start. Meta enables firstboot/firewall-bootstrap for
future boot, reinforcing configured seed/service assessment before reboot. No
archive maintainer script was executed by reviewer.

Verdict APPROVE exact source applicability under parent's installation phase
release, retaining safeguards. Actual installation exit/package integrity,21-state
suppression, fresh authenticated22, protected PCI and full L3 evidence remain
required for actual installation acceptance. Forwarding/service activation and
interface-binding acceptance are still later work, not inferred from simulation.


## Pre-install correction: native VPP sysctl skip

Before any installation, operator found the actual VPP postinst also executes
`sysctl --system` before standard policy-aware service starts. The reviewer's
earlier selected activation-line inspection omitted sysctl and was incomplete;
its previous source applicability approval is superseded by this corrected exact
transaction source. No installation or system-setting change occurred from the
previous source.

Independent FULL packaged postinst read SHA256
6271cd49ebcf4ebffcebeffd5beb10c70ca13c670f67da690fe16b20a38d458c:
`if [ -z "${VPP_INSTALL_SKIP_SYSCTL}" ]; then sysctl --system; fi`.
Shipped80-vpp.conf active settings include vm.nr_hugepages=1024 and
vm.hugetlb_shm_group=0. Broad system reapply was outside the released installation
scope. Native supported VPP_INSTALL_SKIP_SYSCTL=1 in transaction environment
skips it; no PATH interception or permanent binary/package mutation needed.

Final install-reviewed.py actual SHA256
0f02b584d1af6d4a07eae9656fadc7555d17663942337661d2eeb5f039256223,
AST2 PASS, focused delta independently read: native flag plus16 explicit /proc/sys
values before/after and equality required. Keys cover hugepages/shmgroup/shmmax,
IPv4/IPv6 forwarding, all/default/management rp_filter, management forwarding/
redirects, IPv6 disable state and RA. Same immutable114 Inst/zero removals plan,
11 archive hashes, guard/marker/L3 checks,21 suppressed units and no dpkg timeout.
Operator checkpoint065cdcd7ca071e011138c6ca394a2b9c9dd585b9 independently
remote-read back before final approval.

Reviewer independently controller-tests exact packaged postinst prefix using
PATH restricted solely to a private harmless shell stub. No real sysctl can be
resolved. Flag unset invokes stub once with --system; flag1 invokes it zero times;
both shell exits0/empty stderr. Prefix SHA256
6e40c4d52a5b0d9da3e4ce3ed51052c0b4a0776feccc52267a20b0ed14458098
includes all text before the autogenerated helper section; operator separately
records a narrower equivalent header proof398 B/0600
SHA2560e11c035f9578b8bc7343c731ab0c4053d334bfb1fc550b45eb3407b94301539
with matching full postinst digest and zero real sysctl execution.

Verdict APPROVE corrected exact source under existing conditional parent release;
no additional manager/owner approval needed. Actual install/16sysctl equality/
fullL3/21-state/fresh22/PCI/package integrity evidence pending. Packaged80-vpp.conf
remains persistent and needs deliberate assessment before a later reboot; native
transaction skip does not change future boot behavior. No hardware acceptance
claim or service/NIC activation release is inferred.


## Actual partial installation and unchanged protection

Actual package-install-20261010T112145Z.json74036 B/0600
SHA25697db5d7e553881ba6b4847137061bc70196536c4ecfef57a3ec4329a69a27367
independently parsed: APT100, agent postinst refuses unmanaged identity symlink;
meta remains unconfigured as a dependency consequence. All21 named services/
sockets inactive, independent fullL3 before==after and all16 sysctl before==after
True, exact101 and persistent masked VPP intact. Native sysctl skip prevented
hugepage/system reapply. Installation acceptance remains withheld until actual
package configuration/audit succeeds; this is a real configuration failure, not
deferred hardware acceptance.

Actual private package-install-failure-readonly.json174463 B/0600
SHA2561289cefbe6ed36f5ef1ca9e9de0f2cbc867185946f8d961ee81c8b7b70469d82
SSH0/empty stderr, all6 diagnosis commands0, protected04/igc/group28 and unchanged
boot, counter6. Sole unmanaged source is root-owned /etc/localtime relative link
../usr/share/zoneinfo/Asia/Tehran, resolving same trusted root regular0644 zonefile
1248 B SHA2562dbd87f410815edcfcd7d14be84de0040ef0d913a22203e0c7e7f4f17a6a915a.
Hostname/issue/issue.net regular; motd ABSENT, not fourth regular. All5 identity
STATE objects absent. Full helper source confirms whole preflight precedes any
public /etc identity replacement, valid_zone allows only absolute canonical
zone paths, arbitrary unmanaged symlinks remain refused.

Parent authorizes bounded configuration adoption: preserve/fsync exact original
link metadata and target hash, verify trusted ancestors/current identity, atomic
ONLY localtime link normalization to same absolute zonefile and directory fsync;
dereferenced timezone bytes, current hostname/time/fullL3 must remain preserved.
Then only dpkg --configure ngfw-agent ngfw-meta under101/mask/native sysctl skip.
No product guard relaxation or arbitrary target replacement. Exact adoption
source publication/review and actual result pending at this checkpoint.

## Source-derived next phase ordering

Actual packaged firstboot.service Requires PostgreSQL/valkey/nftables. Nftables
drop-in requires firewall-bootstrap and replaces ExecStart/Reload with packaged
ngfw-base.nft, clears ExecStop; effective installed unit inspection matters before
start. Static renderer owns only inet ngfw_base, allows established traffic and
management22/443 plus IPv6 neighbor discovery; no foreign-table flush.

Firstboot renders empty dataplane startup, durable provisioning and nginx validation
before completion marker/credential deletion. Initial api.env allows only three
canonical database/secret/JWT keys, so seed opt-in belongs in reviewed unit environment
or a later supported override. Current packaged API unit does not include
NGFW_SEED_DEFAULT_NICS; source SeedService is fail-closed until explicitly enabled.
Do not infer automatic seed from a running API. Actual canonical revision1 and
protected management inventory must precede data-driver binding. HostNics reader
can enumerate without a connected VPP and never binds/restarts/changes startup.
These are source facts, not firstboot or NIC acceptance results.
