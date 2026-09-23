# Task: F-acl — MACIP / L3 / L4 ACLs, attachments, hit counters, 100k-rule editor   (prepend 00-CONTEXT.md)

## Goal
Stateless and stateful (reflect) L3/L4 ACLs and L2 MACIP ACLs on VPP interfaces (in/out, per zone), with per-rule hit
counters and a rule editor that stays usable at 100 000 rules. Reference: TNSR "ACL / MACIP ACL"; VPP plugin `acl`
(WBS D5.2, D5.4, D5.5 in `plan/wbs.csv`).

## Inputs to read first
- `packages/schema/src/domains/acl.ts` + `semantic/acl.ts` (P02b, merged): `acl.{lists, macip, attachments[],
  macipAttachments[]}` exist with rules `acl.rule-consistency`, `acl.rule-references`, `acl.rule-sequences-unique`,
  `acl.attachments`, `acl.macip-rules`, `acl.macip-attachments`, `acl.tags-exist`; D-070 `macPattern` for MACIP. `acl.host*` is
  F-host-acl-nftables' — do not touch.
- `apps/agent/internal/descriptors/acl/` + `docs/agent/descriptors/acl.md` (DF-4): `acl.acl/<name>`, `acl.macip-acl/<name>`,
  `acl.interface-binding/<ifname>`, `acl.macip-interface-binding/<ifname>`, `acl.etype-whitelist/<ifname>`,
  `acl.stats-enable/global`, `acl.StatsReader` (`/acl/<index>/matches`), `acl.LookupIndex`. D-066 (tag `<owner>:<name>`,
  duplicates `#<index>`, foreign ACLs on an interface preserved in order), D-065/D-069 (`interface/<name>`, logical names).
- `apps/agent/internal/objects` (F-object-model) — the only way to expand address/service/group references and schedules
- `apps/agent/binapi/acl/` — verify every message (`acl_add_replace`, `acl_interface_set_acl_list`, `macip_acl_add_replace`,
  `macip_acl_interface_add_del`, `acl_stats_intf_counters_enable`)
- D-071 (`acl.stats-enable` is a global: only the globals owner sets it; slots require it), D-082 (globals lock in tests),
  D-080 (boot identity for the stats flag), `docs/vpp-code-track.md` **V7** (stats-enable replies with the wrong message id and has
  no getter/disable → fallback in place: raw stream, treat mismatched reply as success, never disabled by the agent)

## Contract changes
The domain is complete for this scope. Anything extra (e.g. `acl.settings.counters: bool`) is additive on `contract/F-acl`
(schema + proto + drift guard + `docs/status/tasks/F-acl-contract.md`), manager told via questions file; continue meanwhile.

## Scope — build exactly this
Files you own: `apps/agent/internal/descriptors/acl/**`, `docs/agent/descriptors/acl.md`, `apps/agent/internal/agent/project_acl*.go`,
`apps/api/src/features/acl/**`, `apps/web/src/domains/firewall/acl/**`, `apps/web/src/locales/*/acl.json`, `docs/user/firewall/acl.md`,
`test/topology/acl/**`. Shared files: one-line appends only (app.module import, router/nav entry, projection hook).
1. **Projection**: `acl.lists.<name>` → one `acl.acl/<name>` (rules expanded through `internal/objects`; `ipVersion: any` → v4 + v6
   rules; disabled rules skipped; inactive schedules → rule omitted, re-projected every 60 s; `reflect` → permit+reflect);
   attachments (interface or zone → every zone interface) → `acl.interface-binding/<ifname>` with in/out lists ordered by
   `sequence`; MACIP → `acl.macip-acl` + `acl.macip-interface-binding`. Expansion > 10 000 VPP rules for one list → DryRun error with
   the pointer. Descriptor changes only if a gap shows up (you own `descriptors/acl` for this task).
2. **Counters**: `acl.StatsReader` → per-rule packets/bytes mapped back to the **config rule** (sequence) through the expansion map;
   requires `acl.stats-enable` (globals owner) — non-owner slots report "counters unavailable" rather than failing.
3. **API**: config via pointer routes; `GET /api/v1/state/acl/lists/{name}/rules?page&pageSize&filter` (server-side paging, expanded
   count + hits per rule), `GET /api/v1/state/acl/attachments`, `POST /api/v1/actions/acl/import` (CSV → rules, dry-run first, max 100k
   rows, streamed) and `GET …/export.csv`.
4. **UI**: ACL page — list of lists; **rule editor** on `ServerDataGrid` (virtualised, 100k rows, search, drag/`move to sequence`
   reorder, bulk enable/disable, inline object-picker), CSV import/export, hit-counter columns refreshed from WS, attachments tab
   (interface/zone, direction), MACIP tab; an info link to ADL/Auto-SDL (F-rpf-adl-pbr) — no ADL screens here; en + fa.
5. **Docs**: `docs/user/firewall/acl.md` — stateless vs reflect, zones, MACIP, counters caveat (V7), CSV format, CLI equivalent.

## Acceptance (paste the evidence)
- [ ] `vppctl show acl-plugin acl` and `show acl-plugin interface` reflect the committed lists/bindings (pasted); `Retrieve()` == desired
- [ ] Hit counters: traffic on the rig (`ip netns exec ns-<p>-lan ping …`) increments the matching rule's counter in the API (pasted)
- [ ] 100k-rule list: projection time, `acl_add_replace` time and first-page API latency measured and pasted; UI scroll screenshot
- [ ] Agent-restart simulation → bindings back within 30 s; foreign ACL on the same interface untouched (D-066)
- [ ] Rollback removes lists and bindings (Retrieve empty for the owner)
- [ ] Rule referencing an empty group → 400 problem+json with `pointer`; `tools/ci.sh --base main` green; UI screenshot pasted

## Out of scope (do not build)
Host/management-plane ACLs and nftables (F-host-acl-nftables); objects CRUD and FQDN resolution (F-object-model); ADL / Auto-SDL /
uRPF / ABF descriptors and screens (F-rpf-adl-pbr, which owns `descriptors/{adl,auto_sdl,abf}`); ACL session state sync for HA
(F-ha-state-sync); NAT (F-nat44-*); classifier-based ACLs; per-interface counters (VPP has none).

## Open questions to surface, not to decide silently
Implicit default action when a list ends: VPP default deny on an interface with an input list — document, do not add hidden rules.
Whether logging (`log: true`) can be honoured at all (VPP acl has no per-rule log) — default: flag it as unsupported in DryRun.
