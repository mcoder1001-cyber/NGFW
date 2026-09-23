# vrrp descriptors (DF-7, WBS D9.1)

Package `apps/agent/internal/descriptors/vrrp` — VPP VRRPv3 plugin. Messages only from `apps/agent/binapi/vrrp`.
DF-7 conventions: see `policer.md`. (The keepalived path of VRRP is RF-4, not here.)

| Object type | Key | Create / Update / Delete | Retrieve | Notes |
|---|---|---|---|---|
| `vrrp.vr` | `vrrp.vr/<if>/<vr id>/<ipv4\|ipv6>` | `vrrp_vr_update` with index ~0 (creates, returns the pool index); Update = `vrrp_vr_update` on the index (priority, interval, preempt, accept, addresses; VPP restarts a running VR); unicast on/off = ErrRecreate; `vrrp_vr_add_del` is_add=0 by key | `vrrp_vr_dump` | interval in centiseconds; addresses canonical + sorted; the dump carries no pool index → after a restart Update walks the pool (another VR's index → `INVALID_ARGUMENT`, free → `NO_SUCH_ENTRY`, both harmless) |
| `vrrp.vr-peers` | `vrrp.vr-peers/<if>/<vr id>/<af>` | `vrrp_vr_set_peers` (a running VR is stopped around it and started again); Delete = no-op | `vrrp_vr_peer_dump` per owned unicast VR | VPP cannot clear peers (empty list refused); they go with the VR |
| `vrrp.vr-track-interface` | `vrrp.vr-track-interface/<if>/<vr id>/<af>/<tracked>` | `vrrp_vr_track_if_add_del` add; Update (priority) = del + add; del | `vrrp_vr_track_if_dump` (dump_all) | the tracked interface is resolved by logical name (never another owner's) |
| `vrrp.vr-state` | `vrrp.vr-state/<if>/<vr id>/<af>` | `vrrp_vr_start_stop` start / stop | runtime state ≠ Init in `vrrp_vr_dump` | `running` must be true (omit the object to stop) |

Events (`StreamEvents`): `vrrp.WatchEvents` — `want_vrrp_vr_events` + `vrrp_vr_event` → `Event{Key, VR, OldState,
NewState}` (init, backup, master, interface-down). Host run: `init → backup` right after the start. `vrrp.States`
gives live state, current priority and master advertisement interval.

Dependencies: vr → `interface/<if>`; peers/track/state → `vrrp.vr/…`; track → `interface/<tracked>`; state →
`vrrp.vr-peers/…` (optional). No `interface-ip` dependency: the VR's addresses are virtual, the interface address
is not part of the VR (DF-7-questions.md). Ownership: the VR's interface (claim of the VR key when untagged).

## VPP 26.06 quirks

- `vrrp_vr_peer_dump` for all VRs answers with `vrrp_vr_details` (wrong message) → dumped per VR.
- `vrrp_vr_add_del` validates priority/interval/vr_id before looking at `is_add` → the delete sends them too.
- A VR on a deleted interface stays in the pool (seen once in development, removed by index; the test cleanup now
  deletes VRs before their loopback). FIB entries: none (VR addresses are added only in accept mode as master).
