# TD-5: the agent quiesces the Linux netdev before af_packet_delete (V24 guard), plus the ifsanitize cap (D-105) and small af_packet rings (D-108)

branch `task/TD-5` · worktree `/root/ngfw-wt/TD-5` · slot 2 (`w2`) · base main@bccb9e4, main merged in twice (last: 3ca2f81) ·
code tip `3381e3e test(agent/af_packet): host test logs the hardware rings (D-108)`. CI (below) ran on this tip; after it only
`docs/status/tasks/TD-5*` files changed. **CI GATE PASSED.** Review: APPROVE WITH CHANGES (`TD-5-review.md`); **fix round 1**
(below, first) addresses H1, M1, L1–L3 and N1–N4.

## Fix round 1 (review e8f3d63, fix envelope `TD-5.fix1.md`, 2026-09-24 15:14–15:5x)

main merged first (`7f90854`, the pre-merge-commit gate green); the git-ignored `apps/agent/bin` and `apps/cli/bin` deleted.
Code commit `5c1148f`. Only files I own changed.

| Finding | Fix | Evidence (below) |
|---|---|---|
| **H1** (D-113) TX frame 2048 can be overrun | `TxFrameSize = 2048 * 33` (67584, VPP's default), `TxFramesPerBlock = 16` → one 1,081,344 B block (264 pages, ≈ 1 MiB); RX stays 2048 × 8 × 160. Comment + `af_packet.md` "Rings (D-108, amended by D-113)" say why the frame must never shrink (`device.c:561-573` copies with no length check). The unit test asserts the four values and that the TX block is a page multiple ≤ 2 MiB | `TestHostInterface`; host `show hardware-interfaces`: `TX Queue 0: block size:1081344 nr:1  frame size:67584 nr:16` |
| **M1** unbroken freed run above an older hole capped at 17, for good | the reviewer's fix at `sanitize.go:333`: after the re-read, a hole still free → `proven()` and the cap recomputed. Success condition unchanged, cap still ≤ 64 | `TestAscendingRunAboveAHole` (the reviewer's probe, r = 16/17/20/40/55/56, two runs each); the same test on the pre-fix `sanitize.go` (go test `-overlay`) fails at r = 17 exactly as the review shows |
| **L2** rollback on the caller's ctx | `rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), RollbackTimeout)` (30 s) around the rollback's `quiescedDelete` | `TestRollbackOutlivesTheCreateContext` (the tag handler cancels the Create ctx; the rollback still quiesces, settles on a live ctx with a ≤ 30 s deadline, deletes; no orphan). With the caller's ctx (overlay) it fails: `untagged orphan … settle: context canceled` |
| **L3** quiesce takes down any netdev | `Links.Lookup` now returns `Netdev{Index, Up, Kind}`; the kind is `IFLA_LINKINFO/IFLA_INFO_KIND` from the same `RTM_NEWLINK` answer (`NLA_F_NESTED` masked; a missing or malformed attribute reads `""` → refused). An **up** netdev whose kind is not `veth` → `ErrQuiesce` wrapping `ErrNotVeth` (names D-105), nothing sent, netdev left up. Already-down non-veth → the delete goes ahead (nothing is brought down) | `TestQuiesceRefusesNonVeth` (kinds none/bond/vlan/tun + the rollback path), `TestQuiesceNothingToDo/already_down,_not_a_veth`, `TestParseAnswer` (veth, no linkinfo, truncated), `TestNetlinkLookup` (lo → kind ""); host log `… netdev=w2-af50 … kind=veth` (the real kernel's answer; a wrong parse would have failed the Delete closed) |
| **L1** failed Delete leaves the netdev down | `quiesce` returns the ifindex it brought down; when `BeforeDelete` fails, or the quiesce fails after its link-down (confirm/settle), `restore` sets IFF_UP again (`Links.SetUp`, `RTM_NEWLINK ifi_flags=IFF_UP`, best effort, WARN). A failed `af_packet_delete` (outcome unknown) leaves it down with a WARN `netdev left down` | `TestFailedDeleteRestoresTheLink` (both cases), `TestSettleCancelled` (link-up after the cancelled settle) |
| **N1** guard matches only the literal | string literals are matched with `(?i)\bdel\w*\s+host-int` | planted `x/cli.go`: `"del host-int …"`, `"DELETE Host-Interface …"` flagged, `"show host-interface"` not |
| **N2** `_ = d.quiesce(…)` passes | the guard requires a **checked** quiesce before `AfPacketDelete`: top-level `if err := d.quiesce(…); err != nil { …; return … }`, or `…, err := d.quiesce(…)` directly followed by `if err != nil { …; return … }` | planted `unchecked.go` (`_, _ =`) and `no_return.go` (checked, no return) flagged; `good.go` + `good2.go` (the product's two-result form) are the 2 permitted sites |
| **N3** ENODEV assumes a shared netns | doc bullet in `af_packet.md` next to CAP_NET_ADMIN | — |
| **N4** veth Cleanup deletes under an attached interface | `veth(…, keep)`: the Cleanup keeps the pair when a descriptor Cleanup Delete failed (log points to `tools/lab rig gc w<slot>`) | host run (normal path: pair removed, 0 `w2-*` left) |
| **N5** | accepted by the reviewer, nothing to do | — |

Inventory update (scope item 5): the only `af_packet_delete` is now at `apps/agent/internal/descriptors/af_packet/quiesce.go:177`
(`quiescedDelete`); nothing else in the inventory changed.

**Left for later:** nothing from the review. Still open as before: the VPP fix itself (V24), CAP_NET_ADMIN in P10's unit, and
the questions in `TD-5-questions.md` (fix-round decisions F1–F3 added there).

### Fix round 1 — unit tests
```
$ cd apps/agent && go test -count=1 -v ./internal/descriptors/af_packet/      (15:2x; pass/fail lines, then the logs that matter)
--- PASS: TestParseAnswer (0.00s)
--- PASS: TestNetlinkLookup (0.00s)
--- PASS: TestEveryAfPacketDeleteIsQuiesced (1.43s)
--- PASS: TestGuardCatchesARawAfPacketDelete (0.00s)
--- PASS: TestHostInterface (0.00s)
--- SKIP: TestHostInterfaceOnHost (0.00s)
--- PASS: TestDeleteQuiescesFirst (0.00s)
--- PASS: TestCreateRollbackQuiesces (0.00s)
    --- PASS: TestCreateRollbackQuiesces/tag_fails (0.00s)
    --- PASS: TestCreateRollbackQuiesces/sanitize_fails (0.00s)
--- PASS: TestQuiesceFailsClosed (0.01s)
    --- PASS: TestQuiesceFailsClosed/EPERM (0.00s)
    --- PASS: TestQuiesceFailsClosed/timeout (0.00s)
    --- PASS: TestQuiesceFailsClosed/lookup_EPERM (0.00s)
    --- PASS: TestQuiesceFailsClosed/still_up (0.00s)
--- PASS: TestRollbackQuiesceFailsClosed (0.00s)
--- PASS: TestQuiesceNothingToDo (0.01s)
    --- PASS: TestQuiesceNothingToDo/ENODEV (0.00s)
    --- PASS: TestQuiesceNothingToDo/already_down (0.00s)
    --- PASS: TestQuiesceNothingToDo/already_down,_not_a_veth (0.00s)
    --- PASS: TestQuiesceNothingToDo/ENODEV_at_the_link-down (0.00s)
--- PASS: TestSettleCancelled (0.00s)
--- PASS: TestQuiesceRefusesNonVeth (0.00s)
    --- PASS: TestQuiesceRefusesNonVeth/kind= (0.00s)
    --- PASS: TestQuiesceRefusesNonVeth/kind=bond (0.00s)
    --- PASS: TestQuiesceRefusesNonVeth/kind=vlan (0.00s)
    --- PASS: TestQuiesceRefusesNonVeth/kind=tun (0.00s)
    --- PASS: TestQuiesceRefusesNonVeth/rollback (0.00s)
--- PASS: TestFailedDeleteRestoresTheLink (0.00s)
    --- PASS: TestFailedDeleteRestoresTheLink/BeforeDelete_fails (0.00s)
    --- PASS: TestFailedDeleteRestoresTheLink/af_packet_delete_fails (0.00s)
--- PASS: TestRollbackOutlivesTheCreateContext (0.00s)
--- PASS: TestHostInterfaceSanitizesReusedIndex (0.01s)
ok  	ngfw/agent/internal/descriptors/af_packet	1.578s

    guard_test.go:238: apps/agent: 0 violations; af_packet_delete is sent at 1 site (quiescedDelete, after quiesce)
    guard_test.go:348: flagged: internal/descriptors/af_packet/bad_order.go:4: af_packet_delete without a checked quiesce before it in quiescedDelete (`if err := d.quiesce(…); err != nil { return … }`, D-101, VPP V24)
    guard_test.go:348: flagged: internal/descriptors/af_packet/no_return.go:7: af_packet_delete without a checked quiesce before it in quiescedDelete (…)
    guard_test.go:348: flagged: internal/descriptors/af_packet/unchecked.go:5: af_packet_delete without a checked quiesce before it in quiescedDelete (…)
    guard_test.go:348: flagged: internal/descriptors/x/cli.go:4: "delete host-interface" CLI string (or an abbreviation): af_packet deletes go through (*HostInterfaceDescriptor).quiescedDelete (D-101, VPP V24)
    guard_test.go:348: flagged: internal/descriptors/x/cli.go:5: "delete host-interface" CLI string (or an abbreviation): …
    guard_test.go:348: flagged: internal/descriptors/x/raw.go:4: af_packet_delete sent outside (*HostInterfaceDescriptor).quiescedDelete: the netdev is not quiesced first (D-101, VPP V24)
    guard_test.go:348: flagged: internal/descriptors/x/raw.go:5: … · raw.go:6: … · raw.go:7: "delete host-interface" CLI string … · raw.go:13: … · xtest/fixture.go:3: …
    quiesce_test.go:409: Delete refused: af_packet: host netdev not quiesced, af_packet_delete not sent (D-101, VPP V24): netdev w2-l0: kind: not a veth: af_packet is lab-only on veth netdevs (D-105), the quiesce brings no other netdev down; link kind none (a physical NIC or another device without rtnl link ops)
    quiesce_test.go:409: Delete refused: … link kind bond   (vlan, tun the same)
    quiesce_test.go:505: Create: sw_interface_tag_add_del: VPPApiError: Unimplemented (-9); rollback ran on a detached context (deadline in 30s)
```
(Lines marked `…` are shortened by me; the flagged list is the full list of 12 planted violations.)

```
$ cd apps/agent && go test -count=1 -v -run 'TestAscendingRunAboveAHole|TestTwelveFreed|TestCappedFailsClosed|TestReplayed' ./internal/vpp/ifsanitize/
    sanitize_test.go:364: 12 out-of-order freed indices: 20 placeholders, cap 28 (holes seen 12), freed [output-acl ip4 table 9 (deleted table, removed through a placeholder)]
--- PASS: TestTwelveFreedOutOfOrderSucceeds (0.00s)
    sanitize_test.go:403: run 1: 25 placeholders, cap 33 (freed indices seen 17), freed [output-acl ip6 table 12 (deleted table, removed through a placeholder)]
    sanitize_test.go:403: run 2: 25 placeholders, cap 33 (freed indices seen 17), freed [output-acl ip6 table 12 (deleted table, removed through a placeholder)]
--- PASS: TestReplayedAscendingRunsAreCounted (0.00s)
    sanitize_test.go:433: r=16 run 1: needed 25, placeholders 25, cap 33, holes seen 17, rereads 9, capped false, err <nil>
    sanitize_test.go:433: r=16 run 2: needed 25, placeholders 25, cap 33, holes seen 17, rereads 9, capped false, err <nil>
    sanitize_test.go:433: r=17 run 1: needed 26, placeholders 26, cap 34, holes seen 18, rereads 10, capped false, err <nil>
    sanitize_test.go:433: r=17 run 2: needed 26, placeholders 26, cap 34, holes seen 18, rereads 10, capped false, err <nil>
    sanitize_test.go:433: r=20 run 1: needed 29, placeholders 29, cap 37, holes seen 21, rereads 13, capped false, err <nil>
    sanitize_test.go:433: r=20 run 2: needed 29, placeholders 29, cap 37, holes seen 21, rereads 13, capped false, err <nil>
    sanitize_test.go:433: r=40 run 1: needed 49, placeholders 49, cap 57, holes seen 41, rereads 33, capped false, err <nil>
    sanitize_test.go:433: r=40 run 2: needed 49, placeholders 49, cap 57, holes seen 41, rereads 33, capped false, err <nil>
    sanitize_test.go:433: r=55 run 1: needed 64, placeholders 64, cap 64, holes seen 56, rereads 48, capped false, err <nil>
    sanitize_test.go:433: r=55 run 2: needed 64, placeholders 64, cap 64, holes seen 56, rereads 48, capped false, err <nil>
    sanitize_test.go:433: r=56 run 1: needed 65, placeholders 64, cap 64, holes seen 57, rereads 49, capped true, err sanitize loop211 (sw_if_index 7, create): no clean sw_if_index obtained (VPP V19 quarantine): placeholder cap reached before every freed classify table index was resurrected (64 placeholders, cap 64 for 57 freed indices seen)
    sanitize_test.go:433: r=56 run 2: needed 65, placeholders 64, cap 64, holes seen 57, rereads 49, capped true, err … (the same)
--- PASS: TestAscendingRunAboveAHole (0.03s)
    sanitize_test.go:476: capped: sanitize loop208 (sw_if_index 3, create): no clean sw_if_index obtained (VPP V19 quarantine): placeholder cap reached before every freed classify table index was resurrected (64 placeholders, cap 64 for 70 freed indices seen)
--- PASS: TestCappedFailsClosed (0.28s)
PASS
ok  	ngfw/agent/internal/vpp/ifsanitize	0.382s

# negative control: the same test against the pre-fix sanitize.go (git show HEAD~:…, via go test -overlay; the worktree unchanged)
    sanitize_test.go:433: r=17 run 1: needed 26, placeholders 17, cap 17, holes seen 1, rereads 10, capped true, err … (17 placeholders, cap 17 for 1 freed indices seen)
    sanitize_test.go:439: r=17 run 1: dirty "output acl"
--- FAIL: TestAscendingRunAboveAHole (0.00s)

$ cd apps/agent && go vet ./internal/descriptors/af_packet/ ./internal/vpp/ifsanitize/ && golangci-lint run --allow-serial-runners ./internal/descriptors/af_packet/... ./internal/vpp/ifsanitize/...
0 issues.
$ go test -count=1 -race ./internal/descriptors/... ./internal/vpp/ifsanitize/      # every package ok (list elided)
```

### Fix round 1 — host run (slot 2, one package, no packets, load ≈ 20 after a spike to 109 from other slots' CI)
```
$ eval "$(tools/lab env 2)"; date -Is; uptime; systemctl show vpp -p NRestarts -p MainPID
2026-09-24T15:36:54+03:30
 15:36:54 up  5:49,  2 users,  load average: 20.60, 79.66, 62.51
MainPID=8760
NRestarts=0
$ cd apps/agent && VRX_INTEGRATION=1 flock -s /run/lock/vrx-lab.lock go test -count=1 -v -run 'OnHost' ./internal/descriptors/af_packet/
=== RUN   TestHostInterfaceOnHost
2026/09/24 15:37:03 INFO interface sanitized (VPP V19/V21 inherited state) interface=w2-af50 sw_if_index=3 phase=create cleared=[] freed=[] placeholders=26 rereads=3 reset="[…]" skipped=[]
    integration_test.go:106: timing: Create af-packet.host-interface/w2-af50 took 4.609s (err <nil>)
    integration_test.go:122: timing: Retrieve took 110ms (err <nil>)
    integration_test.go:130: af-packet.host-interface: Retrieve == desired: af-packet.host-interface/w2-af50 name:"w2-af50" host_if_name:"w2-af50"
    integration_test.go:106: timing: Create af-packet.host-interface/w2-af50p took 4.412s (err <nil>)
    integration_test.go:122: timing: Retrieve took 4ms (err <nil>)
    integration_test.go:130: af-packet.host-interface: Retrieve == desired: af-packet.host-interface/w2-af50p name:"w2-af50p" host_if_name:"w2-af50p" mode:MODE_IP
    integration_test.go:132: vppctl show interface (after Create):
        host-w2-af50p                     5     down          0/0/0/0
        host-w2-af50                      3     down         9000/0/0/0
    integration_test.go:133: vppctl show hardware-interfaces host-w2-af50 (D-108/D-113 rings):
          RX Queue 0:
            block size:16384 nr:160  frame size:2048 nr:1280 next block:0
          TX Queue 0:
            block size:1081344 nr:1  frame size:67584 nr:16 next frame:0
            available:16 request:0 sending:0 wrong:0 total:16
2026/09/24 15:37:08 INFO af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24) netdev=w2-af50 descriptor=af-packet.host-interface ifindex=65 kind=veth
    integration_test.go:138: timing: Delete af-packet.host-interface/w2-af50 took 327ms (err <nil>)
    integration_test.go:145: after Delete af-packet.host-interface/w2-af50: netdev w2-af50 reads down (net.FlagUp clear)
2026/09/24 15:37:09 INFO af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24) netdev=w2-af50p descriptor=af-packet.host-interface ifindex=64 kind=veth
    integration_test.go:138: timing: Delete af-packet.host-interface/w2-af50p took 306ms (err <nil>)
    integration_test.go:145: after Delete af-packet.host-interface/w2-af50p: netdev w2-af50p reads down (net.FlagUp clear)
    integration_test.go:147: timing: Retrieve took 8ms (err <nil>)
    integration_test.go:162: after Delete: Retrieve has neither key; vppctl show interface has no host-w2-af50*:
--- PASS: TestHostInterfaceOnHost (11.10s)
PASS
ok  	ngfw/agent/internal/descriptors/af_packet	11.137s
exit=0
$ date -Is; systemctl show vpp -p NRestarts -p MainPID
2026-09-24T15:37:09+03:30
MainPID=8760
NRestarts=0
$ journalctl -u vpp --since "2026-09-24 15:36:54" --until "2026-09-24 15:37:15" --no-pager | grep -E "af_packet|epoll_ctl|w2-af50|SIGSEGV|signal"   (the duplicate "vpp[8760]: vpp[8760]:" copies dropped)
Sep 24 15:37:08 ubuntu-26.04 vpp[8760]: af_packet: fd 29 reason af_packet_fd_error: Network is down
Sep 24 15:37:08 ubuntu-26.04 vpp[8760]: vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w2-af50 queue 0' (fd 29), errno 9
Sep 24 15:37:08 ubuntu-26.04 vpp[8760]: af_packet: fd 31 reason af_packet_fd_error: Network is down
Sep 24 15:37:08 ubuntu-26.04 vpp[8760]: af_packet: fd 33 reason af_packet_fd_error: Network is down
Sep 24 15:37:08 ubuntu-26.04 vpp[8760]: vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w1l0 queue 0' (fd 31), errno 9
Sep 24 15:37:08 ubuntu-26.04 vpp[8760]: af_packet: fd 30 reason af_packet_fd_error: Network is down
Sep 24 15:37:08 ubuntu-26.04 vpp[8760]: vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w1w0 queue 0' (fd 33), errno 9
Sep 24 15:37:09 ubuntu-26.04 vpp[8760]: vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w2-af50p queue 0' (fd 30), errno 9
$ ip -br link | grep -c "^w2-"; vppctl show interface | grep -c host-w2
0
0
```
(Elided: the two create-phase `interface sanitized` lines' `reset=` list, the two delete-phase `interface sanitized` lines
(placeholders=0), the vppctl table headers and the non-ring lines of `show hardware-interfaces`. The rest is verbatim.)
NRestarts 0 → 0, MainPID 8760 unchanged, no SIGSEGV/signal line. `kind=veth` in the quiesce log is the real kernel's
`IFLA_INFO_KIND`. The ring is the D-113 one. **Timings:** Create 4.6 s / 4.4 s (sanitize with 26 placeholders on a host that
had just come off load 109; D-113 measured 1.8 s for the rig), Delete 0.33 s / 0.31 s (the review's run: 1.3 s / 0.7 s create,
0.7 s / 3.6 s delete). Slot 1's rig (`host-w1l0`, `host-w1w0`) was deleted in the same second, with its veths down (`Network is
down` first), and logged the same D-107 `errno 9`: the double close stays until the VPP fix (V24).

### Fix round 1 — CI
```
$ TMPDIR=/tmp/g-td5 tools/ci.sh --base main   # HEAD 5c1148f, 2026-09-24T15:37:25+03:30
no contract files changed in the 13 commit(s) of HEAD since main (8e38a40)
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~160690 bytes (160.69 KB) in 1.25s no leaks found
Tasks:    30 successful, 30 total Cached:    24 cached, 30 total Time:    1m42.023s
== apps/agent: make lint test build ==   (07-agent.log: "0 issues.", 88 packages ok, among them)
ok  	ngfw/agent/internal/descriptors/af_packet	7.614s
ok  	ngfw/agent/internal/vpp/ifsanitize	8.669s
== summary (quick) ==
  generate + generated-output gate                   2m04s
  lint · typecheck · unit tests · build (turbo)   1m46s
  apps/agent: make lint test build                   1m17s
  apps/cli: make lint test build                     0m16s
  mode quick · wall time 5m36s · logs /root/ngfw-wt/logs/ci/TD-5-20260924-153725-732988
CI GATE PASSED
exit=0
```
After the CI only `docs/status/tasks/TD-5*` changed; `apps/agent/bin` and `apps/cli/bin` (rebuilt by the CI) were deleted again.

## What was built

1. **Quiesce helper** (`apps/agent/internal/descriptors/af_packet/quiesce.go`, netlink in `quiesce_linux.go`). It sits behind the
   `afpacket.Links` interface (`Lookup(name) → ifindex, up` / `SetDown(ifindex)`), which unit tests replace with a fake:
   - `RTM_GETLINK` by `IFLA_IFNAME` resolves the name to ifindex and `IFF_UP`. If the link is up, `RTM_NEWLINK` (`ifi_change = IFF_UP`,
     `ifi_flags = 0`, `NLM_F_ACK`) brings it down. A second lookup must then read down, followed by a **200 ms settle** (`DefaultSettle`,
     injectable through `WithSettle`). It uses only `golang.org/x/sys/unix`, which was already a direct dependency: no new module and no exec of `ip`.
   - ENODEV (at the lookup, at the link-down or at the confirm) means there is nothing to quiesce, so the delete goes ahead. A link that
     is already down is a no-op.
   - Any other failure (EPERM, a 2 s netlink timeout via SO_RCVTIMEO/SO_SNDTIMEO, IFF_UP still set, a settle cancelled by ctx) returns
     `afpacket.ErrQuiesce`, which names the netdev and D-101/VPP V24, and **no `af_packet_delete` is sent**.
   - `(*HostInterfaceDescriptor).quiescedDelete` is the **only** `af_packet_delete` sender in the agent.
2. **Delete**: quiesce → `ifsanitize.BeforeDelete` → `af_packet_delete`.
3. **Create's rollback** (the `del` closure given to `iface.AcquireAndTag`, used for a tag failure, a sanitize failure and quarantine; TD-3
   re-review L7): quiesce → `af_packet_delete`. If the quiesce fails, it returns `untagged orphan host-<dev> (sw_if_index N) left in VPP: …
   D-101 …` and deletes nothing. `Update` is ErrRecreate, which goes through Delete.
4. **Guard** `TestEveryAfPacketDeleteIsQuiesced` (`af_packet/guard_test.go`). It scans every non-test Go file under `apps/agent/`,
   including test-helper packages and excluding the generated `binapi/`. It flags any `AfPacketDelete` identifier outside `quiescedDelete`
   (a service call, a request literal for a raw Invoke, `new(...)`), an `AfPacketDelete` placed before the `quiesce` call inside
   `quiescedDelete`, and any `"delete host-interface"` CLI string. `TestGuardCatchesARawAfPacketDelete` plants seven violations in a
   `t.TempDir()` tree, plus a test file and generated binapi files that must not be flagged.
5. **Host test** (`integration_test.go`, rewritten). Both veth ends get `disable_ipv6=1` before `up`. The test creates both
   host-interfaces through the descriptor and checks Retrieve == desired; it then **Deletes through the descriptor** and asserts the netdev
   reads **down** (`net.InterfaceByName(...).Flags&net.FlagUp == 0`), that Retrieve has neither key, and that `vppctl show interface` has
   no `host-w2-af50*`. Every call has a 6-minute deadline and its duration is logged (D-107/I6). The veth helper refuses a leftover netdev
   instead of deleting it, because it might still have a VPP interface attached.
6. **ifsanitize cap** (D-105, TD-3 re-review M1 option b). `PlaceholderCap(holes) = holes + 2 × FreshRun`, at most `MaxPlaceholders` = 64,
   replaces the fixed 16. It still fails closed (`ErrCapped` wraps `ErrNoCleanIndex`, and quarantine happens only when a binding is proven
   unclearable). "Holes" counts every freed index the run has seen: gaps, input-ACL tables, and pops proven freed by the pop order. See the
   decision **D1** in `TD-5-questions.md`: the gap-only reading failed on the first host run. `Report.Holes`/`Report.Cap` are new, and the
   capped WARN logs `pops`.
7. **Rings (D-108, added by the manager)** — *superseded in fix round 1 by D-113: `TxFrameSize=67584 TxFramesPerBlock=16`, RX
   unchanged*: every `af_packet_create_v3` carries `TxFrameSize=2048 TxFramesPerBlock=256 RxFrameSize=2048
   RxFramesPerBlock=8`, about 3 MiB instead of 76 MiB. The field names come from `binapi/af_packet`. The unit test asserts all four, and the host
   run shows them in `show hardware-interfaces`.
8. **Docs**: `docs/agent/descriptors/af_packet.md` gets the "Delete ordering (D-101/V24)" and "Rings (D-108)" sections;
   `docs/agent/descriptors/interface.md` gets the new placeholder-cap paragraph; the `docs/vpp-code-track.md` V24 row gets the agent-side
   status appended.

**Stated plainly (D-107):** the quiesce **lowers** the V24 risk. It does **not** remove the double close. VPP still logs `epoll_ctl() failed …
errno 9` on every delete with the netdev down (below: 14:35:15 and 14:41:15, both interfaces), and fd numbers are reused across interfaces
(fd 29 was `host-w2-af50p` at 14:35:15 and slot 11's `host-w11a0` at 14:36:32). Only the VPP fix (V24) removes it.

**CAP_NET_ADMIN:** the product agent needs it for the link-down. That is P10's systemd unit. Without it, every af_packet Delete fails
closed with EPERM.

**Settle:** 200 ms, the same as `tools/lab`. I picked it and did not tune it. The open question is in `TD-5-questions.md` Q2.

## Verification

### Unit tests (fake VPP with TD-3's sanitizing model + fake link controller recording the order)
The slog lines are filtered out (`grep -v '^2026/'`). Everything else is verbatim.
```
$ cd apps/agent && go test -count=1 -v ./internal/descriptors/af_packet/
=== RUN   TestParseAnswer
--- PASS: TestParseAnswer (0.00s)
=== RUN   TestNetlinkLookup
--- PASS: TestNetlinkLookup (0.00s)
=== RUN   TestEveryAfPacketDeleteIsQuiesced
    guard_test.go:192: apps/agent: 0 violations; af_packet_delete is sent at 1 site (quiescedDelete, after quiesce)
--- PASS: TestEveryAfPacketDeleteIsQuiesced (1.89s)
=== RUN   TestGuardCatchesARawAfPacketDelete
    guard_test.go:262: flagged: internal/descriptors/af_packet/bad_order.go:4: af_packet_delete before the quiesce in quiescedDelete (D-101, VPP V24)
    guard_test.go:262: flagged: internal/descriptors/x/raw.go:4: af_packet_delete sent outside (*HostInterfaceDescriptor).quiescedDelete: the netdev is not quiesced first (D-101, VPP V24)
    guard_test.go:262: flagged: internal/descriptors/x/raw.go:5: af_packet_delete sent outside (*HostInterfaceDescriptor).quiescedDelete: the netdev is not quiesced first (D-101, VPP V24)
    guard_test.go:262: flagged: internal/descriptors/x/raw.go:6: af_packet_delete sent outside (*HostInterfaceDescriptor).quiescedDelete: the netdev is not quiesced first (D-101, VPP V24)
    guard_test.go:262: flagged: internal/descriptors/x/raw.go:7: "delete host-interface" CLI string: af_packet deletes go through (*HostInterfaceDescriptor).quiescedDelete (D-101, VPP V24)
    guard_test.go:262: flagged: internal/descriptors/x/raw.go:13: af_packet_delete sent outside (*HostInterfaceDescriptor).quiescedDelete: the netdev is not quiesced first (D-101, VPP V24)
    guard_test.go:262: flagged: internal/descriptors/x/xtest/fixture.go:3: af_packet_delete sent outside (*HostInterfaceDescriptor).quiescedDelete: the netdev is not quiesced first (D-101, VPP V24)
--- PASS: TestGuardCatchesARawAfPacketDelete (0.00s)
=== RUN   TestHostInterface
--- PASS: TestHostInterface (0.00s)
=== RUN   TestHostInterfaceOnHost
    integration_test.go:83: integration test: set VRX_INTEGRATION=1 (and run under the shared lab lock)
--- SKIP: TestHostInterfaceOnHost (0.00s)
=== RUN   TestDeleteQuiescesFirst
    quiesce_test.go:199: Delete: link-down(w2-l0) → settle → … → af_packet_delete(w2-l0)
--- PASS: TestDeleteQuiescesFirst (0.00s)
=== RUN   TestCreateRollbackQuiesces
=== RUN   TestCreateRollbackQuiesces/tag_fails
    quiesce_test.go:229: tag fails: af_packet_create_v3(w2-w0) → … → link-down(w2-w0) → settle → af_packet_delete(w2-w0) (err: sw_interface_tag_add_del: VPPApiError: Unimplemented (-9))
=== RUN   TestCreateRollbackQuiesces/sanitize_fails
    quiesce_test.go:229: sanitize fails: af_packet_create_v3(w2-w0) → … → link-down(w2-w0) → settle → af_packet_delete(w2-w0) (err: sanitize w2-w0 (sw_if_index 2, create): classify_set_interface_ip_table (reset ip4): vpp refused)
--- PASS: TestCreateRollbackQuiesces (0.00s)
    --- PASS: TestCreateRollbackQuiesces/tag_fails (0.00s)
    --- PASS: TestCreateRollbackQuiesces/sanitize_fails (0.00s)
=== RUN   TestQuiesceFailsClosed
=== RUN   TestQuiesceFailsClosed/EPERM
    quiesce_test.go:270: Delete refused: af_packet: host netdev not quiesced, af_packet_delete not sent (D-101, VPP V24): netdev w2-l0: link down: operation not permitted
=== RUN   TestQuiesceFailsClosed/timeout
    quiesce_test.go:270: Delete refused: af_packet: host netdev not quiesced, af_packet_delete not sent (D-101, VPP V24): netdev w2-l0: link down: netlink: no answer within 2s (timeout): resource temporarily unavailable
=== RUN   TestQuiesceFailsClosed/lookup_EPERM
    quiesce_test.go:270: Delete refused: af_packet: host netdev not quiesced, af_packet_delete not sent (D-101, VPP V24): netdev w2-l0: lookup: operation not permitted
=== RUN   TestQuiesceFailsClosed/still_up
    quiesce_test.go:270: Delete refused: af_packet: host netdev not quiesced, af_packet_delete not sent (D-101, VPP V24): netdev w2-l0: confirm down: IFF_UP still set after RTM_NEWLINK
--- PASS: TestQuiesceFailsClosed (0.00s)
    --- PASS: TestQuiesceFailsClosed/EPERM (0.00s)
    --- PASS: TestQuiesceFailsClosed/timeout (0.00s)
    --- PASS: TestQuiesceFailsClosed/lookup_EPERM (0.00s)
    --- PASS: TestQuiesceFailsClosed/still_up (0.00s)
=== RUN   TestRollbackQuiesceFailsClosed
    quiesce_test.go:298: Create: sw_interface_tag_add_del: VPPApiError: Unimplemented (-9) (and removing the untagged orphan 2: untagged orphan host-w2-w0 (sw_if_index 2) left in VPP: af_packet: host netdev not quiesced, af_packet_delete not sent (D-101, VPP V24): netdev w2-w0: link down: operation not permitted)
--- PASS: TestRollbackQuiesceFailsClosed (0.00s)
=== RUN   TestQuiesceNothingToDo
=== RUN   TestQuiesceNothingToDo/ENODEV
=== RUN   TestQuiesceNothingToDo/already_down
=== RUN   TestQuiesceNothingToDo/ENODEV_at_the_link-down
--- PASS: TestQuiesceNothingToDo (0.00s)
    --- PASS: TestQuiesceNothingToDo/ENODEV (0.00s)
    --- PASS: TestQuiesceNothingToDo/already_down (0.00s)
    --- PASS: TestQuiesceNothingToDo/ENODEV_at_the_link-down (0.00s)
=== RUN   TestSettleCancelled
--- PASS: TestSettleCancelled (0.00s)
=== RUN   TestHostInterfaceSanitizesReusedIndex
--- PASS: TestHostInterfaceSanitizesReusedIndex (0.00s)
PASS
ok  	ngfw/agent/internal/descriptors/af_packet	2.032s
exit=0
```
The mapping to the acceptance items is:
- `TestDeleteQuiescesFirst`: link-down comes before the settle, which comes before BeforeDelete's first message, which comes before `af_packet_delete`.
- `TestCreateRollbackQuiesces`: both the tag failure and the sanitize failure put link-down before the rollback `af_packet_delete`.
- `TestQuiesceFailsClosed`: EPERM, timeout, lookup EPERM and still-up all send no VPP message at all, and the error names D-101/V24 and the netdev.
- `TestRollbackQuiesceFailsClosed`: the orphan is named and not deleted.
- `TestQuiesceNothingToDo`: ENODEV, already-down, and ENODEV at the link-down all let the delete go ahead with no link-down and no settle.
- `TestParseAnswer`/`TestNetlinkLookup`: the netlink codec, plus a read-only lookup of `lo` and of a missing name.

### ifsanitize cap (D-105)
```
$ cd apps/agent && go test -count=1 -v -run 'PlaceholderCap|TwelveFreed|Replayed|CappedFailsClosed|AcquireCapped|HoleTaken' ./internal/vpp/ifsanitize/
=== RUN   TestAcquireCappedFailsClosed
--- PASS: TestAcquireCappedFailsClosed (0.01s)
=== RUN   TestAcquireCappedAndUnclearable
--- PASS: TestAcquireCappedAndUnclearable (0.01s)
=== RUN   TestHoleTakenBySomeoneElse
    sanitize_test.go:308: placeholders 10, rereads 1, API calls 159, freed [input-acl ip4 table 3 (deleted table; its index was taken by another client during the run, removed through that table) output-acl ip6 table 2 (deleted table, removed through a placeholder)]
--- PASS: TestHoleTakenBySomeoneElse (0.00s)
=== RUN   TestPlaceholderCap
--- PASS: TestPlaceholderCap (0.00s)
=== RUN   TestTwelveFreedOutOfOrderSucceeds
    sanitize_test.go:364: 12 out-of-order freed indices: 20 placeholders, cap 28 (holes seen 12), freed [output-acl ip4 table 9 (deleted table, removed through a placeholder)]
--- PASS: TestTwelveFreedOutOfOrderSucceeds (0.00s)
=== RUN   TestReplayedAscendingRunsAreCounted
    sanitize_test.go:403: run 1: 25 placeholders, cap 33 (freed indices seen 17), freed [output-acl ip6 table 12 (deleted table, removed through a placeholder)]
    sanitize_test.go:403: run 2: 25 placeholders, cap 33 (freed indices seen 17), freed [output-acl ip6 table 12 (deleted table, removed through a placeholder)]
--- PASS: TestReplayedAscendingRunsAreCounted (0.00s)
=== RUN   TestCappedFailsClosed
    sanitize_test.go:436: capped: sanitize loop208 (sw_if_index 3, create): no clean sw_if_index obtained (VPP V19 quarantine): placeholder cap reached before every freed classify table index was resurrected (64 placeholders, cap 64 for 70 freed indices seen)
--- PASS: TestCappedFailsClosed (0.00s)
PASS
ok  	ngfw/agent/internal/vpp/ifsanitize	0.070s
exit=0
```
`TestTwelveFreedOutOfOrderSucceeds` first shows that the same pool **fails with the old fixed cap of 16** (`MaxPlaceholders = 16` → `ErrCapped`),
then that it succeeds under the new cap. `TestCappedFailsClosed` needs 70 + 8, is capped at exactly 64 and moves
`vrx_agent_iface_sanitize_capped_total{phase="create"}`.

### Host (shared VPP, slot 2, one package, no packets). NRestarts before and after every run
Run 1 (14:32) failed closed on the sanitize cap, before my D1 fix. The Create was rolled back **with the quiesce** (the log line
`af_packet quiesce: netdev down before af_packet_delete`), and NRestarts stayed 0:
```
$ date; systemctl show vpp -p NRestarts
2026-09-24T14:31:59+03:30
NRestarts=0
$ cd apps/agent && VRX_INTEGRATION=1 flock -s /run/lock/vrx-lab.lock go test -count=1 -v -run 'OnHost' ./internal/descriptors/af_packet/
=== RUN   TestHostInterfaceOnHost
2026/09/24 14:32:02 WARN interface sanitize: placeholder cap reached (VPP V19); the create fails closed sw_if_index=3 placeholders=23 cap=23 holes_seen=7 holes_left=0 fresh_run=5
2026/09/24 14:32:02 ERROR interface sanitize failed (VPP V19) interface=w2-af50 sw_if_index=3 phase=create err="no clean sw_if_index obtained (VPP V19 quarantine): placeholder cap reached before every freed classify table index was resurrected (23 placeholders, cap 23 for 7 freed indices seen)" cleared=[] freed=[] unclearable=[] placeholders=23 capped=true rereads=3
2026/09/24 14:32:02 INFO af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24) netdev=w2-af50 descriptor=af-packet.host-interface ifindex=53
    integration_test.go:97: timing: Create af-packet.host-interface/w2-af50 took 696ms (err sanitize w2-af50 (sw_if_index 3, create): no clean sw_if_index obtained (VPP V19 quarantine): placeholder cap reached before every freed classify table index was resurrected (23 placeholders, cap 23 for 7 freed indices seen))
    integration_test.go:99: Create af-packet.host-interface/w2-af50: sanitize w2-af50 (sw_if_index 3, create): no clean sw_if_index obtained (VPP V19 quarantine): placeholder cap reached before every freed classify table index was resurrected (23 placeholders, cap 23 for 7 freed indices seen)
--- FAIL: TestHostInterfaceOnHost (1.11s)
FAIL
FAIL	ngfw/agent/internal/descriptors/af_packet	1.235s
FAIL
exit=1
$ date; systemctl show vpp -p NRestarts
2026-09-24T14:32:03+03:30
NRestarts=0
```
Run 2 (14:35), after D1 (TX ring only at that point):
```
$ date; systemctl show vpp -p NRestarts
2026-09-24T14:35:10+03:30
NRestarts=0
$ cd apps/agent && VRX_INTEGRATION=1 flock -s /run/lock/vrx-lab.lock go test -count=1 -v -run 'OnHost' ./internal/descriptors/af_packet/
=== RUN   TestHostInterfaceOnHost
2026/09/24 14:35:15 INFO interface sanitized (VPP V19/V21 inherited state) interface=w2-af50 sw_if_index=3 phase=create cleared=[] freed=[] placeholders=26 rereads=3 reset="[l2-mode l3 ip-classify ip4 ip-classify ip6 l2-classify input l2-classify output adl adl-input vxlan-bypass ip4 vxlan-bypass ip6]" skipped=[]
    integration_test.go:97: timing: Create af-packet.host-interface/w2-af50 took 167ms (err <nil>)
    integration_test.go:112: timing: Retrieve took 1ms (err <nil>)
    integration_test.go:120: af-packet.host-interface: Retrieve == desired: af-packet.host-interface/w2-af50 name:"w2-af50" host_if_name:"w2-af50"
2026/09/24 14:35:15 INFO interface sanitized (VPP V19/V21 inherited state) interface=w2-af50p sw_if_index=1 phase=create cleared=[] freed=[] placeholders=26 rereads=3 reset="[l2-mode l3 ip-classify ip4 ip-classify ip6 l2-classify input l2-classify output adl adl-input vxlan-bypass ip4 vxlan-bypass ip6]" skipped=[]
    integration_test.go:97: timing: Create af-packet.host-interface/w2-af50p took 180ms (err <nil>)
    integration_test.go:112: timing: Retrieve took 1ms (err <nil>)
    integration_test.go:120: af-packet.host-interface: Retrieve == desired: af-packet.host-interface/w2-af50p name:"w2-af50p" host_if_name:"w2-af50p" mode:MODE_IP
    integration_test.go:122: vppctl show interface (after Create):
                      Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count     
        host-w2-af50                      3     down         9000/0/0/0     
        host-w2-af50p                     1     down          0/0/0/0       
2026/09/24 14:35:15 INFO af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24) netdev=w2-af50 descriptor=af-packet.host-interface ifindex=55
2026/09/24 14:35:15 INFO interface sanitized (VPP V19/V21 inherited state) interface=w2-af50 sw_if_index=3 phase=delete cleared=[] freed=[] placeholders=0 rereads=0 reset="[l2-mode l3 ip-classify ip4 ip-classify ip6 l2-classify input l2-classify output adl adl-input vxlan-bypass ip4 vxlan-bypass ip6]" skipped=[]
    integration_test.go:127: timing: Delete af-packet.host-interface/w2-af50 took 256ms (err <nil>)
    integration_test.go:134: after Delete af-packet.host-interface/w2-af50: netdev w2-af50 reads down (net.FlagUp clear)
2026/09/24 14:35:15 INFO af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24) netdev=w2-af50p descriptor=af-packet.host-interface ifindex=54
2026/09/24 14:35:15 INFO interface sanitized (VPP V19/V21 inherited state) interface=w2-af50p sw_if_index=1 phase=delete cleared=[] freed=[] placeholders=0 rereads=0 reset="[l2-mode l3 ip-classify ip4 ip-classify ip6 l2-classify input l2-classify output adl adl-input vxlan-bypass ip4 vxlan-bypass ip6]" skipped=[]
    integration_test.go:127: timing: Delete af-packet.host-interface/w2-af50p took 247ms (err <nil>)
    integration_test.go:134: after Delete af-packet.host-interface/w2-af50p: netdev w2-af50p reads down (net.FlagUp clear)
    integration_test.go:136: timing: Retrieve took 1ms (err <nil>)
    integration_test.go:151: after Delete: Retrieve has neither key; vppctl show interface has no host-w2-af50*:
                      Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count     
--- PASS: TestHostInterfaceOnHost (0.97s)
PASS
ok  	ngfw/agent/internal/descriptors/af_packet	0.996s
exit=0
$ date; systemctl show vpp -p NRestarts
2026-09-24T14:35:15+03:30
NRestarts=0
```
Run 3 (14:41), final code (both rings, D-108). This is the acceptance run. `show hardware-interfaces` shows RX `block size:16384 nr:160
frame size:2048` and TX `block size:524288 nr:1 frame size:2048 nr:256`. Timings: Create 181 / 218 ms, Delete 252 / 263 ms (200 ms of
each Delete is the settle). No stalls.
```
$ eval "$(tools/lab env 2)"; date -Is; systemctl show vpp -p NRestarts
2026-09-24T14:41:12+03:30
NRestarts=0
$ cd apps/agent && VRX_INTEGRATION=1 flock -s /run/lock/vrx-lab.lock go test -count=1 -v -run 'OnHost' ./internal/descriptors/af_packet/
=== RUN   TestHostInterfaceOnHost
2026/09/24 14:41:14 INFO interface sanitized (VPP V19/V21 inherited state) interface=w2-af50 sw_if_index=1 phase=create cleared=[] freed=[] placeholders=26 rereads=3 reset="[l2-mode l3 ip-classify ip4 ip-classify ip6 l2-classify input l2-classify output adl adl-input vxlan-bypass ip4 vxlan-bypass ip6]" skipped=[]
    integration_test.go:97: timing: Create af-packet.host-interface/w2-af50 took 181ms (err <nil>)
    integration_test.go:112: timing: Retrieve took 1ms (err <nil>)
    integration_test.go:120: af-packet.host-interface: Retrieve == desired: af-packet.host-interface/w2-af50 name:"w2-af50" host_if_name:"w2-af50"
2026/09/24 14:41:15 INFO interface sanitized (VPP V19/V21 inherited state) interface=w2-af50p sw_if_index=3 phase=create cleared=[] freed=[] placeholders=26 rereads=3 reset="[l2-mode l3 ip-classify ip4 ip-classify ip6 l2-classify input l2-classify output adl adl-input vxlan-bypass ip4 vxlan-bypass ip6]" skipped=[]
    integration_test.go:97: timing: Create af-packet.host-interface/w2-af50p took 218ms (err <nil>)
    integration_test.go:112: timing: Retrieve took 1ms (err <nil>)
    integration_test.go:120: af-packet.host-interface: Retrieve == desired: af-packet.host-interface/w2-af50p name:"w2-af50p" host_if_name:"w2-af50p" mode:MODE_IP
    integration_test.go:122: vppctl show interface (after Create):
                      Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count     
        host-w2-af50p                     3     down          0/0/0/0       
        host-w2-af50                      1     down         9000/0/0/0     
    integration_test.go:123: vppctl show hardware-interfaces host-w2-af50 (D-108 rings):
                      Name                Idx   Link  Hardware
        host-w2-af50                       1     up   host-w2-af50
          Link speed: unknown
          RX Queues:
            queue thread         mode      
            0     main (0)       interrupt 
          TX Queues:
            TX Hash: [name: hash-eth-l34 priority: 50 description: Hash ethernet L34 headers]
            queue shared thread(s)      
            0     no     0
          Ethernet address 02:fe:0e:90:02:14
          Linux PACKET socket interface v3
          FEATURES:
          Host Interface Offload:
            creation time:
             rx checksum
             tx checksum
             tcp segmentation offload
             generic segmentation offload
            now:
             rx checksum
             tx checksum
             tcp segmentation offload
             generic segmentation offload
          RX Queue 0:
            block size:16384 nr:160  frame size:2048 nr:1280 next block:0
          TX Queue 0:
            block size:524288 nr:1  frame size:2048 nr:256 next frame:0
            available:256 request:0 sending:0 wrong:0 total:256
2026/09/24 14:41:15 INFO af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24) netdev=w2-af50 descriptor=af-packet.host-interface ifindex=57
2026/09/24 14:41:15 INFO interface sanitized (VPP V19/V21 inherited state) interface=w2-af50 sw_if_index=1 phase=delete cleared=[] freed=[] placeholders=0 rereads=0 reset="[l2-mode l3 ip-classify ip4 ip-classify ip6 l2-classify input l2-classify output adl adl-input vxlan-bypass ip4 vxlan-bypass ip6]" skipped=[]
    integration_test.go:128: timing: Delete af-packet.host-interface/w2-af50 took 252ms (err <nil>)
    integration_test.go:135: after Delete af-packet.host-interface/w2-af50: netdev w2-af50 reads down (net.FlagUp clear)
2026/09/24 14:41:15 INFO af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24) netdev=w2-af50p descriptor=af-packet.host-interface ifindex=56
2026/09/24 14:41:15 INFO interface sanitized (VPP V19/V21 inherited state) interface=w2-af50p sw_if_index=3 phase=delete cleared=[] freed=[] placeholders=0 rereads=0 reset="[l2-mode l3 ip-classify ip4 ip-classify ip6 l2-classify input l2-classify output adl adl-input vxlan-bypass ip4 vxlan-bypass ip6]" skipped=[]
    integration_test.go:128: timing: Delete af-packet.host-interface/w2-af50p took 263ms (err <nil>)
    integration_test.go:135: after Delete af-packet.host-interface/w2-af50p: netdev w2-af50p reads down (net.FlagUp clear)
    integration_test.go:137: timing: Retrieve took 1ms (err <nil>)
    integration_test.go:152: after Delete: Retrieve has neither key; vppctl show interface has no host-w2-af50*:
                      Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count     
--- PASS: TestHostInterfaceOnHost (1.05s)
PASS
ok  	ngfw/agent/internal/descriptors/af_packet	1.084s
exit=0
$ date -Is; systemctl show vpp -p NRestarts
2026-09-24T14:41:15+03:30
NRestarts=0
```
VPP journal for the same window. Each link-down is followed by the socket error, and `errno 9` still appears on every delete (D-107):
```
$ journalctl -u vpp --since "2026-09-24 14:31:55" --until "2026-09-24 14:41:30" --no-pager | grep -E "af_packet_fd_error|epoll_ctl" | grep -v "vpp\[[0-9]*\]: vpp\["
Sep 24 14:32:02 ubuntu-26.04 vpp[8760]: af_packet: fd 28 reason af_packet_fd_error: Network is down
Sep 24 14:32:02 ubuntu-26.04 vpp[8760]: vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w2-af50 queue 0' (fd 28), errno 9
Sep 24 14:35:15 ubuntu-26.04 vpp[8760]: af_packet: fd 28 reason af_packet_fd_error: Network is down
Sep 24 14:35:15 ubuntu-26.04 vpp[8760]: vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w2-af50 queue 0' (fd 28), errno 9
Sep 24 14:35:15 ubuntu-26.04 vpp[8760]: af_packet: fd 29 reason af_packet_fd_error: Network is down
Sep 24 14:35:15 ubuntu-26.04 vpp[8760]: vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w2-af50p queue 0' (fd 29), errno 9
Sep 24 14:36:31 ubuntu-26.04 vpp[8760]: af_packet: fd 29 reason af_packet_fd_error: Network is down
Sep 24 14:36:32 ubuntu-26.04 vpp[8760]: vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w11a0 queue 0' (fd 29), errno 9
Sep 24 14:41:15 ubuntu-26.04 vpp[8760]: af_packet: fd 28 reason af_packet_fd_error: Network is down
Sep 24 14:41:15 ubuntu-26.04 vpp[8760]: vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w2-af50 queue 0' (fd 28), errno 9
Sep 24 14:41:15 ubuntu-26.04 vpp[8760]: af_packet: fd 29 reason af_packet_fd_error: Network is down
Sep 24 14:41:15 ubuntu-26.04 vpp[8760]: vlib/file: vlib_file_update: epoll_ctl() failed on epfd (8), file 'host-w2-af50p queue 0' (fd 29), errno 9
```
Host af_packet cycles in total: 5 creates and 5 deletes on w2 (run 1: 1, whose rollback was quiesced; runs 2 and 3: 2 each), all
quiesced. There were no other host runs.

### Inventory: every path that deletes af_packet interfaces (scope item 5)
```
$ grep -rnI -e AfPacketDelete -e af_packet_delete -e "delete host-interface" --exclude-dir={node_modules,.git,binapi,docs,prompts,plan} . | grep -v -e "^apps/agent/internal/descriptors/af_packet/[a-z_]*_test.go:" -e "^[^:]*:[0-9]*:\s*//"
apps/agent/internal/descriptors/af_packet/quiesce.go:39:var ErrQuiesce = errors.New("af_packet: host netdev not quiesced, af_packet_delete not sent (D-101, VPP V24)")
apps/agent/internal/descriptors/af_packet/quiesce.go:72:		log.Error("af_packet quiesce failed: af_packet_delete not sent (D-101, VPP V24)", "step", step, "err", err)
apps/agent/internal/descriptors/af_packet/quiesce.go:107:	log.Info("af_packet quiesce: netdev down before af_packet_delete (D-101, VPP V24)", "ifindex", idx)
apps/agent/internal/descriptors/af_packet/quiesce.go:123:	if _, err := d.svc().AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: dev}); err != nil {
apps/agent/internal/descriptors/af_packet/quiesce.go:124:		return fmt.Errorf("af_packet_delete %s: %w", dev, err)
tools/lab:349:    # V24/D-101: quiesce the socket first — af_packet_delete_if closes the fds before clib_file_del, so the epoll DEL fails
tools/lab:352:    vpp_cli_ok delete host-interface name "$h" >/dev/null && echo "  delete vpp $vif" || warn "rig: VPP could not delete $vif"; fi
tools/lab:405:        vpp_cli_ok delete host-interface name "${n#host-}" >/dev/null && echo "  delete vpp $n" || warn "could not delete $n"; done
$ grep -rlI -e AfPacketDelete -e af_packet_delete --include=*_test.go apps/agent   # test files: fake VPP handlers, the guard's planted source, the host test (through the descriptor)
apps/agent/internal/descriptors/af_packet/sanitize_test.go
apps/agent/internal/descriptors/af_packet/host_interface_test.go
apps/agent/internal/descriptors/af_packet/guard_test.go
apps/agent/internal/descriptors/af_packet/quiesce_test.go
apps/agent/internal/descriptors/af_packet/integration_test.go
$ grep -n "HostInterface{\|d.Delete(\|func lose\|restartVeth(t" apps/agent/internal/descriptors/interface/restart_integration_test.go
183:func restartVeth(t *testing.T, name, peer string) {
225:	restartVeth(t, af, afPeer)
237:		&afpacket.HostInterface{Name: af, HostIfName: af},
296:			if err := d.Delete(context.Background(), kv.Value, metas[kv.Key]); err != nil {
395:		if err := d.Delete(ctx, kv.Value, actual[kv.Key].Meta); err != nil {
420:func lose(t *testing.T, c vpp.Client, actual map[scheduler.Key]scheduler.KV, tapRef, memifRef, sockFile string, sockID uint32, l3xcTap string) []string {
$ git grep -n -e "AfPacketDelete(" -e "peers(t, false)" -e "deleteBehindBack(t" task/P08 -- test/   # P08 is not merged
task/P08:test/topology/interfaces/interfaces_test.go:222:	r.peers(t, false)
task/P08:test/topology/interfaces/interfaces_test.go:223:	for _, l := range deleteBehindBack(t, conn, r.lanDev, r.wanDev) {
task/P08:test/topology/interfaces/interfaces_test.go:394:		r.peers(t, false) // no packet may enter re-created interfaces before the V19 guard
task/P08:test/topology/interfaces/interfaces_test.go:395:		for _, l := range deleteBehindBack(t, conn, r.lanDev, r.wanDev) {
task/P08:test/topology/interfaces/interfaces_test.go:495:		r.peers(t, false) // D-101: the agent deletes the af_packet interfaces only with their veths down
task/P08:test/topology/interfaces/shots_test.go:41:	r.peers(t, false)
task/P08:test/topology/interfaces/shots_test.go:42:	deleteBehindBack(t, conn, r.lanDev, r.wanDev)
task/P08:test/topology/interfaces/shots_test.go:97:	r.peers(t, false)
task/P08:test/topology/interfaces/vpp_test.go:155:func deleteBehindBack(t *testing.T, conn vppapi.Connection, netdevs ...string) []string {
task/P08:test/topology/interfaces/vpp_test.go:170:		if _, err := afpapi.NewServiceClient(conn).AfPacketDelete(ctx, &afpapi.AfPacketDelete{HostIfName: nd}); err
```
| Path | What it does | Quiesce status |
|---|---|---|
| `apps/agent/internal/descriptors/af_packet/quiesce.go:123` `quiescedDelete` | the agent's only `af_packet_delete`: Delete (`host_interface.go`) and Create's rollback | **quiesces** (TD-5), guarded by `TestEveryAfPacketDeleteIsQuiesced` |
| `apps/agent/internal/descriptors/af_packet/integration_test.go` | host test: deletes through the descriptor, and its Cleanup only through the descriptor | **fixed here**: it used to delete with the veth up in Cleanup; now it quiesces via the descriptor and asserts the netdev is down |
| `apps/agent/internal/descriptors/interface/restart_integration_test.go:237/296/395` (TD-3/DF-1, not mine) | creates `host-<af>` on an up veth; deletes only through the af_packet descriptor; `lose()` (`:420`) deletes tap and memif, not af_packet | **quiesces** through TD-5's Delete, with no file change (note in questions Q4: its veth has no `disable_ipv6`) |
| `tools/lab:349-352` `rig_side_down`, `:402-405` `rig gc` (manager) | `ip link set <veth> down; sleep 0.2`, then `delete host-interface` | **quiesces** |
| `test/topology/interfaces` on `task/P08` (not merged): `vpp_test.go:170` `deleteBehindBack` via binapi; agent deletes through the API | `r.peers(t, false)` (host veths down) before each `deleteBehindBack` (`interfaces_test.go:222/394`, `shots_test.go:41`) and before the API deletes (`:495`, `shots_test.go:97`) | **quiesces** (link down, no explicit settle); once TD-5 is merged the agent side quiesces by itself |
| `*_test.go` in `af_packet/` (fake VPP handlers, the guard's planted source) | no real VPP | n/a |
| `apps/agent/binapi/af_packet` | the generated message definition | n/a (sends nothing) |

No file outside my ownership deletes an af_packet interface without quiescing, so there is nothing to escalate.

### `make lint test` (apps/agent)
This ran at 14:38, after the lint fixes and before the RX-ring commit and the second main merge. The CI gate below re-ran
`apps/agent: make lint test build` on the final tip `3381e3e` and it was green.
```
$ cd apps/agent && time make lint test
go vet ./...
0 issues.
go test -race -count=1 ./...

real	1m28.675s
user	10m57.172s
sys	10m7.777s
exit=0
(… 88 packages `ok`, lines elided)
```

### CI gate
```
$ TMPDIR=/tmp/g-td5 tools/ci.sh --base main
== install (pnpm --frozen-lockfile --prefer-offline) ==
Progress: resolved 593, reused 593, downloaded 0, added 593, done Done in 18.1s using pnpm v12.5.1 

== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated

== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~67419 bytes (67.42 KB) in 1.3s no leaks found 

== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    24 cached, 30 total Time:    2m15.318s  

== apps/agent: make lint test build ==
ok  	ngfw/agent/cmd/vrx-startupgen	1.442s; ok  	ngfw/agent/cmd/vrx-vppcheck	1.868s; ok  	ngfw/agent/internal/agent	15.266s; ok  	ngfw/agent/internal/contracttest	8.388s; ok  	ngfw/agent/internal/descriptors/abf	1.365s; ok  	ngfw/agent/internal/descriptors/acl	1.596s; ok  	ngfw/agent/internal/descriptors/adl	1.268s; ok  	ngfw/agent/internal/descriptors/af_packet	13.948s; ok  	ngfw/agent/internal/descriptors/arp	1.328s; ok  	ngfw/agent/internal/descriptors/bfd	1.427s; ok  	ngfw/agent/internal/descriptors/bond	1.568s; ok  	ngfw/agent/internal/descriptors/classify	2.756s; 

== apps/cli: make lint test build ==
ok  	ngfw/cli/internal/api	1.348s; ok  	ngfw/cli/internal/cli	1.932s; ok  	ngfw/cli/internal/cpath	1.093s; ok  	ngfw/cli/internal/jschema	1.208s; ok  	ngfw/cli/internal/render	1.124s; ok  	ngfw/cli/internal/safe	1.093s; ok  	ngfw/cli/test/e2e	1.078s; 

== test/ Go modules, unit mode (test/integration/smoke) ==
test/integration/smoke: gofmt ok · go vet ok · ok  	ngfw/test/integration/smoke	0.016s; 
integration tests inside these modules skip here (VRX_INTEGRATION unset); 'tools/ci.sh full' runs them on the CI slot

== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m18s
  generate + generated-output gate                   4m30s
  forbidden patterns (+ gitleaks)                    0m05s
  lint · typecheck · unit tests · build (turbo)   2m17s
  apps/agent: make lint test build                   1m24s
  apps/cli: make lint test build                     0m17s
  test/ Go modules, unit mode (test/integration/smoke)   0m03s
  mode quick · wall time 8m58s · logs /root/ngfw-wt/logs/ci/TD-5-20260924-144132-365121

CI GATE PASSED
exit=0
```

## Out of scope (not built)
The VPP fix (V24 stays in `docs/vpp-code-track.md`; only the agent-side status is appended); other interface types; ifsanitize beyond
the cap; `descriptors/interface/**`; `iface.AcquireAndTag` internals; `tools/lab`; `test/topology/**`; bringing the netdev back up
(VPP does it on create); the management-NIC deny-list; the systemd unit and capabilities (P10); any new Go dependency; packet tests.

## Open questions and decisions
See `docs/status/tasks/TD-5-questions.md`:
- Q1: fail closed for the rollback orphan (done as specified; please confirm).
- Q2: 200 ms settle.
- **D1**: what "holes seen" counts in the cap (decided (b); please confirm).
- Q3: a capped classify pool is sticky because the pop order replays (for M3).
- Q4: inventory notes.
- Q5: minor items (`%v` wrapping in AcquireAndTag; CAP_NET_ADMIN; an unformatted file on main).
