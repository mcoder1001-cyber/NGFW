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
recursion, addresses that collector error. A new independent source/AST finding
blocks retry of this exact source: generated passwd/group/shadow/nsswitch/hosts
contain literal backslash-n separators instead of real newline characters.
Malformed NSS files cannot serve as a vetted authentication runtime. The earlier
source-only approval is superseded for execution by this concrete finding.

Actual read-only commands and selected output:

```text
git show 7f8e342681c9f1bdfa41b375400bdc92d9446924:docs/status/tasks/hardware-211-20261010-ram-stage.py
python3: hashlib.sha256(source bytes); ast.parse(source)
sha256 743272ac8c82282886215b651c582d94bf02d018263e61895875b9bc4b4d4c2a
syntax parse: success
AST /etc/passwd join Constant: literal backslash followed by n (two characters)
AST /etc/shadow and /etc/nsswitch.conf: same literal separator
```

Verdict on7f8e3426: **BLOCK retry until generated-file newline correction**;
**APPROVE applicability of the isolated flattened ldd change**. Cleanup/retry may
touch only exact worker-owned partial RAM mount/unit/tree, after checking identity
and no running survivor/holders; no cleanup of existing system resources.
The worker and manager were notified immediately. All actual stage/auth/process
and network comparisons still need published receipts after a corrected retry.

Additional transition issue under investigation: stock soft reboot transfers the
old /run mount over the candidate /run. Candidate /run/sshd may therefore become
hidden; the original ssh.service may remove its old runtime directory when stopped.
A surviving service is restored without executing its ExecStart again. Do not
assume an active survivor's RuntimeDirectory directive alone recreates the missing
directory. Require exact source/runtime resolution and an authenticated RAM PTY
kept open before transition; chroot authentication alone is not proof of boot.
[Matching root switch source](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/shared/switch-root.c),
[Matching service coldplug source](https://raw.githubusercontent.com/systemd/systemd/v259.5/src/core/service.c).

Next: inspect corrected immutable worker source, then actual staged tree/units,
parser warnings, global mount namespace, RAM-only exe/maps/root/cwd/fds and key-only
SSH command/PTY receipts. Keep /run/nextroot absent throughout staging. Positive
target O_EXCL plus all-namespace/reference audit and budgeted preservation remain
mandatory before any fsck.
