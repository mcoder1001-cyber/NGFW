# F-rpf-adl-pbr — questions and decisions for the manager

Written while working; nothing here blocks the task (00-CONTEXT: never stop on a conflict). Each item says what I did.

## Q1 — Incident 18:41:08 (VPP SIGSEGV, NRestarts 0→1): not this slot
My host runs on the shared VPP (slot 10, `w10`): `TestRpfAdlPbrOnHost` + `TestADLRetrieveV23OnHost` 18:36:07–18:36:35
(loopbacks `loop1001`/`loop1002`/`loop1003` at sw_if_index 5/3/5, all deleted by 18:36:35, NRestarts 0 before and after),
`TestAutoSdlOnHost` ~18:37 (skip: FEATURE_DISABLED, no interface touched). From 18:38 until the evidence run after 19:15 this
slot sent nothing to VPP (API unit/e2e tests use the fake agent). No code of this task sweeps classify bindings: interfaces are
created by P05 core / DF-1 (TD-3's `ifsanitize.Acquire` inside), the tests delete them with `ifsanitize.BeforeDelete`
(the per-interface clearing D-126 allows); the 18:40:57/59 sweeps on sw_if_index 2/7 are not from slot 10.

## Q2 — PBR needs acl.acl, which only F-acl registers (decision taken: observe-only bridge)
`abf.policy` depends on `acl.acl/<name>` (DF-4 key). This build registers no `acl.acl` descriptor (F-acl does, with the
`acl` domain), so every PBR policy would fail with "dependency missing" in the product agent. **Decision:** an agent-local,
observe-only `pbr.acl-ref` descriptor (`subsystems/rpf_adl_pbr.go`, no domain) retrieves this owner's tagged ACLs and
provides `acl.acl/<name>` as a KeyProvider alias; it never writes and switches itself off as soon as an `acl.acl` descriptor
is registered (checked on the registry at plan time), so after F-acl merges nothing changes and F-acl's descriptor (which
also sees ACL deletes) is the only source. Options were: (a) this bridge [chosen]; (b) register DF-4's `acl.acl` here — a
duplicate registration (panic) once F-acl merges; (c) leave PBR unusable until F-acl. Please confirm or pick (c); dropping
the bridge later is a 3-line removal.

## Q3 — ABF policy ids and names (decision taken)
The contract keys policies by name; VPP numbers them and stores no name. Ids are FNV-1a(name) into the agent's range
(`SlotIDRange()` on the shared host, 1..2^31-1 in the product), linear probing in name order (stable unless names collide).
An agent-local `pbr.policy/<name>` record (persisted `<state dir>/pbr-<owner>.json`, depends on its `abf.policy`) maps ids
back to names for Retrieve and keeps the policy's priority (VPP keeps priorities only on attachments). Alternative: a
persisted allocator (sticky ids, but projection then depends on store state). A policy/attachment VPP holds without a
record is reported as `#<id>` (a leftover the next commit deletes).

## Q4 — `/state/pbr` has no ACL hit counters
DF-4's `acl.StatsReader` exists, but the agent's stats reader (A5 `telemetry.go`, read-only for me) exposes interface
counters only, and the ACL plugin counts only with `acl-stats` on (globals owner). A `PbrState` RPC with counters would need
both. `/state/pbr` therefore returns `counters: {available: false, reason}`; no RPC was added (no `contract(proto)` commit).

## Q5 — ADL allow-list: VPP crash vector (V-new) and `defaultAllow` semantics (decision taken)
Source analysis of `plugins/adl/adl.c`: every `adl_allowlist_enable_disable` removes the families not requested and a
remove of an unconfigured family stores config index `~0` → `adl-input` reads `heap[~0]` on the next packet of that family;
the non-IP ("default") allow-list node is a stub that leaks buffers. DF-2's old Create (`ip4` only) and Delete (all clear)
both hit it (no packets were ever sent through an ADL port on the shared host, so it never fired). Fixed in `descriptors/adl`
with the two-call sequences (V-new appended to docs/vpp-code-track.md). The contract's `defaultAllow` is read as "non-IP
frames pass" (default true); `false` would need the stub, so the agent refuses it (`interfaces.rpf-adl-pbr-adl-non-ip`).
Please confirm the naming/semantics; the alternative (map `defaultAllow` directly to `default_adl`) would make `true` the
broken setting.

## Q6 — drift: write-only leaves and the services domain
- ADL's allow-list leaves (`allowVrf`, `ipv4`, `ipv6`, `defaultAllow`) cannot be read back; DryRun marks them
  `agent.write-only` (warning). P08's `driftOf` (`apps/api/src/state/state.controller.ts`, not mine) ignores only
  `agent.unsupported-field` / `agent.unimplemented-domain`, so `/state/drift` will show those four leaves as drift while ADL
  is on. Proposal (one line for the owner of state.controller.ts): add `agent.write-only` to `COVERAGE_RULES`.
- This task is the first to implement `services` (`Domains["services"] = auto-sdl.config`). Nothing of services is
  retrievable (autoSdl is write-only; the rest is not implemented), so Retrieve reports no services member and DryRun notes
  `/services` as `agent.unimplemented-domain` (keeps the domain out of drift) plus `agent.unsupported-field` for every other
  non-empty member. When F-loopback-bvi-gso-lldp-span (nsim) adds a readable services leaf, that note must become
  field-level; please keep this in mind at merge (both envelopes claim `Domains["services"]` — the `Services = "services"`
  constant and the map entry will conflict textually; the union is `Services: {rpfAdlPbrAutoSdl, <nsim>}`).

## Q7 — edits outside my file list (all minimal, listed as shared hunks in F-rpf-adl-pbr.md)
- `apps/agent/internal/descriptors/core/coretest/fakevpp.go`: A6 said "existing files are read-only", but `coretest.New()`
  has no extension point, and registering uRPF/ADL/ABF in the product registry makes P08's agent unit tests call
  `urpf_interface_dump` etc. on the model. I added a generic seam (`var extensions []func(*VPP)` + a loop after the
  sanitizer model); my model lives in the new `coretest/rpf_adl_pbr.go` (init appends). Other wave-A features can use it.
- `apps/agent/internal/agent/service_test.go` (P08): two assertions hard-coded the implemented domains as
  `"interfaces,vrfs,routing"`, and two "a converged resync sends only dumps" loops did not allow `feature_is_enabled`
  (adl.interface's read-only read-back; the adl plugin has no dump). Now they compare with `implementedDomains()` and allow
  that one getter. Every feature adding a domain would have hit the first.
- `descriptors/adl` (gap fix, owned): `NewAllowlist`/`RegisterWriteOnly` got variadic options (source-compatible; DF-2's
  idempotency test still compiles).

## Q8 — interface drawer strings (i18n)
The P08 drawer titles fields/groups from the `interfaces` namespace (`interfaces.json`, not mine). The Security group's
strings live in my namespace (`drawer.*`) and `domains/routing/rpf-adl-pbr/drawer-i18n.ts` merges them into the
`interfaces` resources before i18next initialises (imported under my i18n.ts anchor; never overwrites an existing key).
Nested labels inside the group (e.g. "IPv4 check") come from the schema title (English) in the drawer — the drawer's
`localizeSchema` is one level deep; the ADL / Auto-SDL page localizes them fully. A `fallbackNS` or a W5 section registry
would make the merge unnecessary.

## Q9 — not done / follow-ups
- "attachment interface is not bridged" (prompt §1): F-bridge-l2's `interfaces.<if>.l2` is not on main — not implemented.
- `classify.output-acl` still trusts `feature_is_enabled` (V23 a) — not projected by this task, not fixed (as instructed).
- The ADL allow-list VRF needs *local* (receive) entries for the allowed sources; `routing.static` cannot express a local
  next hop — a follow-up for F-vrf-static-ecmp if the product owner wants ADL usable end to end.
- Schema examples: `packages/schema/examples/rpf-adl-pbr-*.json` would fail P02's `examples.test.ts` ("every other file
  belongs to a sibling group"), so the valid document went to `packages/proto/test/fixtures/rpf-adl-pbr-full.json` and the
  schema cases into `semantic/rpf-adl-pbr.test.ts`. Adding `rpf-adl-pbr` to that test's SIBLING regex would allow them.
- Sub-interfaces: urpf/adl are not mirrored on `Subinterface` (16–17 unused); ADL cannot work on sub-interfaces anyway.
- Auto-SDL host test runs only in a manager window with the session layer's SDL backend (`VRX_AUTOSDL_GLOBALS=1`).

## Fix round 1 (review 9da6a2f) — status of the items above
- Q2: kept (review: sound). The harness no longer registers `acl.acl` when the name is taken, the product skips the bridge
  when `acl.acl` is registered first, and `TestACLBridgeRegistration` covers both orders. **Please put the removal of
  `pbr.acl-ref` + `TestRpfAdlPbrWithoutFAcl` into F-acl's envelope** (the bridge is inert after F-acl).
- Q3: ids are now sticky (recorded ids from the `pbr.policy` store are kept; only new names are probed) — review L2.
- Q6: resolved — the manager-approved line adds `agent.write-only` to `COVERAGE_RULES`; the `/services` domain-level note is
  gone (field-level notes only), implemented services members are the append-only `desired.ServicesMembers` set
  (`func init() { ServicesMembers["<member>"] = true }` in the feature's own file). Retrieve now reports `services` as a
  present, empty domain, so P08's canonical documents in `agent/service_test.go` and `agent/agent_integration_test.go` gain
  `"services": {}`. F-loopback's envelope: reuse the `Services` const and map entry, append its descriptor name there.

## Q10 — coretest handler collisions at merge (review M1, for the manager)
`coretest/rpf_adl_pbr.go` installs, through the `extensions` seam (after everything in `New()`), handlers for
`feature_is_enabled` (answers only device-input/adl-input and ethernet-input correctly, `true` for everything else, like VPP's
error encoding), `adl_interface_enable_disable` (replaces sanitizetest's model for every coretest user) and `acl_dump`.
F-bridge-l2 installs its own `feature_is_enabled` model for mactime (also device-input) in `New()`; after both merge, mine
wins silently (`fake.Client.On` overwrites) and mactime reads "enabled" everywhere. F-acl will bring its own `acl_dump`
model. Git shows no conflict. At merge: fold the `feature_is_enabled` handlers into one dispatcher keyed by (arc, feature)
(or seed a shared coretest feature registry), keep one `extensions` seam (F-nat44-ed-sessions added an identical one), and
let F-acl's ACL model replace my `acl_dump`/`AddACL`.
