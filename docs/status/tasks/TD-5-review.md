# TD-5 review: af_packet quiesce before delete, ifsanitize cap (D-105), small rings (D-108)

Reviewer: slot 2 (`w2`) · branch `task/TD-5` @ `90cc0cb` (code tip `3381e3e`) · merge-base main `3ca2f81` · 2026-09-24 14:55–15:10.
I read 00-CONTEXT, shared-host-rules, REVIEW-PROMPT, the review envelope, prompts/tech-debt/TD-5.md, the TD-5 envelope, TD-5.md,
TD-5-questions.md, and LOG D-101/D-105/D-107/D-108/D-110. I read the VPP source (`/root/vpp/src/plugins/af_packet`) and did not change it.
Probes ran through `go test -overlay` from my scratchpad, so no file in the worktree changed.

## Verdict first

**APPROVE WITH CHANGES.** The quiesce is correct and fail-closed. It is the only `af_packet_delete` sender, and the guard really
covers the tree. The netlink code is right, the host run is clean (NRestarts 0 → 0), and CI is green. Two small changes are needed
before merge: **H1**, where the D-108 TX frame size lets VPP write past a TX ring slot, and **M1**, where the cap formula has a sticky
liveness gap well below 64. L1–L3 are for the manager to decide. Nothing here meets a BLOCK criterion in REVIEW-PROMPT.

## Findings (ranked)

### H1 (High, D-108 follow-up; manager decision + a constant change in TD-5's file): a 2048-byte TX frame lets VPP write past the TX slot
- **Where:** `apps/agent/internal/descriptors/af_packet/host_interface.go:29-34` (`TxFrameSize = 2048`, `TxFramesPerBlock = 256`), sent at
  `:93`. The same values are in `tools/lab:334` (manager-owned).
- **Why:** VPP's TX path copies every packet into its frame slot with **no length check against `tp_frame_size`**
  (`device.c:537` slot pointer, `:562-573` `clib_memcpy_fast` of the first buffer and of every chained buffer; the v2 path `:458-494` is the same).
  The payload starts at `TPACKET_ALIGN(sizeof(tpacket3_hdr_t))` = 48 (`device.c:523`), so any frame longer than about 2000 bytes overruns its slot.
  VPP's default TX frame, `2048 * 33` (`af_packet.c:39`, "GSO packet of 64KB"), is sized for exactly this.
  - Frames longer than 2000 bytes do reach this path. The RX ring is TPACKET_V3 with 16 KiB blocks (host run: `block size:16384`). The kernel
    delivers whole frames up to about 16 KiB, and VPP drops only truncated ones (`node.c:429`). The lab veths have TSO/GSO on (host run:
    "Host Interface Offload … tcp segmentation offload, generic segmentation offload"), so a TCP flow from a rig netns hands VPP skbs of
    2–16 KiB without segmenting them. The agent's ethernet-mode interface has an L3 MTU of 9000 (host run: `host-w2-af50 … 9000/0/0/0`;
    the agent sets no MTU, unlike `tools/lab:338`), and L2 paths (xconnect/bridge) never check the MTU.
  - An overrun of slot *n* overwrites the `tp_status` of slot *n+1*. With one TX block that is the last region of the ring mmap
    (`af_packet.c:404`, `:524-542`: RX blocks first, then TX), so an overrun of slot 255 writes past the mapping: SIGSEGV or corruption of the
    neighbouring mapping in the **shared VPP**. That is the failure class TD-5 exists to prevent.
- **Not observed:** TD-5 sends no packets. This comes from reading the source, and no packet test was run (the envelope forbids one).
- **Fix:** keep D-108's small *count* and restore the GSO-safe frame *size*: `TxFrameSize = 2048 * 33` (67584) and `TxFramesPerBlock = 16`.
  The block is 1,081,344 B = 264 pages, a page multiple, so the TX ring is about 1 MiB and the total about 3.5 MiB instead of 76 MiB, which
  keeps the I6 intent. Update the unit assertion (`host_interface_test.go:103-108`), the "Rings (D-108)" paragraph, and `tools/lab:334`
  (manager). If the manager rules out 64 KiB frames (only when no tap/GSO source is ever bridged to af_packet), the minimum is a frame
  larger than the RX block, for example `20480 × 32` (640 KiB). Either way, amend D-108.

### M1 (Medium): the D-105 cap fails closed, and stays failed, for pools that need far fewer than 64 placeholders
- **Where:** `apps/agent/internal/vpp/ifsanitize/sanitize.go:331-346` (the cap check), with `proven()` called only at `:365` and `:370`.
- **Scenario:** above the highest live table sits an **uninterrupted ascending run** of `r ≥ 17` freed indices (tables created in order, then
  deleted in reverse, e.g. a test's LIFO `t.Cleanup`), and an older hole lies below it on the free list. The run is counted only when a gap or
  a lower pop interrupts it. Until then `seen` = 1, so the cap is 17. The run reaches 17 placeholders **before** it pops the hole and fails
  `ErrCapped`, although it needs only `r + 1 + FreshRun` (26 for r = 17). `dropPlaceholders` frees the pops in reverse, so every later run
  replays the same pops and every interface Create on the shared VPP fails. This is the sticky failure D1 fixed for broken runs, and it
  remains for unbroken ones.
- **Reproduced** (overlay probe `TestReviewAscendingRunAboveHole`, live {0,1,3,4}, free list `[2, 5+r-1 … 5]`, a stale output-ACL binding to 2):
  ```
  r=16 run 1: needed 25, placeholders 25, cap 33, holes seen 17, capped false, err <nil>, dirty ""
  r=17 run 1: needed 26, placeholders 17, cap 17, holes seen 1, capped true, err … (17 placeholders, cap 17 for 1 freed indices seen), dirty "output acl"
  r=17 run 2: needed 26, placeholders 17, cap 17, holes seen 1, capped true, …   ← sticky
  r=20 run 1: needed 29, placeholders 17, cap 17, … capped true
  r=40 run 1: needed 49, placeholders 17, cap 17, … capped true
  ```
- **Fix (validated):** a hole that is still free after `reread` proves the free list is not empty. VPP pops the free list before it grows the
  pool, so every pop of the current ascending run came from the free list. At `sanitize.go:333-335`:
  ```go
  if err := s.reread(holes); err != nil {
      return err
  }
  if len(holes) > 0 { // a hole still free: the free list is not empty, so the ascending run came from it
      proven()
      s.rep.Holes, s.rep.Cap = len(seen), PlaceholderCap(len(seen))
  }
  ```
  The success condition (`len(holes) == 0 && consec >= FreshRun`) does not change, so this is safety-neutral: it only raises the effort bound,
  still at most 64. With this overlay, `go test ./internal/vpp/ifsanitize/ ./internal/descriptors/...` all pass, and the probe succeeds
  with exactly the needed count (r=17: 26 placeholders, cap 34; r=40: 49, cap 57; run 2 the same). Add the probe as a unit test.

### L1 (Low, manager decides): a failed Delete leaves the netdev down while the object stays live
- **Where:** `quiesce.go:114-126`. The link-down succeeds, then `between()` (`ifsanitize.BeforeDelete`) or `AfPacketDelete` fails, for example
  on an I6 API stall hitting the deadline. The scheduler does not journal a failed delete (`scheduler/reconciler.go:943-945`), so rollback
  leaves the interface in VPP and in running config, with its Linux netdev **down**. The data path is dead and nothing says why, until a recreate.
  The prompt excludes bringing the link up after a *successful* delete. This is the failed case.
- **Fix:** in `quiescedDelete`, when this call cleared IFF_UP and a later step fails with the interface provably still present, restore IFF_UP
  (`RTM_NEWLINK ifi_change=IFF_UP ifi_flags=IFF_UP`, best effort, logged). At minimum, log a WARN ("netdev left down") and add a doc line.

### L2 (Low): Create's rollback runs on the caller's context
- **Where:** `host_interface.go:103`. When Create failed *because* its ctx expired (I6 stalls; the tests use 6-min deadlines), the rollback's
  link-down still runs, because netlink ignores ctx. `sleep()` then returns `ctx.Err()` at once (`quiesce.go:57-59`), so the result is
  `ErrQuiesce`, an untagged orphan, and a netdev left down. Every retry then fails with `IF_ALREADY_EXISTS` (`af_packet.c:643`) until an
  operator cleans up. That is D-110 Q1's orphan in a case that is not a permission problem and can be avoided.
- **Fix:** `rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)` for the rollback's `quiescedDelete`.

### L3 (Low, TD-5 or P08): the quiesce takes down whatever netdev `host_if_name` names
- Before TD-5, deleting an af_packet interface only detached VPP. Now it clears IFF_UP. D-105 F4's veth-only check lives on `task/P08`
  (`subsystems/netdev.go` `vethOnly`), which is not merged and wraps **Create only**. Until P08 merges, and for owner-tagged objects that predate
  the check, a Delete or a Create rollback on a non-veth netdev takes it down. On this host that includes the management NIC `ens192`.
- **Fix:** `Lookup` already receives the full `RTM_NEWLINK` answer, so read `IFLA_LINKINFO/IFLA_INFO_KIND` from it and fail closed
  (`ErrQuiesce`) when the kind is not `veth`. Alternatively, have P08's `vethOnly` wrap Delete too.

### Nits
- N1 `guard_test.go:113`: the CLI check matches only the exact literal `delete host-interface`. VPP's CLI accepts unique prefixes
  (`del host-int name …`), and a `Sprintf` or concatenation gets past it. The agent has `CliInband` callers (`ifsanitize/preflight.go:166`,
  `cmd/vrx-vppcheck`). A regex such as `(?i)\bdel\w*\s+host-int` on literals would catch these.
- N2 `guard_test.go:126-129`: "quiesce before delete" checks only source position, so `_ = d.quiesce(...)` passes. Require that the quiesce
  error is checked (`if err := …quiesce(…); err != nil { return … }`).
- N3 `quiesce.go:81`: treating ENODEV as "gone, go ahead" assumes the agent and VPP share a netns. VPP resolves the name in its own netns
  (`af_packet.c:663`). Document that next to CAP_NET_ADMIN in `af_packet.md`.
- N4 `integration_test.go:44`: the veth Cleanup runs `ip link del` even when the Cleanup Delete failed, which removes the netdev under an attached
  af_packet interface. Skip it when the descriptor Delete failed and leave the veth to `tools/lab rig gc`.
- N5 `docs/agent/descriptors/interface.md:134`: one sentence outside the "Placeholder cap" paragraph changed (the cap mention in step 2).
  It follows the spirit of "cap paragraph only", so I accept it.

## Checklist (REVIEW-PROMPT 1–11)
1. **Contract:** no hits in packages/schema, proto, gen, api-client (CI: "no contract files changed in the 10 commit(s)"). go.mod/go.sum untouched.
2. **Real verification:** the host test uses `/run/vpp/api.sock` and asserts Retrieve, `vppctl show interface` and the kernel IFF_UP flag.
   The order is proven in unit tests with a fake link controller merged into the fake VPP call log (`quiesce_test.go:124-148`). VPP's
   `af_packet_delete_if` never touches IFF_UP, so "reads down" after Delete can only come from the quiesce.
3. **Restart safety:** there is no new object type and Retrieve is unchanged. TD-3's `restart_integration_test.go` deletes af_packet only
   through the descriptor, so it now quiesces. No VPP restart.
4. **Provenance:** `AfPacketDelete`, `AfPacketCreateV3.{Rx,Tx}{FrameSize,FramesPerBlock}` come from `binapi/af_packet/af_packet.ba.go:335-338`,
   the generated bindings. `binapi/` and `tools/binapi-gen.sh` are untouched.
5. **Shared host:** names are `w2-af50*`, one package, `flock -s`, `disable_ipv6` before up, and the test refuses to reuse a leftover veth.
   Every changed file is in the owned list, and `vpp-code-track.md` has only the V24 row appended.
6. **Security:** product code does not exec `ip`; the quiesce is raw rtnetlink over `x/sys/unix`. `exec` appears only in the test helpers
   (fixed argv, `ip`/`vppctl show`). No secrets.
7. **Transactions:** L1 and L2. The rollback fails closed as D-110 Q1 decided.
8. **UI / 10. i18n:** not applicable.
9. **Scope:** the rings (D-108) and `Report.Holes/Cap` are additive and were added by the manager's decision. No creep.
11. **Tests run:** my CI run and the pasted CI both pass; the unit output matches the paste.

**Netlink check:** `RTM_GETLINK` by `IFLA_IFNAME` (ifi_index 0) returns an `RTM_NEWLINK` answer and no ACK. `RTM_NEWLINK` with
`ifi_index`, `ifi_change=IFF_UP`, `ifi_flags=0` and `NLM_F_REQUEST|NLM_F_ACK` (no CREATE) goes to `do_setlink`, which clears IFF_UP. The
`NLMSG_ERROR` code is decoded as `-errno` (0 = ACK), answers are matched on seq, there is a 2 s SO_RCV/SNDTIMEO, EINTR is retried, and
ENODEV is handled at lookup, at link-down and at confirm. `binary.Write` zero-fills the blank field of `IfInfomsg`. `TestNetlinkLookup`
exercises the codec against the real kernel (lo, a missing name → ENODEV).

**Guard coverage:** the tree has 388 non-test `.go` files under `apps/agent` outside `binapi/`, none in dot-dirs or `bin/`. The scan
reports exactly 1 permitted site and 0 violations. The planted tree flags all 7 raw forms.

## Host re-verification (one run, slot 2, one package, no packets)
```
$ eval "$(tools/lab env 2)"; date -Is; systemctl show vpp -p NRestarts
2026-09-24T14:58:30+03:30
NRestarts=0
$ cd apps/agent && VRX_INTEGRATION=1 flock -s /run/lock/vrx-lab.lock go test -count=1 -v -run 'OnHost' ./internal/descriptors/af_packet/
=== RUN   TestHostInterfaceOnHost
2026/09/24 14:58:37 INFO interface sanitized (VPP V19/V21 inherited state) interface=w2-af50 sw_if_index=3 phase=create cleared=[] freed=[] placeholders=26 rereads=3 reset="[…]" skipped=[]
    integration_test.go:97: timing: Create af-packet.host-interface/w2-af50 took 1.312s (err <nil>)
    integration_test.go:120: af-packet.host-interface: Retrieve == desired: af-packet.host-interface/w2-af50 name:"w2-af50" host_if_name:"w2-af50"
    integration_test.go:97: timing: Create af-packet.host-interface/w2-af50p took 676ms (err <nil>)
    integration_test.go:120: af-packet.host-interface: Retrieve == desired: af-packet.host-interface/w2-af50p name:"w2-af50p" host_if_name:"w2-af50p" mode:MODE_IP
    integration_test.go:122: vppctl show interface (after Create):
        host-w2-af50                      3     down         9000/0/0/0
        host-w2-af50p                     1     down          0/0/0/0
    integration_test.go:123: vppctl show hardware-interfaces host-w2-af50 (D-108 rings):
          RX Queue 0:
            block size:16384 nr:160  frame size:2048 nr:1280 next block:0
          TX Queue 0:
            block size:524288 nr:1  frame size:2048 nr:256 next frame:0
2026/09/24 14:58:38 INFO af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24) netdev=w2-af50 descriptor=af-packet.host-interface ifindex=59
    integration_test.go:128: timing: Delete af-packet.host-interface/w2-af50 took 726ms (err <nil>)
    integration_test.go:135: after Delete af-packet.host-interface/w2-af50: netdev w2-af50 reads down (net.FlagUp clear)
2026/09/24 14:58:39 INFO af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24) netdev=w2-af50p descriptor=af-packet.host-interface ifindex=58
    integration_test.go:128: timing: Delete af-packet.host-interface/w2-af50p took 3.606s (err <nil>)
    integration_test.go:135: after Delete af-packet.host-interface/w2-af50p: netdev w2-af50p reads down (net.FlagUp clear)
    integration_test.go:152: after Delete: Retrieve has neither key; vppctl show interface has no host-w2-af50*:
--- PASS: TestHostInterfaceOnHost (9.77s)
PASS
ok  	ngfw/agent/internal/descriptors/af_packet	9.937s
exit=0
$ date -Is; systemctl show vpp -p NRestarts
2026-09-24T14:58:45+03:30
NRestarts=0
$ journalctl -u vpp --since "2026-09-24 14:58:28" --until "2026-09-24 14:58:50" --no-pager | grep -E "af_packet|epoll_ctl|w2-af50" | grep -v "vpp\[[0-9]*\]: vpp\["
Sep 24 14:58:38 … af_packet: fd 28 reason af_packet_fd_error: Network is down
Sep 24 14:58:38 … vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w2-af50 queue 0' (fd 28), errno 9
Sep 24 14:58:38 … af_packet: fd 29 reason af_packet_fd_error: Network is down
Sep 24 14:58:39 … vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w2-af50p queue 0' (fd 29), errno 9
```
(Elided: the Retrieve timing lines, the other three `interface sanitized` INFO lines (placeholders=26 at create, 0 at delete), the
`reset=` lists, the vppctl table headers and the non-ring lines of `show hardware-interfaces`. The journal prefix `ubuntu-26.04 vpp[8760]:`
is shortened to `…`. The lines that remain are verbatim.) NRestarts stayed at 0 → 0 (MainPID 8760
unchanged). Afterwards there are no `w2-*` netdevs and no `host-w2*` in VPP. `errno 9` still appears on each delete with the netdev down,
as D-107 says: the quiesce lowers the risk and the double close remains. The 3.6 s delete overlapped my CI run (the CPU was loaded).
It is not a stall.

## CI
```
$ TMPDIR=/tmp/g-td5r tools/ci.sh --base main     # in /root/ngfw-wt/TD-5, HEAD 90cc0cb
no contract files changed in the 10 commit(s) of HEAD since main (3ca2f81)
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
ok: gitleaks — … no leaks found
Tasks:    30 successful, 30 total
… ok  ngfw/agent/internal/descriptors/af_packet 6.764s …
  mode quick · wall time 6m16s · logs /root/ngfw-wt/logs/ci/TD-5-20260924-145650-574561
CI GATE PASSED
exit=0
```
The unit run of `./internal/descriptors/af_packet/` matches the pasted TD-5.md output test for test: guard 0 violations and 1 site,
7 planted flags, and all the order, fail-closed and nothing-to-do cases. The author's CI (log `TD-5-20260924-144132-365121`) ran on
`3381e3e`, and only `docs/status/tasks/TD-5*` changed after it.

Housekeeping: my CI run rebuilt `apps/agent/bin/` and `apps/cli/bin/` (both git-ignored). My `rm` was denied by permissions, so the manager
can delete them.

**APPROVE WITH CHANGES**: H1 (TX frame size, D-108 amendment) and M1 (count the ascending run as proven when a re-read leaves holes)
before merge. L1–L3 are for the manager to decide.
