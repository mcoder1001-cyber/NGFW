# F-vrf-static-ecmp — questions and notes for the manager

Written while working; nothing here blocks the task (defaults taken and stated).

## Q1 Contract commits are on the task branch (tell-the-manager item)
`contract(schema): vrfs source-select, next-hop vrf, viaFrr` and `contract(proto): ListRoutes` are on
`task/F-vrf-static-ecmp` (details: `F-vrf-static-ecmp-contract.md`). Numbers: Vrf 3, StaticRoute 7, NextHop 4 (the last
one is only "proposed" in wave-A-hotspots §2 — please add it there). ActionRequest 6 is not used.

## Q2 Traceroute (prompt's open question) — default taken: UNIMPLEMENTED + V-item
VPP has no traceroute API. The agent answers `UNIMPLEMENTED` with a message naming the V-item; `docs/vpp-code-track.md`
gets `### V-new (F-vrf-static-ecmp)` covering traceroute and VRF-aware ping. Option (b) "needs linux-cp (P12)" stays
open: once P12 has a linux-cp host path, traceroute can run as a fixed-argv `traceroute -n` in the VRF's netns.

## Q3 Ping in a non-default VRF / with a source / with a size — default taken: INVALID_ARGUMENT + the same V-item
`want_ping_finished_events{address, repeat, interval}` has no table, source or size (checked in `apps/agent/binapi/ping`
and VPP's `ping_api.c`: `table_id = 0`, `data_len = PING_DEFAULT_DATA_LEN` hard-coded).

## Q4 `packages/schema/examples/` cannot take a `vrf-static-ecmp-*.json` (C4)
`packages/schema/src/examples.test.ts` fails any file that is neither in its group (a) list nor matches the P02b/P02c
sibling regex `^(?:invalid-)?(?:nat|objects|acl|vpn|tunnels|services|ha)-…`. Default taken: the valid fixture lives in
`packages/proto/test/fixtures/vrf-static-ecmp-full.json` (both round-trip corpora read it); the invalid cases are unit
tests in `semantic/vrf-static-ecmp.test.ts`. Every wave-A feature hits this — the manager may want to widen the regex once.

## Q5 One import line per C1/C2 file sits outside the anchor
A key line under the anchor needs its identifier imported at the top of `domains/vrfs.ts` and `domains/routing.ts`
(one `import … from './ext/vrf-static-ecmp.js'` line after the last import). Listed under "Shared hunks"; expect a
trivial union conflict with F-neighbors-ra / F-rpf-adl-pbr, which add their own import at the same place.

## Q6 `packages/proto/test/desired-state.test.ts` one-word edit (not an owned file)
`toEqual` → `toMatchObject` on the `vrfs['customer-a']` assertion: any additive repeated field on `Vrf` breaks the exact
match (ts-proto materialises `[]`). F-neighbors-ra's `proxy_arp_ranges` needs the same; the identical edit merges.
