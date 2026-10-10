# Independent task-only block guard review

Reviewed manager source checkpoint:
fec319edcf84e72f407713d2a77a645854c4a467,
[hardware-manager-20261010-block-check.c](https://github.com/mcoder1001-cyber/NGFW/blob/fec319edcf84e72f407713d2a77a645854c4a467/docs/status/tasks/hardware-manager-20261010-block-check.c).
This operational helper is not package/product code. Reviewer owns only this
documentation; no target command, mount or device write was performed.

Verdict: **APPROVE as an additional read-only exclusivity guard**, subject to
mounted-negative target validation and the separate namespace/reference/no-remount
requirements. This is not recovery/repair or hardware acceptance approval.

Source review: numeric major/minor parsing refuses invalid/out-of-range arguments;
lstat requires a direct block node with expected identity; open uses
O_RDONLY|O_EXCL|O_CLOEXEC|O_NOFOLLOW; fstat revalidates device identity after opening;
the descriptor closes before exit. There is no read/write/ioctl/mount operation.
EBUSY returns3; refusal/other errors return2; only a successful validated exclusive
open and close return0. Exit0 checks an exclusive kernel holder, not physical
drive health, data integrity or absence of every possible nonexclusive raw user.
Keep automounts and other device users excluded throughout diagnosis/repair.
[Linux open semantics](https://man7.org/linux/man-pages/man2/open.2.html).

The reviewer independently compiled the source in a private temporary directory
and removed only that reviewer-owned temporary directory on completion. Actual
command arguments:

```text
gcc -static -O2 -Wall -Wextra -Werror \
 /root/ngfw-wt/hardware-manager-20261010/docs/status/tasks/hardware-manager-20261010-block-check.c \
 -o <reviewer temporary directory>/block-check
independent compile exit: 0
independent binary bytes: 860432
independent binary SHA256: 02a2270ad8e0f480efc7464b26d2216fec8a915aa12755da6339015abed0f41c
matches supplied binary: True
file <reviewer temporary directory>/block-check
static ELF confirmed: True
```

hashlib.sha256 compared the fresh binary to the supplied private controller helper;
source content was compared to git show fec319ed:<published path> with equality.
These test invocations performed no block-device open:

```text
block-check                                        -> exit2 usage
block-check /dev/null 1 3                          -> exit2 nonblock refusal
block-check /dev/null oops 3                       -> exit2 usage
block-check /dev/null -1 3                         -> exit2 usage
block-check /dev/null 999999999999999999999999999999 3 -> exit2 usage
block-check /dev/ngfw-r7-nonexistent 8 2            -> exit2 missing-node refusal
```

Actual output for refusal cases was either usage or the explicit direct-device
refusal; every stdout was empty. Python subprocess assertions verified each exit
and output, and final line printed "no block-device operation, mount or target
command performed". Private helper contents were not committed to this branch.

Pending target evidence: exact helper must return3 on each mounted target,
then return0 only after real root users/mounts are removed. O_EXCL itself does not
write the device; neither success nor refusal permits bypassing the audited
offline-root gate. No target positive-case PASS is claimed.

## Independent isolated behavioral check

The reviewer ran five real behavioral cases on one newly allocated, reviewer-owned
8 MiB loop device backed by a temporary regular file in controller /dev/shm. No
physical disk or target was touched. The test ran inside a private mount namespace;
the exact owned backing-file identity was checked before testing and detach.
The supplied helper SHA256 above was verified immediately before execution.

Concrete setup and action arguments, executed through Python subprocess.run:

```text
mkfs.ext4 -q -F <owned RAM backing file>
unshare --mount --propagation private --fork python3 -c <reviewer assertion harness>
losetup --find --show <owned RAM backing file>
losetup --noheadings --output BACK-FILE <owned loop>
block-check <owned loop> <stat-derived major> <stat-derived minor>
mount -o nosuid,nodev <owned loop> <owned mount directory>
open(<owned mount directory>, O_RDONLY|O_DIRECTORY) # pins the mounted filesystem
umount --lazy <owned mount directory>
close(<owned directory fd>)
mount -o nosuid,nodev <owned loop> <owned mount directory>
umount <owned mount directory>
losetup --detach <same owned loop>
```

Assertions called the exact helper after each state change and compared exit and
output. Actual selected output:

```text
allocated reviewer-owned loop; private namespace; exact RAM backing identity verified
unmounted: exit0 EXCLUSIVE_OPEN_OK: no filesystem/block holder; also require namespace/reference audit
mounted: exit3 BUSY: block device still has an exclusive holder; no repair permitted
lazy-detached with directory fd still open: exit3 BUSY: block device still has an exclusive holder; no repair permitted
lazy-detached after final reference release: exit0 EXCLUSIVE_OPEN_OK: no filesystem/block holder; also require namespace/reference audit
ordinary successful unmount: exit0 EXCLUSIVE_OPEN_OK: no filesystem/block holder; also require namespace/reference audit
only reviewer-owned loop detached; private mount namespace exits
5 behavioral cases PASS; owned RAM backing removed; no physical or target device touched
```

The lazy-detached path no longer appeared mounted while the guard correctly
refused it. This checks the central failure mode of interpreting mount visibility
as proof of offline ext4. Finally cleanup closed the owned fd, removed only the
owned mount/loop/backing file and temporary directory; the process exited0. This
local result does not establish target-root exclusivity or approve target repair.
