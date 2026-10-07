# pppoe — PPPoE client renderer (F-pppoe-client)

Mechanism (task decision, measured against the alternatives): **pppd with the `rp-pppoe.so` kernel plugin**
on a linux-cp tap of the WAN parent. VPP 26.06's `pppoe` plugin is the **AC/decap** side only
(`pppoe_add_del_session`, `pppoe_add_del_cp`) — it has no client dialer (PPPoE Discovery, LCP/PAP/CHAP/IPCP),
so a client must run in Linux. The parent interface is mirrored into Linux by a linux-cp pair (P12); pppd runs
PPPoE discovery on that tap; the agent mirrors the ISP-assigned address and default route into VPP's FIB and
clamps the forwarded TCP MSS on the WAN.

## What this package renders (pure `Render(sessions)`)

Per enabled `interfaces.<name>.pppoe`, resolved by the agent into a `Session` (password from its
`password/<name>` secret, `HostIf` = the parent's linux-cp tap):

| path | mode | contents |
|---|---|---|
| `/etc/ppp/peers/ngfw-<hostif>` | 0644 | pppd options: `plugin rp-pppoe.so`, `nic-<hostif>`, `user`, `remotename ngfw-<hostif>`, `mtu`/`mru`, `defaultroute`/`nodefaultroute`, `usepeerdns`, `+ipv6`, `persist`, `holdoff`, `maxfail`, LCP echo keepalives |
| `/etc/ppp/chap-secrets`, `/etc/ppp/pap-secrets` | 0600 **Secret** | one `"<user>" ngfw-<hostif> "<password>" *` line per session |
| `/etc/ppp/ip-up.d/ngfw-<hostif>`, `/etc/ppp/ip-down.d/ngfw-<hostif>` | 0755 | hook pppd runs on link up/down; writes `<StateDir>/<hostif>.state` for the state reader |
| `/etc/ppp/ipv6-up.d/ngfw-<hostif>`, `/etc/ppp/ipv6-down.d/ngfw-<hostif>` | 0755 | only with IPv6 on: kernel SLAAC on the PPP link (`accept_ra=2`, `autoconf=1`, `accept_ra_defrtr` = default route), re-adds the link-local to send an RS, keeps `<StateDir>/<hostif>.state6` current (global addresses, RA default router, PD prefix) from a refresher that lives while pppd and the link do; down stops it first |
| `/etc/ppp/ngfw-dhcpcd-<hostif>.conf`, `/etc/ppp/ngfw-dhcp6-<hostif>` | 0644 / 0755 | only with `dhcpv6`: `dhcpcd` (dhcpcd-base) config — `ipv6only`, `noipv6rs` (the kernel does RA), `ia_na 1`, `ia_pd 2` — and its event script, which records a validated delegated prefix in `<hostif>.pd` |
| `/etc/systemd/system/ngfw-pppoe-<hostif>.service` | 0644 | one unit per session, `ExecStart=pppd call ngfw-<hostif> … ipparam ngfw-<hostif>`, `Restart=on-failure` |

Every user-controlled string goes through `ident`/`quoted` in the templates; `Session.validate` rejects a
host interface that is not a Linux name, a username/service-name with quotes or control characters, a password
with a newline, and an out-of-range MTU. The password never appears in the peer file (only the Secret files);
`Files.Redacted()` hides it in logs and diffs.

## Validate

pppd has **no offline configuration checker** (unlike `vtysh -C` or `unbound-checkconf`). `Validate` therefore
re-renders and re-checks structurally (the same `ErrInput` rules and the shared `CheckRendered` backstop).
The live dial is validated by the session coming up (state phase `up`), reported per interface.

## State

`ReadState(hostIf, failCount, lastError)` parses `<StateDir>/<hostif>.state` (written by the hook) into
`PppoeSessionState` (phase, local/peer IPv4, DNS, since); a missing file is `down`; `failCount > 0` while not
up is `failed`. `ReadIPv6(hostIf)` parses `<hostif>.state6` (values are validated: global unicast addresses, a
link-local or global router, a /16–/64 delegated prefix); `ReadState` puts its summary
(`"2001:db8::5/64, delegated 2001:db8:100::/56"`) in the `ipv6` field and reports an IPv6-only session as `up`.
A session with IPv6 on must have MTU ≥ 1280. `failCount`/`lastError` come from the agent's pppd supervisor, not the hook.

## Not in this package (agent side, `F-pppoe-client-host`)

Starting/stopping the units, tracking pppd exits for `failCount`/`lastError`, mirroring the negotiated
address/route into VPP, and the MSS clamp are the agent's Apply on the box — they need `/dev/ppp` and VPP, so
they are proven on the lab host (topology test against an accel-ppp/rp-pppoe server: connect, traffic through
NAT, server restart → reconnect within holdoff, wrong password → clear error).
