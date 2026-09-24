# Task: F-vrrp-config-sync — VRRPv3 (VPP plugin + keepalived path), config sync, cluster UI   (prepend 00-CONTEXT.md)

## Goal
First-hop redundancy and a two-node cluster in FAST MODE: VRRPv3 virtual routers on the native VPP `vrrp` plugin (primary engine) and on
keepalived (alternative engine per instance), configuration synchronisation between cluster peers, and a cluster view. Reference: TNSR
"VRRP", "HA / config sync"; VPP `vrrp` plugin; keepalived. WBS D9.1 (VRRPv3 + keepalived path, T1), D9.2 (config sync + membership, T1),
D9.5 (cluster view in UI, T2).

## Inputs to read first
- `packages/schema/src/domains/ha.ts` — existing: `ha.vrrp` **record keyed by name** (D-053, map field 2, D-067) of `{enabled, description,
  interface, vrId, addressFamily, priority, advertisementIntervalMs (×10 ms), preempt, acceptMode, unicast{peers[]}, addresses[], vrf,
  engine: vpp|keepalived, track[{interface, priorityDecrement}]}` and `ha.cluster{enabled, nodeName, peers[{name,address}], port, secretRef
  ("key/<name>", D-051), interface, vrf, configSync, stateSync{nat,ipsec,acl}}`. Extend only additively, as separate `contract(schema|proto): …`
  commits on your task branch (no `contract/` branch; numbers from `docs/status/wave-BC-numbers.md`) — e.g. `cluster.syncExclude[]` pointers
  that stay node-local (at minimum `/ha/cluster/nodeName`, management addresses, `system.hostname`), and the D-086 keepalived stand-ins.
- VPP VRRP descriptors (DF-7, merged: `docs/agent/descriptors/vrrp.md`): `vrrp.vr`, `vrrp.vr-peers`,
  `vrrp.vr-track-interface`, `vrrp.vr-state`, `vrrp.WatchEvents` (`want_vrrp_vr_events`); binapi `apps/agent/binapi/vrrp/` (`vrrp_vr_update`,
  `vrrp_vr_add_del`, `vrrp_vr_set_peers`, `vrrp_vr_track_if_add_del`, `vrrp_vr_start_stop`, `vrrp_vr_dump`, `vrrp_vr_peer_dump`). V20: the
  all-VR peer dump is broken → dumped per VR (already handled). V22b: DF-7's vrrp host test crashed the shared VPP on the packet path →
  VPP-engine host steps are opt-in and run alone in a manager window (D-087, D-090).
- keepalived renderer (RF-4, merged: `apps/agent/internal/renderers/keepalived/{renderer,model}.go`, its README, `docs/agent/renderers/keepalived.md`):
  renders only `engine: keepalived` instances; its `InterfaceMapper` defaults to `NoMapper` (rejects every instance) — **you pass the linux-cp
  mapping** from P12's LCP pairs (`apps/agent/internal/lcpmap`, merged); `vrx-keepalived-notify` state files; reload via SIGHUP. Without LCP
  pairs the keepalived path is skip-with-reason.
- `apps/api` (P06): commit engine, revisions (`config_revision.kind` is a text column), secrets store, the `commit.events` bus topic — config
  sync is API-to-API, never agent-to-agent. **P08 (merged)** patterns: builders in `apps/agent/internal/desired/`, registration + `Domains`
  in `subsystems.go`, the hook in `projection.go`, events through the W-seed `subsystems.Env` event sink.

## Scope — build exactly this
Files you own (the envelope's list wins): `apps/agent/internal/descriptors/vrrp/**` and `apps/agent/internal/renderers/keepalived/**` (gap-only),
`docs/agent/descriptors/vrrp.md`, `docs/agent/renderers/keepalived.md`, `apps/agent/internal/desired/vrrp*.go`, `apps/agent/internal/subsystems/{vrrp,keepalived}*.go`,
`apps/agent/internal/agent/rpc_vrrp*.go`, `apps/api/src/features/vrrp-config-sync/**`, `apps/web/src/domains/system/vrrp-config-sync/**`,
`apps/web/src/locales/*/vrrp-config-sync.json`, `docs/user/system/vrrp-config-sync.md`, `test/topology/vrrp-config-sync/**`. Shared files: one-line appends only.
1. **Schema** (semantic; new fields as `contract(schema)` commits on your branch): (interface, family, vrId) unique across `ha.vrrp`; priority 255 only when a
   virtual address is an interface address (owner); tracked interfaces exist; cluster peers ≠ own node; `syncExclude` pointers valid.
2. **Agent**: project `engine: vpp` instances → DF-7 VRRP objects; `engine: keepalived` → RF-4 renderer with the LCP interface mapper;
   VRRP state (init/backup/master) from `vrrp.WatchEvents` + keepalived notify into agent events.
3. **API**: pointer routes; `GET /api/v1/state/ha/vrrp` (state, current priority, master adv. interval per VR); **config sync**: on commit on the
   primary, push the running revision minus `syncExclude` to each peer's API over mutual auth (cluster key via the secrets store, TLS, peer
   pinned), peer applies as a normal commit tagged `origin: cluster` (revision `kind: 'cluster-sync'`, no migration); `GET /api/v1/state/ha/cluster`
   (members, last sync revision, lag, errors);
   `POST /api/v1/actions/ha/sync` (force). Conflict rule: a peer with local unsynced edits refuses and reports.
4. **UI**: System → High availability: VRRP list with live role chips; cluster view (members, roles, revision per node, sync status, force-sync);
   en + fa; screenshot against the real endpoints.
5. **Docs**: `docs/user/system/vrrp-config-sync.md` (two-node active/standby, unicast VRRP, tracking, what is and is not synced).

## Acceptance (paste the evidence)
- [ ] Packet-level failover on the veth rig (`path: af_packet`; VPP-engine steps opt-in, alone, in a manager window — V22b): two VRs
      (node A priority 200, node B priority 100 — both inside your slot, fixture per the envelope) on rig interfaces; `ns-<p>-lan`
      pings the virtual address continuously; stop A's VR (or down its interface) → B becomes master (event pasted) and pings resume within 3 s; A back → preempts
- [ ] `vppctl show vrrp vr` reflects the committed config; rollback removes the VRs (Retrieve)
- [ ] Config sync: commit on node A (your slot's API) → node B (a second API+agent inside your slot: port +50, DB `vrx_w<slot>b`, prefix `w<slot>b` — envelope) shows the same revision minus excluded pointers within 10 s
- [ ] Agent-restart simulation → VRs recreated and started within 30 s (log excerpt)
- [ ] Duplicate (interface, family, vrId) → 400 problem+json with `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
NAT/ACL session and IPsec SA state sync, failover automation/tuning (F-ha-state-sync); conntrackd; BFD-driven VRRP (F-bfd-redistribution);
LCP pair management (P12); more than 2 nodes in tests; multi-master config merge; VDOM (D-059).

## Open questions to surface, not to decide silently
Sync transport (proposed: peer API over HTTPS with the cluster key, not a new daemon); whether secrets are synced (proposed: yes, re-encrypted per node —
a secret-storage decision, decision-policy #4: do not sync secrets until the manager's PENDING is answered; a missing ref on the peer fails the sync
with a clear error that names the refs).
