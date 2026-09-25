# F-loopback-bvi-gso-lldp-span — loopback/BVI, GSO, LLDP, SPAN/ERSPAN, nsim

Slot 7 (`w7`), branch `task/F-loopback-bvi-gso-lldp-span`, speculative base `task/F-bridge-l2@a735aa9` (D-114).
**Merge F-rpf-adl-pbr first** (manager 23:30, D-131: this branch reuses its `services` seam — see "Expected union at merge").

## What was built

| layer | what |
|---|---|
| contract (schema) | `packages/schema/src/domains/ext/loopback-bvi-gso-lldp-span.ts`: `interfaces.<if>.gso?`, `interfaces.<if>.mirror?[]{destination, direction rx\|tx\|both (both), level device\|l2 (device)}` (≤ 8), `services.nsim?{delayMs, bandwidthMbps, packetSize (1500), dropFraction (0), crossConnect?{a,b}, outputInterfaces[]}`; semantic rules `interfaces.loopback-bvi-gso-lldp-span-{reserved-loopback (D-105: loop16000–16383), gso-interface, mirror-destination, mirror-loop, mirror-duplicate}`, `services.loopback-bvi-gso-lldp-span-nsim-{range, interfaces}` |
| contract (proto) | `Interface.gso` 20, `Interface.mirror` 21 (`MirrorSession`), `ServicesConfig.nsim` 9 (`NsimService`), `rpc LldpNeighbors` (`LldpNeighborsRequest/Response`, `LldpNeighbor`); fake-agent stub; proto.md §11; details in `F-loopback-bvi-gso-lldp-span-contract.md` |
| agent descriptors | new `descriptors/gso` (`gso.interface`: applied once per VPP boot + `feature_is_enabled` read-back → readable, questions Q4), new `descriptors/nsim` (`nsim.config`, `nsim.cross-connect`, `nsim.output`: write-only, applied once per boot and value, globals owner only); DF-7 `lldp` / `span` wired (span: a vanished destination is cleared by Delete); TD-11b ownership declarations on all seven (`ownership.go`) |
| agent wiring | `desired/{gso,mirror,lldp,nsim}.go` + `gso_lldp_mirror_nsim.go` (entry points), `desired/lldp_services_seam.go` (merge seam — delete at the merge), `subsystems/loopback_bvi_gso_lldp_span.go` (registration, `services` names appended in init), `LldpNeighbors` RPC (`agent/rpc_loopback_bvi_gso_lldp_span.go`, owner-scoped, paged ≤ 1000, one walk at a time), coretest model `coretest/loopback_bvi_gso_lldp_span.go` |
| API | `GET /api/v1/state/lldp/neighbors?page&pageSize` (`LoopbackBviGsoLldpSpanController`, `apps/api/src/features/loopback-bvi-gso-lldp-span/`, fake in `fake.ts`); config through the generic pointer routes; GSO and mirror sessions appear in `/state/interfaces` `config` |
| UI | Interfaces → **LLDP** (settings form + live neighbour table, 30 s + Refresh), Interfaces → **Port mirroring** (all sessions, live status from `/state/interfaces`, pending mark, add/edit/remove as merge patches, 30 s + Refresh), Tools → **Delay simulator (lab)**; GSO and mirror in P08's generated drawer (no drawer code, no model.ts exclusion needed); en + fa (`loopback-bvi-gso-lldp-span` namespace) |
| docs | `docs/user/interfaces/loopback-bvi-gso-lldp-span.md` (loopback as BVI, GSO, SPAN/ERSPAN to a GRE tunnel, LLDP, nsim as a lab tool, CLI), `docs/agent/descriptors/{gso,nsim}.md` (new), `{lldp,span}.md` (wiring notes), see-also line in `basics.md`, `### V-new (F-loopback-bvi-gso-lldp-span)` in `docs/vpp-code-track.md` |

Loopbacks: nothing new — P08 creates `loop<N>`, F-bridge-l2 makes it the BVI; this task verifies and documents it (host check
below: `show bridge-domain 7750 detail` → `BVI-Intf loop775`, restart and rollback included).

## Shared hunks (all under `wave-A: F-loopback-bvi-gso-lldp-span` unless said otherwise)

| id | file | hunk |
|---|---|---|
| A1 | `apps/agent/internal/subsystems/subsystems.go` | `loopbackGso`, `loopbackSpanMirror` in `Domains[Interfaces]`; one comment line under the new-domain anchor (services names come from init in my file); `w.registerLoopbackBviGsoLldpSpan(r)` in `Register` |
| A2 | `apps/agent/internal/agent/projection.go` | one call in `project()`, one in `assemble()` |
| A6 | `apps/agent/internal/descriptors/core/coretest/fakevpp.go` | 2 lines after `sanitizetest.Clean` (comment + `v.installLoopbackBviGsoLldpSpan()`) — **no anchor** (as F-bridge-l2 Q9; D-134: becomes one TD-23 registration line) |
| A7 | `docs/vpp-code-track.md` | appended `### V-new (F-loopback-bvi-gso-lldp-span)` |
| C1 | `packages/schema/src/domains/interfaces.ts` | `gso`, `mirror` key lines; **one import line at the top (no import anchor)** |
| C1 | `packages/schema/src/domains/services.ts` | `nsim` key line; **one import line at the top (no import anchor)** |
| C2 | `packages/schema/src/semantic/index.ts` | one import, one spread |
| C3 | `packages/schema/src/index.ts` | one export |
| C5 | `packages/proto/vrx/v1/dataplane.proto` | RPC under the service anchor; `gso` 20, `mirror` 21 under the Interface anchor; `nsim` 9 under the ServicesConfig anchor; messages in `// ----- F-loopback-bvi-gso-lldp-span -----` |
| C6 | `docs/contracts/proto.md` | `### F-loopback-bvi-gso-lldp-span: LldpNeighbors` |
| C7 | generated | `pnpm gen && make -C apps/cli gen docs` (never hand-edited) |
| P1 | `apps/api/src/app.module.ts` | one import, one spread in controllers, one in providers |
| P4 | `apps/api/src/agent/agent.client.ts` | two type imports, `lldpNeighbors()` |
| P5 | `apps/api/src/testing/fake-agent.ts` | `lldpNeighbors` UNIMPLEMENTED stub |
| W1 | `apps/web/src/router.tsx` | routes `interfaces/lldp`, `interfaces/mirroring`, `tools/nsim` |
| W2 | `apps/web/src/nav/nav.ts` | one push of the two interfaces items, one push of the Tools item |
| W2 | `apps/web/src/nav/nav.test.ts` | `'lldp'`, `'mirroring'` under the anchor; **`'nsim'` after `'revisions'`** (the Tools group has no anchor) |
| W3 | `apps/web/src/i18n.ts` | two imports, the namespace, the en and fa entries |
| D1 | `docs/user/interfaces/basics.md` | one see-also line at the end ("Not in this release" untouched) |
| — | `apps/agent/internal/agent/{service_test,agent_integration_test}.go` (P08 tests, no anchor) | `"services": {}` in the canonical documents and the `implementedDomains()` subsystem assertions — **byte-identical to F-rpf-adl-pbr's hunks** (they merge cleanly) |
| — | `apps/agent/internal/agent/projection_test.go` (P08, no anchor) | one line: `withoutWriteOnly(pj.kvs)` in the round trip (write-only LLDP of the services example cannot round-trip; helper in my test file) |

## Expected union at merge (with F-rpf-adl-pbr on main first)

1. **Delete `apps/agent/internal/desired/lldp_services_seam.go`.** It is my copy of F-rpf-adl-pbr's `ServicesMembers`
   declaration and "unsupported member" loop; with it gone, `lldp.go`'s `init()` registers `lldp`/`nsim` in F-rpf-adl-pbr's
   map, `reportUnsupportedServices` stays nil and F-rpf-adl-pbr's `projectServices` reports the rest. (Go reports
   "ServicesMembers redeclared" until the file is deleted — that is the reminder.)
2. `subsystems.go`: nothing to union — my services names are appended to F-rpf-adl-pbr's `Services: {…}` entry from
   `init()` in `subsystems/loopback_bvi_gso_lldp_span.go` (`servicesDomain = "services"`, no second `Services` constant).
3. The P08 test hunks are identical to F-rpf-adl-pbr's; the `feature_is_enabled` allowance lines are theirs only.
4. Drift: my write-only notes use F-rpf-adl-pbr's `agent.write-only` rule; with its `COVERAGE_RULES` line on main the
   `/services/lldp` drift seen below on my branch disappears.
5. TD-11b: my `ownership.go` files use the structural `Persistent()` check; after the rebase they may call
   `dfkit.CheckClaims` / `dfkit.CheckBoot` (mechanical, questions Q6). TD-23 (D-134): the coretest hook line becomes one
   registration line in its registry.

## How it was verified

(CI and the final host run are pasted below; everything else was run on the host VPP 26.06, slot 7, no packets sent;
NRestarts 1 → 1 on every run.)

### Unit tests
PLACEHOLDER-UNIT

### Host checks (descriptor level, `VRX_INTEGRATION=1`)
PLACEHOLDER-HOST

### The ONE host integration check through the real API + agent + VPP (`test/topology/loopback-bvi-gso-lldp-span`)
PLACEHOLDER-TOPO

### API e2e (host PostgreSQL + fake agent)
PLACEHOLDER-E2E

### UI — screenshots against the real endpoint (`TestLoopbackBviGsoLldpSpanScreenshots`)
Production build under `vite preview` on the slot web port, real API + agent + VPP (loopbacks, the ERSPAN fixture, LLDP
on an index-aligned loopback, a pending mirror change). Playwright is not installed: a node script with playwright-core
from the npx cache and the Chrome-for-Testing headless shell from the session scratch drives the browser (P07a/P07b/P08
approach; nothing installed, the script is not committed).
PLACEHOLDER-SHOTS

### CI
PLACEHOLDER-CI

## Acceptance
PLACEHOLDER-ACCEPT

## Out of scope (not built)
Bridge domains and BVI membership descriptors (F-bridge-l2), loopback creation (P08), GRE tunnel creation (F-tunnels — a
fixture tunnel is used), bonds, sub-interfaces, DPDK checksum/TSO offload flags (F-startup-gen), SNMP LLDP-MIB, packet
capture, the other `services.*` leaves (reported `agent.unsupported-field`), new lldp/span descriptors.

## Decisions taken (with options) and open questions
See `F-loopback-bvi-gso-lldp-span-questions.md`: Q2 examples in the proto fixture corpus; Q3 the services seam (merge
F-rpf-adl-pbr first); Q4 gso readable (read-back + boot record) vs write-only; Q5 LLDP system name not defaulted from
system.hostname in the agent; Q6 TD-11b declarations; Q7 D-132 / WEB-1; Q8 coretest hook (TD-23). Envelope open
questions: LLDP on slot agents — `lldp.global` is registered only on the globals owner; the host evidence shows
`/services/lldp/txHold|txIntervalSec` reported `agent.unsupported-field` and VPP keeps its timers (show lldp); nsim stays
in the product UI under Tools, marked "lab tool" (default kept; product owner to confirm).
