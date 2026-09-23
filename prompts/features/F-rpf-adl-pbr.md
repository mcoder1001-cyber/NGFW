# Task: F-rpf-adl-pbr — uRPF strict/loose, ADL allow/deny lists (+ Auto-SDL), ABF policy-based routing   (prepend 00-CONTEXT.md)

## Goal
Implement anti-spoofing and policy routing end to end in FAST MODE: per-interface uRPF (loose/strict, v4/v6, rx/tx), ADL allow/deny lists
with the Auto-SDL knob, and ACL-based forwarding (PBR: match an ACL, forward via paths/VRF) incl. classifier tables and IP session
redirect as the advanced form. Reference: TNSR "Policy-based routing, uRPF"; VPP `urpf`, `adl`, `auto_sdl`, `abf`, `classify`,
`ip_session_redirect` (WBS D2.4, D2.7, D3.9 in `plan/wbs.csv`).

## Inputs to read first
- DF-2 descriptors (merged), reuse by name: `urpf.interface/<if>/<af>/<rx|tx>` (`urpf_update_v2`, Retrieve `urpf_interface_dump`),
  `adl.interface/<if>` (presence via `feature_is_enabled`) and `adl.allowlist/<if>` (**write-only**, `RegisterWriteOnly` only),
  `abf.policy/<id>` (ACL by name via DF-4 `acl.LookupIndex`) and `abf.attach/<id>/<if>/<af>`, `classify.table/<name>`,
  `classify.session/…`, `classify.input-acl/<if>`, `classify.output-acl/<if>`, `classify.interface-ip-table` (**write-only**),
  `ip-session-redirect.redirect/<table>/<hex(match)>`; docs `docs/agent/descriptors/{urpf,adl,abf,classify,ip_session_redirect}.md`
- `apps/agent/binapi/auto_sdl/` (`auto_sdl_config` — global) and `apps/agent/binapi/session/` (`session_sdl_*` — the SDL table Auto-SDL fills)
- `packages/schema/src/domains/acl.ts` (ACL lists referenced by name — F-acl owns them), `interfaces.ts` — **no uRPF/ADL/PBR fields exist**
- LOG D-063 + D-076 (write-only types: idempotent Create or a persisted applied-once record keyed by the D-080 boot identity), D-066
  (ACL naming), D-071 (auto_sdl is a global), `docs/vpp-code-track.md` **V19** (classify/ADL per-interface state survives interface
  deletion → fallback: clear the settings before deleting an interface; tests reset inherited state)

## Contract changes
Additive on `contract/F-rpf-adl-pbr`: `interfaces.<if>.urpf?{ipv4?: loose|strict, ipv6?: loose|strict, direction: rx|tx}`,
`interfaces.<if>.adl?{allowVrf, ipv4, ipv6, defaultAllow}`, `routing.pbr?{policies{<name>:{acl, priority, paths[{address, interface,
vrf, weight}]}}, attachments[{policy, interface, family}]}`, `services.autoSdl?{enabled, threshold, remainingTimeoutSec}` + proto.

## Scope — build exactly this
1. **Schema**: PBR policy's ACL exists (acl.lists); attachment interface exists and is not bridged; path VRF exists; uRPF strict not on
   an interface that is an ECMP egress of the same prefix (warning only); names ≤ 63, no `#` (D-066).
2. **Agent**: projection onto the DF-2 descriptors; add `descriptors/auto_sdl/` (global, globals owner only, D-071; write-only if no
   getter); ensure every write-only type is registered via `RegisterWriteOnly` and counted in `retrieve_unsupported`. Fake-client unit
   tests incl. duplicate-add behaviour; ONE host check (prefixed taps, slot table range): Retrieve == desired for readable types,
   `vppctl show urpf` / `show abf policy` / `show abf attach` contain it, rollback clears, restart simulation re-applies write-only once.
3. **API**: config via pointer routes; `GET /api/v1/state/pbr` (policies, attachments, per-policy ACL hit counters if DF-4 stats exist).
4. **UI**: PBR screen (policy list + SchemaForm with ACL picker and path editor, attachments table); uRPF/ADL in the interface drawer
   "Security" group; ADL/Auto-SDL page; en+fa.
5. **Docs**: `docs/user/routing/rpf-adl-pbr.md` (source-based routing to a second uplink, strict uRPF on WAN; CLI equivalent).

Files you own: `apps/agent/internal/descriptors/{urpf,adl,auto_sdl,abf,classify,ip_session_redirect}/**`,
`docs/agent/descriptors/{urpf,adl,auto_sdl,abf,classify,ip_session_redirect}.md`, `apps/agent/internal/agent/project_rpf_adl_pbr*.go`,
`apps/api/src/features/rpf-adl-pbr/**`, `apps/web/src/domains/routing/rpf-adl-pbr/**`, `apps/web/src/locales/*/rpf-adl-pbr.json`,
`docs/user/routing/rpf-adl-pbr.md`, `test/topology/rpf-adl-pbr/**`.
Shared files: one-line appends only (agent registry/projection hook, `app.module.ts`, web router/nav); `descriptors/df2` read-only;
`packages/api-client` regenerated.

## Acceptance (paste the evidence)
- [ ] `vppctl show abf policy`, `show abf attach <if>`, `show urpf …` reflect the commit; Retrieve == desired for readable types (pasted)
- [ ] Agent-restart simulation → everything back within 30 s; write-only objects re-applied exactly once (log excerpt)
- [ ] Rollback removes attachments before policies and clears uRPF/ADL (Retrieve + `vppctl` for write-only)
- [ ] PBR policy naming an unknown ACL → 400 problem+json with `pointer`
- [ ] UI screenshot against the real endpoint in `docs/status/tasks/F-rpf-adl-pbr.md`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
ACL lists, rule editor and ACL attachment (F-acl — you reference ACLs by name; F-acl's WBS D5.5 only links to your ADL page); host/local-in
ACL (F-host-acl-nftables); static routes/VRFs (F-vrf-static-ecmp); QoS classification (F-qos-flat); NAT session redirect; Auto-SDL
session-layer tuning beyond the three knobs (F-host-stack).

## Open questions to surface, not to decide silently
Whether PBR belongs under `routing` or `acl` in the model (chosen: `routing.pbr`); Auto-SDL defaults.
