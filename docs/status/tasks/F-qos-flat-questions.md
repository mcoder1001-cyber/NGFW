# F-qos-flat — questions and decisions for the manager

Written while building; nothing here blocked the work. Numbers are referenced from F-qos-flat.md and code comments.

## Q1 — hunks without an F-qos-flat anchor (appended at the end of the block, marked "unanchored")
The W-seed-BC anchor pass seeded `// wave-BC: F-qos-flat` in subsystems.go `Domains`, app.module.ts, agent.client.ts,
fake-agent.ts, i18n.ts, services/tabs.ts and dataplane.proto — but **not** in:
- `apps/agent/internal/subsystems/subsystems.go` — the `register()` feature-families block (`w.registerQoS(r)` after
  `// wave-A: F-unbound-chrony-syslog`), plus two import lines (`descriptors/policer`, `descriptors/qos`);
- `apps/agent/internal/agent/projection.go` — `project()` and `assemble()` (one `if in["services"]` block each, after the
  last wave-A anchor);
- `packages/schema/src/semantic/index.ts` — the import and the `...qosFlatValidators` spread (after the last anchor);
- `docs/contracts/proto.md` §11 — the `### F-qos-flat: QosPolicerState, QosPolicerReset` section (at the end);
- `apps/web/src/nav/nav.ts` `BUILT_DOMAINS` and `nav.test.ts` — `'services'` (see Q4).
Please seed anchors there for the remaining wave-C tasks, or keep these as they are at merge.

## Q2 — "mark requires map" and "shaper vs policer.output" are not repeated as semantic rules (decision)
Both are already enforced by the **schema tier** (`QosInterfaceSchema`: `mark.map` is a required key; its superRefine
refuses `shaper` + `policer.output` at `/…/shaper`). Tier (b) only ever sees schema-valid documents, so a semantic copy
would be dead code with a second rule id. `semantic/qos-flat.ts` adds the real gap only (`services.qos-flat-store-source`);
`semantic/qos-flat.test.ts` pins both schema rules, the API e2e shows all three as 400 with pointers, and the agent's
projection refuses the same three again (DryRun ERRORs with pointers) for a document that reaches it another way.
Options were (a) add duplicates anyway, (b) pin the existing ones — took (b).

## Q3 — no `packages/schema/examples/qos-flat-*.json`
`packages/schema/src/examples.test.ts` fails any example whose name is outside its sibling groups
(`(invalid-)?(nat|objects|acl|vpn|tunnels|services|ha)-…`), so the envelope's `qos-flat-*.json` name cannot live there
without editing that test (not mine). The valid document is `packages/proto/test/fixtures/qos-flat-full.json` (picked up by
the proto round-trip tests and the Go contract tests); the semantic cases are inline in `semantic/qos-flat.test.ts`.
If you want a schema example, it would be `services-qos-flat.json` (matches the sibling regex).

## Q4 — merge with the other `services` features (F-kea-dhcp-relay approved, F-unbound-chrony-syslog, F-rpf-adl-pbr)
Every branch that implements a `services` sub-key brings the same scaffolding; the union at merge:
- `desired`: keep ONE copy of `ServicesImplemented` / `ServicesUnsupported` (mine: `desired/qos_services.go`, identical in
  shape to F-kea's `desired/kea.go`); keep each feature's `init()` line (`ServicesImplemented["qos"] = true` is in
  `desired/qos.go`); `ServicesUnsupported` must be called **once** per projection — my projection hunk calls it, F-kea's
  `desired.DHCP` calls it too: drop one of the two calls.
- `subsystems.go`: one `Services` const (F-kea/F-rpf add `Services = "services"`; I wrote the key as the literal
  `"services"`) and ONE `Domains` entry: the union of the name lists (mine: policer.policer, policer.interface,
  qos.egress-map, qos.record, qos.store, qos.mark, qos.meta).
- `nav.ts` / `nav.test.ts`: `'services'` once in `BUILT_DOMAINS` and once in the expected list.
- `service_test.go` `canonicalDoc`: `"services": {"dhcp": {}, "qos": {}}` (F-kea adds `dhcp`, I add `qos`); the two
  `implementedDomains()` hunks are identical to F-kea's.

## Q5 — shapers kept, shown as "Rate limits (egress)"; derived burst ≥ 3000 B (decisions)
The prompt's open question: keep `shapers`, realise them as an egress policer, label them "rate limit" in the UI — done
as proposed (UI tab "Rate limits (egress)" with the V3 caveat; API/agent keep the schema name). One deviation from the
schema's help text "≈ 10 ms worth of traffic": the derived burst is `max(ceil(rateKbps × 1.25), 3000)` bytes, because a
1r2c bucket smaller than one frame drops **every** full-size packet (VPP conforms a packet only when the bucket holds its
whole length): at 1 Mbit/s, 10 ms is 1250 B < 1500 B. Options: (a) pure 10 ms, (b) 10 ms with a two-frame floor — took
(b); the same formula is in the agent (`desired.ShaperBurstBytes`), the API (`shaperBurstBytes`) and the UI.

## Q6 — `agent.write-only` in the drift view
Policer and shaper attachments are write-only (no VPP dump, D-063): Retrieve leaves them unset and DryRun marks them with
rule `agent.write-only` (at `/services/qos/interfaces/<if>/policer`, `/shaper`, or the whole interface entry when it has
nothing retrievable). The API's drift view ignores that rule only after F-rpf-adl-pbr merges (it adds `agent.write-only`
to `COVERAGE_RULES` in `state.controller.ts`, not mine). Until then `/state/drift` lists QoS attachments as missing.

## Q7 — VPP findings (vpp-code-track "V-new (F-qos-flat)", please number)
(a) `qos_mark` has no interface-delete hook: the mark config survives the delete, and on a reused sw_if_index
`qos_mark_enable` returns 0 without enabling the feature (silent, reported as marking by `qos_mark_dump`). Proposal for
the ifsanitize owner (TD-3): `ifsanitize.Acquire` sends `qos_mark_enable_disable(enable=0)` for each source on a new
interface. (b) `policer_del` leaves interface bindings dangling (bound by pool index): packet-path use-after-free on the
attached interface. The agent re-points attachments after a policer re-creation (fixed here, tested) and never deletes a
policer before its attachments; nothing else can be done without C code.

## Q8 — files touched outside the envelope's lists (all minimal, please confirm)
- `apps/agent/internal/descriptors/core/coretest/fakevpp.go`: ONE line `v.installQoSFlat()` in `New()` (A6), plus the new
  `coretest/qos_flat.go` model. Needed: with `services` implemented, every agent test that retrieves all domains dumps
  `policer_dump_v2` / `qos_*_dump`, and the bare coretest model answered "no handler". TD-23's `RegisterExtension`
  turns it into one `init()` line in `qos_flat.go`.
- `apps/agent/internal/agent/service_test.go` (A5 test): `canonicalDoc` gains `"services": {"qos": {}}`; the two
  `interfaces,vrfs,routing` subsystem checks use `implementedDomains()` (identical to F-kea's hunks); the three
  "read-only calls only" checks accept `policer_dump_v2` (DF-7's read-only policer walk; its name does not end in `_dump`).
- `apps/api/src/testing/fake-agent.ts`: the import of `qosFlatFake` (outside the anchor, like F-kea's).
- `apps/web/src/domains/services/tabs.ts`: `import { lazy } from 'react'` at the top (outside the anchor, like F-kea's).

## Q9 — no id range (Env.IDs zero) → QoS registers fail-closed instead of failing the agent (decision)
`Wiring.IDRange()` recommends failing the registration on `ErrNoIDRange`. That would fail every existing agent test that
registers without an id range. The QoS family registers with the empty range instead (logs a warning): policers, records,
stores and marks work, every egress map is refused with a pointer (`services.qos-flat-map-id`). The product agent always
has a range (TD-8b). Options: (a) fail the registration, (b) fail closed per object — took (b).

## Q10 — the projection's map-id range is a package variable (`desired.SetQoSMapIDRange`)
`project()` has no access to the Wiring, and its signature is A5/A2 core. The F-qos-flat registration sets the range the
projection allocates from (the same range the egress-map descriptor enforces). A cleaner seam would be a projection
option on the Service (A5, manager). One process = one agent, so the global is safe in the product.

## Q11 — new `qos.meta` record descriptor (please review)
VPP cannot name egress maps or keep descriptions, so Retrieve could not reproduce `services.qos` (maps would come back as
ids). `descriptors/qos/meta.go` adds an agent-local record (`qos.meta/services.qos`, `<state dir>/qos-<owner>.json`,
persisted, `CheckPersistent`) holding names ↔ ids, descriptions, explicit id/burst flags and row order; the assembler
uses it only to name and decorate objects VPP returned (it never makes an object appear). Same idea as F-kea's
`dhcp.relay` record.

## Q12 — no `vrx show qos` CLI command
apps/cli is not mine; the user guide documents `vrx set/merge services qos …`, the REST state/reset routes and the
`vppctl show …` equivalents. A `show qos policers` command (→ `QosFlat_policers`) is a small P13 follow-up.

## Q13 — pending until host runs reopen (TD-25)
`test/topology/qos-flat/run.sh` (loopbacks only, no packets, no af_packet): commit → Retrieve + `vppctl show policer`,
`show qos egress map`, `show qos mark`, `show qos record`, `show qos store`, `show interface features`; agent restart with
simulated loss; rollback; NRestarts before/after. Also the screenshots of the QoS tab against the real endpoint. Nothing
was run on the shared VPP.

## Q14 — web: the attachment dialog is a custom form
On main, SchemaForm materialises optional nested objects (`store`, `mark`, `policer`) as `{}`, which the schema then
refuses; WEB-1 (approved, merging) fixes that. The attachment dialog is therefore a small custom form; switch it to
SchemaForm once WEB-1 is on main? (Policers, rate limits and maps use SchemaForm / the grid.)

## Q15 — cost note
`policer.interface` Create now looks the policer's pool index up (one policer walk) to detect a re-created policer; a
resync costs one walk per attachment. Fine at flat-QoS scale (tens of policers); a shared per-transaction cache would be
the fix if it ever matters.
