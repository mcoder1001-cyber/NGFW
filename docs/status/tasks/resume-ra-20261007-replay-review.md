# One-shot RA diagnostic replay proposal — NOT EXECUTED

R2 inspects the frozen proof diagnostics at `f3ae748601acfc768661c7503cedd0aa2c7092e5` plus supplemental timing changes through product freeze `eaa476a82083a1ebb160d5b66049a3d3c4fc80a7`, tree `24a64e66c76637cb89d9efadde1d6e7bba7ae12a`. Manager authorization after R2 permits ONE finite isolated diagnostic boot, not acceptance, repeated experiments or deadline changes. Subsequent documentation-only commits do not change this product freeze.

All new artifacts belong to newly allocated `/root/ngfw-ra-replay-20261007-AUVylC`. Its `source` is a clean detached build checkout of the exact product freeze, not a developer worktree. Existing `/root/rct`, `/root/ngfw-source-vm-20261005`, host units and shared VPP are read-only inputs. Old builder MUST NOT run: it overwrites old root/probe/initramfs paths and copies fixed producer21 binaries, so merely changing its SOURCE_REF would not deliver current diagnostics.

Preserved inputs inspected:

| Input | SHA256 / provenance |
| --- | --- |
| Boot38 raw console | `ea98ea4708bae6e59f49f1e865b444751cd937df6724e0b5cd366ed2a48c21b2` |
| `/root/ngfw-source-vm-20261005/initramfs-boot38.gz` | `87efa2ed9017cf93be4cefd71e01cfe8eafaee277a8246a60370c1a1f11df1f4` |
| Preserved `build_vm21_receipt_corrected_console.py` | `ee539871014c237b8dbabdfc983ed4e72f2e287fa98ce0c6f5c4baa2a39cf4a3` |
| `/root/rct/ra-default-vm21-receipt.json` | `330593cda7a982fc48481f89f0f345b8548db89e346172f306af0d7329687c22` |
| Producer21 source | local `237cf1cbdb3c994a5d0f27947f7d5fa7ae1fd63e`, published `711e18e12ac2776ad4fc15066f811b60599e63f2`, tree `6db582df113585954acc6bfc4a2255ef1f72c0b6` |
| Original agent / broker / daemon ELF | `aa68853ac669a4865260502012ca61fda75508108e4f4bbded3aa9a253a6dd70` / `da41bcb13797fefcb6149ba2ac6dc277c171acc95f9f503c10d3a891286e863b` / `1404691850915327e6728888fbc117219ba3134d213535f80352955f442ca351` |
| `/boot/vmlinuz-7.0.0-31-generic` | `19fd789cd5e6b4adfe18c9c6b92a1ce8c8b2eef6ff88456cef80ee2dca9b3f72` |
| Reviewed offline archive transformer at freeze | `984e4b1bfb78efedc7ab15549ba28a6cba8608c9d9195b5975a0f1ca04ff24af` |

The archive transformer parses the pinned standard newc archive without extracting guest paths. It updates exactly agent/broker/daemon ELF payloads and the two installed helper digest receipts; it reparses and checks every other payload AND metadata unchanged, preserving all original units, hardening, coordinator, systemd/VPP libraries, probes and guest config. New output uses exclusive creation; no old evidence overwritten. Inspect-only execution already verified original ELF/receipt pins. Current rebuilt binary/image pins are recorded below after offline preparation.

Offline preparation COMPLETE (no VM): all three actual CGO0 builds exit0, each embedded VCS revision equals the product freeze with modified=false (Go1.26.0). Agent size52404039; broker15831202; daemon15487138 bytes. Archive transform exit0; original helper receipts validated, output reparsed and every unowned entry/metadata compared equal. Output compressed114172192 bytes. Original nine RA units, agent unit and hardening also have empty git diff between producer21 published711e and current product freeze.

| New owned artifact under AUVylC | SHA256 |
| --- | --- |
| `binaries/ngfw-agent` | `ed5c269f8ed252c5a98819abc155478f3eb9e3efa9c7f3f2d635c996be4a1a5c` |
| `binaries/ngfw-ra-namespace-broker` | `d9b8d62d897cc3d2a36af6c829ade84fd86fb267c434a75495077ec21e0e5a5a` |
| `binaries/ngfw-ra-daemon` | `c04217a1b224d8ac5375de138ec7b19c98982bd92298fe87b73d5f5c4a06acec` |
| `initramfs-diagnostic.gz` | `962a135153fad17cbfeeb9c9cbd5e91f42e7da630443552b3f16631f972bf1be` |
| `vmlinuz` | `19fd789cd5e6b4adfe18c9c6b92a1ce8c8b2eef6ff88456cef80ee2dca9b3f72` |

Actual offline assembly command (already completed once, cannot overwrite output):

```sh
python3 /root/ngfw-wt/resume-ra-20261007/docs/status/tasks/resume-ra-20261007-replay-image.py \
  /root/ngfw-source-vm-20261005/initramfs-boot38.gz \
  --binaries /root/ngfw-ra-replay-20261007-AUVylC/binaries \
  --output /root/ngfw-ra-replay-20261007-AUVylC/initramfs-diagnostic.gz
```

All source, binaries, cache, tmp, image and copied kernel now exist in new scratch; `boot-diagnostic.log`, replay.lock and any guest result do NOT exist yet. Host snapshot after build was15816MiB available RAM and16GiB free disk before image assembly; do not substitute that snapshot for a fresh launch check.

Build recipe in `source/apps/agent`: CGO_ENABLED=0, GOMAXPROCS=2, Go `-p 2 -trimpath -buildvcs=true`; agent retains debug symbols, broker/daemon use `-ldflags='-s -w'` as producer21. All writable GOCACHE/TMPDIR/GOTMPDIR paths are new `cache`/`tmp` beneath AUVylC. Source cleanliness and exact SHA checked before build. Dependency module cache/toolchain are existing inputs; no packages installed. Build pin receipt must include actual `go version -m` VCS result, not assume embedded metadata from requested flags.

Boot38 environment evidence is Linux7.0.0-31, QEMU TCG, four CPUs and about2GiB physical guest RAM. Exact historical host QEMU argv was not preserved in inspected builder/receipt/raw console; do not falsely claim recovery of its accelerator thread flags. The proposed explicit argv below uses qemu64, four CPUs,2048MiB and TCG multi-threading, with zero networking, host mounts, disk drives, monitor or KVM. These are proposed launch flags for manager review, not a recovered command. Current installed QEMU is10.2.1. On preparation inspection host MemAvailable was15726MiB and free disk19GiB before clone/cache creation; refresh before launch. Reserve at least4GiB available host RAM (2GiB guest plus emulator/headroom),4 logical host CPUs and8GiB free disk; at most one VM, outer240s plus5s forced-stop grace. Original guest fixture TimeoutStartSec240 remains unchanged. Capacity shortages mean no launch, not host tuning.

Proposed actual launch command, ONLY after R2 and manager authorization and exact pin/capacity checks:

```sh
set -eu
exec 9>/root/ngfw-ra-replay-20261007-AUVylC/replay.lock
flock -n 9
test ! -e /root/ngfw-ra-replay-20261007-AUVylC/boot-diagnostic.log
sha256sum -c <<'PINS'
962a135153fad17cbfeeb9c9cbd5e91f42e7da630443552b3f16631f972bf1be  /root/ngfw-ra-replay-20261007-AUVylC/initramfs-diagnostic.gz
19fd789cd5e6b4adfe18c9c6b92a1ce8c8b2eef6ff88456cef80ee2dca9b3f72  /root/ngfw-ra-replay-20261007-AUVylC/vmlinuz
PINS
timeout --signal=TERM --kill-after=5s 240s /usr/bin/qemu-system-x86_64 \
  -machine pc -accel tcg,thread=multi -cpu qemu64 -smp 4 -m 2048 \
  -kernel /root/ngfw-ra-replay-20261007-AUVylC/vmlinuz \
  -initrd /root/ngfw-ra-replay-20261007-AUVylC/initramfs-diagnostic.gz \
  -append 'console=ttyS0 rdinit=/init systemd.unit=default.target systemd.log_target=console systemd.log_level=info panic=-1' \
  -display none -monitor none -serial file:/root/ngfw-ra-replay-20261007-AUVylC/boot-diagnostic.log \
  -nic none -no-reboot
```

The private replay lock serializes launches; the log-absence check prevents reuse, and set-e refuses an existing log. Preserve actual exit status (including124 timeout), image/kernel SHA, current host resource snapshot and console SHA in a NEW AUVylC receipt. Kill only the spawned QEMU process if outer timeout must terminate it; no pattern-based cleanup. Guest coordinator retains original private-VPP stop/poweroff. Guest VPP is inside this private RAM filesystem; it cannot reach the host VPP socket. No KVM/security/host unit/sysctl/package changes.

Exact Boot38 client-clock decomposition (all times relative to client entry):

| Interval | Milliseconds | Code boundary |
| --- | ---: | --- |
|0→1952|1952|new held installation proof, including initial helper hash|
|1952→2909|957|source trust and canonical reference|
|2909→3001|92|preflight manager validation|
|3001→3050|49|probe Verify/request/parent/socket preparation|
|3050→3058|8|socket timeout setup and connect|
|3058→10492|7434|SO_PEERCRED check and receive validation-ready|
|10492→10979|487|decode/source executable and active helper manager validation|
|10979→10997|18|READY ACK, IPC bound, marshal and probe send|
|10997→13871|2874|probe reply, validation/ACK/final Verify, helper exit wait, publish preparation; previously unpartitioned|
|13871→13872|1|second socket bound and connect|
|13872→19565|5693|SO_PEERCRED check and second validation-ready receive|
|19565→20352|787|second READY/source executable/active helper validation|
|20352→20357|5|second READY ACK, IPC bound, marshal and publish send|

Sum20357ms. The two credential-check/READY-receive intervals sum13127ms; they locate64.49% of pre-publication wall time, but do not prove that SO_PEERCRED, a particular server primitive or scheduler delay dominates. Server stage3→4 installation takes1729/1790ms and6→7 source trust1086/1256ms on probe/publish. Stage17→9 includes fresh manager checks, send READY AND waiting for the caller's ACK; it cannot be labeled solely manager-query cost. Old Boot38 has no probe response/exit milestones or actual entry-budget field. The new common-clock completion line closes those particular gaps. Agent parent30 is explicit at `internal/agent/agent.go` connectHookTimeout and cctx→wiring.Connected; source initialization/global Stop precede target initialization/publication under that same parent. Modeled initial26825ms and6468ms remaining at publication are consistent with expiry, not measured Boot38 entry budget.

R2 scope: verify additive client timing marks preserve guards, successful context sampling count and descriptor cleanup; common monotonic origin, entry caller budget clamp, unvisited sentinel and silent success; frozen proof reason/index/state formatter unchanged; image rewrite exactly five entries and original unit/hardening bytes; offline actual binary provenance; launch isolation/capacity/finite termination. R2 does not implement product fixes.

One-run evidence goal: obtain the closed proof failure reason/index/state paired with existing client/server stage and supplemental completion line. At client entry measure remaining parent budget directly. Split probe request→response, response→verified, verified→exit and exit→second-connect intervals; compare with both READY waits. Original server stage records distinguish installed-artifact validation and source authentication from waiting for client ACK, but do not retrospectively make journal timestamps an exact cross-process profiling clock.

Decision after ONE run: if proof reason3/4/5/6/7 has live context, investigate that exact held/canonical/process boundary using preserved evidence. If cancellation reason1/8/9, use actual entry-budget plus the split timeline to select a narrowly owned setup/phase defect; do not infer TCG as unavoidable or widen deadlines. If no new line appears, report exact failure stage and missing observation; no automatic second run. Any positive READY is a diagnostic outcome only, not full EAP/PKI/session/rollback/packaging/browser acceptance. Canonical READY remains realfailure until resolved; exact hosted gate and inherited-history correction remain manager-owned.
