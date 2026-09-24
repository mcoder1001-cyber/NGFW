# P08 — questions / incidents for the manager

## Q1 — INCIDENT: VPP restarted (NRestarts 5 → 6) at 07:27:32 during P08's second topology run — ANSWERED by D-101 (V24)

**Resolution (manager, D-101):** not TD-3 — V24, af_packet_delete with the veth up double-closes the socket fds; P08's test had
deleted the rig's host-w1l0/host-w1w0 with their host-side veths up 10 s before the crash. Fixed in the test: every af_packet
delete (hand-over from the rig, simulated loss, the agent's delete commit) now happens with both veths of the pair down
(`rig.peers(false)`), and main's `tools/lab rig down/gc` quiesce as well (merged 21c81a4). My first reading below is kept for the record.

Timeline (journalctl -u vpp, P08 test log):
- 07:27:12 P08 run 2 starts (slot 1, rig w1). 07:27:22 the rig's untagged VPP side is removed (af_packet_delete w1l0/w1w0,
  freeing sw_if_index 5 and 2); the P08 agent re-creates host-w1l0 = 5, host-w1w0 = 4 from the committed config.
- 07:27:16 TD-3 commits 6c4e11a "… host test for freed tables"; 07:27:23 VPP logs 508 lines
  `vnet_set_flow_classify_intfc / vnet_set_policer_classify_intfc: Non-existent intf_idx=2 with table_index=0..9 for delete`
  — a sanitizer run on sw_if_index **2** (not a P08 interface; P08 registers no classify/policer descriptor).
- P08: V19 guard on 5 and 4 clean (no input classify, no ACL, no SPD; write-only ip/l2 tables reset to ~0); ping OK (2/3),
  one traced ping OK, `show trace` then returned nothing, the next 20 pings got no reply.
- 07:27:32 `received signal SIGSEGV, PC 0x0, faulting address 0x0`; systemd restarted VPP (counter 6). No core dump.
Run 1 (07:25:4x, same code, no concurrent TD-3 host test) passed every packet step including trace and counters with
NRestarts 5 → 5. I cannot prove the trigger; the correlation points to TD-3's freed-tables host test (V19 second path) running
concurrently on the shared VPP. Per the envelope I stopped host runs; P08 continues with UI/docs and re-runs the topology test
only when no TD-3 host test is running (checked with `ps` before the run). Please confirm or tell me to wait for TD-3's merge.

## Q2 — ui-kit SchemaForm turns an absent optional object into its defaults (for P07a / ui-kit owner)
`withDefaults` fills an absent optional object member with its default object (`dhcpClient` → `{setBroadcastFlag:false}`),
so saving an unrelated field would enable a DHCP client. P08 drops such phantom members in its own screen
(`dropPhantomOptionals`, unit-tested); the proper fix is in `packages/ui-kit/src/schema-form/form-value.ts` (not P08's file):
optional object members should stay absent until the user opts in (a presence toggle).

## Q3 — BLOCKER (environment, fix round 1, 14:25): every interface create that gets freed sw_if_index 3 fails closed in TD-3's sanitizer
- Symptom (P08 and **main** alike): `interface.loopback/loop101: sanitize loop101 (sw_if_index 3, create): no clean sw_if_index
  obtained (VPP V19 quarantine): placeholder cap reached before every freed classify table index was resurrected (16 placeholders)`;
  sanitizer log `placeholders=16 holes_left=2 fresh_run=0 rereads=4`. VPP had no live classify table (`show classify tables` empty),
  only `local0` in `show int`. So the freed index 3 still carries two bindings to deleted classify tables (V19) and the pool does not
  hand those table indices back within `MaxPlaceholders = 16`.
- Reproduced three times in a row: P08 `go test -run 'TestAgentOnHost|TestAgentProcessOnHost' ./internal/agent/` 14:25 and 14:26, and
  **main @ a8d1efb** (a `git archive` export in my scratchpad, no worktree touched) 14:27 — identical failure, same index. NRestarts 0 → 0.
- Consequence: any slot (and tools/app's agent) whose next interface create pops index 3 fails; `tools/ci.sh full` on main would fail
  in `internal/agent` the same way. Owner: TD-3/TD-5 (D-105 M1 cap = holes + 2×FreshRun ≤ 64) and the manager (VPP state).
- What I do: to verify N1 on slot 1 I park index 3 under an admin-down, tagged slot-1 loopback (`loop199`, tag `w1park:v19-index-3`,
  never up, no traffic — the same idea as TD-3's quarantine holder) for the duration of my run, and delete it afterwards (index 3
  goes back to the pool exactly as I found it). Documented with output in P08.md.
