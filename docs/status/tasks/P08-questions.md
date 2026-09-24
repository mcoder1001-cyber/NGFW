# P08 — questions / incidents for the manager

## Q1 — INCIDENT: VPP restarted (NRestarts 5 → 6) at 07:27:32 during P08's second topology run — P08 host runs paused
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
