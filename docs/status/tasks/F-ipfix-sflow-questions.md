# F-ipfix-sflow — questions for the manager (written, not waited on)

**Q1 — sFlow without hsflowd.** VPP's sflow plugin samples but exports nothing; hsflowd is not installed/shipped.
Taken: ship the config now with a warning (UI banner, `IpfixState.notes`, `sflow.exportsToCollectors: false` in REST,
`agent.unsupported-field` warnings on `collectors`/`agentAddress`/`vrf`). Options: (a) ship with warning [taken]
(b) hide sFlow until P10 packages hsflowd (c) reject `sflow.enabled` until then. **Follow-up to record:** hsflowd
packaging + rendering (P10 or a new row), reading `services.ipfix.sflow.{collectors,agentAddress,vrf}`.

**Q2 — flowprobe "one variant per interface" vs the schema default.** The schema defaults each monitored interface to
`ip4: true, ip6: true`; VPP enables only one variant per interface (ENTRY_ALREADY_EXISTS for a second). The shipped
example `services-snmp-lldp-ipfix-ntp.json` (P02c) uses that default, and `TestProjectSchemaExamples` projects it, so a
hard error was not possible without editing files I do not own. Taken: the agent realises ip4+ip6 as **ip4** (precedence
ip4, ip6, l2) and reports the dropped flag as `agent.unsupported-field` at `/…/interfaces/<i>/ip6`; the UI offers a
single variant select. Proposal (contract change, not made): default `ip6` to `false` in `IpfixFlowprobeInterfaceSchema`
and add the one-variant rule back as a semantic error.

**Q3 — Flowprobe params change blocked by another owner's interface on the shared host.** VPP refuses
`flowprobe_set_params` while any owner has flowprobe enabled. Taken: acceptable documented skip (the host test skips
with that reason; the agent's error names it). Options: (a) skip [taken] (b) serialise all flowprobe host tests under the
globals lock.

**Q4 — NAT44-ED IPFIX logging ownership (`nat_ipfix_enable_disable`).** Not built (no assignment). DF-3 → "DF-8", DF-8
did not build it, F-nat44-ed-sessions fences it to "DF-8 / F-ipfix-sflow", F-det44 names me for CGNAT logging export.
M7 recommended (a) an F-ipfix-sflow gap descriptor in `descriptors/ipfix`. Needs an explicit assignment; the exporter
those loggers use is exporter 0, which this task projects.

**Q5 — Shared files touched outside the anchors / files_owned (please fold in at merge):**
- `apps/agent/internal/descriptors/core/coretest/{ipfix_sflow.go (new), fakevpp.go (+1 field, +1 install line)}`:
  without fake handlers every agent unit test that retrieves all domains fails (A6 pattern: own file + one line). The
  install also sets `dfkit.IdentitySource` to a fixed identity (as `dfkittest.NewFake` does) — sflow's learned map needs
  a complete D-080 identity.
- `apps/agent/internal/agent/service_test.go`: expected `Health.subsystems` / Retrieve subsystems now include
  `services`; `ipfix_all_exporter_get` (a read-only getter, not a `_dump`) allowed in the DryRun / converged-resync
  "no writes" assertions. Every future domain will need the same edit.
- Unanchored hunks (no `wave-BC: F-ipfix-sflow` anchor in the block): `projection.go` (project + assemble),
  `subsystems.go` register(), `semantic/index.ts` (import + spread), `nav/nav.ts` BUILT_DOMAINS, `nav/nav.test.ts`,
  `fake-agent.ts` import line, `docs/contracts/proto.md`.
- `Domains["services"] = ipfixSflowDescriptors` (a slice in `subsystems/ipfix_sflow.go`): the next services family must
  extend that entry (e.g. `append(ipfixSflowDescriptors, …)`) — `services` becomes an implemented domain with this task,
  so the other `services.*` sub-trees are reported as `agent.unsupported-field` at `/services/<key>` by this build.

**Q6 — Exporter names in Retrieve (resolved after review).** Names are derived only from the agent's STORED desired state:
`desired.SetIpfixExporterNames` is called from `Service.refreshSnapshotLocked` (one line in service.go — outside files_owned,
please fold in). DryRun, failed and rolled-back transactions do not change them (`TestIpfixExporterNamesOnlyFromAppliedState`).

**Q7 — Environment (this cloud container, not the host):** `buf` is not installed — I built it (and pinned
protoc-gen-go v1.36.12 / protoc-gen-go-grpc v1.6.2) into my scratch dir; buf's bundled `timestamp.ts` WKT text differs
from the committed one, so that file was left as committed. The host golangci-lint is built with go1.25 while go.mod
says 1.26 (fails to load config); I used a go1.26 build of the pinned v2.13.2. pnpm 12.5.1's managed binary is a
placeholder here (turbo: Exec format error); CI ran with `npm_config_manage_package_manager_versions=false`.
