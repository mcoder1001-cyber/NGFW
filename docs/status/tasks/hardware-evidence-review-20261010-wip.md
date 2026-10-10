# R7 evidence review WIP — 2026-10-10

Branch/worktree: `codex/hardware-evidence-review-20261010`,
`/root/ngfw-wt/hardware-evidence-review-20261010`.
Starting local/base SHA: `d2d55984d74fa1d06c32e8271886f11f16375407`.
Owned files: docs/status/tasks/hardware-evidence-review-20261010-* only. This checkpoint's local SHA
is its commit ID; publication is claimed only after successful push and remote
readback and is sent to the manager. Last verified published checkpoint:
`65ca4275cd7bdf161d88b4b9d34fc4c9b94f7427` (focused source approval; matching CLI
push/readback). Initial checkpoint was `df276e94a1d12febc95b64c9232e609029c10620`.

Completed: mandatory instructions read; PR217 actual HEAD, changed paths,
archive ref, historical hosted gate and current pending hosted gate inspected.
Remote worker/reviewer receipts and external build log selections corroborate
the ext4 blocker, protected management ports, seven/seventeen data NIC inventories,
no target installation/acceptance and the narrow one-line packaging correction.

Initial review commands run in this worktree and actual output:

```text
git diff --stat origin/main...bde83bc87ae817a61bbc70e4029f76109ae77c35
4 files changed, 121 insertions(+), 1 deletion(-)
git log --oneline origin/main..bde83bc87ae817a61bbc70e4029f76109ae77c35
bde83bc87 fix(packaging): declare native API library dependencies
git ls-remote origin refs/heads/codex/archive-hardware-manager-20261010 refs/heads/main
2045ab8b3d2f477bb23446fb5e58b7d9d3abea3c refs/heads/codex/archive-hardware-manager-20261010
d2d55984d74fa1d06c32e8271886f11f16375407 refs/heads/main
gh run view 37903333143 --json status,conclusion,headSha
{"conclusion":"success","headSha":"30de26ee6a5697a3713fe4375399b86c5588a472","status":"completed"}
gh run view 38033113750 --json status,conclusion,headSha,jobs
headSha=bde83bc87ae817a61bbc70e4029f76109ae77c35; status=in_progress; conclusion=""
python3 tools/board.py --help
board ok: 212 tasks; progress 98.4% by hours, 205/212 merged; ready=0 running=0 parked=7
tools/ci.sh check --base origin/main
check PASSED (0m14s)
```

The board tool prints validation rather than help; no board file was changed.
Historical gate success is not final-head gate success. Fixed native build log
contained 16+12+6+3+4=41 successful fixtures, while build/archive inspection was
pending at that initial snapshot; this reviewer did not rerun those tests or claim them as
its own test execution. External old runtime manifest explicitly blocks installation.

Observed operational roles via collaboration inventory: root, host_37 (R1/T1),
install_review (R8) and this R7 reviewer running; host_211 not listed after its R2
handoff. This is a transient inventory, not proof of persistent developers or a
service. Hardware installation roles remain awaiting offline recovery.

Current verdict: APPROVE final candidate
`5bd7e8b545fc765fd2babd8dda15175d6f33af1b`. R7-1 closed: WIP links a committed
evidence appendix with exact commands, real output and immutable receipt links.
Full focused command/output record is `hardware-evidence-review-20261010-review-R7.md`.
Product/control/test/build/CI unchanged from approved compiled2045ab8; D112 count1,
remote integration archivebde verified. R8 final archive receipt at1e31b4ae is
published and matches four actual package controls/hashes and unchanged safeguards.
One optional NIT: quoted gitleaks trailing blank at evidence.md:129 makes
`git diff --check` exit2; manager notified, not a blocking evidence issue.
Complete hosted quick run38033766837 was independently observed in_progress on
exact5bd7 head. No final quick/T1 or hardware acceptance PASS is claimed here.
Reviewer docs check before this checkpoint: `tools/ci.sh check --base origin/main`
exited0 and printed `check PASSED (0m15s)`.

Exact next command if manager supplies a later changed head:
`git fetch origin codex/hardware-manager-20261010`, then compare docs/product delta
against approved5bd7e8b before any revised verdict. Manager must complete mandatory
final-head gate/T1 and verify expected main/PR heads before merge. Hardware recovery
requires owner console/rescue information and clean offline filesystem repair.

Operational handoff: focused R7 recheck complete; reviewer is **awaiting resume**
for a later changed head or evidence question after publication. No reviewer-owned
process remains running and no target actions occurred. The earlier live-agent
snapshot is historical evidence, not current active-worker status.

## Postmerge/private recovery evidence follow-up — 07:56 UTC

Resumed independently for manager public925703c2/final0f3ab280 recovery docs and
ready private configuration backup metadata only. Current follow-up verdict:
APPROVE0f3ab280f3d3941b21e2580d584bdaddc614822c. Addendum
`hardware-evidence-review-20261010-postmerge.md` preserves exact commands/output,
published worker links and privacy/recovery limitations. Original5bd7 source
APPROVE remains separate; no product or target writes occurred.

Actual observed merge4908716b exact parents/treea0d7b7 match GitHub's tested
integration8a15d644 tree. PR quick38033766837 completed success; bare main
38035583209 observed in_progress on4908716b. No main quick PASS claimed.
Private parent and both host dirs0700; all24/.37 and26/.211 regular artifacts0600;
valid manifests, two readable5-member config archives with management netplan;
all5 checked public receipt hashes match. Contents were never printed/committed.
Native nft capture exit127 remains explicit; empty successful compatibility exports
do not prove no native nft rules. Config snapshots are not full system/data backup.
Candidate23-member tar includes0private-directory or private-artifact names and
0unsafe paths. Public runbook preserves console/fullbackup/unmounted repair gates;
install/API/forwarding/reboot acceptance remains NOT RUN.

Remaining: manager/main tester confirms actual bare main completion and publishes
its new status; any refreshed candidate needs exclusion recheck. Hardware recovery
awaits owner input. Exact next command on later status change:
`gh run view 38035583209 --json headSha,status,conclusion`, then fetch and compare
manager docs SHA before extending this scoped approval. Reviewer ends this follow-up
after publication and is awaiting resume; no persistent worker service is claimed.
Own-doc check before publication: `tools/ci.sh check --base origin/main` exited0,
`check PASSED (0m14s)`; no duplicate full quick ran.

## Owner-authorized root recovery safety review — 08:32 UTC

Operational role resumed: independent read-only reviewer, not target installer.
Last own published checkpoint at resume0eb7036450d410b7540e05a786205c72bd814acf.
Owner now explicitly authorized repairing disk/root filesystem; old "awaits owner
input" wording above is historical. Current open requirements concern verified
technical recovery and data preservation, not repeated authorization.

Completed: read current recovery runbook; reviewed sanitized fresh worker reports;
checked matching primary systemd259.5 docs/source and kernel/e2fsck/open manuals.
Concrete supported candidate: executable RAM root plus independently authenticated
RAM-only survivor SSH, then supported userspace soft reboot with unchanged kernel.
Direct switch-root is initrd-only. Stock soft reboot internally uses lazy detach;
SSH reconnection/mountinfo alone cannot certify ext4 offline. Proposed validation
includes a separately vetted read-only O_EXCL device-busy probe and all-namespace/
old-reference audit; no helper/test/target transition is currently claimed.

Current execution verdict BLOCK until survivor/rehearsal/offline-device proof and
measured metadata/image/data-recovery plan are established. Detailed proposed safe preparation,
actual evidence attribution and exact alternative external inputs are recorded in
owned `hardware-evidence-review-20261010-ram-recovery.md`. Immutable fresh host
diagnostic receipts pending. Existing backups remain private; no contents read or
published in this follow-up. No product/target changes or duplicate full quick.

Next command: fetch published worker branches once fresh receipt SHAs arrive,
read sanitized receipts, and compare each proposed capability with recorded actual
commands/output before finalizing this independent review. Manager may assign
RAM preparation to installers only after exact plan review; this reviewer owns docs.

Preparation checkpoint validation: git diff --check exited0; unchanged required
tools/ci.sh check --base origin/main exited0, check PASSED (0m13s). Manager proposed
metadata e2image plus undo with explicit limits; full59GiB image is an option,
not a universal requirement. .37 mounted-negative exclusivity probe reports EBUSY;
pending positive/lazy-pinned tests remain explicit. Controller measured free space
31,129,600 bytes; no preservation capacity PASS. All target preparation remains
installer-owned and pending exact staged-artifact review.

08:38 follow-up: preparation checkpoint5fb0ad29a3a196993509df29d3961c2578c979b8
published/readback. Read immutable fresh host receipts37=0fb0d56f,211=2efc98ee;
seven named private diagnostics0700-parent/0600-file and7/7 hash comparisons PASS.
No payload printing. Matching systemd serialization/lookup/executor source supports
manager's global-namespace chroot→RAMsshd survivor design, with direct new-root
same-name /etc unit; exact staged artifacts/auth/runtime proof remain pending.
Design APPROVE only reversible RAM preparation; transition/repair remains BLOCK.
Old pinned executor closes on manager cleanup; new manager reopens RAM helper.
Live networkctl reload would reconfigure links; no network modification in staging.
RAM subtree and backup return lifetime, unit precedence/drop-ins and policy-rule
preservation are explicit in report. Controller new free-space observation198184960B
does not certify actual preservation size. Task-only static block-check source at
fec319ed independently compiled warning-clean, static ELF860432B matching supplied
SHA02a2270a…d0f41c, six refusal tests PASS; additional guard APPROVE is separate
from pending target mounted/positive/lazy-pinned behavior. Owned block-check report
records actual commands/output. Reviewer continues awaiting exact staged receipts;
no target mutation and no duplicate full quick.

Current recovery follow-up: last verified published own checkpoint
fe910846d6b9b95b235bb32de27da1a2df9927d1. Independent controller-only guard test
used a newly owned8 MiB RAM-backed loop in a private mount namespace; five real
cases PASS including mounted refusal and lazy-detached/open-directory-fd refusal
until final release. Exact helper hash matched; cleanup removed only owned test
resources; process exit0. No physical/target device touched. Guard report contains
actual actions/output; target offline proof remains pending.

Independent .211 stage source review: c4027e96 source design approved RAM-only,
first actual collector attempt safely halted before keys/binds/sshd. Published
7f8e3426/SHA743272ac flattened ldd fix is applicable and APPROVE for RAM-only
owned partial cleanup/retry. Correction after worker challenge: the newline BLOCK
in publishedccd6581d was incorrect; reviewer misread extra JSON escaping. Direct
ordinal check proves passwd/group separators length1 ordinal10 and shadow/NSS/hosts
newline counts1/4/2 with zero backslashes. No source change. Both worker and manager
notified immediately; owned ram-stage report retracts the finding explicitly.
No transition/repair/reboot approval. Next exact command: read published actual
stage/auth/global-namespace/process-fd receipts after worker retry, then independently
compare units/tree/runtime and network snapshots against source7f8e3426.

Further review: matching259.5 source confirms /run/sshd coldplug concern; workers
now include explicit new-RAM runtime-preparation oneshots. Focused .211 discovered
chroot fix9baf631c and full prep candidate91926edc APPROVE applicability for RAM-only
owned cleanup/retry after publication. .37 full source8dc72ca5 independently read
and outer/REMOTE/AUDIT/STOP_START AST parses PASS4/4; matching shutdown runtime,
early exact-path preflight, runtime helper/default ordering APPROVE RAM-only after
publication. Actual staged/live/auth/PTY receipts pending, no scripts executed by
reviewer. Proposed return report supports single-force normal kernel reboot only
after actual offline clean repair, offhost verified preservation and original
boot/auth/network/config checks. No transition, repair or return execution approval.
Next: read immutable worker staged/test receipts, compare hashes/units/properties
and independently inspect private manifest metadata only. Private contents remain
unprinted/uncommitted. Last verified own remote31872b92238e282f69a8c710128156a13e84e033.

09:10 actual-evidence checkpoint: last own remote0da3aefdb3ad5735c7a841e092b9d342a39842a0.
Independent ready private metadata/selected receipts corroborate .211 stage+four
persistent RAM/global-namespace/cgroup processes, exact unit equality, empty successful
parser, mounted-negative static BUSY3,8GiB RAM budget and selected auth/boot backups.
Signed SMART7.5-2 exit0/media0/ATAerrors0; historicalCRC1003 and SCSIioerr6→9 remain
explicit unexplained baselines. Formal owned transition-211 verdict APPROVE only
supported RAM transition then strong offline proof/read-only diagnosis/metadata;
worker actual public receipt must publish/readback before crossing. Corrections,
return and hardware acceptance not approved. Next: read worker actual transition
receipt, verify RAM PID1/newruntime/freshSSH/management, all namespace/nsfs/reference
audit and positive0 guard before extending any operation scope.

.37 source final5b9a8e65/b9da683b independently readback APPROVE RAM staging/test;
actual first final readiness check raced chroot startup and was honestly preserved.
Read-only completion17c370c9 and functional aec1a0f1 receipts privately parsed:
network5/5 matches, guard3, RAM refs/global namespace, authenticated PTY twice,
owned daemon restart3669→3866 and original22 preserved. Further .37 SMART reveals
wear126%/two remaps despite aggregate PASS. Owner explicitly authorized logical
repair despite known wear; do not manufacture another replacement/full-image
approval blocker. Actual new read/reset/uncorrectable failures remain stop conditions;
.37 transition is not approved by .211's phase verdict. Own docs check PASS14s,
no target commands/product edits/full-quick duplication.

09:14 classification addendum: formal phase review45b08eab2e9bab83e8327c58951acfbc4e2d8190
published/readback. Independent actual private37844f38/844194cb receipts show uutils
dd direct option failed EINVAL/counterunchanged, separate aligned preadv256MiB PASS0
ioerr12→12/no new storage errors; repeated SMART query reproduces +3 with unchanged
CRC/media/ATA evidence. Upstream7.0 SCSI/ATA completion sources support possible
CHECK_CONDITION query association; exact command causality/downstream equivalence
not claimed. Transition/read-only approval unchanged; full metadata read still
monitored before correction review. Next command: fetch worker's actual public
receipt and inspect posttransition RAM/runtime/offline guard evidence once ready.

09:24 independent public readback: worker actual .211 e65e4f86e87664eef20b370f05a31d84036c5fdb
confirmed remotely; exact stage source e33b95 unchanged. Publication prerequisite
closed. Private network-stop receipt5803a360 independently hash/mode/selector checked:
static infinite management IPv4/kernel IPv6LL, no dynamic/expiring routes or leases,
no stop hooks/unit overrides. Matching259.5 source confirms ordinary termination
preserves these static objects; static ACD removal exception is inapplicable under
the observed unset non-link-local IPv4 DAD default. Matching .network.d inventory
requested to exclude a merged DAD override before final network-premise closure.
No live network edit/reload requested. Last verified own remote5af876c00bc7f93366dd1f1806b3eb3d485bb933.
Current operational role: reviewer running this read-only follow-up; prior awaiting-resume
notes and worker inventories are historical. Next: read ready override inventory,
then actual transition PID1/runtime/SSH/network/offline proof from the operator.

09:29 final network premise closed: independently parsed84a51a1e private ACD/drop-in
receipt13208B/0600/0700, matching/generic/prefix overrides absent at allfour roots,
actual networkd JSON dropins[] and static/configured IPv4. Preserved networkctl-cat
exit1; successful JSON/inventory are proof. Source-supported normal-stop static
management retention APPROVE without KeepConfiguration mutation/reload. Parent
conditional transition/read-only release still requires actual copied RAM audit
helper readiness, live PTY/.37 checks and posttransition offline proof. Last own
verified remote3628f4a93f0c139a9ad0a042e7c726e8a994e7ff. Next exact operation: read
worker's staged audit-helper source/receipt, then actual phase results; reviewer
never executes target changes. No corrections or normal-return approval.

09:33 actual RAM audit readiness review: exact C36e12485 and final Bash e5608afb
APPROVE within parent-issued transition/read-only phase after publication. Independent
controller static compilation warning-clean, exact1,014,992B binary2907bb03, five
argument/type/mounted-namespace behavior checks matched expected outcomes. Temporary
controller resources cleaned. Ready private395eff/97d73 receipts hash/mode verified;
actual mounted nsfs refusal3, whole mounted-root audit refusal2, final parser and
checked namespace selector0. Corrected unchecked-stat and namespace-selector issues
recorded. Root-filtered mountinfo limitation explicit; exact exclusive guard0 still
mandatory after all helpers exit. Parent/worker notified final focused applicability.
Last verified own remote cb7ed8d7a41648d5e807b002591a937f80884617. Next exact command:
read worker immutable published helper checkpoint, then private actual posttransition
RAM PID1/runtime/auth/network and conclusive offline audit/positive guard receipts.
No correction, normal return or .37 transition approval implied.
