# Independent RAM staging source review

Scope: read published host-worker source and runtime receipts; reviewer writes
only owned documentation, never executes the target staging scripts. Reversible
RAM staging is authorized by the manager. Root transition, disk repair and reboot
are separate gates and remain unapproved here.

The initial .211 source73a41569 (SHA2564e2f4f41...) was blocked before staging:
SurviveFinalKillSignal was in the wrong section, ordinary shutdown ordering was
missing, bootstrap WorkingDirectory created implicit mount dependencies, devpts
was missing and the RAM budget was only1 GiB. Corrected c4027e96493f29edbc79b3475751403f95fde705
(source SHA256517d789ecd751a85e383385f26baa6dcb3fa5e468d79cb6bdb1bdaa70e4113ed)
was approved for RAM-only preparation on the source/unit/dependency design;
actual authentication and live property proof remained pending.

The first actual staging attempt safely halted in dependency collection before
keys, virtual binds or sshd: independently ldd'ing a private library discarded
the executable RUNPATH. Worker reports only the owned RAM mount and initial
binaries were created. No root transition or filesystem repair occurred.

Focused follow-up reviewed published7f8e342681c9f1bdfa41b375400bdc92d9446924:
[.211 staging source](https://github.com/mcoder1001-cyber/NGFW/blob/7f8e342681c9f1bdfa41b375400bdc92d9446924/docs/status/tasks/hardware-211-20261010-ram-stage.py).
Its flattened executable ldd closure, copying resolved libraries without standalone
recursion, addresses that collector error. This exact source is approved for
reversible worker-owned partial RAM cleanup/retry; actual runtime/authentication
proof remains pending. No root transition is approved.

Reviewer correction: the immediately preceding checkpointccd6581d incorrectly
alleged literal backslash-n NSS separators. The reviewer misread additional JSON
escaping in tool output. The worker challenged the finding; direct AST ordinal
checks independently confirm real newline characters and zero backslashes.
The allegation and associated retry BLOCK are withdrawn. No source bytes changed.

Actual read-only commands and selected output:

```text
git show 7f8e342681c9f1bdfa41b375400bdc92d9446924:docs/status/tasks/hardware-211-20261010-ram-stage.py
python3: hashlib.sha256(source bytes); ast.parse(source)
sha256 743272ac8c82282886215b651c582d94bf02d018263e61895875b9bc4b4d4c2a
syntax parse: success
AST /etc/passwd and /etc/group join Constant: length1, ordinal[10], backslashes0
AST /etc/shadow: newlines1, backslashes0
AST /etc/nsswitch.conf: newlines4, backslashes0
AST /etc/hosts: newlines2, backslashes0
```

Verdict on7f8e3426: **APPROVE RAM-only cleanup/retry with the flattened ldd change**.
Cleanup/retry may
touch only exact worker-owned partial RAM mount/unit/tree, after checking identity
and no running survivor/holders; no cleanup of existing system resources.
The worker and manager received the correction immediately. All actual stage/auth/process
and network comparisons still need published receipts after a corrected retry.

Additional transition issue under investigation: stock soft reboot transfers the
old /run mount over the candidate /run. Candidate /run/sshd may therefore become
hidden; the original ssh.service may remove its old runtime directory when stopped.
A surviving service is restored without executing its ExecStart again. Do not
assume an active survivor's RuntimeDirectory directive alone recreates the missing
directory. Matching exec_runtime_make also does not create those directories;
setup_exec_directory is in the executor spawn path. This is an inference from
the complete coldplug/runtime/spawn implementations, not observed transition proof.
Require exact source/runtime resolution and an authenticated RAM PTY kept open
before transition; chroot authentication alone is not proof of boot.
[Matching root switch source](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/shared/switch-root.c),
[Matching service coldplug source](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/core/service.c),
[Matching runtime creation source](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/core/execute.c),
[Matching executor directory source](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/core/exec-invoke.c).

## Further focused candidate changes

Worker .211's second actual RAM attempt reached tree/key/virtual mounts but stopped
before daemon activation because /usr/sbin/chroot was absent. Focused source
SHA2569baf631c1895f4ac0ce1b64b2e4d3ab5413f1f7d2d55847e62e4d38406d5e3bb
discovers/validates the actual chroot before mutation, uses it in bootstrap/checks,
copies its runtime and explicitly sets the SSH session tool PATH. Independently
read diff/hash: APPROVE focused applicability to reversible owned cleanup/retry.
Unmount only owned devpts/proc/dev children and then the owned tmpfs, after proving
no running survivor/listener; keep original SSH22 available and nextroot absent.

Complete .211 candidate SHA25691926edc5b1463066b9487f9937d06aa41980c00c6ac7f5adb7068dbec233719
adds a new-root-only oneshot mkdir/chmod/chown for /run/sshd, with default target
Requires/After and SSH After that helper. Focused source/design APPROVE RAM-only
cleanup/retry after durable source publication; no helper execution or transition
PASS is claimed. Direct helper command paths are checked; prefer explicit RAM log
output so minimal boot does not depend on a default journal socket.

Complete .37 candidate SHA2568dc72ca5703113ebef579c2c55eb0550aca754c285d9e8ce4e5c5c585b2b92ac
adds matching systemd-shutdown/dependencies, validates hardcoded tool paths before
mutations and adds equivalent RAM runtime-preparation/default ordering. Its global
namespace bootstrap has no WorkingDirectory/RootDirectory/mount Requires; same-name
RAM unit uses direct sshd. The test mode requires authenticated PTY, child cgroup,
RAM root/cwd/exe/maps/fds and owned-service restart with original22 still listening.
Reviewer read the complete source and independently ast.parse'd the outer source
plus REMOTE/AUDIT/STOP_START: four syntax checks PASS. APPROVE RAM staging/test
after durable source publication. Actual stage/test receipts remain pending.

Both candidates are operational task scripts, not package changes; the reviewer
did not execute either script. These hash-based reviews must be paired with the
worker's published commit/readback before execution. Neither proves soft reboot,
fresh posttransition SSH, offline root, successful repair or normal reboot.

Next: inspect actual staged tree/units,
parser warnings, global mount namespace, RAM-only exe/maps/root/cwd/fds and key-only
SSH command/PTY receipts. Keep /run/nextroot absent throughout staging. Positive
target O_EXCL plus all-namespace/reference audit and budgeted preservation remain
mandatory before any fsck.
