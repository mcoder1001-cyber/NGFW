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
