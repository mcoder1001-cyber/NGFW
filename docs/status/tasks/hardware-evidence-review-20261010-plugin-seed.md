# Independent resumed plugin/seed evidence review

Same existing hardware task; branch codex/hardware-evidence-review-20261010.
Owned documentation only; no product or target writes.

Actual native runtime371137 B/0600 receipt98b516a17a02cc82a8b6e26b3ac81d3e1ea8b3cf28fb73b1c0a68bd41b9376fe remains failed: all four services/TLS login work, but22 config/event polls stay revision0/empty. Diagnostic49788 B/0600 receipt41dcecbe3e9f47e96a8ab9d9ecd5c0e5ca2b5e0668cd1e112f728fa6e932f383 reports downstream agent validation failure. Files present is not loaded capability proof. No manual revision/database bypass or early binding.

Canonical renderer preserves current plugin switches when dataplane.plugins is absent; stock firstboot previously supplied {dataplane:{}}. Primary VPP26.06 registration marks each of linux_cp, linux_nl and npt66 default_disabled. Thus an explicit canonical configuration is supported; root has also implemented a shipped default bootstrap asset to repair the actual native bootstrap gap. Source/tests/final hosted gate for that product correction remain separately under review.

Primary sources: [LCP registration](https://raw.githubusercontent.com/FDio/vpp/v26.06/src/plugins/linux-cp/lcp_api.c), [netlink registration](https://raw.githubusercontent.com/FDio/vpp/v26.06/src/plugins/linux-cp/lcp_nl.c), [NPT66 registration](https://raw.githubusercontent.com/FDio/vpp/v26.06/src/plugins/npt66/npt66_api.c).

Actual independent private-receipt check ran from this reviewer worktree with Python JSON/hash parsing and exact safe selectors, never printing configuration/secret payload. Selected output:

```text
file=noPCI-plugin-preflight-20261010T123700Z.json
size=274716 mode=0600
sha256=35131df9f0853f2865b197251f067db7762ec664a3dd5cdcd3ca509e234678c6
render_exit=0 dryrun_exit=0
render_stderr_empty=false dryrun_stderr_empty=false
network_equal=true ioerr_before=0x6 ioerr_after=0x6 new_storage_errors=[]
no_pci=true management_blacklist=true physical_dev_rows=0 required_plugins_enabled=3
dryrun_no_apply_flag=true
will use: tcp:172.30.126.195:22 (passes now)
gate: product mode — appliance approval of rendering 367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184 (dead-man mandatory)
dry run: nothing changed
document size=3170 mode=0600 sha256=3c1ba66789d713ac8c2a8b8fb55b021ba8279c153d5b68a57794f04289419d4b
```

APPROVE exact readonly preview/source4a63dcc56bbd7343dd218c5773c2257920a026af427bd2d5175ca553c45b9856 and root-only noPCI apply applicability: finite sealed document/source/live/render approval hashes, unchanged protected management, mandatory product dead-man and rollback before native postapply/real seed verification. Original live608B c892394e36bfc45950407128a81c849fdaa1c66a1fbe64ce9154fa271f215b5e; rendered735B367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184 reported by operator, exported originals/render independently readback pending. No binding, actual correction/seed PASS or .37 provisioning approval is claimed.

Product checkpointb1f5bea6bb8a7964165b9ba314d09ecbd238750a has been read: fixed bootstrap asset enables these three switches without devices; firstboot installs0600 and invokes canonical generator; meta manifest includes asset; real CLI regression tests noPCI/protected management/missing-LCP refusal. Final focused tests/evidence review underway, not a merge approval.


## Independent product/management source follow-up — 12:46 UTC

Source checkpointb1f5bea6bb8a7964165b9ba314d09ecbd238750a correctness/management
applicability APPROVE. No API/schema/binapi/privilege change; fixed shipped JSON
has only the three required switches, no physical device selection. Meta manifest
ships it; install0600 and canonical startup generation precede firstboot completion
and bootstrap cleanup, so missing asset/plugin fails closed. Existing nonbootstrap
plugin-preservation/authoritative semantics remain unchanged.

This reviewer extracted exact source with `git archive --format=tar b1f5bea6bb8a7964165b9ba314d09ecbd238750a apps/agent deploy/debian/ngfw/assets deploy/debian/ngfw/tests deploy/debian/ngfw/debian/ngfw-meta.install deploy/systemd/ngfw-firstboot.service` into OWN0700 private controller scratch `/dev/shm/ngfw-r7-firstboot-b1f5bea6`. No manager worktree or product source was changed. The private fixture commands never invoke the real target/shared-host services. Actual independent commands/output:

```text
# cwd /dev/shm/ngfw-r7-firstboot-b1f5bea6/apps/agent
go test -mod=readonly -count=1 -run TestApplianceFirstbootPolicyPluginsWithoutPCI -v ./cmd/ngfw-startupgen
=== RUN   TestApplianceFirstbootPolicyPluginsWithoutPCI
--- PASS: TestApplianceFirstbootPolicyPluginsWithoutPCI (0.03s)
PASS
ok ngfw/agent/cmd/ngfw-startupgen 0.073s
# cwd /dev/shm/ngfw-r7-firstboot-b1f5bea6
python3 deploy/debian/ngfw/tests/test_firstboot.py -v
test_completed_marker_still_runs_unit_cleanup_after_crash ... ok
test_critical_environment_duplicates_refuse_before_completion ... ok
test_failures_retain_credentials_and_do_not_publish_completion ... ok
test_invalid_existing_jwt_and_random_failure_retain_credentials ... ok
test_key_file_override_refuses_even_with_valid_secret ... ok
test_retry_preserves_secrets_and_cleans_credentials_only_after_success ... ok
Ran 6 tests in 20.635s
OK
# own reviewer worktree
tools/ci.sh check --base origin/main
check PASSED (0m21s)
```

Exact exported artifacts now independently read: noPCI-current-startup.conf.before
608B/0600/c892394e36bfc45950407128a81c849fdaa1c66a1fbe64ce9154fa271f215b5e;
noPCI-required-plugins.conf735B/0600/367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184;
noPCI-required-plugins.doc.json3170B/0600/3c1ba66789d713ac8c2a8b8fb55b021ba8279c153d5b68a57794f04289419d4b.
Private rendering matches actual receipt stdout byte-for-byte. Actual unified
startup diff is ONLY the added three-plugin block. Nonempty nested stderr403B/348B
contains protected-management facts/render digest and a warning that unspecified
mainCore selects online nonisolated CPU1; original already has main-core1. No nested
empty-stderr claim. Public operator handoff27538d4f6a85f49c92782255da47f5bfc0d63118
was independently observed remotely.

APPROVE root-only transaction order: fresh fullL3/protected04igc28/seventeen kernel
data NICs/four original PID states; private fsynced exact document/original startup/
unit+network record; explicitly stop API then agent; native detached product apply
with exact367e/c892 approval hashes and mandatory dead-man; verify finished committed
transaction/live render/management; explicitly start agent then API and capture
actual normal seed. Nginx stays active. On actual refusal/rollback, restore original
unit active states only after native transaction finishes. Expected agent/API PID
changes must be recorded; old preview PID assertions are not reusable postrestart.
No reviewer/worker real apply, PCI binding or datastore bypass.

Final R7 merge closure still needs a concrete final candidate with linked exact
commands/output, firstboot/plugin packaging documentation and nontrivial decision
trace linked to D060, plus actual unchanged complete quick/integration evidence.
Current manager WIP is an unfinished checkpoint, not a false final gate PASS.
Focused correctness approval is separate from final-head R7/hosted quick and actual
post-plugin seed acceptance. Current main has moved tobd25d9b; no stale-tree gate
claim is made. .37 firstboot remains unexecuted pending .211 real seed resolution.


## Actual manager noPCI transaction and native observer — 12:52 UTC

APPROVE/PASS actual root-only guarded transaction, independent read-only private
receipt parsing. No PCI binding or native seed outcome is inferred. Exact metadata:

```text
manager-plugin-result-v2-211-20261010T125013Z.json
6730B mode0600 SHA=d63451ad9d5bb5f19fce04e2d0f30bbaefb5f81444f75017503eea0179207065
committed=true installed=true
rolled-back=false console-needed=false superseded=false deadman-fired=false
live=new=367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184
backup=c892394e36bfc45950407128a81c849fdaa1c66a1fbe64ce9154fa271f215b5e
recorded=calculated plan seal=db9bbf47907dd236a7dad4e1d731030e711df9037361a73ea426a7a013ee0339
VPP PID33868 active NRestarts0; agent/API intentionally inactive
binary bootid/plugins/version exits0/0/0 stderr empty; required3plugins loaded=true
network_equal=true all17_kernel_equal=true protected_management=true
ioerr=0x6 nr_hugepages=1024 new_storage_errors=[]
manager-plugin-services-211-20261010T125039Z.json
1103B mode0600 SHA=6e45514aff73887848d0e101dc08aa8d86d5fa9f5ca53e5a00afbeb5c50fae96
systemctl start ngfw-agent.service exit0 stderr0
systemctl start ngfw-api.service exit0 stderr0
VPP33868 agent42444 API42449 nginx9281 SSH1033 active NRestarts0
```

Earlier root readonly checker refusal is retained, not a target transaction failure:
it incorrectly treated the single concatenated-plan digest as a checksum manifest
and omitted original IOMMU members from inventory comparison. Corrected v2 uses
the native recorded/calculated digest and full inventory; no extra restart occurred.

Readonly native-seed-observe source402f0bbc9f1ee534a0809f3b719450729412f513ec1971dec34dd4a01b3f4a99
APPROVE after full source/outer+remote AST2 and exact published400165574b297ae57a8f9fadf0c4492414086117 readback. It makes no service/startup/driver/config/DB revision writes.
Private trusted TLS login sends credentials only via SSH stdin and omits bearer
tokens. Native after mode requires exact367e, actual3 loaded plugins/new VPP PID,
revision1/system.seed-defaults/seventeen original name+PCI builtIn rows and protected
management exclusion, candidate equal/noPending/RPC, stable new unit PIDs and full
network/17kernel/DNS/sysctls/foreignNFT/storage proof. Before capture29c443 already
exists and need not repeat. Type=simple active alone is not HTTPS readiness; operator
was reminded to establish bounded unauthenticated protected GET401 before the single
login, or use actual manager ready proof. Actual seed receipt remains pending.

Draft VFIO configured-option premise independently parsed08a7c393bf0602897ac3345fe2c187073109fb9f69ebc61828cb484c9d5fe583/2702868B0600:
three commands0/empty stderr, zero effective options/install vfio lines, zero loaded
VFIO modules, actual unsafe and ids sysfs absent. Draft binder remains unapproved
pending exact final source/finite original-state rollback. Runtime ids must be empty
when present before/after load, so persistent boot cannot inherit global-ID matching.
No module loading/binding by reviewer. .37 installed old2045 firstboot is not changed
by source merge alone; its fixed-package or explicit canonical noPCI correction
provenance must be reviewed before initial runtime/seed. No .37 firstboot execution.


## Actual postapply agent startup failure — 12:53 UTC

Native after-observer stopped at its unit-state gate before authentication/seed.
Actual zero-byte JSON and89-byte stderr retained by operator; this is not a seed
acceptance or a changed-datastore claim. Independently parsed protected readonly
postapply-units-diagnostic-20261010T125214Z.json30653B/0600 SHA4fd9aa4da68fcdf15d6ac8d0d3ab3b05238b47008298fc60276345b15d9f0c59:

```text
4 readonly commands exits0/0/0/0 stderr0
VPP active PID33868 NRestarts0
agent activating/auto-restart PID0 Result=exit-code ExecMainStatus1 NRestarts29
API active PID43592 NRestarts0; nginx active PID9281 NRestarts0
live startup367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184
ioerr0x6
```

Mixed120-line journal selected actual API warnings report agent.sock ENOENT;
these explain unavailable agent RPC but do not establish the cause of agent exit.
Bounded agent-only startup diagnostics requested under existing readonly scope;
root alone owns any containment. No blind service restart, rollback, datastore
change or binding by reviewer/worker. Earlier committed noPCI transaction was
actual PASS at its checkpoint; current native agent/runtime is FAIL and remains
a real failure requiring resolution before hardware acceptance. .37 firstboot held.

## Actual owner-cache cause and contained services — 13:07 UTC

Independent bounded private selectors confirm the actual agent-only journal
80423B/0600 SHA86c50356cb7c100b81d0659290de4ae180d93b0bc9cf6e0a0ec1455666ba827d
has four structured startup ERRORs with static literal `auto-block cache owner
mismatch`. Two readonly commands exited0. Fixed-cache metadata receipt672B/0600
SHA318a6e694f99b4297ca267d8bf4c654788612d751e42ccb9201f2e8ccdd7b33e
reports root UID0/GID107,0600,device8:2,size2,empty field inventory, absent owner,
zero entries. No cache contents/IPs were emitted or changed. Preserve GID107 as
well as UID/mode in any separately reviewed root-owned finite recovery.

Root containment receipt395B/0600 SHAdc95818ca54d78d3350e3aea64cbd632215add776f5ae14cc2e118ac7d430902
records explicit API stop then agent stop, exit0/empty stderr. API/agent inactive,
agent historical NRestarts151; VPP33868/nginx9281 active NRestarts0. No cache,
startup or NIC writes occurred in that containment. This receipt does not itself
provide a new complete network snapshot.

Source ee20250072938a46407c5ff541e61e1afb7db2d5 repairs the real persistence bug:
checkOwner permits omitted RPC owner, while strict load correctly requires the
service owner. The accepted cloned snapshot now receives that effective owner
before persistence. Independent whole TestAutoBlock selection PASS0.350s, including
caller unchanged/restart/foreign RPC unchanged cache/foreign persisted rejection;
exact command/output and PR225 blob comparisons are in
[the final-candidate review](hardware-evidence-review-20261010-pr225.md).
Source applicability APPROVE; live fixed artifact/known-cache recovery/native seed
remain pending, no .37 firstboot or data binding.

Conditional APPROVE readonly data-preflight source
f7d067b706e922fcef1dca427b611e7c66df790edbb19e42a2aae041c480a491 only after an
actual passed native after-observer receipt. It requires real seeded17 rows,
loaded3plugins and protected kernel management, then canonical render/product
dryrun without apply. Its modprobe invocation is dry-run only; no module or driver
mutation. Failed native seed cannot satisfy that prerequisite. Finite rollback
source93bff522 now has the preparation-only approval below, not execution readiness.

## Finite recovery source review — 13:17 UTC

APPROVE manager-only finite rollback source
93bff522c2dba173c39ac64631b2dbd74e01b986cfe5778986ade1b27cc0cbdf and its contract
published af289e547948e882a58d829f775f14dc3077cd60. Independent AST PASS and full
source read; default validation is not target execution. All17 original/current
driver scopes validate before writes; protected04/group28 excluded; API/agent/VPP
must be inactive; nonblocking canonical VPP then lab locks prevent racing native
apply. Exact735B367e original startup and exactly3 unchanged owned files only;
per-device override/unbind/probe, recorded driver/name/master/up-down, repeated
protected L3/TCP22, no global IDs/driverctl/service start. Immutable actual
manifest/offhost copy/combined manager consumer and terminal native transaction
remain pending. Any actual partial restoration or regenerated data link-local
state must be classified, never broadly cleared. No binding/rollback was executed.

APPROVE narrow known-empty-cache source
4a5d13130963c24ed54e164e8d6304d08347a63f153a67f5f251ce9de9373717 after the actual
fixed-agent attestation/hash is available and installed. Full read/AST PASS;
published operational3acf03df0a9abf9168e9f0256497064c8f0fc861 source bytes match that
digest. Exact observed2-byte empty cache/root:GID107/0600/nlink1 only, O_EXCL
original backup and fsync, atomic owner-only replacement retaining UID/GID/mode;
same boot/VPP33868/stopped API-agent/live367e/protected management and wholeL3;
no service start. Product default Owner=ngfw and unit/default state directory
/var/lib/ngfw/agent confirmed, so absent env assignments retain these defaults.
Retain original metadata/private backup-directory identity with the digest in
actual receipt. Fixed artifact/cache mutation/seed outcome are still pending;
source approval does not claim any of those actions occurred.
