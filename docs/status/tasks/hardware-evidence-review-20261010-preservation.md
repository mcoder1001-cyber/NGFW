# Independent .211 preservation and targeted correction review

10:04UTC verdict, with10:11 input correction below: **APPROVE the targeted interactive correction phase** after
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
instead of the full private transcript. The initial review wrongly concluded its
newline-terminated answers could not accept another default; actual behavior and
the corrected one-byte input path are recorded below. Independent
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

## Actual input bug and scoped live recovery

Fresh pre-correction-offline-audit.json independently parsed24506B/0600,
SHA256cbfebefb4c3c1f6dd6f5a0974ae81a84c8f391bc64390fd40012109d06fecd20:
exit0/stderr0,262 processes/93FDs/nsfs0/races0/failures0, final exclusive0.
Repair then began. The private transcript showed known journal extent/accounting
and cache directory reconstruction answers, but they were not all individually
sent: old driver0186 wrote y plus newline. The newline accepted the next default.
The reviewer missed this interaction when approving that input path.

[Matching ask_yn implementation](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/util.c)
clears ICANON/ECHO, reads one character at a time and accepts newline as the
default answer. Worker identified the bug before any unknown object was accepted;
input was held at known config259602 missing-dot repair. The original driver
remains private capture-only. Its answer entry point must not be used again.
Known directories' missing '.' and '..' metadata reconstruction is in scope,
with actual valid /root parent linkage checked by pass3.

Exact future driver source
SHA256dc3219514cc7be54a07d83120b704811dfe3d8c6a0566a06c158ac545ab939d7
replaces answer-plus-newline with answer.encode(), one byte. This does not change
the already running driver's code. Narrow live helper fsck-single-byte.py
SHA256e6b6d00e27f6c86ba53679c33b0f708c4d02fa133458738dc0afce6584de34e0
independently APPROVE for this task's existing owned controller process only:
unique exact driver argv/cwd/root UID/Python executable/open private transcript;
unique exact SSH child argv/PPID/root UID/executable/start time; writer FIFO
device/inode matching the child's stdin; O_WRONLY|CLOEXEC|NONBLOCK own descriptor,
fstat and process/pipe identity rechecks; exactly one y/n byte and no newline.
Only the helper's own opened descriptor is closed. No PID1 or arbitrary process
descriptor manipulation, driver restart or live fsck interruption is requested.

Independent AST parsing of both exact sources passed. Literal cmdline separators
were independently evaluated as single NUL bytes, avoiding escaped-output ambiguity.
Actual initial private probe ledger722B/0600
SHA256162de6b45b9c177fa254baa32e189e86668836e7ea51d85fab314a6aeaca8e4d
records0 writes, driver3850996/SSH3851087, FIFO21573919 and newlineFalse.
Worker published/read back87cc97e7be2ff7fa1e3cb8725f43a01c13d5b0bb before
resuming known-scope single-byte answers. Preserve the old implicit responses;
do not relabel them as individual manual answers.

For concrete newly exposed orphans, matching
[pass3 reconnect](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/pass3.c)
links an existing inode under lost+found/#<inode> and adjusts its reference count;
directory reconnect also repairs '..'.
[Pass4](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/pass4.c)
uses that preservation path and repairs counted link references. Classify the
actual inode/type/path before accepting a new object's prompt. Unknown no-block
inode Clear is destructive and remains unapproved; declining it can lead to a
preserving Connect choice. No arbitrary complete file-data image prerequisite is
added to the owner's authorized logical repair.

Next exact reviewer action: inspect actual one-byte input outcomes and classify
any newly exposed orphan prompt, then actual completed undo/clean-check evidence.

## Newly exposed directory259603

Actual prompt is held, not answered: directory259603 block0/offset0 Salvage.
Private new-inode-259603-readonly.json1120B/0600
SHA2564a2409d4de11c01a88109c9526b9c9802444dfcc433823656d9499e7def6fd04
independently parsed: root-owned directory0700,4096B, links2, one block15503875.
Ncheck exit0 emitted directory-checksum diagnostics and returned no pathname;
it does not establish an original name or a successful complete scan.
Original inode metadata is already in the native preservation file. Parent
authorizes this additional block's private read-only preservation before deciding
whether to rebuild the directory.

Exact raw reader v2 source
SHA256c5c0649f6150b869042e8890977bc84f5d2e8f18f736451cfe833cd0e5355216
adds only15503875 to the prior allowlist. Independent controller-only static
warning-clean build reproduces821424B
SHA256050316c810573137bd76d313415d9e5877d5dd92f841cdbcbbc09d671905d4e5;
six invalid/nonallowlisted/missing/extra argument checks refuse2/empty output.
No valid block call was executed by the reviewer; temporary resources cleaned.
Focused RAM staging/read-only capture APPROVE after operator publication and
the parent's live paused-fsck identity/no-other-writer/unchanged-prompt checks.
The still-offline device is now held by the known live fsck: its self-held BUSY
is expected, so an exclusive0 prerequisite must not be falsely imposed here.
No mount, other device writer, fsck restart or repair-scope expansion is approved.
Actual new block/offhost integrity/classification and salvage verdict were pending
at that source checkpoint; the subsequent actual check follows.

Private new-directory-259603-supplement.json230597B/0600
SHA256e8ab1ad9b9ad517105e9a6671f8712b32968eddf147199b1fbafc547373eab83
independently parsed; persistent offhost root-before-block-15503875.bin4096B/0600
has exact source SHA256ad7facb2586fc6e966c004d7d1d16b024f5805ff7cb47c7a85dabd8b48892ca7,
and independent byte classification confirms it is zeroed. Capture/transfer0,
source_hash_matchesTrue, fsync recorded by operator. Kernel snapshot independently
exactly equals the previous snapshot; counter remains18. Initial holder preflight
f4443c20522535b33466e8c7efd7fdbe64c1b92f74b12e2ea7c0f1c8c86a5692
reported the sole known device writer, paused e2fsck13880 fd3/access2/exact command,
but exited1 on an incorrect sysfs health path. Preserve that failed invocation.
Retry180863d8aa73a91bde3c56cb3403b200b652f02b04d0409679d2388f954460d0
uses the actual /sys/block/sda/device/ioerr_cnt path, exit0/counter18, and records
the unchanged held259603 prompt.

Specific259603 salvage/self-dot reconstruction APPROVE after this preservation.
Matching pass2 check_dotdot deliberately installs temporary EXT2_ROOT_INO when
the parent is unknown; accepting that missing-'..' placeholder is APPROVE to
reach pass3, which must verify/reparent the actual relationship. It is not an
assertion of an original path or parent. Preserving lost+found Connect and
consequent counts for this concretely classified directory are approved if offered.
Unknown destructive inode clear remains excluded. Operator continued using
explicit one-byte helper answers and then held the next new object.

## Newly exposed home-user cache259594

New-inode-259594-readonly.json862B/0600
SHA2567e4dde9cf610e5991da264a3cc9ab4e6ecbb307972d979d5fb58013ec968b96d
independently classified: UID/GID1000 directory0700,4096B, links2, one block15505492;
successful ncheck identifies a home-user cache directory. Its full name remains
private. Actual salvage prompt is held pending this block's preservation.
Parent authorizes a narrow read-only capture under the same paused-writer checks.

Source v3 SHA256b76411e2c3279fb3b7dc0fdfdc4f64c510491fe585e7a0ad00c0eb3d07facda9
APPROVE for this scoped capture after publication203ea2a84bf410117ef5a2b354388c0be98d4dd0.
Independent immutable8d09a644->203ea2a8 diff changes only15505492 allowlist insertion.
Independent static warning-clean build reproduces821424B
SHA256d8d8aa8316e7fff7bc568d44979e70e66f12742d1a826c6111d6f109fad1c80e;
six invalid/missing/extra/out-of-range/nonallowlisted argument refusals2/empty.
No valid target block execution by reviewer. Actual new block/offhost/health and
specific259594 correction verdict follow.

New-directory-259594-supplement.json460615B/0600
SHA256a281c91e2aeda18797a5b218f117fae791004520a426e1266f0cf13f5bec0d2d
independently parsed: known paused fsck13880 fd3/access2 sole writer,
preflight/stage/capture/transfer0, exact v3 source/binary digests, source/offhost
4096B/0600/ad7facb2 match. Persistent root-before-block-15505492.bin independently
hashed and classifiedzero. Kernel before/after independently exact equal True,
counter18 before/after; fsync recorded by operator. Specific home-usercache259594
salvage/self-dot/temporary-'..' reconstruction APPROVE, followed by actual pass3
parent verification. Original entries in this block are already lost, not intact
recoverable data erased by a new approval. New unknown objects remain held.
For concretely classified valid regular-file orphans, preserving Connect plus
consequent counts can be assessed from actual inode/type/size/ownership and native
original metadata even when an original name is lost; no unavailable full-data
backup is imposed. No blanket approval for unknown destructive Clear/special types.

## Separate .37 actual preparation

Preservation-stage.py47da77ec source independently AST2/2 approved RAM-only.
Actual private ram-preservation-readers-stage-20261010.json5315B/0600
SHA256847e1527df8335d2ee40faf786d7051b7b315823bf555c464fc6548666ccc167
independently parsed: exact static readers and existing debugfs/libss digests,
all four loader checks0, version commands0 with65/76B banners, invalid selectors2,
mounted guard3, originalroot8:2/RAM51/MainPID3866/held3940/nextroot absent unchanged.
No valid block/kernel capture or .37 transition occurred.
The .37 copied future driver5f45bf37db7d5e066165b07f66bbbe55a4e383aadd3f483525ba50222e3ed973
matches fixeddc321951 with only privatefolder/IP changes, AST PASS.
Wrapper0c76f5cba2cce5f088cb3485abee0ca45ed1fa2d0cfdae6091be2792c4927e8c
matches61e40215 with only actual RAM46->51 checks/status, Bash-n0. Source preparation
APPROVE; a mounted-root RAM preflight must stop at expected self-independent
BUSY3 after its finite limit/probe checks, not claim full offline readiness.
Missing-tool stage source f8551f064633014b55ad10ef7b2ac2045a0b17479b97f4633a595d803ac1479a
independently APPROVE RAM-only after publication: outer/REMOTE AST2PASS, actual
ldd regular-expression semantics checked, only missing df/tail/sha256sum with
flattened installed-MD5-verified closure, different existing files refused and
RAM51 parent checks. Exact wrapper/v2 bytes, expected mounted preflightBUSY3,
probe/undo absence, held/MainPID/nextroot invariants are checked. Actual runtime
outcome remains pending. Root holds .37 transition
until .211 normal return; neither copied source approves a .37 repair.

## Actual reference accounting and classified regular-file reconnection

Actual Pass3 corrected the reconstructed home-user cache259594 parent to31,
root cache259599/config259602 parents to152, and reconnected classified directory
259603 to lost+found. The transcript then offered root inode2 reference count
14->18, parent31 5->4 and parent152 9->7. These consequential metadata corrections
are APPROVE: matching [pass4](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/pass4.c)
PR_4_BAD_REF_COUNT assigns the counted i_links_count and writes the inode; this
operation does not clear that inode or its contents. Original inode metadata and
the bounded active undo cover these approved metadata writes.

Private new-inode-259595-readonly.json813 B/0600
SHA256c06cbc16a58fe547a55814894570d590feccd79d646d3fd9c3589b714456193f
independently parsed: regular0644, UID/GID1000, size0, links1, blockcount0,
no extents or recovered original pathname. The operator correctly declined
zero-length Clear. Preserving Connect to lost+found and its counted-link fix
are APPROVE for this actual inode, using individual one-byte answers.

Private new-inode-259600-readonly.json1496 B/0600
SHA256de64762841163fa25daed98938aaa99a3068234d53e6eff22e9f92c75aa1b6c3
independently parsed: all three queries exit0, regular0644/root-owned, size0,
links1/blockcount0/no extents. Current and original-native-image stat outputs
are independently byte-identical. Clear was declined; preserving Connect and
consequent link-count correction are APPROVE under the same classified regular
file scope. Unknown destructive clear or special-type changes remain excluded.

Matching [pass4 disconnect_inode](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/pass4.c)
offers empty-inode deletion separately from reconnection, while
[pass3 e2fsck_reconnect_file](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/pass3.c)
links the existing inode beneath lost+found and updates reference accounting.
This preserves the classified inode without claiming its lost original name.

An actual intermediate transcript snapshot2521 B/0600
SHA256325e6a56af76ba38b7c14d26d01febe2d6bc6db811a068345129c877ba710803
corroborates root/parent corrections,259595 Clear-no/Connect-yes/count-yes and
259600 Clear-no followed by its held Connect prompt. This is a snapshot of an
unfinished fsck, not a final transcript hash or successful clean-check receipt.
All previous known-scope implicit default responses from the old newline driver
remain recorded; the old process is now capture-only. Subsequent individual
inputs use the verified controller one-byte helper.

The .37 unexecuted f855 source was superseded by
SHA2568cc54c278d7c08848934265ee60d98f0253334519fa3c258850bb1eafc949247.
Independent actual-byte hash and outer/REMOTE AST2 PASS; its focused change moves
mandatory installed package MD5 verification before executable ldd inspection.
RAM-only preparation applicability APPROVE after durable publication. Actual .37
runtime outcome, .211 completed repair, clean readonly check, offhost undo and
normal return remain pending; no hardware installation/acceptance is inferred.

## Consequential allocation-map accounting

Actual intermediate Pass5 transcript2880 B/0600
SHA256ed70b5cc10353b12fc2e2e2d7350ea78e650eea234b3411d77b2830a526ca0a6
independently corroborates the held block-bitmap differences:

```text
-(365920--367966) -(370653--371567) -(378770--379334)
-(15503360--15503362) -15503872 -15504384 -15505493
Fix<y>?
```

Matching [pass5 check_block_bitmaps](https://raw.githubusercontent.com/tytso/e2fsprogs/v1.47.2/e2fsck/pass5.c)
replaces the on-disk allocation map with block_found_map constructed from the
preceding passes, marks it dirty and recomputes group/global free counts. Under
the actual nodiscard option this correction does not discard those data blocks.
These allocation-map/count fixes consequent to the already approved damaged
journal extent-branch clears are APPROVE with active bounded undo and individual
one-byte answers; the classified disconnected inodes remain reconnected. This
does not approve clearing an additional inode or claim the removed journal
branches' original contents can be restored. Actual final fsck/undo/offhost,
complete clean readonly pass and normal return remain separately pending.

## Actual .211 completed logical correction and clean validation — 10:37 UTC

Scoped correction and subsequent read-only validation PASS; normal return remains
held for the separately released original-root/EFI integrity inspection. Private
repair-final-preservation.json1052 B/0600
SHA2567f15a9f0fe29b1735b6d0d4253fe20382d263fe47cb191d23349242fbe071c99
records corrective fsck exit1 (modified), clean readonly fsck exit0, offhost
transfer0 and source/offhost undo hash equality. Reviewer independently read the
three persistent regular files, verified0600 and exact bytes/hash:

| Artifact | Bytes | SHA256 |
| --- | ---: | --- |
| root-repair.undo | 749568 | 92df3461b98f30de5f52d308f99e152bba598476cda02d10d055726c2a92b477 |
| repair-interactive-transcript.raw | 3270 | 790f88f2c37f4dad36612ca8912da4ef45a006c92b1b0e4dc2c0f99fac415799 |
| repair-single-byte-answers.jsonl | 20099 | 716dc358b4f824379290ae5c3d2e2a2ba507fe67564418e334f9a9d252cd70c9 |

Actual transcript has37 answered prompts:35yes/2no. The two zero-length regular
Clear prompts were declined; three approved damaged-journal extent Clear prompts
were accepted. The driver bug caused five queued-newline implicit defaults after
five initial old-driver y inputs; these are preserved and not described as
manually answered. Independent ledger parsing confirms one0-write probe plus27
explicit answers (25y/2n), every answer exactly one byte with newline_writtenFalse.
All transcript inode IDs are known/classified2,31,152,259594/5/6/7/8/9,259600/2/3.
Pass5 freed-block ranges total3533, exactly matching group11 increase3527 plus
group473 increase6 and the global free-count increase3533.

Private post-repair-clean-health.json461056 B/0600
SHA256bb09796a8d02333d5f1f3f9859e6a3f38186bf4c93dc977cfc064ec94dc16906
independently parsed: exclusive guard0; full e2fsck-f-n exit0 traverses all five
passes, optional narrowing259597/259816 declined, only27 B version banner on
stderr. It reports29655/3845088 files and4268770/15505494 blocks. Counter before
and after is0x12 (18); reviewer independently compares actual kernel strings
before==after==mid-repairTrue. This sample supports no new observed storage fault,
not a whole-drive health guarantee.

Private post-repair-offline-audit.json24401 B/0600
SHA2562b3aa09b0ce690601581be10aff8cd66c001972e92e0d614c6c6e647bcaa4b0c
independently parsed: SSH0/stderr0,260processes/93FDs/nsfs0/races0/failures0,
final exclusive guard0 after helpers. Actual fsync/transfer assertions are in the
operator receipt; reviewer directly corroborates persistent offhost contents and
hashes. Native image, supplemental blocks and undo remain scoped metadata
preservation; no full user-data or power-failure recovery claim is made.

Current verdict: APPROVE completed scoped logical correction/clean validation.
Actual selected original-root/EFI hashes, private auth/config integrity, matching
RAM shutdown closure/manager viability and post-inspection offline predicate are
still required for the separate normal-return verdict. Target packages/NIC
binding/forwarding acceptance have not started. Reviewer performed no target action.

## Actual .37 finite undo tool preparation

Private ram-finite-undo-tools-stage-20261010.json7525 B/0600
SHA25655b5f250d9fc7ce3bd04c6f287d8734463e417e0e10eea6b3863f9d4bcb16e64
independently parsed:17 source-package MD5 closure entries verified; missing
df/tail/sha256sum runtime version checks0; invalid V2 selector2/empty. Wrapper
preflight3 records actual hard/soft1073741824 B limits,1MiB600 probe+sync and
expected mounted-device BUSY. Inner Bash locale warning97 B is retained; outer
stderr0 does not conceal it. Undo/probe absent, originalroot8:2/RAM51,
MainPID3866/held3940/nextroot absence unchanged; no repair/raw capture.

Exact separate V3 RAM-only staging wrapper
SHA256f505722458752fcdac8ed003ff09126df743fadae15466a28b72f548289ab146
independently actual-byte hashed and outer/REMOTE AST2 PASS. Scoped source APPROVE
after publication: exact reviewed V3 bytes under a separate RAM path with differing
existing-file refusal; invalid selector2, mounted guard3 and runtime identity
checks only. Valid raw capture, .37 transition and correction remain unreleased
until the parent's separate phase release after .211 normal return.

## Further .37 prepared-only review

Actual ram-v3-reader-stage-20261010.json860 B/0600
SHA2560fd8ebb3789d10eac379b6edd3e3b197517f793c5b2eea1c3e72d079c085aeaf
independently parsed: exact V3static821424 B/d8d8aa83, invalidselector2/empty,
mountedguard3, originalroot2050/RAM51/MainPID3866/held3940/nextrootabsence
unchanged; valid_block_captureFalse/repair_startedFalse. Published operator
checkpoint24a5859daa3f89e1330eec9ef75a12a80c05fe1f; this remains RAM preparation.

Future package-input.py
SHA256d5294674a9eede245900d54240c58ce8c88ed8e96282b39e19ab0532c1c21def
source-only scoped APPROVE after publicationa04409b860487aa0e26473f31675d80ef996ce74.
Reviewer independently hashes actual bytes and outer/PREFLIGHT/SIMULATE AST3 PASS;
POLICY literal19 B has two newline bytes and no escaped backslash-n. Future target
mode guards originalroot, absent RAM/nextroot, clean state/trusted time, management
route/PCI/driver, exact root-owned101start policy and persistent inactive VPP mask.
Exact11 archive hashes/controls and RAM upload checks precede apt simulation
-s/--no-remove/--no-install-recommends with disabled package-cache outputs. No
install, service activation or NIC binding is performed by this source. All actual
Inst/Remv output needs manager assessment; critical-package regex is an aid, not
an exhaustive safety proof. Target mode is not authorized during recovery.

Future offline-preserve.py
SHA256bdb2a28a5235142dc60c3ce72f2577acdee7ead4667f19b3733c22acef4c4301
source-only applicability APPROVE after publication and parent's phase release.
Actual-byte hash and outer/IDENTITY/REMOTE/STREAM AST4 PASS; full source inspected.
Only audit, readonly-f-n capture, plain native plus seven allowlisted raw blocks,
actual inode classification and private offhost size/hash/fsync/readback are
included. ExactRAM51/PID1/master3866/held3940/device8:2/geometry/tool digests,
fresh full audits/guard0, RAM/controller budgets, new-file refusal and actual
kernel/counter health comparison are enforced. Diagnostic4/12 is capture, not
filesystem acceptance. No correction/mount/reboot/SMART action. Existing .37 stage
copies Python/full stdlib/extensions; operator asked to prove actual -B imports
of required modules before crossing. These are source reviews, not peer facts
substituted for actual .37 preservation or transition outcomes.
