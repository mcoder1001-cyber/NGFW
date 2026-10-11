# PPPoE accepted; RA canonical acceptance remains open

Base main d44c68edccd16e8adc9ac2545c0b61f5154ed36e; 210/212 merged.

PPPoE primary-address repair: after the existing actual kernel forwarding, lease,
namespace, MTU, VLAN, cross-connect, invocation and NCP proofs, retain the learned
mirror before writes and validate the live owner/index plus exact native TAP ID,
namespace, host name, MTU and absent bridge before each operation. Only the
negotiated IPv4 /32 and deterministic transit prefix are admissible; foreign,
duplicate and wrong-length prefixes refuse before writes. The native NAT pool
must see negotiated IPv4 first. A correct pair is strictly no-write on repeated
polls. Interrupted exact subsets recover by additions or the same bounded
reordering; no delete-all, foreign writes, detached context or deadline increase.
Read back exact order and repeat process/NCP proofs before publishing readiness.
IPv6 and legacy PPP do not use this IPv4 promotion. Failures remain tracked for
mirror withdrawal and cannot publish forwarding readiness.

Focused race tests cover old order, no-write repetition, all interrupted subsets,
foreign-prefix/native-identity/tag refusal and ambiguous replies/cancellation at
every mutation. First test execution exposed an uninitialized mirror map in the
direct preparation fixture; corrected initialization and fixed explicit fixture
mirror identity. Second focused run PASS, 1.631s. Full gate and actual dual-host
NAT/lifecycle acceptance are still required. The original .195 11-second recovery
failure remains unresolved.

RA supplemental immutable supplier10 was rerun using a TCG CPU without AVX,
AVX2 or SHA-NI, while production units and all original deadlines stayed unchanged.
This diagnostic FAILED readiness (EXIT2); publisher initialization stage6/stage19
reached the existing caller deadline. This CPU change slowed validation and is
not a product remedy or acceptance success. Private raw output is root0600 under
/tmp/ngfw-completion-evidence-20261011; public kernel certificate identifiers are
not republished. Canonical RA activation/recovery/API/browser acceptance remains
open; no task is marked Done from a supplier-only result.

Complete current PPP-related package race tests PASS: subsystems39.217s,
descriptors/pppoe1.431s, renderers/pppoe62.451s. Contract check PASS47s.
Draft PR228 opened; full unchanged hosted gate38125408678 in progress.
Fresh static CGO0 product agent c55 SHA907dd8d8b65ba8b556e26ada6d909b5f23c33ac093582b40cae62c6f66072a46.
Actual .195 full API attempt1 failed before product launch: fixed .5s fixture wait
observed ip-netns-exec before it became the genuine peer executable. All strict
process/name checks stayed enabled and cleanup/shared VPP1014/NRestarts0 and host
identity checks passed. Fixture now waits at most two seconds for that genuine
transition, matching its existing bounded restart guard; no fake identity.

Primary upstream ppp v2.5.2 plugin.c and discovery.c show PADI/PADR timeout doubling
(default initial5s), allowing the retained15.096s restart gap. Carrier-only peer
configuration now proposes initial2s/two attempts per discovery phase, so a cycle
is bounded and persist returns to the configured holdoff. Legacy peers, holdoff,
maxFail, authentication and the original11s acceptance remain unchanged. Actual
new-source peer-restart/NAT acceptance and fresh paired binary remain required.
Sources: https://github.com/ppp-project/ppp/blob/v2.5.2/pppd/plugins/pppoe/plugin.c
and https://github.com/ppp-project/ppp/blob/v2.5.2/pppd/plugins/pppoe/discovery.c .

Capacity remedy: no running Go tool used the default root build cache. Standard
GOCACHE=/root/.cache/go-build go clean -cache is releasing its22GiB generated
compilation artifacts; source/configuration/evidence and module cache retained.

Current source18f mandatory hosted gate38126271268 SUCCESS (20m34s); previous c55 run canceled by source18f push, not failed. Real .250 source18f full default-NAT API/lifecycle attempt2 EXIT0: IPv4/IPv6 three-packet forwarding, native MSS1452/1432 RX/TX, paired NAT100.64.20.10, reconnect, peer-loss withdrawal, restart10.08s under original11s, wrong-password error, correct-password recovery, both-credential raw-log/config/audit absence, validated committed cleanup and all temporary asset/namespace/host identity cleanup PASS. Static agent SHA0a9823ac217fb42518d9815bd05272dcd457c438af8638d92e91bf37d2b7b467. Getter-only test ELF predates source18f; its readonly getter source is unchanged. Actual product agent is the new paired source18f ELF.

.195 source18f attempt8 passed initial both-family packets, native MSS/NAT and real peer restart10.417s, then FAILED recovered IPv4 packets. Live readback showed no negotiated VPP prefix and an empty NAT pool. Existing configure unconditionally withdraws its firewall and rebuilds policy rules on every poll; long configure/verification cycles produced repeated convergence failures. New helper fast path accepts only exact desired boolean policy and a fresh complete existing verify proof; no flag-only readiness. Already-converged polls perform no kernel writes. Validation drift uses unchanged guarded withdrawal/repair; other read failures withdraw and propagate. All36 helper controls PASS, including no-write repeat, foreign-link refusal and failure withdrawal. New dual-host full acceptance remains required.

Capacity incident: local shared-memory filesystems were nearly full, with48GiB shmem and only4GiB available. Kernel OOM killed shared VPP1014; systemd restarted it as556416/NRestarts1. No host VPP stop/restart/config command was issued. Own root-cache cleanup was terminated; verified inactive generated manager build cache15GiB and owned tmp build cache4GiB were cleaned using standard go clean only. No source, configuration, package outputs or evidence were removed. Delayed exact7-asset cleanup PASS; host identity unchanged. Root reserved blocks restored to original2099368; memory available24GiB and I/O recovered. The original unchanged-PID claim does not apply across this incident.

RA source18f supplier11 initialization completed but actual readiness EXIT2. Diagnostic supplier12 limited failure to canonical source observer preflight; supplier13 normal CPU failed bounded post-publisher VPP manager query, and SHA-NI-disabled8-vCPU diagnostic completed initialization but observer source query still refused. Diagnostic ELFs are not acceptance artifacts. Native isolated systemd component guest uses exact source18f product ELF/eleven original assets and only private AF_PACKET fixture configuration, no host module loading or shared VPP access. Actual canonical driver runs only after a positive supplier/RPC capability proof. RA activation/API/browser and original restart/rollback acceptance remain open.

### Current source c28 acceptance and subsequent regressions

The full hosted mandatory gate passed at c28c9c4a5 (run 38134546504).
Both full real-API PPPoE drivers passed: native IPv4/IPv6 packets,
bidirectional MSS, automatic negotiated-address NAT, peer-loss withdrawal,
wrong-password rejection, password recovery, raw-log / GET / audit credential
absence and committed teardown. The original 11-second restart bound was met
in 9.890 seconds on .195 and 9.164 seconds on .250. The agent was source18f
SHA256 0a9823ac217fb42518d9815bd05272dcd457c438af8638d92e91bf37d2b7b467;
18f to c28 has no Go source differences. Both used the c28 helper SHA256
dc8e4a2df34537f89bf85f33d60636e5fb1ac7797e9d6c6f606dffcf5a9784a7.
These passes do not establish acceptance for later Go changes.

Both fresh extended MultiWAN tests passed 1000 weighted plain and automatic
NAT flows and preserved sessions after agent restart, but failed the original
8-second dead-link cleanup assertion. Exact package identity provisioning
inside private mount/UTS namespaces was required for the finite fixture;
shared hosts were not provisioned or renamed. Cleanup exited after every
512-row page. Continuing within the original 32-dump / 512-delete / time
budgets removes the dead sessions in a mixed 1000-session scan, preserving
healthy sessions. The new regression is red against the old implementation
(128 deleted, incomplete) and green after the change. Fresh packets remain
required for the changed source.

The isolated original-unit RA diagnostic established all five helper
capability sets are zero with NoNewPrivileges=1. The actual original Accept=no
socket was active/running while its supplier served the connection, rejected
by the old unconditional listening predicate. The revised predicate requires
listening before activation and running only for a complete supplier identity;
all subsequent exact unit, cgroup, executable, boot and capability proofs
remain. No units, authority masks or timing budgets changed. Diagnostic
binaries are not acceptance. Fresh canonical activation remains required;
RA stays open.

### Subsequent actual source93 observations

The genuine source93 agent SHA256
c6a13be4779dc40551dbd2ac4fad45860bfb96ec657fe8491ecfb2f7339834db
passed the baseline real-API PPPoE driver on both hosts (the extended flag
was accidentally omitted, so these runs do not prove NAT/MSS/peer restart/
wrong-password acceptance) and the full extended
1000-flow / automatic-NAT / restart / selective dead-link / PBR MultiWAN
acceptance on .195. MultiWAN on .250 still exposed an observation/save race:
resync durably saves the same WAN settings but replaces their snapshot pointer,
invalidating an in-flight cleanup. The strict transaction-locked pointer check
is retained. Durable save now preserves that pointer only when the complete
WAN snapshot (interfaces, VRFs, NAT, WAN groups and PBR) is protobuf-equal.
A meaningful regression verifies that identical saves retain cleanup authority
and changed WAN settings revoke it. Packet acceptance of this later source is
still pending.

The genuine original-unit native RA cohort became operational and entered the
unmodified canonical test. Its baseline Retrieve exposed missing fixture base
policy, then an unloaded LCP plugin. The exact package nft renderer is loaded
inside the independent network namespace and genuine linux_cp/linux_nl plugins
are explicitly enabled. These are fixture prerequisites; baseline rejection
was retained, with no partial desired-state fallback. Canonical activation,
restart, rollback, API/browser and final-source CI remain pending.

### Final source61 PPPoE and MultiWAN acceptance

Product Go source 61c82b65b2295991f1602c88eaaac0e71b7e8197, static
agent SHA256 83e7b49dd071ddc34bb45ffc21467eecacb2c9756825bcb0fcf6453e9dbcbb62;
carrier helper SHA256 dc8e4a2df34537f89bf85f33d60636e5fb1ac7797e9d6c6f606dffcf5a9784a7.
Mandatory hosted CI 38137613429 PASS (21m13s); packaging 38137613401 PASS.
The entire local unchanged quick gate passed at c28 (33m51s); that earlier
local gate is not asserted to cover later Go changes.

Both final PPPoE drivers explicitly used NGFW_WAN_API_EXTENDED=1. On .195
and .250 all original extended assertions PASS: actual three-packet IPv4/IPv6
forwarding, default automatic NAT with the negotiated address, native MSS
1452/1432 in RX/TX, explicit reconnect, loss withdrawal, peer restart and
recovered packets, real wrong-password error, restored credential and cleared
errors, both credentials absent from raw logs/config/audit, committed cleanup.
Restart times were 10.275s and 10.346s against the unchanged 11s criterion.
The first .195 extended browser run passed genuine endpoint login in English
and Persian and captured the real active PPP drawer, but FAILED later IPv6
recovery after peer restart. Its failure is retained; a subsequent full extended
run passed. The screenshot has a generic engine-observation warning behind
the drawer. No browser run is represented as full lifecycle acceptance.
Earlier source93 and initial source61 baseline runs omitted the extended flag;
the misleading full-acceptance interpretation above is corrected here.

Both final original extended MultiWAN tests PASS (.195 100.42s, .250 79.50s):
1000 plain flows, 1000 automatic-NAT flows, stable weighted distribution, real
VPP session persistence after agent restart, complete selective dead-link
cleanup within the original budget with healthy sessions preserved, restored
health, PBR precedence over a more-specific ordinary route and 1000 stable
PBR flows. The private package-identity wrapper changes no shared identity.
Cleanup and post-incident shared VPP identity checks PASS; the earlier OOM
incident remains disclosed above. Raw evidence stays private/root0600.

RA is still open. Original-unit native source61 readiness, baseline Retrieve
and foundation Apply pass after genuine package fixture prerequisites are
present. Original canonical activation fails at namespace handoff; rollback
also fails, leaving the isolated agent degraded. These failed cohorts are
not canonical acceptance, and no EAP connection, restart, rollback or browser
completion is claimed. No production units, capability masks or deadlines
were changed to admit these cohorts.

### RA consecutive-capture repair

Diagnostic39 observed exact TARGET_SOCKET_DIAG running false false at the
pre-connect check, followed by refusal in handoff.call. The preceding capture's
second supplier was completing post-ACK checks. The preliminary fixed socket
observation now admits active/listening or active/running; it grants no authority.
The received response still requires a complete supplier identity with running
socket and all unchanged executable, cgroup, boot, five zero capability sets
and two fresh activation proofs. Original deadlines and units remain unchanged.
Focused regression PASS1.182s and FAIL against the previous predicate. Full
RA package race suite PASS3.749s. Initial local suite attempts failed the two
protected-file fixtures because Go uses GOTMPDIR as the test's TMPDIR; rerunning
with both under the protected owned /run tree passes without weakening the guard.
Fresh canonical activation is still required.

Fresh formal helper builds use the existing Debian prepare.sh flags (-s -w);
previous diagnostic helper builds included debug sections. The installed agent
uses the unchanged packaging CGO0/trimpath build. No production build recipe
or binary authority rule is changed.

### Merge and remaining engine activation

PR228 exact deb4ecb2049df5c7720b91e83ccda32b86a89c48 merged as
14c940f585cc61739cbb2a624d561f960ffee6e1. Final branch mandatory38140671164
PASS21m16s, packaging38140671199 PASS25s. Main incorporated an independent
setup-only contract change; its agent/runtime/deployment code is identical to
the tested source. Main38142531175 PASS; packaging38142531220 PASS.
PPPoE host task is accepted; RA remains running.

Source8 original canonical driver remains FAIL. Isolated diagnostic46 with
eight CPUs and the genuine existing /dev/vhost-net bound into the native guest
created namespace, TAPs, interfaces, routes and ACL transport, then failed
remote-access.engine activation and rollback. Diagnostic ELFs do not prove
formal acceptance. Missing vhost device explained the preceding TAP syscall
failure; no host modules were loaded and no production units/capabilities or
deadlines changed. Other retained cohorts failed readiness or namespace
capture at the existing D-Bus timeout. Further engine diagnostics are required.

The private-unit observer has the same original Accept=no socket lifecycle
as the target supplier. Source inspection found its manager validation still
required listening even after a complete serving supplier response. It now
uses the same fixed-role lifecycle predicate; exact unit fragment/path, all
remaining image/identity/capability checks and timing budgets are retained.
Cross-role/altered-property and consecutive-capture controls pass; full RA
package race suite PASS3.431s. This repairs an additional deterministic
observation obstruction; diagnostic46's earlier engine failure is still being
traced and is not asserted to be caused by this predicate.

Source9 exact e8a72fd77 PR229 mandatory38143207221 PASS17m54s.
Read-only actual host PID1 probe: five connections with40 fixed Version replies
each within original2s/16384-byte bounds, plus one ordinary request PASS.
Native diagnostic49 still failed namespace capture; fixed outgoing property
request metadata is valid. This host probe does not certify guest activation.

### Authentication boundary synchronization

PR229 exact32f2899a37136bff723fc50b120a8e58f1fc1fab mandatory38145016303
PASS, merged asd14f1901bc9f57823366ee0708462b4a5678d941. PPPoE board closure
is now on main; RA remains running. Merged-main38146549365 was cancelled when superseded by PR230.

Diagnostic55 actual private PID1 syscall trace shows BEGIN and the first fixed
VPP Id request received together; corresponding replies were attempted after
2.003s at client disconnect and failed EPIPE. Independent literal read-only
requests also timed out during the same period. A private Go stack captured
the SDK reader waiting for the reply; five ordinary VPP role probes otherwise
completed in9–15ms. These are diagnostic observations, not canonical acceptance.
Native50 mistakenly copied the added agent diagnostic into /usr/bin instead
of the original /usr/sbin ExecStart, so its added diagnostic was not executed.
Earlier copied journals included prior boots; subsequent collections use -b.
Retained private logs preserve these limits. Owned inactive cohorts40/41/44/46/
47/49/50 were removed only after process/root-inode/mount checks and log hashes.

The transport now waits until the peer consumes BEGIN before allowing binary
requests, using the owned Unix socket's TIOCOUTQ and the original caller-clipped
2s context. It sends the same fixed authentication/property bytes and performs
no Hello, descriptor negotiation, extra requests or deadline/capability changes.
Socket queue consumption grants no identity or unit authority. Protected socket,
PID1 credential, reply framing, serial, role, byte and ancillary guards remain.
The owned stream primitive proves partial consumption retains a queued packet
and complete consumption releases it. New regressions FAIL against the prior
transport; full current RA race suite PASS3.545s after supplying the existing
bounded context in the owned pipeline fixture. The first local run failed those
fixture constructors; its failure is retained. Fresh original canonical
activation, EAP/TLS, restart, rollback, API and browser proof remains required.


### Genuine canonical peer fixture corrections

PR230 exact f0cf81f3b mandatory38146843479 PASS, merged as
c0ba3a1dd2a1c04009a95a7fb6495363e196147c. Main38147967231 PASS.
Native56–64 now activate the original engine and obtain fresh VICI readback.
These are progress observations; no full canonical campaign has passed.

Native57 found a strict credential refusal: the disposable EAP peer passed the
combined CA/CRL bundle retained for later TLS cases into an EAP-only snapshot.
The test now selects only the server certificate/key for that snapshot, preserving
the full separate TLS bundle and the production trust guard. A real certificate
control checks both EAP rejection/selection and subsequent TLS validity.

Native58/59 identified independent launcher prerequisites: its original protected
artifact/workspace paths must remain under private /dev/shm. The corrected fixture
uses its own mode0700 256MiB shm mount, without changing shared host mode1777 or
the launcher guard. Native60–63 start the genuine peer but fail initiate. Native62
was stopped after a failed diagnostic compilation; no acceptance is attributed.

Native63 private VICI control-log shows IKE proposal selection and successful server
certificate authentication, followed by failed MSCHAPv2 NT-response generation.
The independent guest omitted the genuine Ubuntu OpenSSL legacy provider from its
runtime library closure. The actual libssl3t64 package already depends on this
provider; no production dependency or ABI change is needed. Native64 with that real
library succeeds at initiate and observed VIP/negotiated selectors, then exposes
another test-fixture error: the DNS reader rejects strongSwan's normal inline comment.
It now strips comments while preserving the exact expected address/count checks.

The final RA package race suite PASS3.512s using a protected /run temporary parent.
Two earlier local runs used /tmp, whose writable ancestor is rejected by the existing
protected-file tests; those failures are retained. Native64 and earlier campaigns
also fail the original30s cleanup deadline. Fresh corrected complete traffic,
restart under30s, TLS, rollback and real API/browser acceptance remain required.
Owned inactive59/61/62/63/64 roots were removed only after process/root-inode/mount
checks, with protected observations/evidence and hashes retained.


### Restart recovery controls (source13–18; task remains running)

PR231 exact27d185103a6b1239c7b8056a6ffe58e33a7ec1ab mandatory38151031338
PASS, merged e2958df326e207c61c8d28148bb1e2ebab62faa1. Main38152724713 PASS. These fixture corrections do not close RA.

Native65 obtains genuine EAP, VIP, DNS, selectors and encrypted traffic through
selected VPP VRF/ACL. Restart reveals systemd removes RuntimeDirectory=/run/ngfw,
including the owned engine snapshot and independent peer. The actual agent unit
now uses RuntimeDirectoryPreserve=restart; regular stop still clears it, and its
exact integrity digest is updated. Native66 proves peer survival but recovery
still fails. No deadline or capability change is made.

Bounded protected-file reads preallocate the already-validated file size while
retaining the same limit+1 read and ownership/path checks. A private 16MiB ELF
benchmark improves 73.971ms/36.23MB allocated to35.861ms/16.79MB. Native67 still
fails its added8s readiness prewait, but complete baseline cleanup passes22.929s
under the original30s. That prewait failure is not evidence the original30s
restart limit failed. Native68 CPU profiling is diagnostic only: SHA256 accounts
for310/390ms of one helper profile. Its campaign fails; no acceptance is claimed.

Each namespace target capture now independently hashes and holds protected broker
ELF and receipt descriptors. Fresh canonical reopen, full file stamps and complete
process boot identity are checked at later boundaries. Proofs are local to one
capture/process; there is no shared cache, cross-capture or restart authority.
Mutation, same-byte inode replacement, writable-parent, cancellation, wrong-path,
closed-proof and digest mismatch controls pass. Source17 full RA race PASS3.740s.

Restart readiness, real negotiation and final observation now share the original
30s context. Native69 source17 and native70 source18 both pass genuine encrypted
traffic but fail actual30s recovery and cleanup; neither is a complete campaign.
The real kernel reports XFRM if_id as the JSON string"0x1". A disposable actual
network namespace reproduces it; the prior reader fails the roundtrip test. The
reader now accepts exactly numeric1 or canonical"0x1", preserving the exact-ID
ownership guard. Old-source regression FAIL0.065s; source18 full RA race PASS3.994s.
This fixes a real reader error, but native70 shows another recovery failure remains.

Early failed campaigns retain the independent resolver file/peer namespace when
strongSwan replaces its inode: the foreign-replacement cleanup guard is retained.
Full restart, EAP-TLS valid/revoked/foreign-CA, transport rollback, both-host genuine
API/license/session/second-pool400 and en/fa browser acceptance remain required.


Native71 repeats the real30s recovery failure. Native72 is diagnostic only: its
daemon reports a bounded startup phase into its owned private writable directory;
no acceptance or cleanup success is attributed to that modified diagnostic ELF.
A live, root-inode/scope/PID1-verified collector captures namespace-8: the existing
outer-rule reader rejects table100. A disposable actual network namespace shows
iproute2 returns table as JSON string"100", while the guard expects numeric100.
The reader now accepts exactly those two representations; other tables and foreign
string formats remain refused. Real-kernel old-source control FAIL0.061s; source19
full RA race PASS3.675s. Source18 subsystem race PASS33.541s after shortening its
private exec-capable temporary path: earlier runs exceeded the unchanged SNMP
path-length guard; /run alone also disallows execution. Those failures are retained.
Owned inactive68–71 roots are archived with evidence hashes after repeated process,
root-inode and mount checks. Complete canonical acceptance remains pending.


Native73 source19 first recovers the actual engine automatically: connect resync
APPLIED33.180s, finishing about42.56s after restart. The original30s readiness
assertion and baseline rollback still fail; full canonical acceptance remains open.
Source20 removes duplicate whole-broker hashes within one handoff preflight using
a capture-local held proof, retaining fresh canonical reopen/stamps and all fixed
unit/capability/socket guards. The independent unit observer uses its existing
numeric installation proof, which already hashes the same fixed broker/receipt,
instead of a duplicate hash. Standalone broker validation streams the whole ELF
through a fresh proof; no cross-call/capture/process cache is introduced.
The added readiness poll uses the existing8s observation bound, always clipped by
the original whole30s restart context; initial2s polling was shorter than the
verified observation path. Source20 full RA race PASS3.838s. Fresh original native
execution must prove that recovery actually meets30s.


PR232 source2d6546bfe mandatory38155671841 fails scoped Go security lint on
synthetic ELF mode0700 and the intentional writable-parent mutation test. Each
case now has a narrow documented rule annotation: execute permission is required
by the owner-only synthetic ELF control, and mode0770 is the isolated rejection
input. No production rule is disabled. Scoped RA/daemon/subsystem lint PASS0issues.
Source20 subsystem race PASS33.570s. Native75 still fails30s: automatic resync
APPLIED30.196s, finishing about39.77s after restart. Native76 is timing-diagnostic
only; actual engine Create9.380s, unit Start6.807s and connect resync30.769s.
These modified diagnostic ELFs cannot establish acceptance. Both-host native
execution, full restart/TLS/rollback and actual API/browser remain open.


The exact source20 ELF/unit/provider cohort is verified SHA-for-SHA on250. Its
first launcher fails139 before guest startup because vendor libc was paired with
the host loader; the retry uses the genuine paired vendor loader/library closure
without host installation or library changes. The real250 campaign then passes
EAP/VIP/DNS/selectors/ESP/VPP ACL traffic but fails original30s recovery and
cleanup (149.65s whole failed campaign). This is partial progress, not acceptance.
Shared250 VPP remains8102/NRestarts0.

Source21 optimizes only one read-only handoff Verify: its fresh post-capture for
one fixed target becomes the next target's pre-capture in the same operation.
Every target retains pre/post linkage; every Acquire retains two independent
manager captures. Export/Remove keep separate original capture pairs, four-role
requests and exact compensation. No snapshot or ELF proof crosses another Verify.
Owned real child MNT/image controls reject intermediate generation/image/closure,
cancellation, reused snapshot and second-hop failure; all descriptors are closed.
Source21 full RA race PASS3.525s; scoped lint PASS0issues. Fresh original native
execution must still prove30s recovery, TLS, baseline rollback and API/browser.


Source21 normal native77 passes genuine EAP and ESP/VPP ACL traffic. Automatic
recovery reaches authenticated empty-session readback, but final observed session
fails the original whole30s restart context. Connect resync is APPLIED26.056s;
engine creation precedes its end by8.52s. Whole canonical campaign FAIL150.45s,
including retained DNS ownership refusal on failure cleanup. No TLS/rollback
completion or whole-task PASS is inferred. Exact c54e47d PR232 mandatory38157714653
and packaging38157714649 both PASS, but PR232 is still open.

Source22 permits concurrent read-only Runtime Ready/Records/Sessions under a
shared lock while lifecycle changes retain the exclusive lock. Each production
observation adapter serializes its own two fresh socket captures with cancellation
inside its original5s budget. No capture is cached. LazyEngineStore serializes
its first open and directory operations. A control proves Sessions proceeds during
held readiness and lifecycle mutation waits; another proves cancelled capture
waits preserve the token. RA race PASS3.653s, subsystem race PASS32.524s, scoped
lint PASS0issues. Normal native78 is running with original budgets. Agent uses
normal unstripped build; helpers use production packaging's -s -w, all CGO0.
Complete native/TLS/rollback and both-host API/browser evidence remains open.


Source22 normal native78: automatic empty-session readback29.403s and genuine
negotiation29.428s; final observed session FAIL under the unchanged whole30s
context. Whole canonical FAIL150.93s. Source23 normal native79 similarly reaches
readback29.045s and negotiation29.068s, then fails final observation; no completion.
Source23 changes classify helper/daemon from one fresh held two-capture snapshot
in Unit.Start, retaining a fresh manager PID and complete live process identity.
Runtime still independently observes again before loading credentials. Full RA
race PASS3.810s. Source23 native80 adds timing logs only and is diagnostic, never
an acceptance receipt: create6.752s, Start4.320s, each two-capture target/unit
acquisition approximately2s; complete unchanged export repair discarded one
fresh initial snapshot before starting another pre/post pair. Instrumented
Acquire's cancelled flag includes its own deferred cancellation and must not be
interpreted as proof of a caller deadline failure.

Source24 retains that initial snapshot only for read-only verification of complete
unchanged stopped exports, using the same fresh pre/post chain. Partial exports,
generation replacement and mutating dispatch retain separate captures and all
existing guards. RA race PASS3.662s, subsystem race PASS32.271s, scoped lint
PASS0issues. Normal native81 on195 and native250-source24 are running; all18
manifest files on250 match the local81 cohort exactly. No full campaign PASS
is asserted. Inactive owned guests77–80 have their private observations/evidence
retained with SHA receipts before removal; the lab services were preserved.

Actual API/browser preparation uses an owned PostgreSQL role/database, normal
bootstrap-admin login HTTP200, normal configured licence trust key (no licence
issued/uploaded yet), fresh public PKI and an isolated byte-only bridge transport.
Normal API/schema/web builds pass. Existing real Chrome154.0.8037.97 runs through
Playwright; the new browser download failed403 and is retained as failed setup,
not acceptance. No live agent/API/browser campaign or whole RA completion is
inferred from these prerequisites.

Source24 mandatory CI run38161228779 and packaging38161228790 PASS.
Original full source24 campaigns recovered sessions on195/250 within30s but
failed at the later repeated TLS Apply against the unchanged240s deadline.
Fixture25 configures one real responder CA/CRL and three matching client-key
cases; its revoked case failed because an earlier same-identity peer credential
could be selected. Fixture26 clears only disposable peer VICI credentials and
requires actual get-keys readback to contain exactly the selected key. Both
hosts then passed valid, revoked and foreign actual TLS outcomes, with restart
readback26.905s/26.782s, but failed at duplicate physical-dump stream Close.
The generated VPP RPC client closes on terminal control-ping and returns EOF
only after successful Close; fixture27 consumes that success without closing
the already-closed stream again. Original dump bounds and rollback assertions
remain. RA race PASS4.589s, lint PASS0issues; whole fixture27 reruns pending.
API hold captures the real DNS ownership after each verified connected view.
Private API84 setup failed because the minimal guest lacks seq; a fresh API86
uses bounded shell arithmetic instead. Normal login, actual capabilities200,
PKI import, secrets, signed licence upload and valid-state readback pass.
Foundation commit failed on nspawn replacing the guest localtime package link;
only the independently owned guest link was restored. A subsequent commit had
unknown outcome after actual transport loss; no API/browser acceptance claimed.

Full normal product24/fixture27 on250 PASS231.07s with campaignEXIT0:
actual EAP/ESP, unchanged normal Apply, original-unit restart26.407s readback,
TLS valid/revoked/foreign, physical transport ownership, connection/pool removal
and complete baseline rollback. Local87 failed248.26s at VPN rollback against
unchanged240s budget; cleanup Apply completed, but this is not local acceptance.
Concurrent local API work and VPP health timeouts were observed; cause remains
unproven. Local revalidation is required. Private failed roots83/84/85/87 were
retained with SHA receipts and removed only after all-process root identity and
mount checks proved inactivity.

API86 normal public activation and complete baseline restore PASS. Its read-only
hold failed on exact profile equality: real Retrieve contains the public schema's
materialized DPD enabled/30/150/restart and rekey14400/3600/false defaults.
Fixture28 expects these exact values, preserving every endpoint, secret reference,
proposal and policy equality assertion. RA race PASS3.462s and lint PASS0issues.
Fresh owned API88 is prepared with VPP main core1 within the original eight CPU
budget to avoid sharing main core0 with the independent full campaign.
The minimal guest's localtime link pointed directly into zoneinfo; nspawn causation
was not proven. The package-required link was restored only in owned guests.
API86 SIGSYS occurred when applying hostname through nspawn's syscall policy;
its owned UTS was initialized to the normal appliance hostname ngfw without
changing host UTS. API88 selects that hostname at launch, timezone initialization
off and journal linking disabled. These are fixture prerequisites, not product
unit overrides or whole API/browser acceptance.
