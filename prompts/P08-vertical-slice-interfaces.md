# Task P08 — Vertical slice: Interfaces end to end   (prepend 00-CONTEXT.md)

## Goal
The first real feature through every layer, proving the architecture: list and edit VPP
interfaces (admin state, MTU, description, IPv4/IPv6 addresses, VLAN sub-interfaces, VRF),
with live counters, through candidate → commit → rollback. When this passes, every later
feature is "more of the same".

## Preconditions
P05, P06, P07b, DF-1, DF-2 merged; `tools/lab rig up <prefix>` available (P04). Data path today = af_packet host-interfaces on the veth/netns rig (D-010); record `path: af_packet`. Do not modify contracts.

## Build exactly this
1. **Agent**: wire the DF-1/DF-2 interface descriptors (host-interface, subinterface, admin-state, MTU, rx-mode, description tag, neighbor)
   and the core ones from P05 into one coherent `interfaces` subsystem apply path. `Retrieve` must
   return the full current interface table with sw_if_index, type, link/admin state, MTU,
   addresses, VRF — the API's `/state/interfaces` is served from it, not from the config.
2. **API**: `GET /api/v1/state/interfaces` (merged live + config view, with `hasPendingChange`),
   `GET /api/v1/state/interfaces/{name}/counters`. Config editing uses the generic pointer
   routes from P06 — add nothing special except semantic checks already in P02.
3. **UI**: Interfaces list (`ServerDataGrid`: name, type, admin, link, MTU, addresses, VRF,
   rx/tx bps + pps sparkline from the WS topic, errors), row click → detail drawer with
   `SchemaForm` for the interface schema and a sub-interface table with add/remove.
   Status chips use the semantic tokens. Everything en+fa.
4. **Topology test** (`test/topology/interfaces/`, `VRX_INTEGRATION=1`, shared lock): `tools/lab rig up <prefix>`; configure both VPP
   host-interfaces with IPs via the API; commit; `ip netns exec ns-<p>-lan ping <wan addr>` through VPP; assert `vppctl show int`
   counters and the WS counters agree within 5%; set MTU 1400 on one side; 1500-byte ping with DF → fails; rollback → succeeds again.
5. **Restart-safety test** (no VPP restart before handover, D-012): stop the agent, delete the prefixed host-interfaces and addresses via
   binapi, start the agent → UI shows the interfaces DOWN then UP; ping works again within 30 s without any API call. After handover,
   the manager may additionally run the `kill -9 vpp` variant.
6. **Docs**: `docs/user/interfaces/basics.md` with screenshots (Playwright captures) and the
   equivalent REST calls; `docs/status/vertical-slice.md` summarising what was proven.

## Acceptance (paste actual output)
- [ ] `vppctl trace` shows the ICMP echo traversing `af-packet-input → ip4-lookup → … → af-packet-output`
- [ ] Rollback removes the address and MTU change from VPP (Retrieve, not assumption)
- [ ] Reconcile after simulated loss under 30 s, evidenced by agent log timestamps
- [ ] Playwright run video attached to the PR

## Out of scope
Bonding, bridge domains, QinQ (VLAN single-tag only), DPDK, LACP, LLDP — those are W5 features.
