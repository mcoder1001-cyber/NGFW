# PPPoE client IPv6 (claude-pppoe-ipv6, 2026-10-07)

Owner decision: implement bug 4 from the live PPPoE acceptance (`claude-pppoe-wip.md`: the schema offered
`ipv6: slaac|dhcpv6` but only IPv4 was mirrored). Branch `claude/pppoe-ipv6-20261007` from
`claude/pppoe-secret-20261006` (`042d013d4`, the password-resolution fix), worktree `/root/ngfw-wt/claude-pppoe-ipv6`.
Local commits only (not pushed, per owner instruction for this task). Author `Claude <noreply@anthropic.com>`.

Commits on top of `042d013d4`:
- `39abb388f` contract(pppoe): IPv6 help matches the client; IPv6 needs MTU >= 1280
- `e7e964d1e` feat(pppoe): IPv6 for the PPPoE client (SLAAC, DHCPv6 IA_NA/PD, VPP mirror)
- `192ae7fe6` fix(pppoe): IPv6 refresher exits and drops its state when the session is withdrawn
- `faf8d66d0` docs(renderers): list dhcpcd (PPPoE IPv6 hook) in the binary allowlist
- this status doc + evidence (the commit that adds this file; amended once to record the gate)

## Owned files

`apps/agent/internal/renderers/pppoe/**` (paths, renderer, state, new `state6.go`, templates `hook6.tmpl`,
`dhcpcd.tmpl`, `dhcp6.tmpl`, tests, golden, README), `apps/agent/internal/renderers/ALLOWLIST.md` (one row),
`apps/agent/internal/descriptors/pppoe/client.go` + `client_mirror_ipv6_test.go`,
`apps/agent/internal/subsystems/{pppoe.go,pppoe_watch.go,pppoe_ipv6_test.go}`,
`packages/schema/src/domains/ext/pppoe.ts`, `packages/schema/src/semantic/pppoe{,.test}.ts`,
`packages/yang/generated/ngfw-interfaces.yang` (regenerated), `apps/api/src/testing/fake-agent.ts` (fake IPv6 text),
`apps/web/src/domains/interfaces/PppoeDrawer.test.tsx`, `docs/user/network/pppoe.md`,
`docs/agent/descriptors/pppoe.md`, `docs/09-os-packages.md`, this file and `claude-pppoe-ipv6-evidence/**`.

## Design

- **pppd**: `ipv6` slaac|dhcpv6 → `+ipv6` (IPv6CP, link-local); off → `noipv6` (unchanged).
- **ipv6-up/ipv6-down hooks** (`/etc/ppp/ipv6-{up,down}.d/ngfw-<hostif>`, rendered only with IPv6 on, run by Debian's
  `/etc/ppp/ipv6-up` via run-parts; act only for `PPP_IPPARAM = ngfw-<hostif>`, reject a non-name `PPP_IFACE`):
  - kernel SLAAC on the PPP link: `accept_ra=2`, `autoconf=1`, `accept_ra_defrtr` = the session's `defaultRoute`;
  - re-add the pppd link-local (`peer`, `nodad`) so the kernel sends a Router Solicitation now (pppd adds it before the
    sysctls, so otherwise the first RA is the ISP's next periodic one) — proven by probe (`03-probe-slaac.txt`);
  - a refresher (background, pid in `<hostif>.ipv6.pid`) rewrites `<StateDir>/<hostif>.state6` atomically, only on
    change, every 2 s: `addr=` global addresses on the link (`ip -6 -o addr … scope global -tentative -dadfailed`),
    `gw=` the RA default router, `pd=` from the DHCPv6 script, `lllocal/llremote`. It exits when the link, pppd
    (`PPPD_PID`) or its own hook file (session withdrawn) is gone; on withdrawal it removes its state;
  - `dhcpv6`: the refresher also starts `dhcpcd -B -f /etc/ppp/ngfw-dhcpcd-<hostif>.conf <ppp>` and kills it on exit.
    dhcpcd comes from `dhcpcd-base` (Ubuntu priority *important*, in the base image; no package installed). The
    project ships no other DHCPv6 *client* (Kea is a server; VPP's `dhcp6_*_client_cp` sends on the VPP Ethernet WAN,
    not inside the PPPoE session). Config: `ipv6only`, `noipv6rs` (the kernel handles RA), `noup`, `ia_na 1`,
    `ia_pd 2`, `script /etc/ppp/ngfw-dhcp6-<hostif>`. The event script records `pd=<prefix>/<len>` only for a
    hex/colon literal and a numeric length (dhcpcd names it `new_dhcp6_ia_pd1_prefix1` — positional, not the IAID;
    found by probe) and removes it on EXPIRE6/RELEASE6/STOP*/NOCARRIER/DEPARTED;
  - down: stop the refresher (wait ≤ 5 s), remove pd, write `phase=down`.
- **State** (`ReadIPv6`): parses state6 with validation (global unicast addresses only; router link-local or global, no
  zone; delegated prefix /16–/64); down → empty. `ReadState` puts the summary in the **existing** `PppoeSessionState.ipv6`
  field — e.g. `2001:db8:9::100/128, 2001:db8:9:0:…/64, delegated 2001:db8:900::/56` — and reports an IPv6-only session
  as `up`. No proto change; the UI already shows `ipv6` in the PPPoE panel.
- **Mirror** (`ClientMirror`, same path as IPv4): `Mirror.LocalIPv6 []string` (each address as **/128** on the WAN, as
  IPv4 is /32 — the on-link /64 belongs to the PPP link, not the Ethernet VPP sees) and `PeerIPv6` (RA router).
  With `defaultRoute`, a single `::/0` path via the router (`FIB_API_PATH_NH_PROTO_IP6`, `IsMultipath=true`) in the
  interface's own **IPv6** table (`sw_interface_get_table is_ipv6`), policy-checked with the same `RouteTablePolicy`
  before any write; requires at least one global address. Withdrawal: routes first, then addresses. MSS clamp: one
  `mss_clamp_enable_disable` covers both families (IPv4 MTU−40, IPv6 MTU−60, RX+TX), unchanged and now exercised.
- **Watcher**: `observe()` reads both state files (IPv6 only when the session has IPv6 on); any change (IPv6 down,
  renumbering) withdraws the old record and re-mirrors the new one, as for IPv4. The runtime-address classifier now
  also covers IPv6 addresses. Redial/removal deletes `.state .state6 .pd .ipv6.pid`; the supervisor removes IPv6
  files of a kept session when IPv6 is turned off (dhcpv6→slaac drops the dhcpcd files) — such a change redials
  (the peer file changes).
- **Schema/UI**: help text now states what each mode does; new semantic rule: IPv6 on requires MTU ≥ 1280 (the
  renderer refuses it too). Options stay `off|slaac|dhcpv6`, all three implemented. The delegated prefix is reported
  but **not** routed or assigned to a LAN interface (the schema has no target for it) — follow-up if wanted.

## Tests (actual)

- Unit (new): renderer `TestRenderIPv6Modes`, `TestIPv6ScriptsParse` (`sh -n` on every rendered script),
  `TestDHCP6ScriptRecordsDelegatedPrefix` (incl. hostile values), `TestIPv6DownHookStopsRefresherAndRecordsDown`
  (runs the real down hook against a live pid), `TestApplyRemovesIPv6FilesWhenTurnedOff`, `TestReadIPv6`,
  `TestReadStateIncludesIPv6`; mirror `TestClientMirrorIPv6Up`, `…WithdrawRoutesBeforeAddresses`,
  `…RouteConditions`, `…RouteRefusedByPolicy`, `…RejectsBadValues`; watcher `TestPppoeIPv6MirrorFollowsHookState`
  (dual stack up, unchanged, renumber, real ipv6-down hook withdraws IPv6 only, removal withdraws all + state),
  `TestPppoeIPv6IgnoredWhenOff`. Golden updated (third session, dhcpv6). Schema: IPv6 MTU rule test. Web:
  PppoeDrawer shows the IPv6 summary.
- `go test -race -count=1` renderers/..., descriptors/pppoe, subsystems, agent: all ok (`evidence/01-go-race.txt`).
  golangci-lint on the touched Go packages: 0 issues.
- Schema vitest `semantic/pppoe.test.ts` 6/6; web vitest `PppoeDrawer.test.tsx` 2/2.
- `TMPDIR=/root/.cache/ngfw-ci-host tools/ci.sh quick --base origin/main`: see "Gate" below.

## Probe (host, private netns, slot-9 lock; `evidence/03-probe-slaac.txt`, `04-probe-dhcpv6.txt`, script `probe-inner.sh.txt`)

veth + `pppoe-server` + rendered client files, no VPP. SLAAC address + default route via the ISP link-local within
2 s; re-adding the link-local triggers an RS (`rs from fe80::…` on the ISP); dhcpcd obtained IA_NA
`2001:db8:9::100/128` + IA_PD `2001:db8:9100::/56`; down hook stopped refresher and dhcpcd, state6 `phase=down`.
Kea 3.0.3 as the DHCPv6 server failed on the ppp interface (binds the *peer* link-local of a point-to-point address),
so the test ISP uses a 44-line DHCPv6 responder (`dhcp6s.py`, test-only).

## Live (slot 9, `/run/lock/ngfw-slot-9.lock`, disposable VPP; driver = claude-pppoe `bc9a69f96` driver + secret-branch
signature fix + IPv6 extension, kept in `.scratch/pppoe-driver/pppoe/` (gitignored); full diff in
`evidence/driver-ipv6.patch` incl. `ra.py`, `dhcp6s.py`)

Commands (worktree root): `python3 .scratch/pppoe-driver/pppoe/run.py diag slaac|diag dhcpv6|product dhcpv6`.
SOURCE_SHA `192ae7fe6` (the allowlist doc commit after it changes no code). Discovery still needs the NON-PRODUCT diag
mode (defect 2: VPP's pppoe_plugin eats PADI); diag mode also installs the non-product name→material shim. So the
IPv6 dataplane rows below are **non-product diag evidence** of the agent's IPv6 logic on a real PPP session, not
product acceptance.

| Case | diag slaac (`11`) | diag dhcpv6 (`12`) | product dhcpv6 (`13`) |
|---|---|---|---|
| Apply with IPv6 + sealed password | PASS | PASS | PASS (rendered `+ipv6`, ipv6 hooks, dhcpcd conf) |
| Dial-up | PASS | PASS | FAIL — no PADO (defect 2, unchanged) |
| IPv6 negotiation (agent `ipv6` field) | PASS `…:9c35…/64` | PASS `2001:db8:9::100/128, …/64, delegated 2001:db8:900::/56` | NOT RUN (no session) |
| IPv6 /128s + `::/0` via ISP link-local in table 9401 | PASS (2.1 s after RA start) | PASS (both /128s, 4.1 s) | NOT RUN |
| MSS clamp IPv6 (1432, RX+TX) | PASS | PASS | NOT RUN |
| Host ping6 over ppp0 (non-product evidence) | PASS | PASS | NOT RUN |
| IPv6 withdrawn while session down (ISP stop) | PASS | PASS | NOT RUN |
| IPv6 re-mirrored after reconnect | PASS (2.6 s) | PASS (4.6 s) | NOT RUN |
| IPv6 withdrawal on pppoe removal | PASS | PASS | PASS (vacuous: nothing mirrored) |
| IPv6 confirm-timeout rollback | PASS | PASS | PASS (vacuous) |
| Withdrawal cleanup (all slot files incl. state6) | PASS | PASS | PASS |
| IPv6 LAN transit through VPP | NOT RUN — blocked by defect 3 (no PPPoE encap in VPP; IPv4 transit FAILs the same way) | same | NOT RUN |
| IPv4 cases (mirror, reconnect, wrong password dialer, rollback) | PASS (traffic FAIL = defect 3) | PASS (traffic FAIL = defect 3) | as before |
| Password grep / leftover processes (pppd, dhcpcd, ra/dhcp6s, refreshers) / netns / shared VPP 1014 NRestarts 0 | PASS | PASS | PASS |

The first slaac run (`10-diag-slaac-run-first.txt`, SHA `e7e964d1e`) found a real defect: after a slot withdrawal
the still-running refresher left `<hostif>.ipv6.pid` (withdrawal-cleanup / confirm-rollback FAIL). Fixed in
`192ae7fe6` (refresher removes its pid file and, when its hook is gone, its state; redial/removal deletes the pid
file); the reruns pass. Each driver run exits 1 only because of the pre-existing IPv4 `traffic` FAIL (defect 3) /
product dial-up FAIL (defect 2).

## Side effects on the shared host (honest record)

- Probe runs (before the driver used private dirs) wrote `/var/lib/dhcpcd/{duid,ppp0.lease6}` and sockets in
  `/run/dhcpcd/`; removed afterwards (`/run/dhcpcd` itself is left, empty). The live driver bind-mounts private dirs.
- One probe ran `kea-dhcp6` in a private netns but the shared mount namespace: it rewrote the pre-existing
  `/var/lib/kea/kea-dhcp6-serverid` (same owner `_kea`, directory untouched, kea services inactive). Content before
  is unknown; Kea reuses an existing server-id file, so it is most likely unchanged.
- No package installed; system VPP pid 1014 untouched (NRestarts 0); no other worktree touched; nothing pushed.

## Not done / follow-ups

- Defects 2 and 3 (discovery through VPP, PPPoE encapsulation in VPP) still block product dial-up and LAN transit for
  both families; IPv6 product acceptance reruns `run.py product slaac|dhcpv6` once they are fixed.
- Delegated prefix is reported only; assigning it to a LAN / RA on LAN needs a schema field (owner decision).
- `deploy/debian` does not declare `ppp`/`pppoe`/`dhcpcd-base` runtime dependencies (pre-existing for ppp);
  `docs/09-os-packages.md` now lists them.
- `sdk/terraform` generated schema is already stale on main (unrelated fields); not regenerated here, so its
  `ipv6` description keeps the old help text until the next `genschema` run.
- Product IPv6 on the globals owner (real systemd unit, `/etc/ppp`) is untested on hardware (lab-only acceptance).

## Gate

`TMPDIR=/root/.cache/ngfw-ci-host tools/ci.sh quick --base origin/main` at `ea1bcbc06` (tree identical to this
commit except this section and the copied log): **CI GATE PASSED**, 16m59s (`evidence/05-ci-quick.txt`; one
apply-startup scenario flaked in the parallel run and passed the serial rerun, host load). An earlier run at
`e7e964d1e` failed only `TestAllowlistDocumented` (dhcpcd not in `ALLOWLIST.md`), fixed by `faf8d66d0`.

## Recovery

Worktree `/root/ngfw-wt/claude-pppoe-ipv6`; driver copy in `.scratch/pppoe-driver/pppoe/` (recreate from
`evidence/driver-ipv6.patch` applied to claude-pppoe `test/topology/pppoe/` + the secret-branch one-line change);
probe in `.scratch/probe6/`. Next: `TMPDIR=/root/.cache/ngfw-ci-host tools/ci.sh quick --base origin/main`.
