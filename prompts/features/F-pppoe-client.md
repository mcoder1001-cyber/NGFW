# Task: F-pppoe-client — PPPoE client on WAN interfaces   (prepend 00-CONTEXT.md)

## Goal
Connect to ISPs that deliver service over PPPoE: username/password dial-up on a WAN interface (optionally a VLAN
sub-interface), keep the session up, and use the ISP-assigned IPv4/IPv6 as the WAN address and default route.

## Inputs to read first
- `packages/schema/src/domains/interfaces.ts`, `tunnels.ts` (the existing `pppoe_session` is the **server/AC side** — not this).
- `apps/agent/internal/lcpmap/**`, P12 (linux-cp), `apps/api/src/secrets/**`.

## Contract changes
`interfaces[].pppoe { enabled, parent, username, passwordRef, serviceName?, mtu (default 1492), mssClamp: true,
defaultRoute: true, dnsFromPeer, ipv6: slaac|dhcpv6|off, reconnect { holdoffSec, maxFail } }`. Password only via secret ref.

## Scope — build exactly this
1. **Mechanism** (surface the choice with a measurement): pppd (rp-pppoe kernel mode) on a linux-cp tap of the parent, with the
   agent mirroring the ppp session address/routes into VPP; or a VPP-native path if one exists in 26.06. Document the choice.
2. **Agent**: renderer for the pppd peer file + supervisor; reconnect with backoff; state (up/down, session id, IPs, uptime, last
   error) published to telemetry; MSS clamping on the WAN.
3. **Interplay**: NAT44 outbound, Global Blocking and ACL attachments must accept the PPPoE interface; address changes re-apply them.
4. **UI**: WAN section on the interface drawer; status chip; "Reconnect" action.
5. **Tests**: renderer unit tests; topology test against a PPPoE server (accel-ppp or rp-pppoe server in a netns): connect,
   traffic through NAT, kill the server → reconnect, wrong password → clear error.
6. **Docs**: `docs/user/network/pppoe.md`.

## Acceptance (paste the evidence)
- [ ] Session up against the test server, default route and NAT work (pasted)
- [ ] Server restart → session re-established within holdoff + 10 s (pasted)
- [ ] Password never appears in config GET, logs or audit (grep)
- [ ] `tools/ci.sh --base main` green

## Out of scope
PPPoE server (exists), L2TP LNS, multilink PPP.
