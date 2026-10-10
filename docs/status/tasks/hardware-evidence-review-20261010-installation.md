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

## Native timezone adoption and configured installation

Exact bounded identity-native.py SHA256
3fed0081c2a138cf2c28c01cd534be5bad5f5973d58911472f2368c402af3c11
on independently remote-readback2aa55c29e54f4fd92e345164bf80ad8d075f22e0:
full source, outer/remote AST2 and preserved original-link inspection reviewed.
Private inspect6736 B/0600 SHA256
93ad1a7a321ec34fdc4f9355303abc79796e57fec1b712a3d79812ff0b3913bd
records three regular sources, absent motd, five absent STATE targets, exact
relative root-owned link and same trusted1248-byte zonefile. Descriptor/no-follow
trusted parents, current inode/link recheck, atomic sole localtime canonicalization
and directory fsync precede only agent/meta configuration under unchanged101,
mask and native sysctl skip. No product guard is relaxed. Source APPROVE under
existing bounded parent release was delivered before operation.

Actual identity-native-canonicalize-configure-20261010T113324Z.json13205 B/0600
SHA256a70c56fe63312a43b8db02db45bf3766694c4517c03a8f93eb4ee9c045f15698
independently parsed, including full before/after comparisons and package rows:

```text
configure_exit=0; internal stderr139 bytes: one Created symlink message
expected package rows=11; all install ok installed; exact versions=True
dpkg --audit exit0; stdout/stderr empty
21 named service/socket states=inactive
five full L3 sections before==after=True
16 sysctl values before==after=True; guard_error=null
identity_contents_equal=True; hostname_equal=True
cached_timezone_equal=True; clock_step_seconds=-0.0000029524067031161394
```

Configured installation PASS. This resolves the actual partial installation
failure; firstboot, forwarding, interface binding and hardware acceptance remain
unperformed at this checkpoint.

Fresh installed inspection184291 B/0600 SHA256
30fb509abc3cc8b2e710385117dc8cb36182e469569670d87a1ddd25494949ce
records the adopted chain /etc/localtime -> managed STATE/localtime -> unchanged
canonical zonefile. No timedated restart was performed. Pinned systemd259.5
[get_timezone source](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/basic/time-util.c)
reads the immediate link and accepts only the absolute or relative zoneinfo prefix;
the managed intermediate link therefore needs fresh timezone-label measurement
after the later normal reboot. [Timedated source](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/timedate/timedated.c)
reads that helper and clears the label on EINVAL. Preserved dereferenced zone bytes
support libc timezone behavior; current cached-label equality is explicitly not
proof of fresh timedated label compatibility. Root/operator were informed without
blocking the authorized same-file adoption or weakening security guards.

## Firstboot-only exact source and read-only preflight

Actual installed inspection30fb509a independently parsed:13 commands exit0/empty
stderr, native nft snapshot has zero tables. Effective nft Start/Reload use solely
ngfw-base.nft and require firewall-bootstrap; actual drop-in clears ExecStop and
effective show omits the empty property. Hugepages/shmgroup remain0. Active persistent
80-vpp.conf has1024/group0 and is explicitly accounted for before future boot.

Firstboot-only source SHA256
ec2c40913f14ee28d9e19fd7b26658c9eea941e4ba1717a66719e4b83f68100e
on independently remote-readback8637789d6297e7140fa60f167500fb84bd3eab4a:
full initial source/canonical firstboot, firewall, DB and TLS bootstrap reviewed,
focused final deltas read, outer/remote AST2 PASS. Fifteen remaining service/socket
states must stay inactive; only six canonical provisioning dependencies become
active. Sixteen sysctl keys compare with only explicit nr_hugepages=1024 expected;
no global sysctl reapply. Existing1024 setting,2MiB pages,NUMA0 and sufficient RAM
checked. Controller-only canonical generic dev-default exclusion and actual PCI
row refusal tests PASS, matching startup template distinction. No VPP/agent/API/
nginx activation or binding. Native firstboot uses private credentials over stdin/
private files, DB/valkey loopback, owned-table renderer preserving management22,
durable completion, and empty dataplane/noPCI management blacklist.

Actual final preflight15789 B/0600
SHA256b89d26998d6707a5af02f326f5007e30d5c22475bd35b4f3ed3c4a941601dd83
independently parsed: read_only=true,21 actual states inactive,16 sysctl values,
PostgreSQL effective listen_addresses localhost, zero hugepages,2MiB page size,
MemAvailable31332168 KiB,NUMA0, zero native nft tables, ioerr6. Original startup
bytes/metadata retained privately for recovery; no secret or configuration contents
committed. Earlier14183-byte72bdab3e preflight remains historical.

Verdict APPROVE exact firstboot-only source applicability under existing conditional
parent release after actual .37 normal-return/fresh original22/full management and
storage confirmation PASS. No second permission loop once those conditions pass.
Actual firstboot results and later activation/seed/binding acceptance remain pending.

## Final firstboot record path and seed ordering

Before execution, operator caught downstream guard-restore contract: a nested
firstboot evidence directory would prevent removal of the emptied guard task
directory. Exact firstboot source1989f04b3a616a57e4db98a8737de91689be7fdc102720d08529f70823b5878a
on independently remote-readbackcf51b3a1cb868f4f3d67c20b0e3efe397666a368
changes one line to a sibling private evidence directory under the retained
recovery parent. Focused diff/AST2 PASS, same original-root/private directory/
collision/fsync gates. Supersededec2c was never executed. Existing conditional
firstboot phase applies after actual37 protected normal-return PASS, now observed.

Source-only seed-inputs.pye57d855d2e2b2dc56ba4f021ea9078cdb1a375d8d6f8823b5fa19e105f69f32d
full read/AST2 PASS: explicit root0600 agent.env management inputs matching the
packaged optional EnvironmentFile, root0644 API unit seed opt-in drop-in, canonical
api.env unchanged. Private fsynced sibling original-absence evidence/collision
refusal and management/L3 checks, daemon reload only, no service start/revision/
binding. Publication and actual passed firstboot proof remain prerequisites to
the parent's subsequent input phase.

Independent caller-aware source inspection resolves the tentative seed-before-bind
concern: projection.go241 calls SplitUnboundPhysical before desired.Interfaces.
desired/hostnics.go18 excludes physical KindExisting rows with successful existing
Linux-kind-empty lookup, reporting agent.nic-not-bound warnings. Alias/admin
objects are consequently absent until binding; ordinary validated agent apply can
promote the canonical seed without missing-VPP aliases. The AliasDescriptor missing
interface failure applies retained rows, not these filtered unbound NICs. No bypass,
database shortcut or product change is warranted absent actual failure. Actual
revision1, original logical names,7/17 data rows and protected management inventory
remain required before binding.


## 12:12 UTC actual firstboot and initial runtime applicability

Reviewer controller commands ran in the owned reviewer worktree:

```text
python3 <focused private JSON/hash/mode/selected-assertion reader>
firstboot-apply-20261010T120628Z.json: 303028 B,0600,
 SHA9134aaf1e3fae358d154bb3af6b69cf3602c1bc0e169fc2fcffa9471930045f8
firstboot.exit=0; fifteen remaining units inactive; six provisioning units active
network_before==network_after=True; only vm.nr_hugepages changed to1024
safe_initial_noPCI=True; owned_table_only=True; bootstrap_removed=True
guard_error=None; new_storage_errors=[]; ioerr_before==ioerr_after=True
no_VPP_API_agent_nginx_activation_or_binding=True
firstboot-fresh22-20261010T120704Z.json: 3643 B,0600,
 SHA1d27ece76d29764f594170656629330ead2c34dea1be40d9bf9f3565eb8b5e9d
exit=0; stderr empty; structured L3/protected PCI/driver/group/SSH/ioerr receipt
seed-inputs-20261010T120704Z.json: 5313 B,0600,
 SHAfdd412f0785b6dabecf8830da49eb005d59cbff49a66e4db694800c2ddd22951
network_equal=True; no_activation_revision_or_binding=True
start-guards-restore-20261010T120729Z.json: 9786 B,0600,
 SHAeb06bcb45e9c41f9f689998f7fdfe581ac8fdd38c2da0b5c571a037c17ef1e3a
original policy=absent; original mask=absent; recovery_marker_present=False
network_before==network_after=True; no_service_activation=True
initial-runtime-inspect-20261010T120858Z.json: 14010 B,0600,
 SHA0c12a7b95166ab38ae37153a098bc73c888db8095841c76203039ca112475fdf
preflight=True; no_manual_revision_driver_binding_or_startup_apply=True
python3 <outer/embedded AST reader>
initial-runtime SHA010eaacdf7842e983e1b23561c573ed6fe7e447b412dcf202f2bf69b91869cd8
AST_parse_count=2
rg -n <collector endpoints/response fields> apps/api/src/{config,state,features/dataplane,audit}
config GET returns document with x-ngfw-revision header
candidate GET returns candidate; commit/pending returns pending
state/system.agent.reachable; events.items; state/dataplane separate feature controller verified
git ls-remote origin refs/heads/codex/hardware-211-20261010
6d8c357496b83e618b8f3074b6622678f7012f68
```

Actual firstboot, seed-input preparation and original guard restoration PASS.
Credentials, configuration payloads and private backups were not printed or
committed. Full initial-runtime source and focused DNS/sixteen-sysctl/foreign-NFT
proof delta reviewed. APPROVE exact010eaacd source on published6d8c3574 for the
parent's already-released INITIAL RUNTIME phase: VPP, agent, first API, then nginx;
observe the real revision1 and original seventeen data NIC rows through native
TLS/RPC. No database bypass, manual revision, driver binding, startup apply or
reboot. Actual runtime/seed results remain pending at this checkpoint.

## .37 normal-root safeguard and narrow pre-install sources

```text
python3 <private metadata/assertion reader>
start-guards-inspect-20261010T120209Z.json: 4652 B,0600,
 SHAf4378c2af96e669c09836a5183d98529d99e9962b6ea9e35be4c0bb4cd03527a
original policy=absent; original mask=absent; target_mutated=False
start-guards-prepare-20261010T120314Z.json: 9491 B,0600,
 SHAa429375aed4ba95a2c04be11b7c562836cd16eb1c8f44bdf576fe872f8c1a6ec
policy=19 B,0755,root:root,SHA c2bcd9decf63ff2c0d9f473f38bc3607900530aad80f99139855d56678456230
mask=root-owned symlink /dev/null; marker_present=True
marker_SHA=e275e5061a30bb452c7f24537280394459e6d26e70c6b11cece7ae2db64d3e60
network_before==network_after=True; no_package_install=True; no_service_activation=True
python3 <outer/embedded AST reader>
package-input SHA4cd015b2a14e75878bd622b72d2da5b367218f233a9a8fdb40ea7cbdde1a67fd; AST_count=3
identity-native SHA38343860d344e47654c7d212f75e7d9af97733fa03dd46fa1a48a9f8c6cda528; AST_count=2
git ls-remote origin refs/heads/codex/hardware-37-20261010
2b481a9821c8ad8190d142011f304c93d84b5048
```

Actual .37 temporary safeguards PASS. APPROVE exact4cd015b2 input mirror for the
already-released eleven verified archive uploads and simulation only: narrow diff
adds the five actual-missing named tools and exclusive/fsynced private receipts.
APPROVE exact38343860 timezone-only source: full peer delta removes all package
configure/product migration assumptions, retains trusted descriptor-based parent
and same-zone checks, original-link backup before sole atomic canonicalization,
identity/L3/sixteen-sysctl/hostname/clock checks and existing service suppression.
Inspect is read-only; the parent's already-released canonicalization requires its
own matching fsynced baseline. No effective timezone-byte change or security guard
relaxation. Actual solver, timezone operation and subsequent native installation
remain pending; no .211 evidence is substituted for .37 actual results.


## Actual .37 configured installation and .211 runtime failures — 12:21 UTC

Controller-only review commands (private payloads remain unprinted):

```sh
python3 - <<'PY_CHECK'
import pathlib,json,hashlib
p=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37/package-install-20261010T121903Z.json')
b=p.read_bytes();d=json.loads(b)
print(len(b),oct(p.stat().st_mode&0o777),hashlib.sha256(b).hexdigest())
assert d['install_exit']==0 and d['simulation_exit']==0 and d['plan_exact']
assert len(d['plan_changes'])==114 and all(x.startswith('Inst ') for x in d['plan_changes'])
assert d['network_before']==d['network_after'] and d['sysctl_before']==d['sysctl_after']
assert len(d['service_states'])==21 and all(s in ['inactive','failed'] for s in d['service_states'].values())
assert d['policy_exact'] and d['persistent_mask'] and d['VPP_INSTALL_SKIP_SYSCTL']=='1'
assert d['ioerr_before']==d['ioerr_after'] and d['new_storage_errors']==[]
assert d['dpkg_audit']=={'exit':0,'stdout':'','stderr':''}
r=d['packages'];assert r['exit']==0 and r['stderr']==''
rows=[x.split() for x in r['stdout'].splitlines()]
prod={'ngfw-agent','ngfw-api','ngfw-web','ngfw-meta'}
vpp={'vpp','libvppinfra','vpp-drivers','vpp-plugin-core','vpp-plugin-dpdk','vpp-crypto-engines','python3-vpp-api'}
assert len(rows)==11 and {x[0] for x in rows}==prod|vpp
assert all(x[2]=='installed' and x[1]==('0.1.0~dev+2045ab8b3d2f' if x[0] in prod else '26.06-release+ngfw3') for x in rows)
print('independent exact11 identities/audit/114plan/21suppression/L3/16sysctl/ioerr PASS')
PY_CHECK
```

Selected actual output:

```text
270474 0o600 51861660d6b82f5bd3576d2228b2754c706766acd415f047271a31f42fc4c696
independent exact11 identities/audit/114plan/21suppression/L3/16sysctl/ioerr PASS
ioerr 0x6 -> 0x6; actual policy denial messages=11
```

Own .37 solver21634 B/0600 SHA07d88eb379ed4a9a813ac8c5cf6d0be9b31e0446ac9cbd5563afdbc49af035d8
was independently byte-identical to the previously fully reviewed .211 plan.
Exactly three actual upgrades matched `^Inst \S+ \[`; ordinary architecture brackets
are not upgrades. Actual .37 timezone inspect6489 B/0600
SHA0704ec548876f7fe10b26665d86d82def948de909807bfc4797f0512e8baecad and canonicalize11536 B/0600
SHAdabf151233bf62c6dacdd8b484f31f480f48dd5995dfb9c47a62b6840da068af
independently preserve backup digest, unchanged trusted1248-byte zone hash, all
four other identities, hostname/cached timezone, current L3 and sixteen sysctls;
all21 services suppressed, clock step approximately -0.86 microseconds.

Final .37 native installation source44e3f38b2402e8d87531a81b97421135c9cefe26a8fdef1d3ac1ffd39d5dc175
was independently read back from published3b913a0650234cdd1207d1e4b50757cbbffeade1.
Focused peer diff and outer/remote AST2 PASS; own constants/guards/proof filenames,
additional readonly exact11/audit and storage-proof checks. No early provisioning.
Actual configured installation PASS; .37 firstboot/runtime/binding remain pending.

Actual .211 initial runtime010 source safely stopped after VPP start0 and immediate
binary-API check2.294155 B/0600 receipt9b486056180f7b15a341fb01a7aa2f33cc79ef62b76a29528b8bef50a3fc591e
records active PID7359/NRestarts0, other three units inactive, all protected
invariants unchanged. No false runtime or seed PASS.

Fresh1706 B/0600 receiptfec24987ae5692b51786c371bab9762c1fdc197cfa379dfc97a9eb8231f14813
independently shows socket exists, same active7359/NRestarts0, all four actual
property/version/bootid/local0 commands exit0/empty stderr. The early probe raced
socket readiness. Narrow source77dfcc9c0bf01c02a9e6d1bce3d65aabca451d89b04557feada0ec278a1a17a1
on independently published/source-readbackf7710855f369181b711a524f8b685b4add05ee90
approved after focused diff/AST2: exact private failed-proof/PID continuation skips
VPP start, bounded same-PID socket readiness and unauthenticated protected HTTPS
readiness precede the single administrator login. Original guards retained.

Actual continuation371137 B/0600 SHA98b516a17a02cc82a8b6e26b3ac81d3e1ea8b3cf28fb73b1c0a68bd41b9376fe
has all four units active/NRestarts0, native TLS login200/admin, binary API tests0,
full L3/seventeen kernel NICs/DNS/sixteen sysctls/foreign NFT unchanged, counter6
stable and no new storage errors. **Real seed FAIL remains open:** all22 config/
event polls200 stayed revision0 with empty event items. No manual configuration,
DB seed or early binding. Actual API-only journal/environment/inventory diagnosis
is underway; the caller-aware source filtering rationale cannot substitute for
actual revision1 acceptance. No concrete cause is claimed yet.

Source-only .37 firstboot mirrorc1ea9741622ad6e8fbc78ce564432b2e52f33590afa745e53aaddabc066e6145
focused peer1989 diff/AST2 APPROVE after own147308 B/0600 baseline
SHA6cf4eaed57b65f4f2ee6e999d11c53f67f8ae1240c5386b9665bd28e0620b3ea:
13 commands0, zero native NFT objects, zero hugepages/group0,2MiB/NUMA0,
protectedPCI0c/igc/group58/root8:2/counter6. Publication/readback and parent phase
release remain explicit before firstboot execution; no peer result substitution.

Draft scoped binder uses per-device override/unbind/probe, avoids driverctl/global
new_id, guards singleton groups and unsafe mode, management and inactive VPP.
Observed .211 data netdevs/bridges all have zero addresses/routes. One remaining
source premise before its later exact verdict: plain modprobe also consumes
configuration/kernel options; [pinned Linux7 VFIO source](https://raw.githubusercontent.com/torvalds/linux/v7.0/drivers/vfio/pci/vfio_pci.c)
turns nonempty ids into pci_add_dynid at module initialization.
[Primary kmod documentation](https://raw.githubusercontent.com/kmod-project/kmod/master/man/modprobe.8.scd)
confirms those option inputs. Actual effective-option evidence or explicit
empty-ID/load-command handling requested. This is separate from initial runtime
and is not a claim of an observed target global-ID configuration.
