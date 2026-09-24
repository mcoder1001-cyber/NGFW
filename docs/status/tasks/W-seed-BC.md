# W-seed-BC: wave-B/C anchor pass (D-119 M1, no behaviour change)

Branch `task/W-seed-BC` (no slot — unit tests + CI only), base `task/W-seed@df67a8e` (SPECULATIVE, D-114; the `task/P08`
merge into `task/W-seed`). Commit: `50b2cce wip(W-seed-BC): salvage uncommitted work after session-limit stop 20:57`
(the whole anchor pass; salvaged verbatim by the manager after a usage-limit stop, then verified and closed out by this
continuation) + this status file. Method: `prompts/tech-debt/W-seed.md`, same as W-seed's own wave-A pass.

## What
One `// wave-BC: <task-id>` line (`#` in shell/debian files — none touched here) per touching task, at every insertion
point of `docs/status/wave-BC-numbers.md` (all three packs: wave-C services/observability + F-licensing; wave-B/C
routing/MPLS/multicast/SRv6/LISP; S5 system; NAT transition/tunnels/VPN/HA), placed **directly above** the block's
first `wave-A:` anchor (or at the natural insertion point for a site with no wave-A anchor yet — SY1–SY4, `app.ts`),
per the placement rule at the end of that file. Plus: the A4 Action-switch cases named in the envelope (det44 ×2,
ikev2_sa, remote_access_disconnect, ha_sync, capture, upgrade/support_bundle), SY1–SY5, the WEB-2 router/nav anchors
(`// web: WEB-2`), and the UI-domain-editor W1/W3 anchors (`// wave-A: UI-domain-editor`, missing until now). The ED
`desired/nat.go`/natTabs EI+CGNAT groups were **not** touched (F-nat44-ed-sessions seeds those itself, per the envelope).

### Per-file anchor count (`grep -c "wave-BC:" <file>`, source only — `dist/`/`gen/` build copies excluded)
| file | site(s) | wave-BC lines |
|---|---|---|
| `apps/web/src/i18n.ts` | W3: en/fa imports, `NAMESPACES`, `en{}`, `fa{}` (4 × 31 tasks) | 124 |
| `apps/api/src/app.module.ts` | P1: feature-module import, `controllers`, `providers` (3 × 31) | 93 |
| `packages/proto/vrx/v1/dataplane.proto` | C5: `service Dataplane` (22) + per-message fields (~40) + 21 end-of-file `// ----- <id> -----` stubs (comment-only, no anchor keyword needed there) | 85 |
| `apps/api/src/agent/agent.client.ts` | P4: RPC type imports + methods (2 × 22) | 44 |
| `apps/agent/internal/subsystems/subsystems.go` | A1: domain const (1), `Domains[Routing]` (4), new domain entries (12), end of `Register()` (14) | 31 |
| `packages/schema/src/semantic/index.ts` | C2: imports + spreads (2 × 15) | 30 |
| `apps/api/src/testing/fake-agent.ts` | P5: `impl()` handlers (22) | 22 |
| `apps/agent/internal/agent/projection.go` | A2: `project()` + `assemble()` (2 × 10) | 20 |
| `apps/web/src/nav/nav.ts` | W2: `BUILT_DOMAINS` (4), non-domain routing items (6), S5 system items (8) | 18 |
| `apps/web/src/nav/nav.test.ts` | W2: `available` list, routing group (10) + S5/WEB-2 (8) | 18 |
| `apps/web/src/router.tsx` | W1: feature screens (14) + `web: WEB-2` + `wave-A: UI-domain-editor` | 16 |
| `packages/schema/src/domains/routing.ts` | C1: `OspfInterfaceSchema`(2)/`IsisInterfaceSchema`(2)/`IsisSchema`(1)/`RipInterfaceSchema`(1)/`RipSchema`(1)/`BfdSessionSchema`(1)/`BfdSchema`(1)/`RoutingSchema`(5) | 14 |
| `packages/schema/src/index.ts` | C3: `export * from './domains/ext/<slug>.js'` (10) | 10 |
| `apps/api/src/infra/bus.ts` | P6: `TOPICS` (9) | 9 |
| `apps/api/src/telemetry/relay.service.ts` | P6: `eventTopic()` cases (7) | 7 |
| `apps/agent/internal/agent/server.go` | A4: `Action` switch (det44 ×2, ikev2_sa, remote_access_disconnect, ha_sync, capture, upgrade/support_bundle) | 7 |
| `packages/schema/src/domains/vpn.ts` | C1: `IpsecSettingsSchema`(1)/`PkiCaSchema`(1)/`PkiCertificateSchema`(1)/`RemoteAccessUserSchema`(1)/`RemoteAccessProfileSchema`(1) | 5 |
| `apps/web/src/domains/vpn/tabs.ts` | shell seam: `vpnTabs` (F-pki, F-ikev2-native, F-srv6, F-lisp, F-ra-vpn) | 5 |
| `apps/web/src/domains/services/tabs.ts` | shell seam: `servicesTabs` (F-host-stack, F-lb, F-qos-flat, F-snmp, F-ipfix-sflow) | 5 |
| `apps/api/src/auth/route-guard.test.ts` | SY1: `PUBLIC` (3), `ADMIN_ONLY` (2) | 5 |
| `packages/schema/src/domains/ha.ts` | C1: `VrrpInstanceSchema`(1)/`HaClusterSchema`(2: cluster + `StateSync`)/`HaSchema`(1) | 4 |
| `apps/api/src/db/schema.ts` | SY3: new `pgTable`s (F-dashboard, F-aaa, F-licensing, F-backup-restore) | 4 |
| `packages/schema/src/domains/management.ts` | SY4: `AaaSchema`(1)/`ManagementSchema`(2) | 3 |
| `packages/schema/src/domains/tunnels.ts` | C1: `TunnelsSchema` (F-tunnels, F-lisp) | 2 |
| `apps/api/src/app.ts` | SY2: content-type parser fallback (F-restconf-yang, F-backup-restore) | 2 |
| `packages/schema/src/domains/nat.ts` | C1: `NatSchema` (F-det44-map-dslite-cnat) | 1 |
| **total (27 code files)** | | **585** |

Not anchor-comment sites but required SY5 rows (`apps/agent/internal/renderers/ALLOWLIST.md`, "Active" table, one row per
binary, fixed argv documented): `F-lb` (VPP `cli_inband` LB-cleanup call, D-090), `F-backup-restore` (`vrx-upgrade`,
`vrx-support-collect`) — 3 rows.

Deliberately untouched (owned by other tasks / not yet a site): `apps/agent/internal/desired/nat.go` + its natTabs
registry (F-nat44-ed-sessions' own EI+CGNAT groups); `apps/agent/internal/agent/metrics.go` (no existing anchor block —
wave-BC-numbers.md's "manager-seeded seam" note is superseded by launch-queue M2: TD-8 owns and creates this seam);
`docs/contracts/proto.md` / `docs/vpp-code-track.md` (C6/A7 — append-only, "no anchor" by the pack rule); SY6–SY9
(deploy install lists, `.gitignore`, `pnpm-lock.yaml`, `tools/ci.sh` — manager/dep-task-owned, not anchor sites).

## How verified

### 1. Diff is comment-only — no behaviour change
Every added line across the 27 code files is a `//` (or `#`) comment, except the 3 ALLOWLIST.md table rows above (a
markdown reference table, not compiled code — already required verbatim by SY5). Checked mechanically:
```
$ git show 50b2cce -- ':!docs/status/tasks/W-seed-BC.envelope.md' | <strip +/+++, keep '+' lines, drop blank/comment>
3 non-comment added lines   (all three are the ALLOWLIST.md rows)
```
No `.go`/`.ts`/`.tsx` logic line, no schema field, no proto field number, no test assertion changed.

### 2. Generated output: byte-identical
```
== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
```
`git status --porcelain` is empty after the gate (nothing regenerated differently). Proto/schema edits are comments
only (framed by a blank line before the next message, per §0 rule 2), so none leaks into `packages/proto/gen`,
`packages/schema/dist`, or the derived `packages/api-client/src/generated` / OpenAPI output.

### 3. Pass counts — unchanged from base (comment-only diff ⇒ counts cannot move)
```
                  W-seed-BC head (50b2cce)
@ngfw/schema  Test Files  37 passed (37)   Tests  1214 passed (1214)
@ngfw/proto   Test Files   2 passed (2)    Tests    68 passed (68)
@ngfw/api     Test Files   8 passed (8)    Tests    50 passed (50)
@ngfw/ui-kit  Test Files  12 passed (12)   Tests    48 passed (48)   (cache hit, replayed unchanged from task/W-seed)
@ngfw/web     Test Files  14 passed (14)   Tests    94 passed (94)
turbo (test)  Tasks: 17 successful, 17 total
apps/agent    go vet + golangci-lint 0 issues · go test -count=1: ok=89 FAIL=0 no-test-files=162
apps/cli      go vet + golangci-lint 0 issues · go test -count=1: ok=7  FAIL=0 no-test-files=5
```
`ui-kit` (89/162 agent, 7/5 cli, 12/48 ui-kit) match the counts W-seed's own status doc recorded for the same base
line, since W-seed-BC touches none of those packages/files. `apps/agent`/`apps/cli` package counts are identical in
shape and count to base (no package gained or lost a test file).

### 4. `TMPDIR=/tmp/g-wsbc tools/ci.sh --base main`: green
```
== VRX CI gate: quick ==
branch    task/W-seed-BC @ 50b2cce   (base: main)
ok — contract commit(s) on the branch (5 pre-existing contract commits from the P08/W-seed history satisfy the guard)
WARN commit subject(s) not in Conventional Commits form: review(W-seed): verify   (pre-existing, not from this task)

== tools (golangci-lint, gitleaks) ==            golangci-lint 2.13.2 · gitleaks 8.30.1
== install (pnpm --frozen-lockfile --prefer-offline) ==   Lockfile is up to date
== generate + generated-output gate ==           clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
== forbidden patterns (+ gitleaks) ==            ok: no shell/VPP/FFI access · no Dockerfile/compose · no kill-by-pattern · no secret-shaped strings · gitleaks: no leaks
== lint · typecheck · unit tests · build (turbo) ==   Tasks: 30 successful, 30 total   Cached: 4 cached, 30 total   Time: 4m42.818s
== apps/agent: make lint test build ==           ok (89 packages, 0 FAIL)
== apps/cli: make lint test build ==             ok (7 packages, 0 FAIL)
== test/ Go modules, unit mode ==                test/integration/smoke ok · test/topology/interfaces ok (VRX_INTEGRATION unset: integration skipped)

mode quick · wall time 8m28s · logs /root/ngfw-wt/logs/ci/W-seed-BC-20260924-224527-3286617
CI GATE PASSED
```
The branch's own `tools/ci.sh` copy (pre-D-127 fix) ran the contract-guard step without hitting the SIGPIPE race this
time; no fallback to main's copy was needed.

## Out of scope
Feature logic, schema/proto fields beyond the anchor comment, RPC bodies, reachable screens, `desired/nat.go` /
natTabs (F-nat44-ed-sessions'), `apps/agent/internal/agent/metrics.go` (TD-8), SY6–SY9, `docs/contracts/proto.md` /
`docs/vpp-code-track.md`, `tools/ci.sh`, anchor removal, host tests, merging.
