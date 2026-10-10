# Independent RAM offline-audit helper review

Verdict: **APPROVE the exact reviewed RAM audit tools for the parent-issued .211
transition/read-only phase**, after worker publication/readback. This does not
approve correction writes, normal return or a .37 transition. Reviewer performed
no target operation and modified no product or other agent worktree.

Exact source files in the host operator's task:

| Source | SHA256 |
| --- | --- |
| hardware-211-20261010-nsfs-check.c | 36e12485aa4e1ca3a85e0a1760fff89604c0d4b3f9b5b190aeee873bc45a66ee |
| hardware-211-20261010-offline-audit.sh | e5608afb712bd86251395800a4e27d9d7f1d8df75d75d0f2c41d827ada89affc |

C source opens namespace references read-only, validates nsfs and NS_GET_NSTYPE,
and for mount namespaces forks a short-lived child which alone calls setns. It
reads /proc/self/mountinfo without executing an original-root program. Malformed
input, unreadable/empty mountinfo, original8:2 mounts and unexamined nested nsfs
mounts refuse. The parent waits for child exit and closes its reference.
Matching Linux7.0 mntns_install resets the short-lived child pwd/root to the
destination namespace root. Its static code still executes from RAM and invokes
no original-root executable. Mount visibility remains insufficient for lazy-detached
references; namespace inspection does not replace the separate exclusive guard. The independently behavior-tested exclusive
block guard remains required after all helper references close.

Bash scans each process root/cwd/executable, mapping devices, file/directory FDs,
FD mount IDs and mountinfo; detects old-root8:2 and open8:0/8:2 devices, enumerates
namespace FDs including those without a process namespace representative, and
checks block holders. Namespace inode dedup preserves every reference in output.
Unknown bound nsfs mounts, unreadable selectors, empty userspace mountinfo and
disappearance races make the result inconclusive/refused. The final exact static
block guard runs only if the scan succeeds and after helper children exit.

Initial Bash e679e64d repeated unchecked stat selectors could treat a race failure
as empty/non-oldroot. Reviewer requested reuse of validated metadata plus refusal
on selector failure. Corrected fdbeadc5 also verifies holder-directory availability
and nonempty userspace mountinfo. Final e5608afb adds a checked userspace mount
namespace ID rather than an unchecked printf substitution. Reviewer independently
read the exact final source and ran bash -n: exit0. No safety gate was removed.

Actual controller-only reviewer command was gcc -static -O2 -Wall -Wextra -Werror
on the exact C source to a newly owned TemporaryDirectory; the binary and temporary
directory were removed when the review test ended. Output:

```text
compile exit0; stdout0bytes; stderr0bytes
static binary1014992B
SHA2562907bb0345fb9dad199e48b2539ce9038b24fbafa00fa9672b4074239a6ac681
missing arguments: exit2
/does-not-exist namespace path: exit2, REFUSE unavailable/not nsfs
/etc/os-release regular file: exit2, REFUSE unavailable/not nsfs
/proc/self/ns/mnt 8 2: exit3
 namespace inode4026531832 type0x20000
 mount_entries28 oldroot_mounts1 unresolved_nsfs_mounts0 malformed0
/proc/self/ns/pid 8 2: exit0, type0x20000000
```

These inspect only the controller's current namespaces; no disk writes, target
commands or persistent namespace changes occur. They do not establish target
posttransition offlineness.

Reviewer independently parsed the operator's private actual receipts at
/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-211, without
printing credential/configuration payloads:

| Receipt | Bytes/mode | SHA256 |
| --- | --- | --- |
| audit-tool-validation.json | 88600/0600 | 395effcb2564445ffaafe202de946f8b1e43671f2c6882f7ba92ec8ab044e3b8 |
| audit-final-source.json | 729/0600 | 97d73bc53b2181c7fef21ea08a589610312db0d096f9a5518665cbe149a8d76f |

Selected actual target-worker output:

```text
RAM chroot bash -n: exit0, empty stdout/stderr
exec9</proc/self/ns/mnt; stat -fLc%T fd9; nsfs-check fd9 8 2:
 nsfs; namespace inode4026531832 type0x20000
 mount_entries29 oldroot_mounts1 unresolved_nsfs_mounts0 malformed0
 exit3; stderr0bytes
whole mounted-root audit: exit2, stderr0bytes
 SUMMARY processes271 fds291 nsfs0 races0 failures1310
final e5608afb RAM bash -n: exit0, empty stdout/stderr
actual checked RAM readlink selector: exit0, mnt:[4026531832], stderr0bytes
```

The whole mounted-root refusal is expected: old-root references remain and /sys
is not yet carried into the staged chroot. It is not an offline PASS, nor should
its exit2 be confused with standalone block guard BUSY3. Previous actual static
guard mounted-negative is independently corroborated in the phase report.
After transition require real /sys API availability, RAM PID1/executor and rescue
runtime, fresh/held authenticated SSH, management baseline equality, conclusive
audit and final exact block guard0 before read-only e2fsck/image. Refusal or missing
evidence stops the phase; no mounted-root fsck or arbitrary PID1-FD closure.

## Child-root source clarification and actual .37 mounted-negative test

The earlier reviewer/manager retained-RAM-root interpretation was inaccurate.
Independent matching upstream mntns_install explicitly calls set_fs_pwd and
set_fs_root after switching the child namespace.
[Linux7.0 namespace implementation](https://raw.githubusercontent.com/torvalds/linux/v7.0/fs/namespace.c).
This explains actual mounted-negative helper3 on both hosts. Parent/global
namespace remains unchanged, only static RAM code runs, and all inspection children
exit before the final guard. .211 actual posttransition audit had nsfs0 and guard0;
this clarification does not invalidate its independent offline proof.

Initial .37 staging wrapper expected namespace selector0 and exitedSSH1 on the
correct helper3. The reviewer had approved that incorrect expectation; initial
failure is preserved, not relabeled PASS. Focused final wrapper
SHA256546f1efb0625094ea0ef24adf9709ef467ded8968a3b61b4d1242151bbe299af
changes only expected mounted3/oldroot1/malformed0 and new receipt names.
Published4f841797361a5fda0b7512e59e4c678f8609accc, outer/REMOTE AST2 PASS.
Independent actual private validated receipt3047B/0600
SHA2566e883d97a5ff278d0a6e87fa5336e948202e2865888e9586cbd4ae44cdc0d19a:

```text
outer SSH0; outer capture stderr0bytes
RAM runtime tools/version checks0; Bash syntax0
inner Bash syntax stderr101B: C.UTF-8 locale unavailable warning
non-nsfs regular file refusal2,47B diagnostic
mounted namespace helper3:47entries/oldroot1/nsfs0/malformed0
mounted block guard3 BUSY; rescue PID3866 unchanged
root8:2/RAM0:51,held shell3940 exists,nextroot absent
full offline audit/transition: NOT RUN
```

Inner locale warning is distinct from empty outer SSH stderr. LC_ALL=C on future
audit invocation avoids it; final reviewed audit already exports C. No rerun is
needed solely for this warning. This is RAM tooling readiness, not .37 offline or
repair acceptance.
