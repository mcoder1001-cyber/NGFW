# F-snmp — SNMP v2c/v3 via snmpd + private MIB (WBS D7.5)

Branch `task/F-snmp` (worktree /home/user/wt/F-snmp), base be53867. Cloud container: **no VPP, no snmpd binary, no
PostgreSQL, no browser** — every host-dependent step is written and gated (`VRX_INTEGRATION`), not run here.

## What was built

| piece | files |
|---|---|
| contract (D-086 stand-ins + `SnmpState` RPC) | `contract(schema)` c93628e, `contract(proto)` 1bd44a0 + 74a3955; `F-snmp-contract.md`; `docs/contracts/proto.md` §11 |
| schema sub-schemas, semantic rules (credential when enabled, view exists) | `packages/schema/src/domains/ext/snmp.ts`, `semantic/snmp{,.fixtures,.test}.ts` |
| renderer gap: typed fields instead of structpb stand-ins | `renderers/snmpd/model.go` (tests **TestTypedStandIns**, **TestSecretRefRules** — 3 new typed cases replace "unknown stand-in key"); goldens unchanged; redaction, parse run, pending restart untouched |
| singleton descriptor `snmpd.config/vrx` under `Domains["services"]` (D-109 d) | `internal/subsystems/snmp.go`, `internal/desired/snmp.go`, one line each in subsystems.go / projection.go |
| AgentX subagent (pure Go RFC 2741 subset, no new dependency) + VRX-MIB | `internal/snmpagent/**`, `deploy/snmp/VRX-MIB.txt` |
| `SnmpState` RPC | `internal/agent/rpc_snmp.go` |
| API `GET /api/v1/state/snmp` (config via generic pointer routes) | `apps/api/src/features/snmp/**`, e2e `apps/api/test/e2e/snmp.e2e.test.ts` |
| UI Services → SNMP (general, communities, v3 users, trap receivers, state), en + fa | `apps/web/src/domains/services/snmp/**`, `locales/{en,fa}/snmp.json` |
| user docs | `docs/user/services/snmp.md` |

### Decisions taken (with options)
- **Renderer stage (D-109 d):** one singleton scheduler descriptor wrapping the renderer — options: (a) singleton
  descriptor [taken, envelope default], (b) a shared renderer stage in the agent core (not on main), (c) API-side hook.
- **D-125 (validate daemon config before any VPP write):** TD-13's Validator seam is not on the base, so the
  projection calls a check hook (Render + daemon parse run, cached by content) for `services.snmp`; a failure is a
  DryRun/Apply issue with a JSON pointer and the transaction never starts (**TestSnmpProjectionChecksBeforeVPP**).
  Options: (a) projection hook [taken], (b) Validate only in Create (after VPP writes of the same txn — rejected).
- **Retrieve:** the applied value (refs only, `snmpd-<owner>.json`, 0600) is reported only while the live
  snmpd.conf is byte-identical to its re-rendering; otherwise absent → drift is re-applied.
- **ActionRequired (D-079):** not a failure; persisted by the renderer, shown as `pendingAction`; the agent never
  restarts/starts snmpd (unit control is P10).
- **VRF:** `vrf` ≠ `default` stays rejected — binding snmpd to a VRF needs a Linux VRF (linux-cp): not cheap.
- **Default listen:** unchanged, loopback only with the `# WARNING:` line (RF-4 M1).
- **Enterprise OID:** placeholder under net-snmp's `netSnmpPlaypen` (`.1.3.6.1.4.1.8072.9999.9999.7853`) — question Q1.
- **Secrets:** refs resolve only through the slot fixture resolver (`VRX_SNMP_FIXTURE_SECRETS`, 0600, values must be
  `VRX_TEST_PSK_F-snmp_*`); otherwise refused with the PENDING-secret-channel message.
- **IF-MIB:** VPP interfaces only in `vrxIfTable`; IF-MIB stays net-snmp's (Linux) — question Q3.

## Verification (real output, this container)

```
$ go test -count=1 -v -run 'TestSubagentWalk|TestSubagentReregisters' ./internal/snmpagent/
INFO VRX-MIB registered with the AgentX master socket=/tmp/TestSubagentWalk…/agentx.sock subtree=.1.3.6.1.4.1.8072.9999.9999.7853 session=42 registrations=1
    subagent_test.go:178: .1.3.6.1.4.1.8072.9999.9999.7853.1.3.0 = type 4 … str "txn-17"
    subagent_test.go:178: .1.3.6.1.4.1.8072.9999.9999.7853.2.1.2.1 = type 4 … str "local0"
    subagent_test.go:178: .1.3.6.1.4.1.8072.9999.9999.7853.2.1.2.2 = type 4 … str "loop0"
    subagent_test.go:178: .1.3.6.1.4.1.8072.9999.9999.7853.2.1.5.2 = type 70 … u64 1099511627776
--- PASS: TestSubagentWalk (0.01s)
INFO VRX-MIB registered with the AgentX master … registrations=1
INFO VRX-MIB registered with the AgentX master … registrations=2
    subagent_test.go:229: re-registered after master restart in 501.014904ms
--- PASS: TestSubagentReregisters (0.51s)

$ go test -count=1 -v -run 'Snmp|TypedStandIns|SecretRefRules' ./internal/subsystems/ ./internal/renderers/snmpd/ ./internal/agent/
--- SKIP: TestSnmpStageIntegration (0.00s)
--- PASS: TestSnmpStageLifecycle (0.09s)
--- PASS: TestSnmpStageRollbackRemovesCommunity (0.04s)
--- PASS: TestSnmpProjectionChecksBeforeVPP (0.01s)
--- PASS: TestSnmpFixtureResolverRefusesRealSecrets (0.00s)
ok  	ngfw/agent/internal/subsystems	0.149s
--- SKIP: TestSnmpdIntegration (0.00s)
--- PASS: TestSecretRefRules (0.01s)
--- PASS: TestTypedStandIns (0.00s)
ok  	ngfw/agent/internal/renderers/snmpd	0.025s
--- PASS: TestSnmpStateResponse (0.00s)
ok  	ngfw/agent/internal/agent	0.025s
```

TS gates, run package by package (turbo cannot spawn tasks in this container: `Exec format error (os error 8)`,
the same on the base, so `tools/ci.sh quick` stops at `pnpm gen`):

```
packages/schema lint rc=0 / typecheck rc=0 / test rc=0      (vitest 1219 incl. semantic/snmp.test.ts 5/5)
packages/proto lint rc=0 (buf lint) / typecheck rc=0 / test rc=0   (70/70 incl. fixtures/snmp-full.json round trip)
packages/api-client lint rc=0 / typecheck rc=0 / test rc=0
apps/api lint rc=0 / typecheck rc=0 / test rc=0           (features/snmp/snmp.test.ts 2/2)
apps/web lint rc=0 / typecheck rc=0 / test rc=0           (101/101 incl. SnmpTab.test.tsx, nav.test.ts)
$ tools/ci.sh check --base be53867   → contract commits found; forbidden patterns + gitleaks ok; check PASSED
$ make -C apps/agent test build       → all packages ok except renderers/strongswan TestWatchResync (45 s timing test,
                                        untouched by this branch; passes alone: `go test -race -run TestWatchResync` ok 1.1 s)
$ make -C apps/agent lint             → not runnable here: golangci-lint built with go1.25 < go.mod's go1.26 (environment)
```

Generated outputs regenerated with the pinned plugins (protoc-gen-go v1.36.12, protoc-gen-go-grpc 1.6.2, buf) and
committed: `apps/agent/gen`, `packages/proto/gen/ts` (timestamp.ts kept as on base), `packages/api-client/src/generated`,
`apps/cli/internal/api/operations_gen.go`, `docs/user/cli/reference.md`.

Secrets: `grep -rn VRX_TEST_PSK_F-snmp_` hits only test literals/prefix constants (snmp.go prefix check, *_test.go,
e2e test, questions, envelope, prompt); `TestSnmpStageLifecycle` asserts no fixture value in the stage log;
`snmp.test.ts` asserts no `password/` ref in the state output; the record file holds refs only.

## Acceptance — status

| item | status |
|---|---|
| gosnmp walk v2c + v3 authPriv returns sysName + VRX-MIB table | **written, not run** (`TestSnmpStageIntegration`, needs snmpd + `VRX_INTEGRATION=1` on vrx-a); protocol + table proven against an in-process AgentX master (above) |
| TrapListener receives a trap (coldStart) | **written, not run** (same test, gosnmp TrapListener on 3<N>62) |
| agent restart → re-rendered, subagent re-registers < 30 s | fake-master re-registration 0.5 s (above); fresh-stage re-render in the integration test (not run) |
| rollback removes the community | unit: **TestSnmpStageRollbackRemovesCommunity** (file); walk-level in the integration test (not run) |
| unknown community → 400 problem+json with pointer | schema refinement (P02c) at `/services/snmp/trapReceivers/<i>/community`: schema unit test; e2e written, **not run** (no PostgreSQL here) |
| secrets absent from logs/GET/status | unit asserts above; grep above |
| `tools/ci.sh --base main` green | **not achievable here** (turbo spawn failure, golangci-lint toolchain); every gate run individually, results above |
| UI screenshot | **not taken**: no browser in this container |

## Shared hunks
Anchored: subsystems.go `Domains` (`"services": {desired.SnmpDescriptorName}`), dataplane.proto (fields, RPC, messages),
agent.client.ts, fake-agent.ts handler, app.module.ts (import, controllers, providers), i18n.ts (4 lines), services/tabs.ts.
**Unanchored** (listed in F-snmp-questions.md Q6): services.ts key lines, schema index.ts, semantic/index.ts,
subsystems.go register(), projection.go (2), fake-agent.ts import, nav.ts + nav.test.ts, proto.md.
Shared test edits: agent/service_test.go (2 expected lists), agent/projection_test.go (1 line) — Q7.

## Out of scope (not built)
Alarm/threshold engine, syslog, IPFIX/sFlow, SNMP SET (TestSet answers notWritable), AAA, ENTITY/HOST-RESOURCES work,
unit start/restart of snmpd (P10), IF-MIB override (Q3), last-trap time (Q4), the API→agent secret channel (PENDING).

## Open questions
docs/status/tasks/F-snmp-questions.md (PEN, secret channel, IF-MIB, last-trap time, unit control, unanchored hunks,
shared tests, examples naming, global check hook, services-domain warnings).

## Cleanup
No process was started outside tests (no snmpd, API, vite); `apps/agent/bin` removed; nothing under /run/vrx-test.
