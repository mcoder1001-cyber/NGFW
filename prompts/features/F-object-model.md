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
The **config** side exists. The **state** side does not: the API can only get FQDN resolution results from the agent through a new
read-only RPC (P08's live-state pattern; name per `docs/status/wave-A-hotspots.md` §2: `FqdnObjectState` → per FQDN object: addresses, lastResolved, nextRefresh, error; never
status in `Retrieve`, docs/contracts/proto.md §5). Add it additively on `contract/F-object-model` (`contract(proto): objects state` +
`docs/status/tasks/F-object-model-contract.md`), plus any config field you need (e.g. `fqdn.refreshSec`, `fqdn.maxAddresses`:
schema + proto + drift guard), tell the manager in the questions file and continue. No renames/reshapes (PENDING).

## Scope — build exactly this
Files you own: `apps/agent/internal/objects/**`, `apps/agent/internal/agent/project_object_model*.go`,
`apps/api/src/features/object-model/**`, `apps/web/src/domains/firewall/object-model/**`, `apps/web/src/locales/*/object-model.json`,
`docs/user/firewall/object-model.md`, `test/topology/object-model/**`. Shared files (P08 layout; protocol and anchors in
`docs/status/wave-A-hotspots.md`; list each hunk in your PR): `apps/agent/internal/subsystems/subsystems.go` (`Domains["objects"]` +
registration; the resolver starts from an owned `subsystems/object_model.go`, never from `agent.go`), `apps/agent/internal/agent/projection.go`
(one call in `project()`, one in `assemble()`; the state RPC method goes in an owned `rpc_object_model.go`), `apps/api/src/agent/agent.client.ts`,
`app.module.ts`, web router/nav/`i18n.ts`; `packages/api-client` regenerated.
1. **Agent library** `internal/objects`: `Expand(doc, ref) → []netip.Prefix` / `ExpandService(ref) → []PortSpec` with cycle
   detection (defence in depth — the schema already rejects cycles), range → minimal CIDR set, IPv4/IPv6 split, deterministic
   sorted output, size cap (error above 10 000 expanded entries per rule, reported as a DryRun issue with the pointer).
   Schedules: `Active(schedule, now, tz)`; the consumer decides what an inactive rule does.
   `objects` has no VPP objects, but it must be an implemented domain in P08's `subsystems.Domains` (so it is in `Health.subsystems`
   and the API sends it) and `Retrieve` must return the applied objects document, or `/state/drift` reports every object as missing.
   Default: an agent-local `objects.*` descriptor family (Create/Delete update a store persisted in the agent state dir, Retrieve
   reads it; the resolver and `Expand` read the same store) — Apply, rollback and restart then work through the scheduler with no
   agent-core edit. The alternative (a D-073b-style hook in `internal/agent/service.go`) touches agent core, which is read-only in
   wave A (`docs/status/wave-A-hotspots.md` A5) — questions file first.
2. **FQDN resolver**: resolves `type: fqdn` objects via the system resolver (no exec), A + AAAA, TTL-bounded refresh (min 30 s,
   max 1 h), last-good answers kept on failure, results persisted in the agent state dir and exposed through the state RPC above;
   a change emits an in-agent notification that triggers a re-projection of dependent consumers (F-acl wires the trigger on its
   side). An unresolved FQDN expands to nothing (schema comment) and raises a warning, never an error.
   Go's `net.Resolver` does **not** return TTLs: either query the `/etc/resolv.conf` servers yourself with
   `golang.org/x/net/dns/dnsmessage` (x/net is already in `apps/agent/go.mod` as an indirect dependency — promote it and say why) or
   refresh on a fixed interval clamped to [30 s, 1 h]; pick one and document it. The test resolver is an in-process DNS responder
   on `127.0.0.1:0` (ephemeral port) — never port 53, never the host's Unbound or systemd-resolved configuration.
3. **API**: config via the generic pointer routes; `GET /api/v1/state/objects/fqdn` (name → addresses, lastResolved, error),
   `GET /api/v1/state/objects/usage?name=` (where-used: ACL rules, NAT, zones — computed from the running document).
4. **UI**: Objects page with tabs Addresses / Address groups / Services / Service groups / Schedules / Zones / Tags (SchemaForm,
   object-picker, tag chips with colour), where-used drawer, FQDN resolution column; en + fa. `object-picker` is **not** a ui-kit
   built-in widget: SchemaForm takes custom widgets through its `widgets` prop — build the picker in your directory and export it
   (F-acl and F-host-acl-nftables import it); do not edit `packages/ui-kit`.
5. **Docs**: `docs/user/firewall/object-model.md` (examples: web servers group, FQDN object, office-hours schedule, LAN zone)
   with the CLI/REST equivalent.

## Acceptance (paste the evidence)
- [ ] Unit tests: nested groups, range→CIDR, v4/v6 split, cap exceeded, schedule edges (DST change, `once` window) — output pasted
- [ ] FQDN: an object for a name served by a test resolver (in-process, 127.0.0.1:0) resolves, refresh observed (TTL-bounded or the fixed interval you chose), resolver down → last-good kept (log)
- [ ] Agent-restart simulation → FQDN state reloaded from the state dir without a new query storm (log excerpt)
- [ ] Deleting an object still referenced by an ACL rule → 400 problem+json with `pointer` to the referencing rule (the existing
      `acl.rule-references` rule should already produce it — prove it end to end, add a rule only if it does not)
- [ ] Rollback restores the previous object set (running doc + where-used output)
- [ ] UI screenshot of the Objects page against the real endpoint; `tools/ci.sh --base main` green

## Out of scope (do not build)
ACL rules, attachments, hit counters, the rule editor (F-acl); host lists / nftables (F-host-acl-nftables); NAT use of objects
(F-nat44-ed-sessions, F-nat44-ei-64-66-nptv6); Unbound configuration (F-unbound-chrony-syslog); zone-based policy semantics beyond
"zone = set of interfaces"; GeoIP/threat-feed objects; VDOM scoping (D-003).

## Open questions to surface, not to decide silently
Should an inactive schedule remove the rule (re-render) or rely on a periodic re-projection? Default: re-projection every 60 s by the
consumer; document. Does the FQDN resolver use the box's Unbound or the management resolver? Default: system resolver.
