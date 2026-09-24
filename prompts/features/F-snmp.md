# Task: F-snmp — SNMP v2c/v3 via snmpd + private MIB   (prepend 00-CONTEXT.md)

## Goal
SNMP monitoring end to end (WBS D7.5 in `plan/wbs.csv`): net-snmp `snmpd` rendered from `services.snmp` (v2c communities, v3 USM users,
views, trap receivers), plus a small **private MIB** served through an AgentX subagent that exposes VPP interface counters and VRX health.
Reference: TNSR "SNMP"; IF-MIB served for VPP interfaces (not the Linux ones).

## Inputs to read first
- `packages/schema/src/domains/services.ts` — `services.snmp{enabled, vrf, listen[], engineId, sysName/Location/Contact, communities{<n>:
  {secretRef, access, sources}}, v3Users{<n>}, trapReceivers[]}` exists; community strings / passphrases are `password/<name>` secret refs (D-051)
- `apps/agent/internal/renderers/snmpd/` (RF-4, merged — use, do not rebuild, D-104) + its README + `docs/agent/renderers/snmpd.md`: model,
  template, goldens (`v2c`, `v3`, `traps`, `hostile-location`, `listen-any`, `full`, `disabled`), AgentX master socket in `Paths`, secret
  redaction (`rfkit.Redactor`), parse-run Validate, persisted restart requests, `TestPaths(prefix)`, in-process gosnmp client; shared helpers
  `renderers/rfkit` are read-only. RF-4 reads **stand-in fields** (D-055) from a `structpb` at the schema's future JSON path —
  `services.snmp.views`, `communities.<n>.view`, `v3Users.<n>.view`, `sysServices`, `monitors{disks,load}` — **D-086: you move them into the
  contract now**. RF-4 rejects `vrf` ≠ `default` "until F-snmp binds snmpd to a VRF" (a Linux VRF needs linux-cp — keep `default` unless cheap,
  document it)
- `apps/agent/internal/renderers/{renderer.go,README.md,ALLOWLIST.md}` — fixed-argv binaries only
- interface counters for the MIB: the stats segment (`/run/vpp/stats.sock`). The P05 reader (`apps/agent/internal/agent/telemetry.go`) is
  unexported agent core (read-only for you) — open your own govpp stats connection inside `internal/snmpagent`
- agent integration: no renderer is called by the agent today (`service.go` runs only the scheduler) — D-109 (d): wrap the snmpd renderer in
  **one singleton scheduler descriptor** (e.g. `snmpd.config/vrx`: Create/Update = Render → Validate → Apply, Delete = the RF-4 disabled
  rendering, Retrieve = renderer state) under `Domains["services"]`; the AgentX subagent is background work started from your own
  `subsystems/snmp*.go`, never from `agent.go`
- `docs/decisions/PENDING-secret-channel.md` — no API→agent secret channel yet: communities and USM passphrases resolve through a slot-local
  fixture resolver (`WithSecretResolver`, values `VRX_TEST_PSK_F-snmp_*`) until the owner decides; only the end-to-end secret step waits
- host facts (checked 2026-09-24): `snmpd` 5.9.4 installed, unit **disabled and inactive** — leave it so; **`snmpwalk`, `snmpget` and
  `snmptrapd` are NOT installed** (net-snmp's `snmp`/`snmptrapd` packages absent; no package installs): walks run in-process through
  gosnmp (BSD-2, already in `apps/agent/go.mod` since D-086), the trap receiver is a gosnmp `TrapListener` on a slot port — and a
  community or passphrase never goes into a process argv anyway (RF-4 README)
- `docs/lab/shared-host-rules.md` — snmpd only as a per-slot test instance (own conf dir, unprivileged port, own AgentX socket); never the
  system `snmpd` unit

## Contract changes
The D-086 stand-ins above, plus a private-MIB/AgentX enable flag if the UI needs it: additive, as separate `contract(schema): …` /
`contract(proto): …` commits on **your task branch** (no own branches; numbers from your envelope / `docs/status/wave-BC-numbers.md`),
`F-snmp-contract.md`, questions file, continue. No reshaping of `services.snmp`.

## Scope — build exactly this
Files you own and shared hotspots: your TASK ENVELOPE is authoritative (the board's old `agent/project_snmp*.go` became
`internal/desired/snmp*.go` + `internal/subsystems/snmp*.go` + `internal/agent/rpc_snmp*.go`, wave-A hotspots A2). Main pieces:
`apps/agent/internal/renderers/snmpd/**` (gap-only), `apps/agent/internal/snmpagent/**`, `deploy/snmp/**` (MIB text file `VRX-MIB.txt`).
Shared files: registration lines under your anchor only (agent registry, `app.module.ts`, router/nav, services tab registry).
1. **Schema** (your task branch, contract commits first): the D-086 stand-ins (views, per-community/user view, sysServices, monitors); trap
   receiver references an existing community/user (exists); v3 `authPriv` requires both refs (exists); `listen` addresses exist in `vrf`; at
   least one community or user when enabled.
2. **Agent**: project `services.snmp` onto the snmpd renderer through the singleton descriptor (secret refs resolved through the secret
   resolver — fixture resolver until PENDING-secret-channel is answered — never logged);
   `snmpagent`: a Go AgentX subagent (pure Go, no cgo, no net-snmp linking — GPL/BSD boundary respected; a new Go module dependency goes to
   the questions file first — the manager runs `go mod tidy` on main, D4 — else a minimal in-repo RFC 2741 subset) registering
   `VRX-MIB` (enterprise OID placeholder, flagged in questions): per-VPP-interface name/oper/admin/in/out octets+packets+errors from the
   stats path, agent health, running revision, commit counter; IF-MIB rows for VPP interfaces only if cheap — else document.
3. **API**: config via pointer routes; `GET /api/v1/state/snmp` (daemon status, engine id, last-trap time from renderer Retrieve).
4. **UI**: Services → SNMP: general, communities (secret input writes a secret, never shows it), v3 users, trap receivers; en + fa; screenshot.
5. **Docs**: `docs/user/services/snmp.md` — v2c read-only community restricted to a management prefix, v3 authPriv user, trap receiver;
   `snmpwalk` examples (for the operator's own station — the tool is not on the appliance); the MIB file location.

## Acceptance (paste the evidence)
- [ ] An in-process gosnmp walk (v2c, and v3 authPriv) against the slot snmpd returns sysName and the `VRX-MIB` interface table (pasted;
      `snmpwalk` is not installed on vrx-a)
- [ ] A gosnmp `TrapListener` on a slot port receives a trap after a test link-down event (or coldStart) — pasted (`snmptrapd` is not installed)
- [ ] Agent-restart simulation → snmpd config re-rendered, subagent re-registers within 30 s (log excerpt)
- [ ] Rollback removes the community (walk with it fails) — evidence, not assumption
- [ ] Trap receiver naming an unknown community → 400 problem+json with a `pointer`
- [ ] Secrets absent from rendered-file dumps in logs, GET, status file (grep pasted); `tools/ci.sh --base main` green

## Out of scope (do not build)
Alarm/threshold engine and email/webhook targets (F-dashboard-prom-alarms — it may later send traps through this renderer), syslog
(F-unbound-chrony-syslog), IPFIX/sFlow (F-ipfix-sflow), SNMP SET/write support of VPP config (read-only MIB), AAA for SNMP (F-aaa),
full ENTITY/HOST-RESOURCES MIB work beyond what net-snmp ships.

## Open questions to surface, not to decide silently
Private enterprise number (PEN) — use a documented placeholder OID until the product owner registers one. Whether IF-MIB should report VPP
interfaces instead of Linux ones by default (net-snmp's own IF-MIB would show the taps).
