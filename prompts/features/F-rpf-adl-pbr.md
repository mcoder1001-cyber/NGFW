# Task: F-rpf-adl-pbr — uRPF strict/loose, ADL allow/deny lists (+ Auto-SDL), ABF policy-based routing   (prepend 00-CONTEXT.md)

## Goal
Implement anti-spoofing and policy routing end to end in FAST MODE: per-interface uRPF (loose/strict, v4/v6, rx/tx), ADL allow/deny lists
with the Auto-SDL knob, and ACL-based forwarding (PBR: match an ACL, forward via paths/VRF). Classifier tables and IP session redirect
(the rest of D2.7) keep their DF-2 descriptors but get **no schema/API/UI in this task** (no contract field for them; board scope is
uRPF/ADL/ABF). Reference: TNSR "Policy-based routing, uRPF"; VPP `urpf`, `adl`, `auto_sdl`, `abf` (WBS D2.4, D2.7, D3.9 in `plan/wbs.csv`).

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
  deletion → fallback: clear the settings before deleting an interface; tests reset inherited state) — TD-3 (merged) adds
  `apps/agent/internal/vpp/ifsanitize` (Acquire on create, BeforeDelete, `vrx-vpp-preflight`): use it, do not re-implement it
- `docs/vpp-code-track.md` **V23 (a)**: `feature_is_enabled` answers true for every VPP error, and DF-2's `adl.interface` Retrieve trusts
  it → a fresh interface can read as "ADL on". Your projection relies on that Retrieve, so fix it in `descriptors/adl` (you own it):
  confirm a "true" with a control query that cannot be on, or report unknown; fake + host test. V23 (b): ABF attachments are not
  sanitized on interface delete (no crash path known) — leave as is
- Registration facts (checked 2026-09-24, prep-waveA, against DF-2 + P08): `urpf.Register` / `adl.Register` / `abf.Register(r, c, owner,
  ids *df2.IDRange, …)` take `df2.WithClaims(<P08 Wiring.KeyedClaims("acl")>)` (persisted; never the in-memory default); the ABF
  policy-id range is the slot range `N000–N999` on the shared host, nil (= all) only for the product agent; `adl.RegisterWriteOnly` for
  `adl.allowlist`; anything classify-backed takes `Wiring.ClassifyStore()`
- `auto_sdl_config{enable, threshold (default 5), remove_timeout (default 300)}` has no getter and is VPP-global; it acts on the session
  layer's SDL table. If it fails on vrx-a because the session layer / SDL backend is not enabled (that is startup.conf =
  handover-gated), keep it fake-tested, make the host test skip-unless-supported with the reason, and write it in the questions file —
  never enable the session layer on the shared VPP

## Contract changes
Additive on `contract/F-rpf-adl-pbr`: `interfaces.<if>.urpf?{ipv4?: loose|strict, ipv6?: loose|strict, direction: rx|tx}`,
`interfaces.<if>.adl?{allowVrf, ipv4, ipv6, defaultAllow}`, `routing.pbr?{policies{<name>:{acl, priority, paths[{address, interface,
vrf, weight}]}}, attachments[{policy, interface, family}]}`, `services.autoSdl?{enabled, threshold, removeTimeoutSec}` (maps
`auto_sdl_config.remove_timeout`) + proto.

## Scope — build exactly this
1. **Schema**: PBR policy's ACL exists (acl.lists); attachment interface exists and is not bridged (only if F-bridge-l2's L2 fields are
   on main when you start — otherwise note it in the questions file); path VRF exists; uRPF strict not on an interface that is an ECMP
   egress of the same prefix (warning only); names ≤ 63, no `#` (D-066).
2. **Agent**: projection onto the DF-2 descriptors; add `descriptors/auto_sdl/` (global, globals owner only, D-071; write-only if no
   getter); ensure every write-only type is registered via `RegisterWriteOnly` and counted in `retrieve_unsupported`. Fake-client unit
   tests incl. duplicate-add behaviour; ONE host check (prefixed taps, slot table range): Retrieve == desired for readable types,
   `vppctl show interface features <if>` (`ip4-rx-urpf-*`, as DF-2 verified) / `show abf policy` / `show abf attach` contain it, rollback clears, restart simulation re-applies write-only once.
3. **API**: config via pointer routes; `GET /api/v1/state/pbr` (policies, attachments, per-policy ACL hit counters if DF-4 stats exist).
4. **UI**: PBR screen (policy list + SchemaForm with ACL picker and path editor, attachments table); uRPF/ADL in the interface drawer
   "Security" group (P08's drawer renders every `interfaces.<if>` leaf through SchemaForm grouped by the `withUi` `group` hint — give the
   new fields one `withUi` group (name per `docs/status/wave-A-hotspots.md` C1) and make absent mean "off"; drawer saves write defaults
   back); ADL/Auto-SDL page; en+fa.
5. **Docs**: `docs/user/routing/rpf-adl-pbr.md` (source-based routing to a second uplink, strict uRPF on WAN; CLI equivalent).

Files you own: `apps/agent/internal/descriptors/auto_sdl/**` (new) and `apps/agent/internal/descriptors/{urpf,adl,abf,classify,ip_session_redirect}/**`
(DF-2's, merged, TD-3-hardened — gap fixes only, e.g. V23 (a)), `docs/agent/descriptors/{urpf,adl,auto_sdl,abf,classify,ip_session_redirect}.md`,
`apps/agent/internal/agent/project_rpf_adl_pbr*.go`, `apps/agent/internal/desired/rpf_adl_pbr*.go` (P08 puts builders in `internal/desired/`),
`apps/api/src/features/rpf-adl-pbr/**`, `apps/web/src/domains/routing/rpf-adl-pbr/**`, `apps/web/src/locales/*/rpf-adl-pbr.json`,
`docs/user/routing/rpf-adl-pbr.md`, `test/topology/rpf-adl-pbr/**`.
Shared files (P08 layout; protocol and anchors in `docs/status/wave-A-hotspots.md`; list each hunk in your PR):
`apps/agent/internal/subsystems/subsystems.go` (register, `Domains`), `apps/agent/internal/agent/projection.go` (one call in `project()`,
one in `assemble()` after `desired.Assemble` — `desired/interfaces.go` stays P08's), `app.module.ts`, web router/nav/`i18n.ts`;
`descriptors/df2` and `internal/vpp/ifsanitize` read-only; `packages/api-client` regenerated.

## Acceptance (paste the evidence)
- [ ] `vppctl show abf policy`, `show abf attach <if>`, `show interface features <if>` (uRPF nodes) reflect the commit; Retrieve == desired for readable types (pasted)
- [ ] Agent-restart simulation → everything back within 30 s; write-only objects re-applied exactly once (log excerpt)
- [ ] Rollback removes attachments before policies and clears uRPF/ADL (Retrieve + `vppctl` for write-only)
- [ ] PBR policy naming an unknown ACL → 400 problem+json with `pointer`
- [ ] UI screenshot against the real endpoint in `docs/status/tasks/F-rpf-adl-pbr.md`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
ACL lists, rule editor and ACL attachment (F-acl — you reference ACLs by name; F-acl's WBS D5.5 only links to your ADL page); host/local-in
ACL (F-host-acl-nftables); static routes/VRFs (F-vrf-static-ecmp); QoS classification (F-qos-flat); NAT session redirect; Auto-SDL
session-layer tuning beyond the three knobs (F-host-stack); schema/API/UI for classify tables/sessions and `ip_session_redirect`
(their DF-2 descriptors stay as they are — a follow-up row if the product owner wants them exposed); enabling the session layer.

## Open questions to surface, not to decide silently
Whether PBR belongs under `routing` or `acl` in the model (chosen: `routing.pbr`); Auto-SDL defaults.
