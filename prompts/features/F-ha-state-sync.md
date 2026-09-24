# Task: F-ha-state-sync — HA state sync (T2, partial ok): NAT/ACL sessions, IPsec SAs, failover test automation   (prepend 00-CONTEXT.md)

## Goal
Deliver whatever state synchronisation VPP 26.06 exposes **through its binary API only** for an active/standby pair, make the gaps
explicit, and automate the failover test on the single-host rig. Honest partial delivery is the goal (D-058: "partial ok").
Reference: TNSR "VPF state sync"; WBS D5.6, D9.3, D9.4 in `plan/wbs.csv`.

## Inputs to read first
- `packages/schema/src/domains/ha.ts`: `ha.cluster{enabled, nodeName, peers[], port (4370), secretRef (key/…, D-051), interface, vrf,
  configSync, stateSync{nat, ipsec, acl}}` and `ha.vrrp.<name>` (D-053, D-067) exist — no reshaping; extras additive
- `apps/agent/binapi/nat44_ei/` — `nat44_ei_ha_set_listener`/`_get_listener`, `nat44_ei_ha_set_failover`/`_get_failover`,
  `nat44_ei_ha_flush`, `nat44_ei_ha_resync` (+ `nat44_ei_ha_resync_completed_event`); `docs/agent/descriptors/nat44-ei.md` lists HA as
  "not used" by DF-3 — you build it in a new package
- `docs/vpp-code-track.md` **V2**: HA exists only for NAT44-EI → fallback: VRRP failover without session preservation for NAT44-ED /
  reflexive ACL sessions, or NAT44-EI where session HA is required. IPsec SAs: no SA state-sync API — `ipsec_sad_entry_update` (binapi `ipsec`)
  carries only `sad_id, is_tun, tunnel, udp_src_port, udp_dst_port` (no sequence/replay state), so the fallback is "re-key on failover"
  (strongSwan side, P11) — propose a new V-item
- `prompts/features/F-vrrp-config-sync.md` (dep: VRRP + config sync + cluster UI — you add only the state-sync part to it),
  `prompts/features/F-nat44-ed-sessions.md` (dep), `docs/agent/descriptors/nat-common.md` (D-071: HA listener/failover are VPP globals →
  globals owner only), D-080 boot identity, D-082 globals lock, D-012 (no VPP restarts before handover)

## Contract changes
If needed (e.g. `ha.cluster.stateSync.natListener{address, port}`, `failoverPeer{address, port, refreshSec}`), additive on
`contract/F-ha-state-sync` + `-contract.md`; manager told via questions file; continue.

## Scope — build exactly this
Files you own: `apps/agent/internal/descriptors/hasync/**`, `docs/agent/descriptors/hasync.md`, `apps/agent/internal/agent/project_ha_state_sync*.go`,
`apps/api/src/features/ha-state-sync/**`, `apps/web/src/domains/system/ha-state-sync/**`, `apps/web/src/locales/*/ha-state-sync.json`,
`docs/user/system/ha-state-sync.md`, `test/topology/ha-state-sync/**`. Shared: one-line appends only (the cluster page of
F-vrrp-config-sync gets a "State sync" panel through its extension hook or a sibling route).
1. **Agent** `descriptors/hasync`: `nat44-ei-ha.listener/global` and `nat44-ei-ha.failover/global` (Retrieve via the `_get_*` messages;
   globals owner only, slots require), resync as an action (`nat44_ei_ha_resync`, wait for the completed event), flush as an action.
   `stateSync.nat` with `nat.mode: ed` → DryRun warning "not supported by VPP (V2)", never an error; `stateSync.acl` → same warning (V2);
   `stateSync.ipsec` → warning "re-key on failover" (no API; new V-item proposed in the questions file).
2. **Failover automation** `test/topology/ha-state-sync/`: the NAT44-EI HA listener/failover are VPP-wide globals, so one host VPP cannot
   be both nodes. On this host: verify config round-trip, the resync action against a peer address on the veth rig (packets seen with
   `tcpdump`), and a VRRP-driven failover (F-vrrp-config-sync) with convergence timings. Write the full two-node script (session created
   on A present on B, `--kill-vpp` mode for the manager after handover, D-012) against the `test/topology/vrx-b.yml` inventory and mark
   the two-node run **deferred** until a second VPP box exists.
3. **API**: `GET /api/v1/state/ha/sync` (per-kind: supported/active/last resync/counters), `POST /api/v1/actions/ha/sync/resync`.
4. **UI**: State-sync panel (per kind: supported / unsupported-by-VPP badge with the V2 reason, last resync, counters); en + fa.
5. **Docs**: `docs/user/system/ha-state-sync.md` — the support matrix (NAT44-EI yes; NAT44-ED, ACL, IPsec no/partial with reasons), design
   of a failover, convergence numbers from the test.

## Acceptance (paste the evidence)
- [ ] `vppctl show nat44 ei ha` (or the `_get_listener/_get_failover` replies) reflects the committed config (pasted)
- [ ] Rig run (path: af_packet rig): resync/HA packets towards the peer address seen in `tcpdump`; VRRP failover timings pasted; for ED the
      documented behaviour (sessions lost) shown; two-node session-continuity script committed and recorded as deferred (needs vrx-b)
- [ ] Agent-restart simulation → listener/failover back within 30 s (log excerpt)
- [ ] Rollback resets HA globals (globals owner) / leaves them untouched (slot) — Retrieve shown
- [ ] `stateSync.nat: true` with `cluster.enabled: false` → 400 with `pointer`; `tools/ci.sh --base main` green; UI screenshot

## Out of scope (do not build)
VRRP, config sync, cluster membership and cluster UI (F-vrrp-config-sync); NAT44-ED sessions/browser (F-nat44-ed-sessions); NAT44-EI
config itself (F-nat44-ei-64-66-nptv6); IPsec/IKE config and strongSwan HA plugin (P11, F-ikev2-native); any C code for ED/ACL/IPsec
sync (V2 — park only); multi-host lab automation beyond the single-host rig; VPP kill tests before handover.

## Open questions to surface, not to decide silently
Should the product steer HA users to NAT44-EI (session sync) or accept session loss with ED? Record the trade-off; the product owner decides.
Propose a new V-item for IPsec SA sequence/replay-window sync (no API in 26.06) — the manager adds it to docs/vpp-code-track.md.
