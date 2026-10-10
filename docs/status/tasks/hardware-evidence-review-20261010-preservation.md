# Independent .211 preservation and targeted correction review

10:04UTC verdict: **APPROVE the targeted interactive correction phase** after
the parent's required refreshed full offline audit/final exclusive guard0 and
publication. Actual preservation and finite undo preparation passed as recorded
below. Correction completion, clean verification and return remain pending.
Reviewer performed no target operation and printed no backup payload.

The failed QCOW capture exited1 on corrupt extent inode259596 and retained0B;
it is not preservation. The plain native fallback is source-supported:
[e2image v1.47.2](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/misc/e2image.c)
dispatches the default format to write_image_file, writing the superblock/group
descriptors, inode tables and allocation bitmaps. The
[matching inode image implementation](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/lib/ext2fs/imager.c)
reads inode-table chunks without following damaged inode extents. The source
filesystem is opened read-only. The output must be a new, privately owned,
seekable regular RAM file: default output opens with truncation, so an existing
backup path must be refused. No -a/-r/-Q/-I or restoration operation is approved.

Independently hashed/parsed private host-211 receipts, all0600:

| Receipt | Bytes | SHA256 |
| --- | --- | --- |
| kernel-reader-runtime.json | 230252 | 4bf012eaeab19822c9d3e1fdb008e00da848099db818b219989f91193213b02b |
| offline-native-metadata.json | 230817 | 40aa67834bf9dd7e92551a217e22778f2c77f031ec699aa0842ac4961c341698 |
| native-metadata-transfer.json | 576 | 3d5fa0ed7cf06a342e3f8c4326a7d384cc868d2268bc169c4a20a6801f5d2a8c |
| debugfs-minimal-runtime.json | 1474 | 519686c329e98e72de9a438662827857740711756ee1fe4ac755eb141b09fdce |
| affected-inode-readonly.json | 4448 | 70964aa20cc7a214d9eb40207cb74384f444beee677c51599a2a41ab9a2c8164 |

Actual native command sections guard/image/hash/space/kernel all exited0.
e2image version banner is28 stderr bytes; it is not an error. Logical image
986808320B, allocated19720*512B, RAM8255084KiB free after capture. Captured SCSI
ioerr0xf before/after. Kernel snapshot storage lines match the pre-image baseline;
no new storage event was found. This establishes the bounded read's stability,
not complete physical health.

The controller archive root-before-repair.e2i.gz is1727953B/0600,
SHA25655ede2b7720d1e8a1e0d3e319857566824c24305d9106cef85ca54d554ec495e.
Transfer receipt records SSH/gzip0, empty stderr and file/directory fsync.
Reviewer independently streamed the entire gzip through Python gzip/hashlib:

```text
decompressed bytes986808320
SHA256b324a9c0ebefd45e2336b8a5bc946adc1a942ccd177eb940c709decee37c7a8d
exact source size/hash match: True
```

Native format omits external directory/extent/EA/journal blocks and user data.
It is not a complete filesystem/data backup. Private selected authentication,
boot and network backups remain distinct. Required supplement: three known
damaged external extent blocks15505493/15503361/15503362 and directory blocks
15503363/15503874, with block-size/geometry checks and actual offhost integrity.
Raw block contents must remain private. Additional unexpectedly affected metadata
requires review and preservation before accepting a destructive prompt.

BusyBox dd was unavailable (exit127), so no supplementary raw artifact was
created by that failed invocation. Narrow static helper source
hardware-211-20261010-raw-block-read.c
SHA2561a6e5a23215376f7daa60c090c5b6ea26709db8d73ec58df5f55aa337c83535c
independently APPROVE for the parent-authorized read-only capture. It opens only
/dev/sda2 O_RDONLY, requires block8:2 and exact BLKGETSIZE64=63510503424,
accepts only the five listed4096B blocks, requires full pread and checked output,
and has no device write. Verified15505494*4096=63510503424, including last-block
boundary15505493. Independent controller-only static build
-O2 -Wall -Wextra -Werror exited0/empty output and reproduced821424B binary
SHA2563256798046a2f81d1b06b3a1b5e93837c7da35a1bcc472dcc095c8330f6a1ea2.
Seven missing/extra/invalid/out-of-range/nonallowlisted argument checks all refused2
with empty stdout/stderr, before device open. No valid-block device call was
executed by the reviewer; owned temporary compile resources were cleaned.
Actual target capture/offhost verification follows below; the source approval
and its earlier pending state remain distinct.

The narrow missing-path-only debugfs/libss RAM runtime actually passed loader
resolution and LD_BIND_NOW=1 version checking; both member hashes match the
reviewed archive. Existing libraries were not replaced. Read-only stat/ncheck
sections all exited0 and establish:

| Inodes | Type and affected area | External block(s) |
| --- | --- | --- |
| 259596/259597/259598 | regular0640, each8MiB, system/user journal files | 15505493/15503361/15503362 |
| 259599 | directory0700, /root/.cache,4096B | 15503363 |
| 259602 | directory0700, /root/.config,4096B | 15503874 |

No inference that all later errors affect only these objects is justified: the
read-only fsck aborted in pass2. Damaged extent removal can lose these journal
file contents; directory salvage may drop malformed entries. Known original
authentication/management/boot snapshots address the return path, not every
user file under these directories.

Source-supported proposed targeted interactive command:

```text
LC_ALL=C e2fsck -f -E fixes_only,nodiscard -z <new private bounded RAM undo> /dev/sda2
```

[Matching e2fsck documentation](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/e2fsck.8.in)
defines fixes_only to skip optimization and -z to save overwritten block contents;
undo does not protect against power/system crash. Interactive answers must be
explicit y/n per reviewed prompt, never a/all/default-Enter or -y/-p/-D.
The incidental optimization259816 is excluded. Actual new undo-path identity,
space/write preflight and a finite file-size bound are required. The
[matching undo writer](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/lib/ext2fs/undo_io.c)
returns a failed backup write before the corresponding real block write; this
does not roll back previously accepted repairs. Preserve actual undo/transcript
offhost before any normal return. Stop on unexpected user data, failed reads,
reset/media errors, undo failure/capacity threshold or loss of management.

## Actual supplemental preservation, health and finite undo

Independently hashed/parsed additional private0600 receipts:

| Receipt | Bytes | SHA256 |
| --- | --- | --- |
| raw-metadata-capture-command.json | 969 | 61d90dce1bf58d9b4f72569995850bc1363b18e5c64386211ab3552c3f545a5c |
| raw-metadata-supplement.json | 2141 | 859d8ac2a83a4f12dfc47cd9ece75e449ca59633483ce70ceef1106944713b0f |
| post-image-health.json | 267681 | 775c972f99530617214f1c91996aa95679ad609302c7b1a5ca061ba70926b80e |
| post-supplement-health.json | 230138 | 7fde03b91343c41f11c88b1cbc892b24f74cb5fd07a3198d863713336b33a803 |
| undo-preflight.json, superseded4GiB preparation | 526 | d76eb281b969848b710cfc56fede22318f43f590975b28cdcf5bec8157fae242 |
| undo-preflight-1g.json, final preparation | 692 | 18059dfd050ab8bfe15fd1f8f3e243beeb0197bd31cabf5f35d941fbd654cefe |

Capture helper digest/guard/command all passed. All five persistent controller
raw-metadata-block-<number>.bin files are4096B/0600, independently hashed against
their RAM source digests:
SHA256ad7facb2586fc6e966c004d7d1d16b024f5805ff7cb47c7a85dabd8b48892ca7.
All five are already zeroed. The saved originals therefore explain the damaged
headers/directories; they do not contain recoverable intact original structures.
No block payload was printed.

Post-image health shows exact SMART query15->18 before the raw-capture timestamp.
Media attributes5/179/181/182/183/187 remain0, CRC199 remains1003, SMART exit0.
Post-supplement counter remains18. Reviewer extracted kernel snapshot lines and
compared all2281 lines against the native-image baseline: exact equality True.
The increased query-associated counter is not misrepresented as raw-read failure
or as a blanket exemption for later storage errors.

Final repair-wrapper.sh source
SHA25661e40215461de05f5a4c3db4d20e54b122551d3af53fe1e136b5abd15fe89422
refuses an existing undo/probe path, requires private700 RAM device46, establishes
LC_ALL=C/umask077, checks space and uses explicit fixes_only,nodiscard/-z options.
Actual final preflight establishes:

```text
exit0, stderr0
RLIMIT_FSIZE hard=soft1073741824bytes, inherited through exec
owned1MiB write + sync +0600 check + unlink: PASS
undo absent; parent RAM46/700/uid0/gid0
RAM8254240KiB available
controller actual root free2707152896B, exceeds1GiB+512MiB reserve
exact static guard: exclusive exit0
```

The earlier4GiB preflight passed in RAM but exceeded the newly observed controller
offhost free space; it was superseded before any repair. The final1GiB cap fits
both actual locations. Controller PTY driver fsck-session.py
SHA2560186e5abf5862bb6f3d7689d8aa2c6fc11d40412a3e39a0aa358588fd03df752
checks private path/capacity, refuses an existing transcript, fsyncs received bytes,
and sends only individually provided y/n or interrupt. It prints prompt markers
instead of the full private transcript and has no automatic answers. Independent
Bash syntax check exited0/empty; driver AST parsed successfully. Reviewer did not
execute either repair entry point.

## Scoped write verdict

APPROVE under the parent's already issued conditional release, after refreshed
complete offline audit and final exclusive guard0: run the exact reviewed wrapper
interactively and individually accept known journal259596/259597/259598 invalid
extent clears and consequent accounting fixes; known cache259599/config259602
directory salvage/checksum fixes. Matching
[pass1 implementation](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/pass1.c)
deletes the invalid extent reference and updates its parents; this can lose that
branch's journal data. Matching
[directory salvage](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/pass2.c)
can remove remaining malformed entries, including the entire remainder at offset0.
The operator must retain each exact prompt/answer in the private transcript.

Unexpected objects, newly exposed user-data changes, or unrelated optimization
remain held for concrete classification. No -y/-p/-D, all/default answers or
automatic prompt acceptance. Stop actual read/reset/media/CRC changes, undo
failure/capacity threshold or management failure. A size-bound abort does not
roll back already accepted changes. Preserve undo/transcript offhost with actual
size/hash verification before normal return; repeat offline read-only fsck until
the complete check is clean and review boot/auth/network integrity separately.
This verdict is not completed repair, return approval, .37 transition approval,
package installation or hardware acceptance.

Next exact reviewer action: inspect the refreshed offline audit and private first
repair prompts/answers, then actual completed undo/clean-check evidence.
