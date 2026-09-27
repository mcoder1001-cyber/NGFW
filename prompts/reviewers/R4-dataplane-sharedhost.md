# Reviewer R4 — data-plane / VPP safety & shared-host rules   (prepend 00-CONTEXT.md, then ../REVIEW-PROMPT.md)

Mandatory when the diff touches `apps/agent/**`, `deploy/vpp/**`, `test/topology/**`, `tools/lab`, or creates VPP objects,
daemons, ports, tables or DB rows on the shared host. Read `docs/lab/shared-host-rules.md` and `docs/lab/host-vrx-a.md` first.

## Check
1. **VPP API provenance:** every message used exists in `apps/agent/binapi/`; the branch does not modify `binapi/` or
   `tools/binapi-gen.sh`. No C code in VPP (park to `docs/vpp-code-track.md`).
2. **Restart safety:** pasted evidence of the agent-restart simulation (stop agent, delete prefixed objects, start → recreated). A
   new VPP object type without `Retrieve` → BLOCKER. Reconcile never deletes objects it does not own (prefix/owner, D-089).
3. **Shared host:** every object/port/database/table carries the slot prefix and stays in the slot's ranges
   (`tools/lab env <N>`, `python3 tools/slot-check.py` for any new per-slot port formula; never a port built from the slot number
   by string concatenation — D-156); ids only from `w.IDRange()` (§12); no `pkill`/`killall`; no system daemon units started;
   daemons on 127.0.0.1 or rig netns; cleanup in `t.Cleanup`; lab lock held only during a run (§10).
4. **Crash vectors:** no packet trace commands or tracedump API (§11, D-128); classify tables deleted only after their bindings
   (V19); FIB flush with other clients (V22); VPP-global settings only behind the globals lock and opt-in (§7).
5. **Handover:** any VPP restart/kill, `/etc/vpp` or `startup.conf` edit while `handover: pending` → BLOCKER.
6. **Packet-level proof** for vertical slice, NAT, IPsec, BGP→FIB, VRRP (and any forwarding path): counters, FIB entry, netns
   tcpdump — not "the object exists".

## Output
`docs/status/tasks/<id>-review-R4.md` — findings, verdict line.
