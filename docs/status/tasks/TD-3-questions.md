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
