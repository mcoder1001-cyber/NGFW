# F-ipfix-sflow — IPFIX/flowprobe + sFlow (WBS D7.6)

Branch `task/F-ipfix-sflow` (base be53867). Cloud container: **no VPP host, no PostgreSQL, no Chrome** — everything
below ran against the fake VPP (coretest) / fake agent. Host evidence is **not done** (see "Not done").

## What
- **Contract** (additive, `F-ipfix-sflow-contract.md`): RPC `IpfixState` + messages; semantic rules
  `services.ipfix-sflow-{flowprobe-exporter,header-bytes,collector-unique}`; api-client regen. Reserved schema numbers unused.
- **Agent**: `internal/desired/ipfix_sflow.go` (projection + assembler), `internal/subsystems/ipfix_sflow.go`
  (`Domains["services"]`, D-071 registration), `internal/agent/rpc_ipfix_sflow.go` (`IpfixState`, /err/sflow counters
  from the stats segment). First enabled IPv4 exporter (by name) → `ipfix.default-exporter`, others → `ipfix.exporter`;
  flowprobe params (only with ≥1 interface) + interfaces; sflow global + interfaces; refs `interface/<name>`.
- **DF-8 gaps** (D-104, named tests): TD-11b ownership declarations in ipfix/flowprobe/sflow (`subsystems.TestRequirePersistentPerFamily`
  failed without them); `ipfix.ExporterDescriptor.StatIndex` (`TestExporterStatIndexRemembered`).
- **API**: `GET /api/v1/state/ipfix` (`features/ipfix-sflow`), fake handler, unit + e2e tests; config via the generic pointer routes.
- **UI**: Services → Flow export tab (Exporters / Flowprobe / sFlow, interface picker, hsflowd banner, non-owner warning), en + fa.
- **Docs**: `docs/user/services/ipfix-sflow.md`; notes appended to `docs/agent/descriptors/{ipfix,flowprobe,sflow}.md`.
- **Topology**: `test/topology/ipfix-sflow` — Go UDP IPFIX collector (unit tested) + opt-in `TestExporterZeroToSlotCollector`
  (VRX_INTEGRATION=1 + VRX_IPFIX_GLOBALS=1, flock -x /run/lock/vrx-globals.lock, exact save/restore of exporter 0 + flowprobe params).

## Obligations
- D-104: descriptors used, only gap changes. D-071: `RegisterGlobals` only when `Env.GlobalsOwner`; otherwise
  requirement-only globals (`TestIpfixSflowNonOwnerOnlyRequires`: refuses, never sets/resets a global).
- D-082: host test opt-in with own var + lock + exact restore (not run here). D-063/V16: classify-* registered, no domain, no UI.
- V17: restart shows exactly one sflow re-create (below). DF-8 params-blocked: documented skip (Q3). D-065/D-069: logical names.

## Verification (pasted)
Agent restart simulation / rollback (`go test -run TestIpfixSflow -v ./internal/agent/`, fake VPP):
```
reconcile done txn_id=i1 mode=apply domains="[interfaces vrfs routing services]" status=APPLY_STATUS_APPLIED summary=created:13
reconcile done txn_id="" mode=resync ... status=APPLY_STATUS_APPLIED summary="created:1 unchanged:12"   <- the one V17 sflow.interface re-create
reconcile done txn_id="" mode=resync ... summary=unchanged:13
reconcile done txn_id=i2 mode=apply domains=[services] status=APPLY_STATUS_APPLIED summary=deleted:6  <- rollback; globals reset (owner)
--- PASS: TestIpfixSflowGlobalsOwnerLifecycle
reconcile done txn_id=n1 ... status=APPLY_STATUS_ROLLED_BACK ... err="create ipfix.default-exporter/global: ... not the globals owner ... (D-071)"
reconcile done txn_id=n3 mode=apply domains=[services] status=APPLY_STATUS_APPLIED summary=deleted:6  <- globals untouched (non-owner)
--- PASS: TestIpfixSflowNonOwnerOnlyRequires
--- PASS: TestIpfixSflowProjection
```
400 with pointer: `apps/api/src/features/ipfix-sflow/ipfix-sflow.test.ts` — commit → 400, tier semantic,
`{pointer: '/services/ipfix/flowprobe/interfaces', …}` (passes). e2e twin `test/e2e/ipfix-sflow.e2e.test.ts` not run (no PostgreSQL).

Gates:
```
tools/ci.sh check --base be53867        -> check PASSED (gitleaks: no leaks)
pnpm turbo run lint typecheck test build -> Tasks: 30 successful, 30 total
apps/agent: go test -race ./... ok; make build ok; golangci-lint: 1 issue = pre-existing service.go:549 revertRetryMin (on be53867)
test/topology/ipfix-sflow: go vet ok; TestParseMessage, TestCollectorUDP PASS; host test SKIP (reason)
tools/ci.sh --base be53867 -> FAILED only at the generated-output gate: packages/proto/gen/ts/google/protobuf/timestamp.ts
  (WKT comment text of my locally-built buf differs from the host's pinned buf); all later steps run by hand above.
tools/ci.sh --base main -> local `main` (7b2f438) is behind the board tip; gitleaks flags pre-existing c2a8ab7 vpn.test.ts, not mine.
```
Manager: re-run `tools/ci.sh --base main` on the host (pinned buf) — expected clean.

## Not done (out of reach in this container)
- vppctl `show flowprobe …`/`show sflow` evidence, collector template/data evidence, 30-s restart on the rig, screenshot
  (no VPP/Chrome). Run `test/topology/ipfix-sflow` in a manager globals window.
- Out of scope (fenced): hsflowd packaging (P10 follow-up), NAT44-ED IPFIX enable (Q4), classify reports UI, dashboards, pcap.

## Shared hunks
Anchored: subsystems.go Domains; app.module.ts (3); agent.client.ts (2); fake-agent.ts handler; dataplane.proto (rpc + section);
tabs.ts; i18n.ts (4). Unanchored: projection.go (2), subsystems.go register(), semantic/index.ts (2), nav.ts, nav.test.ts,
fake-agent.ts import, proto.md. Outside files_owned (Q5): coretest/ipfix_sflow.go + fakevpp.go (2 lines), service_test.go (3 edits).

## Open questions
`F-ipfix-sflow-questions.md` Q1–Q7 (sFlow shipped with warning; ip4+ip6 default realised as ip4; params skip; NAT44-ED owner; shared edits; exporter names; env).

## Review round 1 (APPROVE with conditions)
1. Exporter names: derived only from the stored desired state (`desired.SetIpfixExporterNames` from
   `Service.refreshSnapshotLocked`, one line in service.go — outside files_owned); projections no longer write them.
   Test `TestIpfixExporterNamesOnlyFromAppliedState` (DryRun, rolled-back apply → unchanged; applied rename → seen).
2. ip4+ip6 default: behaviour kept; user guide states IPv6 flows are not recorded under the default; LOG.md D-146.
3. User guide sFlow section starts with "no collector receives anything until hsflowd is packaged (P10 follow-up)".
5. coretest: `dfkit.IdentitySource` set once per test binary (`sync.Once`), not on every `coretest.New()`.
Reruns: gofmt clean; go vet + `go test -race` ok for internal/{agent,desired,subsystems,descriptors/core/...,ipfix,flowprobe,sflow};
web vitest (services, nav): 7 passed; schema vitest: 1219 passed.
