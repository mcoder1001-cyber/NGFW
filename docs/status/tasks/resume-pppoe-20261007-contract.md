# PPPoE IPv6 contract status (PR196 recovery)

Inherited contract commit: `39abb388f contract(pppoe): IPv6 help matches the client; IPv6 needs MTU >= 1280`.
Source: `packages/schema/src/domains/ext/pppoe.ts`, `packages/schema/src/semantic/pppoe.ts`
and its test; generated description: `packages/yang/generated/ngfw-interfaces.yang`.

Shape, field names, enum values (`off`, `slaac`, `dhcpv6`), defaults (IPv6 `off`),
REST routes and protobuf field numbers are unchanged. Help now describes address
negotiation plus display-only delegated prefix. No LAN prefix assignment contract exists.

Semantic compatibility change: enabled PPPoE with IPv6 on and MTU below 1280 is
now rejected by schema semantic validation and renderer. Previously stored low-MTU
IPv6 configurations can therefore fail validation/reapply; raise the MTU to at
least 1280 if the parent link supports it, or explicitly turn IPv6 off. No silent
stored-config migration or universal backward-compatibility claim is made.

The original branch generated YANG from the schema source; this recovery has not
hand-edited generated output or changed contract sources. Final hosted generation
verification is owed on the manager's exact integration tree; the contract guard
has passed locally. Historical generator/quick evidence is in the original IPv6 WIP.
Diagnostic session up/mirroring does not promise product discovery or LAN transit;
both product gaps remain explicit in the PPPoE support docs and recovery WIP.
