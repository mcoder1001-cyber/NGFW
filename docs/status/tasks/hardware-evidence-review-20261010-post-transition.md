# Independent actual .211 RAM transition and read-only diagnosis review

Actual transition/runtime/offline proof verdict: **PASS within the released
read-only phase**. Filesystem remains corrupt; correction, normal return and
hardware acceptance remain pending. Reviewer performed no target operation.
Worker source/readiness publication f5018b5710dcc78a602e478c19565f81d441c83b
independently read back remotely; git-show source hashes remain C36e12485 and
final Bash e5608afb as approved in the audit report.

Named actual private host-211 receipts independently hashed/parsed, all0600:

| Receipt | Bytes | SHA256 |
| --- | --- | --- |
| post-transition-initial.json | 153 | de8c6113fca547b7e46d33029bbe17d7235b1920b2df559905d4323b2652b1a9 |
| post-transition-fresh-auth.json | 577 | 8d8935aab15f80eadaf75926fae80bde34d6e7de24d63abb9807f4495d8142f7 |
| pre-transition-baseline.json | 23506 | e30f3ffa1e846705608465bfc0dafd1b907a159945e9068baca90a163ee35fa8 |
| post-transition-network-runtime.json | 22250 | 1dffaa1a0bf0da26871aa3a128306c381c4e6a0a684407dd6694db521791142c |
| post-transition-offline-audit.json | 25017 | b74d8312d66acfcf7483a83a05cada8da896a5ed9ccd99e73a720b5c27cf61d6 |
| offline-fsck-readonly.json | 1878 | 03190e6c7c7994f01a6897105ec83aa4fed68ebc1fbe56aa32674db053167f3a |

09:34:27.730827UTC initial fresh authentication exited255, connection reset.
The held RAM PTY survived, so the operator stopped further diagnosis and inspected
runtime. Worker reports /run/sshd was not yet prepared; no manual fix/restart
was needed. At09:35:57.843812UTC fresh SSH exited0 with empty stderr; selected
actual output showed PID1 root dev46/tmpfs and matching systemd, rescue PID6421
loaded from direct /etc RAM unit, preparation helper active/exited Result=success
status0, and /run/sshd0755 owner0:0. The initial failure is preserved.

Reviewer independently parsed all NGFW_BEGIN/NGFW_EXIT blocks and compared actual
before/after JSON, without normalization or printing network/configuration payloads:

```text
all11 runtime/network command sections: exit0
address28 entries: exact JSON equality True
link28 entries: exact JSON equality True
IPv4 routes7 / IPv6 routes4: exact JSON equality True
IPv4 rules3 / IPv6 rules2: exact JSON equality True
PID1 root and executable devices46; executor fd9 device46
executor SHA256d254a2e199cec12e63188f78bd073bafe89dc29fbe3c860c2d626fce2563bbf5
actual sysfs present; exact owned nextroot marker removed
09:37:39.087853UTC offline audit SSH exit0, stderr0bytes
AUDIT rootdevice46 target8:2 parentdisk8:0
SUMMARY processes267 fds93 nsfs0 races0 failures0
userspace8, all same global mnt:[4026531832]; old device/reference records0
refusal records0; FINAL_BLOCK_GUARD exit0 after helper exit
```

This satisfies the phase's actual fresh-auth/runtime/network/strong offline
prerequisites. Root-filtered mountinfo and the separate exclusive guard remain
distinct evidence; no orphan namespace reference was observed in this snapshot.
The host operator proceeded within the released read-only diagnostic scope.

Actual e2fsck -f -n declined every proposed write and aborted during pass2:

```text
pre-diagnostic exact block guard: exit0
invalid extent nodes: inodes259596/259597/259598
corrupted directory: inode259599, block0/offset0; salvage declined
incidental extent optimization proposal: inode259816, declined
e2fsck exit12 (uncorrected errors plus operational abort)
ioerr before/after0xc; no correction performed
```

This is a partial diagnostic, not a clean complete filesystem pass. Tool invocations
for kernel health and df-B failed (BusyBox dmesg missing, df-B unsupported), so
only the captured SCSI counter stability is established at this point. Metadata
image was held until actual kernel/space baselines; preserve these failures.
Read-only inode stat/ncheck can establish affected types/names; no debugfs-w or
file-content dump is approved. Generic optimization of valid inode259816 is
incidental and need not be accepted during a future targeted correction.
Actual measured image/offhost preservation, health comparison, affected paths and
undo budget remain required before any correction review.

## Narrow kernel-health reader

Exact source hardware-211-20261010-kernel-read.c
SHA25678f74c244897cf6009f63570e11fa0c6e91b3da6bcc14dc7b89917b1ff3add4b
APPROVE for parent-authorized RAM staging/runtime baseline. Source hardcodes only
klogctl10 buffer-size query and klogctl3 non-clearing snapshot, bounds allocation
to64MiB, refuses extra arguments and checks read/output failure. Action3 is
nondestructive;10 queries capacity. No consume, clear, read-clear, loglevel or
write action exists. [Linux syscall documentation](https://man7.org/linux/man-pages/man2/syslog.2.html).
Independent controller-only static build -O2 -Wall -Wextra -Werror exited0/no output:

```text
binary856288B SHA2563ebe9a3e308b2d05485035582c961eb272f826fa5d3c059080f82a85d13bdd2d
extra arguments refuse2, empty stdout/stderr
non-clearing controller snapshot exit0,204613B,stderr0bytes
contents never printed; digest e445d3916d63661e1d7f68458221c2ebedafbd671906e133f411a46ea6a9bfe1
```

The owned TemporaryDirectory and binary were cleaned automatically. This does
not claim an actual target baseline; operator must privately capture/compare
new storage events before/after metadata reads, along with counters/SMART limits.

## Separate .37 RAM-only tooling

Independent focused source approval for hardware-37-20261010-audit-stage.py
SHA2569b9ba20bf71eefd39d1cc5c05647e13f4e8ef695d9745b3aefa03bef37651ebf:
controller plus embedded REMOTE ASTs pass, shared exact C36e/Bash e560 and static
2907 hashes agree. Final delta uses the operator-owned private static input path.
Candidate restricts writes to existing RAM usr/bin, refuses different existing
helpers, verifies original8:2/8:0 identities, RAM device/live rescue/held shell and
nextroot absence. Approval covers RAM-only staging after publication, not a .37
transition or repair. Parent keeps .37 reachable while .211 recovery completes.
