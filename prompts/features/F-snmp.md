# Task: F-snmp — SNMP v2c/v3 via snmpd + private MIB   (prepend 00-CONTEXT.md)

## Goal
SNMP monitoring end to end (WBS D7.5 in `plan/wbs.csv`): net-snmp `snmpd` rendered from `services.snmp` (v2c communities, v3 USM users,
views, trap receivers), plus a small **private MIB** served through an AgentX subagent that exposes VPP interface counters and VRX health.
Reference: TNSR "SNMP"; IF-MIB served for VPP interfaces (not the Linux ones).

## Inputs to read first
- `packages/schema/src/domains/services.ts` — `services.snmp{enabled, vrf, listen[], engineId, sysName/Location/Contact, communities{<n>:
  {secretRef, access, sources}}, v3Users{<n>}, trapReceivers[]}` exists; community strings / passphrases are `password/<name>` secret refs (D-051)
- `task/RF-4:apps/agent/internal/renderers/snmpd/` (branch — read with `git show`; merged before you start, dep RF-4): model, template,
  goldens (`v2c`, `v3`, `traps`, `hostile-location`), `AgentX` master socket field, secret redaction
- `apps/agent/internal/renderers/{renderer.go,README.md,ALLOWLIST.md}` — fixed-argv binaries only
- the P05 stats path (`apps/agent/internal/agent/telemetry.go`, `StreamStats`) — the source for interface counters in the MIB
- `docs/lab/shared-host-rules.md` — snmpd only as a per-slot test instance (own conf dir, unprivileged port, own AgentX socket); never the
  system `snmpd` unit

## Contract changes
Private-MIB enable flag / AgentX socket, if the UI needs them: branch `contract/F-snmp`, additive, `F-snmp-contract.md`, questions file,
continue. No reshaping of `services.snmp`.

## Scope — build exactly this
Files you own: `apps/agent/internal/renderers/snmpd/**`, `apps/agent/internal/snmpagent/**`, `deploy/snmp/**` (MIB text file
`VRX-MIB.txt`), `docs/agent/renderers/snmpd.md`, `apps/agent/internal/agent/project_snmp*.go`, `apps/api/src/features/snmp/**`,
`apps/web/src/domains/services/snmp/**`, `apps/web/src/locales/*/snmp.json`, `docs/user/services/snmp.md`, `test/topology/snmp/**`.
Shared files: one-line appends only (agent registry, `app.module.ts`, router/nav).
1. **Schema** (contract branch only if missing): trap receiver references an existing community/user (exists); v3 `authPriv` requires both
   refs (exists); `listen` addresses exist in `vrf`; at least one community or user when enabled.
2. **Agent**: project `services.snmp` onto the snmpd renderer (secret refs resolved through the secret resolver, never logged);
   `snmpagent`: a Go AgentX subagent (pure Go library, no cgo, no net-snmp linking — GPL/BSD boundary respected) registering
   `VRX-MIB` (enterprise OID placeholder, flagged in questions): per-VPP-interface name/oper/admin/in/out octets+packets+errors from the
   stats path, agent health, running revision, commit counter; IF-MIB rows for VPP interfaces only if cheap — else document.
3. **API**: config via pointer routes; `GET /api/v1/state/snmp` (daemon status, engine id, last-trap time from renderer Retrieve).
4. **UI**: Services → SNMP: general, communities (secret input writes a secret, never shows it), v3 users, trap receivers; en + fa; screenshot.
5. **Docs**: `docs/user/services/snmp.md` — v2c read-only community restricted to a management prefix, v3 authPriv user, trap receiver;
   `snmpwalk` examples; the MIB file location.

## Acceptance (paste the evidence)
- [ ] `snmpwalk -v2c` and `snmpwalk -v3 -l authPriv` against the slot snmpd return sysName and the `VRX-MIB` interface table (pasted)
- [ ] `snmptrapd` on a slot port receives a trap after a test link-down event (or coldStart) — pasted
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
