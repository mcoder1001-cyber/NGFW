# Task: F-nat46 — NAT46 (IPv4 clients to IPv6-only servers)   (prepend 00-CONTEXT.md)

## Goal
Implement **NAT46 — stateless SIIT (RFC 7915) 1:1 translation of an IPv4 service address to an IPv6-only server** in FAST MODE,
design spike first: schema → agent descriptor/projection → API → UI → verification → docs, as far as the spike decides.
Reference behaviour: TNSR "NAT / MAP-T" and VPP feature/plugin "map (MAP-T, 1:1 mode, ea_bits_len 0)" (WBS D4.5 in `plan/wbs.csv`:
"DS-Lite, MAP-E, MAP-T, LW4o6 BR, 464XLAT", T2). VPP 26.06 has **no NAT46 plugin** (binapi NAT families: nat44_ed, nat44_ei,
nat64, nat66, map, cnat, pnat); domain D4 "NAT & CGNAT" in `docs/08-master-schedule-en.md` §3.

## Inputs to read first
- `packages/schema/src/domains/nat.ts` — extend additively (contract rule below), do not fork; `nat.map` exists (F-det44-map-dslite-cnat)
- `packages/proto/vrx/v1/dataplane.proto` — the `NatConfig` message
- `apps/agent/internal/descriptors/mapnat/` (built by DF-3) — reuse its `map.domain` / `map.interface` descriptors; **do not edit it**
  (owned by F-det44-map-dslite-cnat; agree changes via the questions file)
- the pattern: F-nat44-ei-64-66-nptv6 (`docs/status/tasks/F-nat44-ei-64-66-nptv6.md`, `descriptors/nat64`, `desired/nat64.go`)
- `apps/agent/binapi/map/` — **the only source of VPP API names** (manager-owned; missing plugin → questions file)
- `docs/lab/shared-host-rules.md` — your slot prefix, ports, table range; `docs/lab/host-vrx-a.md` — `map_plugin.so` is part of
  vpp-plugin-core (loaded by default; not in the "still not loaded" list)
- VPP 26.06 docs: https://s3-docs.fd.io/vpp/26.06/

## Contract changes
If the schema/proto lacks fields you need: commit them **first** with subject `contract(<pkg>): …` and
`docs/status/tasks/F-nat46-contract.md`, tell the manager via `docs/status/tasks/F-nat46-questions.md`, then continue — do not wait.
Renaming/reshaping existing fields is not allowed (PENDING).

## Scope — build exactly this
0. **Design spike** (in `docs/agent/descriptors/nat46.md`): (a) stateless SIIT via MAP-T 1:1 (`ea_bits_len 0`, IPv4 /32 ↔ IPv6 /128,
   `ip6_src` = RFC 6052 /96 for the IPv4 clients) — feasible or not; (b) stateful NAT46 has no VPP implementation → an explicit
   out-of-scope decision or a VPP-code-track item (`docs/vpp-code-track.md`, 2x rule) — surfaced, not decided silently.
1. **Schema**: Zod model for `nat.nat46` (only if the spike says (a) works; contract rule) with semantic rules: IPv4 service address
   unique, IPv6 server a unicast host address, client /96 prefix an RFC 6052 length (/96 only for 1:1), interfaces exist.
2. **Agent**: `apps/agent/internal/descriptors/nat46/` — the NAT46 → MAP-T projection (mapping → `map.domain`, interfaces →
   `map.interface` translation mode); unit tests with the fake client; ONE integration check on the host VPP (`VRX_INTEGRATION=1`,
   shared lock, prefixed objects): after Apply `Retrieve()` == desired and `vppctl show map domain` contains it; after rollback nothing
   remains; agent-restart simulation recreates it. Real-VPP tests skip without `VRX_INTEGRATION`.
3. **API**: config via the generic pointer routes; no state route (stateless — nothing to page).
4. **UI**: NAT46 tab (list + schema-driven form); en + fa strings; screenshot against the real endpoint.
5. **Docs**: `docs/user/firewall/nat46.md` with an example and the CLI equivalent.

## Acceptance (paste the evidence)
- [ ] `vppctl show map domain` reflects the committed config (pasted)
- [ ] Agent-restart simulation → config back within 30 s (log excerpt)
- [ ] Rollback removes the objects (Retrieve output, not assumption)
- [ ] Validation failure for a duplicate IPv4 service address → 400 problem+json with a `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree
- [ ] Packet-level test (NAT family): IPv4 client → IPv6-only server through the `af_packet` rig; path recorded

## Out of scope (do not build)
- Stateful NAT46 (a port-sharing IPv4 pool in front of IPv6 servers) — no VPP implementation; question/code-track item only
- Editing `descriptors/mapnat`, `desired/nat*.go` other than an anchored dispatch hunk, or the MAP / DS-Lite / 464XLAT / DET44 /
  CNAT / PNAT features (F-det44-map-dslite-cnat)
- NAT44-ED/EI, NAT64, NAT66, NPTv6 (merged rows), DNS46/DNS64 synthesis, session browser, IPFIX NAT logging
- New VPP plugins, `apps/agent/binapi/**`, startup.conf plugin changes

## Open questions to surface, not to decide silently
- Stateful NAT46: out of scope decision vs. VPP code track
- Ownership of `map.domain` objects produced by NAT46 vs. `nat.map` domains (same descriptor, same tag space) — name prefix rule
- Does VPP 26.06 accept a MAP-T domain with `ip6_prefix` /128 and `ea_bits_len 0` (host check)
