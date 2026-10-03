# F-pppoe-client — PPPoE client on WAN interfaces (cloud session charming-johnson, 2026-09-27)

A PPPoE dial-up client per interface: `interfaces.<name>.pppoe`. The box runs pppd on a linux-cp tap of the WAN
parent, authenticates, and the agent mirrors the ISP-assigned address and default route into VPP. User page:
`docs/user/network/pppoe.md`.

## Decisions (the prompt's open questions)
- **Mechanism (measured against the alternatives):** pppd + `rp-pppoe.so` on a linux-cp tap. VPP 26.06's `pppoe`
  plugin is the **AC/decap** side only (`pppoe_add_del_session`, `pppoe_add_del_cp`) — no client dialer (PPPoE
  Discovery, LCP/PAP/CHAP/IPCP), so the client must run in Linux and be mirrored into VPP. Documented in
  `apps/agent/internal/renderers/pppoe/README.md`.
- **Where it lives:** `interfaces.<name>.pppoe` (per-interface, like `lcp`/`adl`/`urpf`), not a new domain. The WAN
  interface is an ordinary interface to NAT/ACL/Global-blocking — they attach by name and survive address changes,
  so no change was needed there.
- **Password:** a `password/<name>` secret reference; `privilegedChanges` already makes any secret ref admin-only, so
  an operator cannot set/redirect it (proven in the e2e).

## Built (all runs in this container)
- **Schema/semantics/proto** (2ec15de0): `domains/ext/pppoe.ts` (enabled, parent?, username, passwordRef, serviceName?,
  mtu 1492, mssClamp, defaultRoute, dnsFromPeer, ipv6 off|slaac|dhcpv6, reconnect{holdoffSec,maxFail}); semantic
  `interfaces.pppoe` (no static address on a client, dial-over parent exists and is enabled, MTU ≤ parent − 8);
  proto `Interface.pppoe=23`, `InterfaceState.pppoe=21` (PppoeSessionState), `PppoeReconnect` RPC.
- **Renderer** (1cb8fba1): `renderers/pppoe` — pure `Render(sessions)` → pppd peer file, chap/pap-secrets (Secret
  0600), ip-up/ip-down hooks (write `<hostif>.state`), per-session systemd unit; `Validate` structural (pppd has no
  offline checker); `ReadState` parses the hook file into `PppoeSessionState`. Golden + hostile-string + state tests.
- **Supervisor** (773d6af0): `Renderer.Apply(ctx, runner, sessions)` — write files, stop+prune gone sessions,
  daemon-reload on unit change, restart only changed/new (a no-op Apply never drops the link), remove shared secrets
  when none remain; owns only `ngfw-pppoe-*` units, never enable/disable. Recording-runner test.
- **API** (812339c7): `/state/interfaces` exposes the session; `POST /api/v1/actions/interfaces/{name}/pppoe/reconnect`;
  fake agent reports a session and answers reconnect. e2e: commit → session on /state/interfaces, reconnect
  (accepted / none), static-address rejected, operator 403 on passwordRef.
- **Web** (79501eca): the interface drawer's SchemaForm renders the pppoe settings; a live **PPPoE session** panel
  (phase chip, local/peer IPv4, IPv6, peer DNS, uptime, session id, fail count, last error) with a **Reconnect**
  button; en + fa; jsdom tests (up + reconnect, failed, RTL).

## Evidence (this session)
- Agent: `renderers/pppoe` golden (two sessions: peer/secrets/hooks/unit), hostile input rejected (bad host if,
  newline user/pass/service, out-of-range MTU), duplicate host if rejected, secrets are Secret 0600 and absent from
  the peer file (`Redacted()` hides them), state reader (up/down/failed), supervisor command sequences (initial /
  no-op / MTU change = one restart / removal = stop+prune / remove-all = secrets gone). `go test -race ./...` (agent)
  all ok; golangci-lint 0 issues; schema-proto drift guard passes; `buf breaking` clean.
- API: unit 300/300; e2e (PostgreSQL + Valkey + fake agent) 3/3 for pppoe plus the full suite unaffected.
- Web: 491/491, lint, `check-logical-css`.
- Container has no kernel PPP (`CONFIG_PPP` unset, no `/dev/ppp`) and no VPP, so the renderer/supervisor are tested
  with golden files and a recording runner; the live dial is the host row below.

## Not done here → `F-pppoe-client-host` (lab: kernel PPP + VPP)
- The agent's live Apply: start the units, track pppd exits for failCount/lastError, mirror the negotiated
  IPv4/IPv6 address and default route into VPP's FIB, and apply the MSS clamp on the WAN; implement the
  `PppoeReconnect` RPC handler in the real agent (it returns 501 until then).
- Topology test against an accel-ppp / rp-pppoe server in a netns: session up, traffic through NAT, server restart →
  reconnect within holdoff + 10 s, wrong password → clear error; `grep` proof the password never appears in config
  GET, logs or audit; screenshot of the drawer panel against the real endpoint.
- Wire the pppoe renderer into the agent subsystem registry (a descriptor that resolves the parent's linux-cp tap
  and the password secret, then drives Apply) — needs the full agent + VPP to test.
