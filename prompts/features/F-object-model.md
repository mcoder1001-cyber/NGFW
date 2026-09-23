# Task: F-object-model — firewall objects: addresses, groups, FQDN, services, schedules, zones, tags   (prepend 00-CONTEXT.md)

## Goal
Make the `objects` domain usable end to end: address / network / range / FQDN objects, address and service groups,
service objects, schedules, zones and tags — with an agent-side **expansion library** that turns object references
into concrete prefixes/ports for the consumers (F-acl, F-host-acl-nftables, NAT) and an agent-side **FQDN resolver**.
There are no VPP objects in this task. Reference: TNSR "Address / port tables", pfSense-style aliases (WBS D5.1).

## Inputs to read first
- `packages/schema/src/domains/objects.ts` + `semantic/objects.ts` (P02b, merged, contracts-v1): `objects.{addresses,
  addressGroups, services, serviceGroups, schedules, zones, tags}` already exist with rules `objects.address-group-members`,
  `…service-group-members`, `…names-disjoint`, `…schedule-valid`, `…zone-interfaces`, `…tags-exist`, `…address-range-valid`
  — reuse, do not fork. D-062: one shared namespace across addresses/services/groups; groups may be empty, a rule may not
  reference an empty group; `nat.enabled` inference is not yours.
- `docs/contracts/schema-nat-objects-acl.md` (objects table, examples `acl-basic.json`), `packages/proto/vrx/v1/dataplane.proto`
  (`Objects` messages) and D-078 (`vrx.model.acl.v1` agent-internal models)
- `apps/agent/internal/descriptors/acl/spec.go` (DF-4) — the shape the expansion must produce for F-acl (prefix + port ranges +
  proto + tcp flags), and `docs/agent/descriptors/acl.md`
- `apps/agent/internal/renderers/unbound` (RF-3) only to know what resolver the box runs — you do **not** configure it

## Contract changes
Everything needed exists. If you need a field (e.g. `fqdn.refreshSec`, `fqdn.maxAddresses`), add it additively on
`contract/F-object-model` (`contract(schema): objects …` + proto + drift guard + `docs/status/tasks/F-object-model-contract.md`),
tell the manager in the questions file and continue. No renames/reshapes (PENDING).

## Scope — build exactly this
Files you own: `apps/agent/internal/objects/**`, `apps/agent/internal/agent/project_object_model*.go`,
`apps/api/src/features/object-model/**`, `apps/web/src/domains/firewall/object-model/**`, `apps/web/src/locales/*/object-model.json`,
`docs/user/firewall/object-model.md`, `test/topology/object-model/**`. Shared files: one-line appends only (app.module import,
router/nav entry, projection registration hook).
1. **Agent library** `internal/objects`: `Expand(doc, ref) → []netip.Prefix` / `ExpandService(ref) → []PortSpec` with cycle
   detection (defence in depth — the schema already rejects cycles), range → minimal CIDR set, IPv4/IPv6 split, deterministic
   sorted output, size cap (error above 10 000 expanded entries per rule, reported as a DryRun issue with the pointer).
   Schedules: `Active(schedule, now, tz)`; the consumer decides what an inactive rule does.
2. **FQDN resolver**: resolves `type: fqdn` objects via the system resolver (Go `net.Resolver`, no exec), A + AAAA,
   TTL-bounded refresh (min 30 s, max 1 h), last-good answers kept on failure, results persisted in the agent state dir and
   exposed as read-only state; a change emits an event that triggers a re-projection of dependent consumers (F-acl wires the
   trigger on its side). An unresolved FQDN expands to nothing (schema comment) and raises a warning, never an error.
3. **API**: config via the generic pointer routes; `GET /api/v1/state/objects/fqdn` (name → addresses, lastResolved, error),
   `GET /api/v1/state/objects/usage?name=` (where-used: ACL rules, NAT, zones — computed from the running document).
4. **UI**: Objects page with tabs Addresses / Address groups / Services / Service groups / Schedules / Zones / Tags (SchemaForm,
   object-picker, tag chips with colour), where-used drawer, FQDN resolution column; en + fa.
5. **Docs**: `docs/user/firewall/object-model.md` (examples: web servers group, FQDN object, office-hours schedule, LAN zone)
   with the CLI/REST equivalent.

## Acceptance (paste the evidence)
- [ ] Unit tests: nested groups, range→CIDR, v4/v6 split, cap exceeded, schedule edges (DST change, `once` window) — output pasted
- [ ] FQDN: an object for a name served by a test resolver (slot port) resolves, TTL refresh observed, resolver down → last-good kept (log)
- [ ] Agent-restart simulation → FQDN state reloaded from the state dir without a new query storm (log excerpt)
- [ ] Deleting an object still referenced by an ACL rule → 400 problem+json with `pointer` to the referencing rule
- [ ] Rollback restores the previous object set (running doc + where-used output)
- [ ] UI screenshot of the Objects page against the real endpoint; `tools/ci.sh --base main` green

## Out of scope (do not build)
ACL rules, attachments, hit counters, the rule editor (F-acl); host lists / nftables (F-host-acl-nftables); NAT use of objects
(F-nat44-ed-sessions, F-nat44-ei-64-66-nptv6); Unbound configuration (F-unbound-chrony-syslog); zone-based policy semantics beyond
"zone = set of interfaces"; GeoIP/threat-feed objects; VDOM scoping (D-003).

## Open questions to surface, not to decide silently
Should an inactive schedule remove the rule (re-render) or rely on a periodic re-projection? Default: re-projection every 60 s by the
consumer; document. Does the FQDN resolver use the box's Unbound or the management resolver? Default: system resolver.
