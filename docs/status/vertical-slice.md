# Vertical slice — interfaces end to end (P08)

What the first feature through every layer proved, on the real host VPP 26.06 through the af_packet veth/netns rig
(`path: af_packet`, D-010), slot 1, 2026-09-24. Full evidence: `docs/status/tasks/P08.md`.

```
Browser (React, /interfaces)  ──REST──▶  vrx-api  ──gRPC──▶  vrx-agent  ──binapi──▶  VPP 26.06
  ServerDataGrid + drawer              config/**  (candidate → diff → commit → rollback)
  SchemaForm (one schema)              state/interfaces = InterfaceState RPC + Retrieve + running/candidate
  WS iface.counters → rates            WS relay of StreamStats
```

| layer | proven by |
|---|---|
| schema → UI form | the drawer form is `z.toJSONSchema(RootConfig.interfaces)` item; no hand-written type (00-CONTEXT rule 5) |
| API config routes | the generic P06 pointer routes only: `PATCH /config/interfaces` (merge patch), `DELETE`, `commit`, `rollback/{rev}` |
| desired-state builder | `interfaces.<name>` → alias objects `interface/<name>` (D-065/D-069/D-073a), creators (af_packet, loopback, sub-interface), DF-1 attributes, addresses, VRF, DHCP |
| scheduler / reconciler | commit applied, verified by Retrieve; rollback deleted exactly `interface.mtu/host-w1w0` and `interface-ip/host-w1l0/10.1.3.1/24` |
| data plane | ICMP echo traced `af-packet-input → ethernet-input → ip4-input → ip4-lookup → ip4-rewrite → host-w1w0-output → host-w1w0-tx` (VPP 26.06 names the af_packet output nodes after the interface) |
| MTU | commit MTU 1400 on the wan side → `vppctl show int` shows `1400/0/0/0`, a 1500-byte DF ping gets "Frag needed and DF set (mtu = 1400)"; rollback → Retrieve has no MTU object, VPP back to 9000, the DF ping passes |
| counters | `vppctl show interface` and the WS `iface.counters` topic agree exactly (26/24/26/25 packets; tolerance 5 %) |
| live state | `/state/interfaces` is served from the agent's `InterfaceState` RPC (sw_if_index, type, admin/link, MTU, addresses, VRF), merged with Retrieve, running config and `hasPendingChange` |
| restart safety | agent stopped, both host-interfaces and their addresses deleted behind its back via binapi, agent started: reconcile finished 0.330 s after start (agent log timestamps), `/state/interfaces` went 503 → admin DOWN → admin UP, ping OK after 1.69 s with no config API call |
| cleanup | interfaces deleted through the API commit; nothing with the prefix left in VPP; NRestarts 6 → 6 |

## What every later feature reuses

- **Wiring:** `apps/agent/internal/subsystems` — one registry, the domain → descriptor map (`Health.subsystems`), the
  persisted stores in the agent state dir keyed with the VPP boot identity (D-080): DF-1 claims, D-076 applied-once/boot
  records, keyed claims (acl/nat), classify store, DF-5 `IPsecOptions/IKEv2Options` (file BootStore + D-096 keyer).
  Feature tasks add their descriptors to `Domains` and take their stores from `Wiring`.
- **Builder:** `apps/agent/internal/desired` — pointer-carrying `Sink`, alias references, assembler back to the document.
- **State route pattern:** a read-only agent RPC for live status (never status in `RetrieveResponse`, proto.md §5), merged
  in `apps/api/src/state` with Retrieve + running + candidate.
- **Screen pattern:** `apps/web/src/domains/<domain>/` — generated OpenAPI types, `ServerDataGrid`, drawer with
  `SchemaForm` + `localizeSchema`, merge-patch saves through the generic route, WS topic hooks, en/fa namespace.
- **Topology test pattern:** `test/topology/interfaces` — own Go module, real agent + API + DB per slot, rig hand-over,
  V19 guard before any packet, veths down before any af_packet delete (D-101), NRestarts before/after.

## Lessons (for the feature waves)

- **D-101 / V24:** never delete an af_packet interface while its veth is up (VPP double-closes the fds; it crashed the shared
  VPP once during this task). The rig tooling and this test quiesce first; the agent-side fix is TD-5.
- **V19:** reused sw_if_indexes inherit classify bindings; every test checks/reset bindings before traffic (TD-3 adds the
  agent-side sanitizer; `ifsanitize.Release` wiring in the agent is pending TD-3's merge).
- **Linux PMTU cache:** after an MTU test the sending host remembers the lower PMTU; flush its route cache before asserting.
- **SchemaForm defaults:** an absent optional object comes back filled with defaults — screens must not write it back
  (P08-questions Q2; ui-kit fix pending).
