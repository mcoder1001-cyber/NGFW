# PPPoE and RA completion work — acceptance remains open

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
