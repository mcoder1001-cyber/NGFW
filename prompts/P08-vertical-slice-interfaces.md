# Task P08 — Vertical slice: Interfaces end to end   (prepend 00-CONTEXT.md)

## Goal
The first real feature through every layer, proving the architecture: list and edit VPP
interfaces (admin state, MTU, description, IPv4/IPv6 addresses, VLAN sub-interfaces, VRF),
with live counters, through candidate → commit → rollback. When this passes, every later
feature is "more of the same".

## Preconditions
`tools/lab up single` running; P05 agent, P06 api, P07 shell merged. Do not modify contracts.

## Build exactly this
1. **Agent**: harden the P05 interface descriptors for the dev stack's `host-interface`s
   and for `loopback`; add `interface-description` (via tag), `rx-mode`. `Retrieve` must
   return the full current interface table with sw_if_index, type, link/admin state, MTU,
   addresses, VRF — the API's `/state/interfaces` is served from it, not from the config.
2. **API**: `GET /api/v1/state/interfaces` (merged live + config view, with `hasPendingChange`),
   `GET /api/v1/state/interfaces/{name}/counters`. Config editing uses the generic pointer
   routes from P06 — add nothing special except semantic checks already in P02.
3. **UI**: Interfaces list (`ServerDataGrid`: name, type, admin, link, MTU, addresses, VRF,
   rx/tx bps + pps sparkline from the WS topic, errors), row click → detail drawer with
   `SchemaForm` for the interface schema and a sub-interface table with add/remove.
   Status chips use the semantic tokens. Everything en+fa.
4. **Topology test** (`test/topology/interfaces/`): the host-lan and host-wan VMs;
   configure both VPP interfaces with IPs via the API; commit; `ping` across VPP; assert
   `vppctl show int` counters and the WS counters agree within 5%; set MTU 1400 on one side;
   send 1500-byte ping with DF → fails; rollback → succeeds again.
5. **Restart test**: `tools/lab kill-vpp vrx-a` → UI shows
   interfaces DOWN then UP; ping works again within 30 s without any API call.
6. **Docs**: `docs/user/interfaces/basics.md` with screenshots (Playwright captures) and the
   equivalent REST calls; `docs/status/vertical-slice.md` summarising what was proven.

## Acceptance (paste actual output)
- [ ] `vppctl trace` shows the ICMP echo traversing `af-packet-input → ip4-lookup → … → af-packet-output`
- [ ] Rollback removes the address and MTU change from VPP (Retrieve, not assumption)
- [ ] Reconcile-after-kill under 30 s, evidenced by agent log timestamps
- [ ] Playwright run video attached to the PR

## Out of scope
Bonding, bridge domains, QinQ (VLAN single-tag only), DPDK, LACP, LLDP — those are W5 features.
