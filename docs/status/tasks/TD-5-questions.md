# TD-5 — questions and decisions (for the manager)

Nothing here blocks the branch; each item states what I did.

## Q1. Fail closed for the Create-rollback orphan? (open question from the prompt)
When Create's rollback cannot bring the netdev down (EPERM, netlink timeout, still up), the rollback does **not** send
`af_packet_delete`. The untagged `host-<dev>` stays in VPP and the Create error names it (`untagged orphan host-<dev>
(sw_if_index N) left in VPP: … D-101, VPP V24 …`). Retrieve does not see it, because it is untagged. **Did:** fail closed, as the prompt says.
Options: (a) fail closed and leave a named orphan (chosen); (b) delete anyway, which risks the V24 crash on the shared VPP; (c) tag it
`orphan:<owner>` so Retrieve/GC can find it later, but tagging is exactly what just failed in the tag-failure case. An operator removes the
orphan after bringing the netdev down (`tools/lab rig gc` does this for rig names). Please confirm (a).

## Q2. Settle = 200 ms (open question from the prompt)
**Did:** 200 ms, the same as `tools/lab rig_side_down`. I picked it and did not tune it (no performance work). After the link-down VPP logs one
`af_packet_fd_error: Network is down` per queue (the handler only reads the socket error, `af_packet.c:165-183`). It lands inside the
settle, while the file is still registered. The quiesce does not drain frames the kernel already queued: frames are no longer *delivered*
to a down netdev, and the settle only lets VPP's main loop run the pending error/read events first. Per D-107 the double close
(`errno 9`) remains on every delete in any case (host run 14:35 and 14:41, pasted in TD-5.md).

## D1. Decision taken: what "holes seen" counts in the D-105 cap
The cap is `holes + 2 × FreshRun ≤ 64` (D-105). My first reading counted only the *gaps* (non-live indices below the highest index seen,
plus input-ACL tables). On the shared VPP it failed closed on the **first host run** (14:32:02: `placeholders=23 cap=23 holes_seen=7
holes_left=0 fresh_run=5 rereads=3`). The run-1 log in TD-5.md shows that this Create's rollback then quiesced before
`af_packet_delete`. Cause: `dropPlaceholders` frees in reverse creation order, so the next run pops **the previous run's sequence**. A free
list with fresh-looking ascending runs above the highest live table therefore replays on every run, and with a gap-only count **every**
interface Create on that VPP stayed capped until the free list changed.
Options: (a) gaps only: sticky cap on the shared VPP; (b) also count every pop that the pop order proves came from the free list
(a pop below the highest index seen, and the ascending run that such a pop or a gap interrupts, because growth stays consecutive), chosen;
(c) change the resurrect algorithm itself, which is out of scope ("only the cap"). With (b) the host run passed (26 placeholders), and
`TestReplayedAscendingRunsAreCounted` reproduces the pattern and shows that two consecutive runs both succeed. It is still fail closed at the
cap and at 64. The capped WARN now logs the pop sequence (`pops`) for diagnosis. Please confirm (b) as the meaning of D-105 "holes the run has seen".

## Q3. Observation for TD-3 re-review M3 / tech-debt: a capped classify pool is sticky
Because of the replay (D1), a pool that exceeds the cap (64) fails every interface Create on that VPP, on every slot and in the product,
until something else changes the classify free list or VPP restarts. The scheduler's retry does not help, because each retry pops the same
64. This is a property of the M1 design, not of TD-5. It belongs to M3 (exact readback). I am noting it here and did not change it.

## Q4. Inventory: files I do not own
No non-quiescing af_packet delete path remains in files I do not own. The details are in TD-5.md. For information only:
- `apps/agent/internal/descriptors/interface/restart_integration_test.go` (TD-3/DF-1) creates an af_packet interface on a veth that is up and
  deletes it only through the af_packet descriptor (`:296`, `:395`), so TD-5's quiesce now covers it without a file change. Its
  `restartVeth` does **not** set `disable_ipv6` before `up`, so IPv6 RS/MLD frames can reach VPP during that test. That is not a delete
  path. I am noting it for its owner.
- P08 `test/topology/interfaces` (task/P08) quiesces via `r.peers(t, false)` before `deleteBehindBack` and before agent deletes, without an
  explicit settle. Once TD-5 is merged, the agent-side deletes quiesce by themselves, and `peers(false)` before them becomes a harmless no-op.

## Q5. Minor
- `iface.AcquireAndTag` and `ifsanitize.Acquire` add the rollback error with `%v`. `errors.Is(err, afpacket.ErrQuiesce)` therefore does not work
  on a Create error, and callers can only match the text. `descriptors/interface/**` is TD-3's, so I did not change it. The unit test matches the text.
- The product agent needs `CAP_NET_ADMIN` (P10's unit). Without it every af_packet Delete fails closed with EPERM.
- `apps/agent/internal/descriptors/core/coretest/fakevpp.go` is not gofmt-clean on main (`gofmt -l`). It is not mine and I did not touch it.

## Fix round 1 — decisions taken (review e8f3d63)
- **F1 (L1, the reviewer left it to the manager):** implemented the reviewer's fix, limited to the cases where the interface is
  *provably* still in VPP: `ifsanitize.BeforeDelete` failed, or the quiesce failed after its own link-down (confirm/settle).
  Then the netdev is brought up again (best effort, WARN). A failed `af_packet_delete` leaves the netdev down with a WARN,
  because its outcome is unknown (a timed-out request may still run in VPP) and bringing a netdev up under a half-deleted
  interface is the V24 risk again. Options: (a) restore on provable presence + WARN otherwise (chosen); (b) WARN only;
  (c) also restore after a failed `af_packet_delete` after an `af_packet_dump` check (another API call on a possibly stalled VPP).
- **F2 (L3):** the veth-only check applies only when the quiesce would bring a netdev **down**. An already-down non-veth
  netdev has its delete go ahead (no frames arrive, nothing is taken down). Refusing it too would leave an interface that
  can never be deleted by the agent. A missing or malformed `IFLA_LINKINFO` reads as "not a veth" (fail closed).
- **F3 (L2):** `RollbackTimeout` = 30 s (the reviewer's value; chosen, not tuned). Under an I6-class stall longer than that
  the rollback fails and names the orphan as before; the D-113 rings make those stalls unlikely (host run: create 4.6 s, delete 0.3 s).
