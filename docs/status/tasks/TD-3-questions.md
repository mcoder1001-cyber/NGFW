# TD-3 — questions / incidents for the manager

## INCIDENT — VPP restarted during fix round 1: NRestarts 5 → 6 at 07:27:32 (cause not established)

Written down as the envelope requires (V19 SAFETY: "stop and write it down if it rises").

**Facts (journalctl -u vpp, `systemctl show vpp`):**

```
07:27:08  NRestarts=5 (my check before the first host run)
07:27:22  vpp[2808617]: vlib_file_update: epoll_ctl() failed on epfd (7), file 'host-w1l0 queue 0' (fd 24), errno 9
07:27:22  vpp[2808617]: vlib_file_update: epoll_ctl() failed on epfd (7), file 'host-w1w0 queue 0' (fd 25), errno 9
07:27:23  my run 1: TestV19InheritanceClearedOnHost PASS, TestV19FreedTableOnHost FAIL at its first readback
          (l2_flags_get → INVALID_SW_IF_INDEX: it only answers for bridged/xconnected interfaces). Left behind:
          loop285 (sw_if_index 2, w2-tagged, admin-down, no address, L3 mode) with L2 input ACL / L2+ip4 output ACL /
          L2 policer classify bound to classify table 1, which was ALIVE. Nothing pointed at a freed table.
07:27:2x  my read-only vppctl: show interface | grep loop; show inacl/outacl type l2; show classify tables (all answered)
07:27:3x  my vppctl "show interface features loop285" printed nothing (no l2-input section, no error)
07:27:32  vpp[2808617]: received signal SIGSEGV, PC 0x0, faulting address 0x0 → systemd restart, NRestarts=6
```

No packet was sent by me at any time (loopbacks only, admin-down, no address).

**Analysis (not proven):**
- PC 0x0 means VPP called a NULL function pointer. That is not the V19 crash (`vnet_classify_find_entry`, a data read). The
  classify state I had in VPP had only live tables.
- `vnet_interface_features_show` / `format_l2_input_features` (`show interface features`) call no function pointers (read
  `feature/feature.c:536-594`, `l2/l2_input.c:87-104`); the same CLI ran fine in run 2 through `cli_inband` on the same
  kind of interface (output pasted in TD-3.md). So the CLI itself is an unlikely cause, but its timing matches: the command
  printed nothing, which is what vppctl does when VPP dies under it.
- 10 s earlier slot 1 (P08, `host-w1l0`/`host-w1w0`) deleted af_packet interfaces; `af_packet_delete_if` closes the fds
  before `clib_file_del_by_index` (`plugins/af_packet/af_packet.c:895-900` then `:827`) — the errno 9 lines. `host-vrx-a.md`
  calls that "harmless log noise"; a NULL read_function called from the file poller when a new file (e.g. a vppctl CLI socket)
  reuses fd 24/25 would also give PC 0x0. This is a hypothesis for the manager/VPP track (worth a V-item), not a finding.
- No core dump (`coredumpctl list` empty), no backtrace in the journal.

**What I did:** stopped host runs after noticing (I saw 6 printed at the start of run 2 but noticed only after it finished;
run 2 passed and left NRestarts at 6). No further host runs in this round. The pre-flight after the restart: `V19 pre-flight
ok (0 warning(s))`. The fresh VPP means the other slots' state was lost at 07:27:32 as well — please tell P08 (slot 1).

**Question:** should the af_packet close-before-file-del sequence be a vpp-code-track item (possible NULL read_function call on
fd reuse)? I cannot prove it and did not try to reproduce (that would mean crashing VPP on purpose).

## Q2 — quarantine release is a library call, not wired into the agent loop

`ifsanitize.Release(ctx, client, owner)` re-sanitizes the owner's `quarantine:<owner>` holders and deletes the clean ones
(host-tested). Calling it periodically / at agent start belongs in `apps/agent/internal/agent` (P08 is wiring interface
creation there; not my files). Proposal: call it once after the initial reconcile and on every full resync. Until then a
quarantine holder stays until something calls Release — harmless (admin-down, never used, it owns the dirty index so no creator
gets it), counted in `vrx_agent_iface_quarantined` by the process that made it (the gauge restarts at 0 with the agent; the
pre-flight still reports every holder as WARN).

## Fix round 2 (2026-09-24 08:xx)

### CONTRACT — reserve loop16000–loop16383 in packages/schema (re-review M2)

Quarantine holders now take the highest free loopback instance of **16000–16383** (VPP's `LOOPBACK_MAX_INSTANCE` is 16384,
`vnet/ethernet/interface.c:740`), never VPP's lowest free one, so a holder can no longer become `loop0`. The schema does
**not** reject these names today: `vppInterfaceName` (`packages/schema/src/primitives.ts:105`) accepts any
`^[A-Za-z][A-Za-z0-9_-]*…` name, so `loop16000`…`loop16383` validate without error. Proposal for a `contract/<id>` branch
(additive, decision-policy: not a reshape): a semantic check on the interfaces map (or a refinement of the loopback key) that
refuses `loop<N>` with 16000 ≤ N ≤ 16383 — pointer `/interfaces/loop16000`, message "loop16000–loop16383 are reserved for the
agent (quarantine holders, VPP V19)". I did not change the schema (not in my files; contracts go through the manager). Until
then the range is documented as reserved in `docs/agent/descriptors/interface.md`; a user `loop16383` only pushes holders down to
16382…, and a holder sitting on an instance a user later configures makes that loopback's Create fail ("instance in use") until
`Release` (P08) or a VPP restart.

### M1 — how "fail closed on cap" is implemented (my reading of the envelope; please confirm)

The envelope says "fail closed (ErrNoCleanIndex → quarantine path) on cap". Implemented:
- `MaxPlaceholders` = 16 per create. A create-phase run that reaches it before proving the pool's free list empty returns
  `ErrCapped` (wraps `ErrNoCleanIndex`) and is counted in `vrx_agent_iface_sanitize_capped_total{phase="create"}`.
- `Acquire` deletes the interface and fails the Create. It quarantines the index **only if the run also proved a binding
  unclearable** (`ErrUnclearable`, e.g. an input ACL naming a freed table the cap kept us from reaching) — and then does
  **not** retry.
- Why not "always quarantine + retry": the cap is a property of the classify pool (a long free list), not of the index. Every
  retry would be capped again, so each failed Create would park up to `MaxAcquireAttempts` (4) holders on indices that are not
  known to be dirty, and every reconcile retry would park 4 more — an unbounded holder leak while `Release` is not wired (Q2).
  With this rule a failed Create makes at most one holder, and only for a proven-dirty index.
- **Liveness trade-off (please decide):** with a cap of 16 and `FreshRun` 8, a free list of ≳ 9 indices that do not pop in
  ascending order (e.g. ≥ 9 classify tables deleted in creation order and not reused) makes **every** interface Create on that
  VPP fail until tables are created again (reusing the freed indices) or VPP restarts. On the shared lab VPP that can be caused
  by another slot's test. Before round 2 the cap was 256 (the run then took ~20 placeholders and succeeded). Options: (a) keep
  16 + fail closed (current; safest against a silent ACL bypass, worst for liveness); (b) cap = holes + 2 × FreshRun, max 64
  (the reviewer's suggestion); (c) on cap, fail closed only if a write-only binding kind is actually in use on this VPP.
  M3's exact readback (tech-debt) removes the trade-off. The knob is `ifsanitize.MaxPlaceholders`.
