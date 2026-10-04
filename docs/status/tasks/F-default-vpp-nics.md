> Historical recovery record from merge `79fff64a` (2026-09-28). Current recovery verification is in `F-default-vpp-nics-wip.md`; historical completion and publication claims do not describe the current main.

# F-default-vpp-nics — Default data-plane ownership of every non-management NIC

Branch `task/F-default-vpp-nics`, base `main@f1f885a6`, current `main` merged (D-191 mitigation included).
Product-owner directive (D-164): out of the box every physical NIC except the management interface belongs to the
engine; the web shows them pre-provisioned and non-deletable; releasing a NIC to the host is a config action, not a
delete. Round 1 of review (R1–R7, D-192/D-193) is addressed below; the table at the end maps every finding to its commit.

## What was built

**Contract (additive; `contract(schema,proto)` first; see `-contract.md` and `docs/contracts/proto.md`
"F-default-vpp-nics: HostNics (+ Interface.physical 24)")**
- `InterfaceSchema.physical?{pci, owner: 'dataplane'|'host' (default dataplane), builtIn (default true)}`.
- `Interface.physical = 24` + `InterfacePhysical`; read-only RPC `HostNics` + `HostNicsRequest/Response/HostNic`.
- Semantic rules (`semantic/default-vpp-nics.ts`): `dataplane.owner-consistent`, `dataplane.physical-name-matches-device`.

**Agent**
- `vppstartup.HostNICs` (`hostfacts.go`, the same reader as `ReadHost`): every `/sys/class/net` NIC with a PCI device
  (netdev, pci, driver, mac, carrier, management) plus every network-class PCI function bound to a DPDK driver
  (`/sys/bus/pci/devices/*`, class `0x02xxxx`, `vfio-pci`/`uio_pci_generic`/`igb_uio`: no netdev, `BoundToDpdk`).
  NICs without a PCI address are not enumerated (D-177). An unreadable `/proc/net/tcp{,6}` falls back to the routes
  with a note (D-177).
- `HostNics` RPC (`rpc_host_nics.go`): `bound_to_dpdk` = DPDK driver on the PCI function, or a live VPP `dpdk` interface
  with the NIC's MAC (bifurcated drivers); management overrides `NGFW_MGMT_IF`/`NGFW_MGMT_PCI`/`NGFW_CONTROL_PORTS`;
  `management_notes` carry the reason class only (no peer address). Read-only: binds nothing, never touches `/etc/vpp`,
  never restarts VPP.
- `desired.SplitUnboundPhysical` (`desired/hostnics.go`), used by `projection.go` for `Interfaces` **and** `Lcp`: an
  engine-owned physical row whose name is still a hardware kernel netdev (rtnetlink kind empty — a linux-cp tap of the
  same name does not count) is dropped with WARNING `agent.nic-not-bound`; a row released to the host is dropped with
  INFO `agent.nic-released`. Neither fails DryRun/Apply; the API counts both as drift coverage notes.

**API**
- `SeedService` (`features/default-vpp-nics/`): first boot → `HostNics` → pure `seedDocument()` (golden-tested) →
  system commit revision 1, audited `system.seed-defaults`. **Fail-closed:** `NGFW_SEED_DEFAULT_NICS` defaults to `0`
  (D-192); refuses to seed without an identified management NIC (warning event `system.seed-defaults-deferred`);
  seeds only an untouched document (`== emptyDocument()`); retries on agent connect and every `retryMs` (60 s).
  NICs without a netdev (DPDK-bound) are named from their PCI address (`enp<bus>s<slot>f<fn>`).
- `datastore/physical-nics.ts` guards every candidate edit (PATCH / PUT / DELETE / import):
  `403 interfaces.physical-nic-not-deletable` (removal) and `403 interfaces.physical-marker-readonly` (adding the
  marker, changing `pci`/`builtIn`); only `physical.owner` is editable. Rollback restores whole revisions; every
  revision since the seed carries the markers, so it cannot drop one.
- `GET /state/interfaces`: `physical{pci, owner enum, builtIn}`, `builtIn`, `awaitingDataplane` (null when the live
  table is unavailable).

**UI** (en + fa, "the engine" wording, D-155): built-in / awaiting-engine / released chips; the drawer form no longer
contains `physical` (pci/owner shown read-only), no Remove button on physical rows, Release to host / Reclaim for the
engine in one root merge patch (whitelist + devices kept consistent) behind a confirm dialog that shows errors inside it.

**Docs**: `docs/user/interfaces/default-dataplane-nics.md` (seeded-document worked example, fail-closed switch,
management detection and the deferred seed, inventoried NICs incl. non-PCI, awaiting engine, release/reclaim, rollback,
real CLI sequence) linked from `basics.md`; `docs/contracts/proto.md` section.

## How verified (real output)

Evidence files (committed): `docs/status/tasks/evidence/F-default-vpp-nics-tests.txt` (every unit / e2e / web command
and its summary) and `docs/status/tasks/evidence/F-default-vpp-nics-hostnics-integration.txt`.

### Integration on this host — HostNics (NGFW_INTEGRATION=1, slot 1, lab lock via vpptest.LockLab)
```
$ NGFW_INTEGRATION=1 go test ./internal/agent/ -run TestHostNicsOnHost -v -count=1
2026-09-28T16:20:12+03:30
    before: vpp NRestarts=2 /etc/vpp/startup.conf sha256=a4763491ad8b173929546ba0cf83f438e7cdfa9aecd9068f15e202852ee22af0
    HostNics: 7 NIC(s)
      ens161   pci=0000:04:00.0 driver=vmxnet3  mac=00:50:56:9f:46:f2 management=false bound_to_dpdk=false link_up=false
      ens192   pci=0000:0b:00.0 driver=vmxnet3  mac=00:50:56:9f:1e:53 management=true bound_to_dpdk=false link_up=true
      ens193   pci=0000:0c:00.0 driver=vmxnet3  mac=00:50:56:9f:4b:c6 management=false bound_to_dpdk=false link_up=false
      ens224   pci=0000:13:00.0 driver=vmxnet3  mac=00:50:56:9f:32:8c management=false bound_to_dpdk=false link_up=false
      ens225   pci=0000:14:00.0 driver=vmxnet3  mac=00:50:56:9f:85:ce management=false bound_to_dpdk=false link_up=false
      ens256   pci=0000:1b:00.0 driver=vmxnet3  mac=00:50:56:9f:73:f8 management=false bound_to_dpdk=false link_up=false
      ens257   pci=0000:1c:00.0 driver=vmxnet3  mac=00:50:56:9f:cd:88 management=false bound_to_dpdk=false link_up=false
      note: ens192 → 0000:0b:00.0 (default route)
    after:  vpp NRestarts=2 /etc/vpp/startup.conf sha256=a4763491ad8b173929546ba0cf83f438e7cdfa9aecd9068f15e202852ee22af0
--- PASS: TestHostNicsOnHost (0.28s)
```
The test asserts the exact ngfw-a set (7 netdev/pci pairs, management = [ens192]) and fails on unreadable
NRestarts/startup.conf. NRestarts is 2 before and after; both restarts are unrelated shared-VPP crashes (the 14:10:55
one was another worker's af_packet delete, V24). `HostNics` is read-only.

### Unit, e2e and web (full output in the evidence file)
```
vppstartup  TestReadHost, TestHostNICs, TestHostNICsNoManagementDetected, TestHostNICsFindsDpdkBoundNIC,
            TestHostNICsToleratesUnreadableTCPTable                                            ok
agent       TestProjectPhysicalNic{AwaitingDataplane,BoundProjectsNormally,NoLookupProjectsNormally,
            LcpTapIsNotUnbound,UnboundHasNoLcpPair,ReleasedToHost}, TestProjectSchemaExamples,
            TestReasonOnlyNotesDropsPeerAddress                                                ok
contracttest (schema↔proto drift), desired                                                      ok
schema      src/semantic/default-vpp-nics.test.ts                                     Tests 10 passed (10)
api unit    src/features/default-vpp-nics/ (seed: no-management, fail-closed, golden, empty-doc only,
            pciIfName, unreachable→ready, timer retry, reason-only notes; drift coverage) Tests 10 passed (10)
api e2e     test/e2e/default-vpp-nics.e2e.test.ts on slot 1 (ngfw_w1 + fake agent)             Tests 6 passed (6)
            seed rev 1 (+ fresh instance → exists, 1 revision, audit system.seed-defaults, state flags);
            PATCH-null / DELETE / import → 403 not-deletable; add marker / change pci / builtIn → 403 marker-readonly;
            PUT without marker → 403; disable → 200; release → commit, pciWhitelist updated
web         InterfacesPage.test.tsx "seeded physical NIC" (chips, no Remove, no form field, dialog, PATCH body)  1 passed
```

## Acceptance checklist
- [x] Unit: fake host tree → HostNics = 6 data + 1 management; seeding produces the golden document (`seed.golden.json`)
- [x] Integration on this host: HostNics pasted; NRestarts and startup.conf sha256 logged before/after, equal
- [x] API e2e (slot 1): seed rev 1 + no re-seed; PATCH/DELETE/import/PUT → 403 with pointer; release → pciWhitelist updated
- [x] Seeded physical row while VPP has no such interface → agent.nic-not-bound warning, no failure (projection tests)
- [ ] UI screenshot (en + fa) — **T4's task** (D-177 #3); the screen is covered by the web test above, not by a PNG
- [x] `tools/ci.sh --base main` green (see CI)

## For P10 (firstboot order)
The product API unit / firstboot sets **`NGFW_SEED_DEFAULT_NICS=1`** (D-192; it defaults to 0). Order:
**(1) API seed** (`HostNics` → revision 1 with `dataplane.*` + `interfaces.<name>.physical`; deferred while no management
NIC is identified) → **(2)** the dataplane document is the source of truth → **(3) `ngfw-startupgen`** renders
`startup.conf` (`dpdk { dev <pci> { name <name> } }`) from it, then the NICs are bound and VPP restarted (TD-17). Until
(3) the seeded NICs are "awaiting engine" (`agent.nic-not-bound`), which is expected.
Agent read access `HostNics` needs (P10 AD-6): `/sys/class/net/*` (`device`, `device/driver`, `address`, `carrier`),
`/sys/bus/pci/devices/*` (`class`, `driver`), `/proc/net/route`, `/proc/net/ipv6_route`, and — optional, D-177 —
`/proc/net/tcp{,6}` (unreadable → default-route fallback with a note).

## Shared-file hunks (each under a `wave-BC: F-default-vpp-nics` anchor)
- `packages/proto/ngfw/v1/dataplane.proto` — `HostNics` RPC, `Interface.physical = 24`, message section.
- `packages/schema/src/domains/interfaces.ts` — `InterfacePhysicalSchema` + one key line.
- `packages/schema/src/semantic/index.ts` — one import + one spread.
- `apps/agent/internal/agent/projection.go` — `SplitUnboundPhysical` call, the two note loops, `Interfaces(boundIfs)`,
  `Lcp(boundIfs)` (the existing `desired.Interfaces` / `desired.Lcp` lines are edited to take the filtered map).
- `apps/api/src/app.module.ts` — import + controllers/providers spreads.
- `apps/api/src/main.ts` — import + `SeedService.start()`.
- `apps/api/src/config.ts` — `NGFW_SEED_DEFAULT_NICS`.
- `apps/api/src/agent/agent.client.ts` — type import + `hostNics()`.
- `apps/api/src/testing/fake-agent.ts` — `hostNics` handler + `hostNicList`/`hostNicNotes`.
- `apps/api/src/datastore/datastore.service.ts` — one import + one `assertPhysicalNicEdit(base, next)` call (the earlier
  duplicate imports are gone; `documents.ts` is unchanged vs main).
- `apps/api/src/state/state.controller.ts` — view fields, their computation, `physicalView()`, two coverage rules.
- `apps/web/src/locales/{en,fa}/interfaces.json` — `physical.*`, chip keys.
- `apps/web/src/config/collection/equivalence.test.ts` — `physical` added to the kit's `omit` list (the drawer form
  leaves it out, like `bond`; outside my files — one anchored line).
- `docs/user/interfaces/basics.md` — one see-also line; `docs/contracts/proto.md` — one section.

## Decisions (for the LOG; D-193 accepted placement and the MAC/driver choice)
- nic-not-bound in `projection.go` via `desired/hostnics.go`; semantic rules in their own file (D-174).
- Strings stay in the `interfaces` namespace (no `i18n.ts` edit).
- `bound_to_dpdk`: DPDK driver on the PCI function, else a live VPP `dpdk` interface with the NIC's MAC.
- DPDK-bound NICs without a netdev are named `enp<bus>s<slot>f<fn>` from their PCI address.
- Release/reclaim is one root merge patch; the marker is otherwise read-only for edits.
- The seed only seeds an untouched document; an operator commit before a deferred seed succeeds abandons it (documented).
- Dedupe by PCI keeps the first netdev in `/sys/class/net` glob order (alphabetical, deterministic); a netdev lookup
  error keeps the row (the alias then reports a missing interface, as for any physical interface).

## Debt (re-review, D-201)
- **MINOR 5 — bifurcated NICs (mlx5 and similar):** such a NIC keeps its kernel netdev while DPDK drives it, so
  `agent.nic-not-bound` (hardware netdev present ⇒ not bound) would keep reporting it as not bound after the engine took
  it over, and the seed names it by its netdev. `bound_to_dpdk` does catch it through the VPP `dpdk` interface MAC, but
  the projection does not consult VPP. Fix later: treat a physical row as bound when VPP has an interface of that name
  (or the MAC match), not only when the netdev is gone. Not reachable on ngfw-a (vmxnet3).
- A physically removed NIC can only be released, never deleted (documented in the user doc).

## Out of scope (not built)
Binding NICs / editing `/etc/vpp/startup.conf` / restarting VPP (D-012; F-dataplane-ui + TD-17 + P10); the Dataplane
screen; P10 packaging (sets `NGFW_SEED_DEFAULT_NICS=1`); host-side management of released NICs; performance work;
the UI screenshot (T4).

## Review round 1 → commits
| finding | fix | commit |
|---|---|---|
| R2R4-1 / D-192 seed default | default `0`; doc + status corrected | 79e0b466 |
| R2R4-2 / R1R3-3 no management NIC | refuse, warning event, retry timer; unit tests (API + fake tree) | 7f2fe08d |
| R2R4-3 / R1R3-1 lcp tap | only kind "" counts; tun test | 1621fe9f, c6bbf8f2 |
| R2R4-4 marker added by users | 403 `physical-marker-readonly`; e2e; rollback documented | ad7fdbe9, 9cce6db5 |
| R2R4-5 peer IP in notes | reason class only (agent + API) | 7f2fe08d |
| R2R4-6 / R7-2 integration helpers | Fatalf, NRestarts/sha256 logged, re-run, evidence .txt | e4b64f9a |
| R2R4-7 / R1R3-2 bound_to_dpdk, bound box | PCI-bus inventory, driver-based flag, PCI-derived names; tests | b5de5f69 |
| R6-1..5 form, wording, dialog error, types, UI test | strip `physical`, read-only line, engine wording, test | 0ccebc8c |
| R7-1 evidence | `evidence/F-default-vpp-nics-tests.txt` | status commit |
| R7-3 anchors / hunks | `physical-nics.ts`, anchors, list above | 9cce6db5, c6bbf8f2 |
| R7-4/5/10 docs | real CLI, worked example, non-PCI NICs, basics link, P10 paragraph | 56bee4fb |
| R7-7 overclaims | acceptance rewritten (golden, audit asserted, screenshot unticked) | status commit |
| R7-8 contract doc | refreshed | 56bee4fb |
| R7-11 envelope copy | untracked | a64a93c1 |
| R1R3-4 seed lifecycle | fresh-instance e2e, unreachable→ready, timer retry | ad7fdbe9, b5de5f69 |
| R1R3-5 proto.md | section added | 56bee4fb, bde28f9c |
| R1R3-6/8/9/11 golden, empty-doc, name/pci rule, EACCES | done | b5de5f69 |
| R1R3-7 PUT / rollback | e2e PUT; rollback documented | ad7fdbe9, 9cce6db5 |
| R1R3-12 Lcp filtered | `Lcp(boundIfs)` + test | c6bbf8f2 |
| R1R3-13 owner enum / awaiting unknown | done | 0ccebc8c |
| R1R3-14 exact ngfw-a set | done | e4b64f9a |
| R1R3-15 released wording | INFO `agent.nic-released`, drift coverage | 484ef7f7 |
| R6-6/7, R1R3-16 NITs | DialogActions + LTR isolate done; status column / rule wording left as is | — |

## CI
`TMPDIR=/tmp/g-w1 tools/ci.sh --base main` on `b6e89a28` (round 1, after merging main):
```
  - uncommitted changes in the worktree — the gate checks the working tree, but only commits get merged:
      ?? docs/status/tasks/F-default-vpp-nics.envelope.md          (the manager's envelope copy, untracked per R7-11)
  - commit subject(s) not in Conventional Commits form (type(scope): subject):
      docs+test(F-default-vpp-nics): user doc + HostNics host integration test
  mode quick · wall time 20m19s · logs /root/ngfw-wt/logs/ci/F-default-vpp-nics-20260928-165246-49840

CI GATE PASSED
```
The previous run of this round (`…-163727-3973356`) failed one web test, `config/collection/equivalence.test.ts`
("item and form schema are the same generated schema"): the kit comparison had to omit `physical` like the drawer form
does now — fixed in `b6e89a28`.

## Fix round (re-review APPROVE WITH CHANGES, D-201)
| finding | fix | commit |
|---|---|---|
| MAJOR 1 default pinned | `loadEnv({NGFW_JWT_SECRET})` → false; `'1'`/`'true'` → true, `'0'` → false | 70be0f87 |
| MINOR 2 deferral noise | `system.seed-defaults-deferred` recorded once per deferral; repeated outcomes log at debug | 70be0f87 |
| MINOR 3 PUT/import marker | e2e: PUT `loop5{physical}` and import with changed `ens193.physical.pci` → 403 marker-readonly | d36150e2 |
| MINOR 4 restore / replacement / removed NIC | user doc paragraph | 17bd7f14 |
| MINOR 5 bifurcated NICs | listed under Debt | 17bd7f14 |
| NIT 6 anchors | fake-agent type imports, interfaces.ts `physical:` | 17bd7f14 |
| NIT 7 owner-consistent message | says which lists, and both fixes (edit the lists, or release/reclaim) | 17bd7f14 |

Targeted runs (host load high, per D-201; the merge gate reruns full CI):
```
$ (cd apps/api && pnpm exec vitest run src/features/default-vpp-nics/)
 ✓ src/features/default-vpp-nics/drift.test.ts (1 test)
 ✓ src/features/default-vpp-nics/seed.service.test.ts (10 tests)
      Tests  11 passed (11)
$ (cd apps/api && pnpm exec vitest run -c vitest.e2e.config.ts test/e2e/default-vpp-nics.e2e.test.ts)   # slot 1
 ✓ test/e2e/default-vpp-nics.e2e.test.ts (7 tests)
      Tests  7 passed (7)
$ pnpm -F @ngfw/schema exec vitest run src/semantic/default-vpp-nics.test.ts
      Tests  10 passed (10)
$ pnpm -F @ngfw/api typecheck        → clean;   eslint / prettier on the touched files → clean
$ TMPDIR=/tmp/g-w1 tools/ci.sh check --base main
check PASSED (0m11s)
```

## Open questions
None open (D-177 answered the three earlier ones). See `docs/status/tasks/F-default-vpp-nics-questions.md`.
