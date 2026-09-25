# F-snmp — questions for the manager / product owner (none blocks the branch)

1. **PEN (product owner).** VRX-MIB lives under the placeholder `1.3.6.1.4.1.8072.9999.9999.7853`
   (net-snmp's `netSnmpPlaypen`, "for local experiments"). Register an IANA Private Enterprise Number, then change
   `snmpagent.VRXMIBOID` and `deploy/snmp/VRX-MIB.txt` together. Default kept: placeholder.
2. **Secret channel (manager; same case as PENDING-secret-channel for P11/F-wireguard/P12/F-unbound).** The agent
   resolves `password/…` refs only from a slot-local fixture file (`VRX_SNMP_FIXTURE_SECRETS`, 0600, values must be
   `VRX_TEST_PSK_F-snmp_*`); without it every community/passphrase is refused at projection time with
   "no API→agent secret channel yet (PENDING-secret-channel)". Only the end-to-end secret step waits.
3. **IF-MIB for VPP interfaces (product owner).** net-snmp's IF-MIB shows the Linux taps. This build serves VPP
   interfaces in `VRX-MIB::vrxIfTable` only; overriding IF-MIB (registering `.1.3.6.1.2.1.2.2` from the subagent at a
   higher priority) is possible but would hide the Linux interfaces — not done. Default: VRX-MIB only.
4. **"Last trap time" in /state/snmp.** snmpd does not expose when it last sent a notification (no MIB object, no
   log line we can rely on without parsing). Not in `SnmpStateResponse`; could come from a notification log MIB
   (`NOTIFICATION-LOG-MIB`, needs `notificationEvent` config) in a follow-up.
5. **Who restarts snmpd (P10).** A startup-only change (listen, AgentX socket, engine id) or a stopped daemon is a
   persisted D-079 request shown as `pendingAction`; the agent never restarts/starts the unit. The unit stays
   disabled until P10 decides unit control.
6. **Unanchored hunks** (no `wave-BC: F-snmp` anchor in these blocks; appended at the end, marked `F-snmp (unanchored)`):
   `packages/schema/src/domains/services.ts` (SnmpSchema / community / v3-user key lines), `packages/schema/src/index.ts`
   (export), `packages/schema/src/semantic/index.ts` (import + spread), `apps/agent/internal/subsystems/subsystems.go`
   (`registerSnmp` in register()), `apps/agent/internal/agent/projection.go` (builder in project(), assembler in
   assemble()), `apps/api/src/testing/fake-agent.ts` (import), `apps/web/src/nav/nav.ts` + `nav.test.ts`
   (`services` into BUILT_DOMAINS), `docs/contracts/proto.md` (§11 section).
7. **Shared test files touched (forced by adding the `services` domain):** `apps/agent/internal/agent/service_test.go`
   (two expected subsystem lists gain `services`), `apps/agent/internal/agent/projection_test.go` (clears the global
   snmp check hook before projecting the schema examples). Every later feature adding a domain edits the same two lines.
8. **Schema examples naming.** `packages/schema/src/examples.test.ts` assigns every example file to a group by prefix and
   has no `snmp-` group, so `packages/schema/examples/snmp-*.json` (my owned glob) would fail it. The fixtures live in
   `packages/schema/src/semantic/snmp.fixtures.ts` and `packages/proto/test/fixtures/snmp-full.json` instead. Adding
   `snmp` to the `SIBLING` regex would allow the owned glob.
9. **Global check hook.** `desired.SetSnmpCheck` is process-global (the projection is a free function). One agent per
   process in the product; tests reset it. A TD-13 Validator seam would replace it.
10. **Domain `services` semantics.** Making `services` implemented means non-SNMP leaves (dhcp, dns, lldp, ipfix, ntp, qos)
    are now `agent.unsupported-field` warnings instead of one `agent.unimplemented-domain` warning. F-kea / F-unbound /
    F-ipfix etc. remove their own names from that list in `desired/snmp.go` when they land (or the manager moves the
    list to a shared place).
