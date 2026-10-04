# Task: F-multiwan-wiring-b — Multi-WAN agent wiring, part B: per-member SNAT + sticky + dead-link clear, ABF pinning   (prepend 00-CONTEXT.md)

> Split of board row F-multiwan-wiring (D-176) — **-a** (monitor + default route / weighted ECMP) must be merged first;
> this row builds R7 BLOCKER 2 items 3+4. Branch from a `main` that contains F-multiwan-wiring-a.

## Goal
Finish multi-WAN in the running agent: each member's traffic leaves with that member's own address (per-member SNAT on
nat44-ed), established flows stay on their member while it is up (sticky), a dead member's NAT sessions are cleared on
failover, and policy routing can pin traffic to a member or to a group — all through the existing DF families (nat44ed,
abf/pbr, core route), no new binapi. Source: `docs/status/tasks/F-multiwan-host-review-R7.md` BLOCKER 2, D-176, F-multiwan-host
§3 (NAT; stickiness explicitly NOT proven there) and §4 (ABF).

## Inputs to read first
- `docs/status/tasks/F-multiwan-wiring-a.md` (what -a built: prober, monitor run, health-driven default route, resync hook) and
  `docs/status/tasks/F-multiwan-host.md` §3/§4 + `F-multiwan-host-evidence/{nat,abf}.sh` (the manual proof you reproduce with the agent).
- nat44-ed family: `apps/agent/internal/descriptors/nat44ed/{interface,pool,sessions,enable}.go` (`OutputFeatureSpec`,
  `InterfaceAddressSpec`, `nat44_del_session` helper), `apps/agent/internal/desired/nat.go` (how `nat.*` is projected), and
  `apps/agent/internal/actions/nat44-ed-sessions/sessions.go` (session listing by user/outside address).
- PBR/ABF: `apps/agent/internal/desired/rpf_adl_pbr.go` (`projectPbr`, `pbrPaths` :259-378), schema
  `packages/schema/src/domains/ext/rpf-adl-pbr.ts` (`PbrPathSchema` :113-135), proto `PbrPath` (fields 1–4).
- Health source: `apps/agent/internal/subsystems/wanmon/store.go` (`StoreFor(owner).Snapshot`), `internal/multiwan/health.go`.
- Global steps: nat44-ed plugin enable is VPP-global → `docs/status/wave-BC-launch-queue.md` §3 and D-167 (exclusive globals
  lock inside the shared lab lock, restore the prior state in the same window, query the prior state — never echo it; R7 #11).

## Contract changes
Group pinning needs one additive field: `routing.pbr.policies.<p>.paths[].wanGroup` (optional `objectName`; a path with
`wanGroup` sets no `address`/`interface`/`vrf` — refine with pointer) + semantic rule "the WAN group exists"
(`packages/schema/src/semantic/multiwan.ts`) + proto `PbrPath` **field 5 `wan_group` (proposed; the manager confirms it in your
envelope; add your own `### F-multiwan-wiring-b` section to `docs/status/wave-BC-numbers.md`)**. Commit it first on
`contract/F-multiwan-wiring-b` with subject `contract(schema): …` / `contract(proto): …`, regenerate (`pnpm gen`: TS types,
OpenAPI, JSON Schema, Go/TS stubs, `packages/api-client`), write `docs/status/tasks/F-multiwan-wiring-b-contract.md`, tell the
manager in your questions file, keep building against it. No other contract change.

## Scope — build exactly this
1. **Per-member SNAT** (`desired/multiwan_nat.go`): for a group with ≥ 2 members, project for every member the nat44-ed
   output-feature on the member interface + its interface address as pool address (`InterfaceAddressSpec`), so traffic leaving
   via member M is translated to M's address. If `nat.*` already configures the same member interface, do not add a second
   writer: projection warning `multiwan.nat-owned-by-config` with the pointer, and leave it to the user's NAT config. The
   objects must not come back as `nat` drift (`/state/drift` clean). Fake-VPP unit tests.
2. **Dead-link session clear** (`subsystems/wanmon/**` flip callback or `desired/multiwan_nat.go`): when a member goes down
   and the group has `stickySessions: true` (default), delete the nat44-ed sessions whose outside address is that member's
   address (nat44ed sessions helper; bounded, ctx-limited, logged with counts). Unit test with the fake VPP.
3. **Sticky**: prove that an established flow keeps its member while that member stays up when the group's path set changes
   (another member leaves/joins in balance mode). If VPP's load-balance rehash moves it and no binapi-level setting prevents
   that, do not invent C code: record evidence + the config-only fallback you chose in the status file and a V-item request in
   your questions file (`docs/vpp-code-track.md` is the manager's).
4. **ABF pinning**: (a) to a **member** — already expressible with a `routing.pbr` path `{address: <gw>, interface: <member>}`;
   no code, but reproduce the host's §4 result through the API as acceptance. (b) to a **group** — resolve a `wanGroup` path in
   `pbrPaths` (one anchored hunk `// wave-BC: F-multiwan-wiring-b` in `desired/rpf_adl_pbr.go` calling a helper in
   `desired/multiwan_pbr.go`): failover → the active member's gateway/interface; balance → the healthy members with their
   weights; no healthy member → the policy's paths fall back to a lookup in the members' VRF (log it). Health flips re-run it
   through the resync hook -a built. Unit tests.
5. **Slot evidence driver**: extend `test/topology/multiwan/` (from -a): NAT window, failover-with-clear, sticky, member pin,
   group pin. NAT enable/disable only inside one exclusive globals-lock window (≤ 10 min, `date -u` start/end, prior state
   queried with `show nat44 interfaces` / `show nat44 addresses` and restored by a trap).
6. **Docs**: `docs/user/network/multi-wan.md` — NAT, sticky, pinning (with a `wanGroup` path example); CLI equivalent.

## Acceptance (paste the evidence; `.txt` under `docs/status/tasks/F-multiwan-wiring-b-evidence/`)
- [ ] Per member: `vppctl show nat44 interfaces` / `show nat44 sessions` show i2o translation to member 1's address, then to
      member 2's after failover — config committed through the slot API, not vppctl writes
- [ ] Failover clears the dead member's sessions (count before/after pasted); restore-to-primary keeps working (-a)
- [ ] Sticky: one long-lived flow keeps its member across a path-set change (counters/session table), or the documented fallback
- [ ] Member pin: pinned source egresses WAN2 while the default is WAN1 (per-interface tx); group pin follows the active member
      across a failover
- [ ] Validation failure: `wanGroup` naming a missing group → 400 problem+json with the `pointer`; a path with `wanGroup` + `address` → 400
- [ ] Rollback leaves no nat44/abf objects of yours (Retrieve output); agent-restart simulation recreates them within 30 s
- [ ] Globals window start/end + restored prior state pasted; NRestarts before/after every host step; `tools/ci-slot.sh --base main` tail

## Out of scope (do not build)
Monitor/prober/default-route logic (that is -a; fix only a proven defect, name the test) · DHCP/PPPoE-learned member gateways ·
NAT44-EI, NAT64, CGNAT pools, port-block allocation · new binapi · any `apps/web/**` or `packages/ui-kit/**` change (the PBR form
is a SchemaForm and picks the new field up; the PBR list's path column showing `wanGroup` is a web follow-up — say so) · alarms on
flips · performance claims.

## Open questions to surface, not to decide silently
- Sticky outcome (works as is / fallback / V-item).
- Whether the manager confirms `PbrPath` field 5 (if not, ship member pinning only and record group pinning as owed).

## Files you own
`apps/agent/internal/desired/multiwan*.go` · `apps/agent/internal/subsystems/wanmon/**` · `apps/agent/internal/desired/rpf_adl_pbr.go`
(one anchored hunk, listed under `## Shared hunks`) · `packages/schema/src/domains/ext/rpf-adl-pbr.ts` (the `wanGroup` field only) ·
`packages/schema/src/semantic/multiwan.ts` · `packages/proto/ngfw/v1/dataplane.proto` (`PbrPath` field 5 only) + the regenerated
outputs of `pnpm gen` · `docs/status/wave-BC-numbers.md` (own `###` section) · `test/topology/multiwan/**` ·
`docs/user/network/multi-wan.md` · `docs/status/tasks/F-multiwan-wiring-b*`.

## Rules
- Files you own: the list above. Everything else is read-only; a needed edit elsewhere → `docs/status/tasks/F-multiwan-wiring-b-questions.md`.
- Shared VPP: slot prefix `w<N>` on every object, tables N000–N999 (`eval "$(tools/lab env <N>)"`); never restart or kill VPP;
  `timeout 10` on every vppctl; packet trace banned (D-128: no `trace add` / `show trace` / `clear trace`); nat44 enable only in
  the exclusive globals window (D-167). Daemons: none.
- D-210a: write tests for your change and get them passing in your package; paste the output; no full suite, no lint, no other
  packages' tests. Run them through `tools/heavy.sh` (D-224), e.g. from `apps/agent`: `../../tools/heavy.sh go test ./internal/desired/...`;
  schema: `tools/heavy.sh pnpm --filter @ngfw/schema exec vitest run src/semantic/multiwan.test.ts`.
- Contracts: additive only — `contract(schema|proto): …` commits first, numbers only from `docs/status/wave-BC-numbers.md`;
  renaming/reshaping = PENDING (decision-policy #1).
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never `tools/ci.sh` directly), commit on your
  branch, `docs/status/tasks/F-multiwan-wiring-b.md` with pasted real output (D-175 evidence rules), stop every process you started.
