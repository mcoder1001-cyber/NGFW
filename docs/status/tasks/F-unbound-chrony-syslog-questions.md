# F-unbound-chrony-syslog — questions for the manager

Written while working; none of these blocks the task (defaults applied, listed with options).

## Q1 (info) Contract commits are on the task branch
`contract(schema): …` and `contract(proto): …` are the first commits after the envelope on `task/F-unbound-chrony-syslog`
(details: `F-unbound-chrony-syslog-contract.md`). Numbers used: SyslogTarget 6–9, ActionRequest 7 — nothing else.

## Q2 (URGENT, incident) My test crashed the shared VPP at 2026-09-25 04:27:21 (NRestarts 1 → 2)
**What happened:** my slot-10 topology run (`test/topology/unbound-chrony-syslog`, agent owner `w10`, `VRX_GLOBALS_OWNER=0`)
called `POST /api/v1/actions/dns-lookup` → agent `ActionRequest.dns_lookup` → DF-8 `dns.ResolveName` → the binary API
message `dns_resolve_name` on the shared VPP, where the dns plugin was **not** enabled. VPP died with SIGSEGV:
`#0 ip4_sas + 0x31 (libvnet) ← #1–#4 dns_plugin.so ← vl_msg_api_socket_handler` (journal `vpp[2006833]`, core
`/var/lib/systemd/coredump/core.vpp_main.0.a93c0e7a….2006833.1790297841000000.zst`). systemd restarted VPP at 04:27:36;
every slot lost its VPP state. I stopped at once (no second lookup was sent).
**Cause (source):** `vnet_dns_resolve_name` does not check `dm->is_enabled` / that name servers exist before
`vnet_send_dns4_request` → `ip4_sas(0, ~0, server, …)` with `server` taken from the empty `ip4_name_servers` vector
(NULL deref). The DF-8 prompt's "works only where the plugin is enabled" is therefore not an error path — it is a crash.
**Fixed in the agent (this branch):** the dns_lookup action is refused with FAILED_PRECONDITION unless this agent is the
globals owner AND its applied configuration enables the VPP cache with at least one upstream; `dns.ResolveName` is never
reached otherwise. The topology test asserts the refusal instead of calling VPP. V-item added to `docs/vpp-code-track.md`
(one-line guard upstream). Please tell the other slot owners that VPP restarted at 04:27:36 because of me.

## Q3 Log explorer source — built: journald (the prompt's default)
RF-4 renders only `omfwd`, so there is no rsyslog-written file. The explorer runs one fixed-argv
`journalctl --no-pager --quiet --output=json --output-fields=… --reverse --lines=5000 --since=@<s> [--priority=N] [--facility=F]`
through its own allow-listed runner (ALLOWLIST row; 16 MiB output bound); the text filter is matched in the agent, never
passed. Alternative (needs your decision): an `omfile` target in RF-4 — breaks its "fixed templates, never omfile" rule.

## Q4 VPP DNS cache state = "configured" — please confirm
VPP has no getter (D-063), so `/state/dns.vppCache` reports the applied configuration (`configured`,
`appliedByThisAgent` = globals owner) with `live: false`, and DryRun notes `/services/dns/vppCache` as
`agent.unsupported-field` so `/state/drift` does not compare a leaf Retrieve can never report.

## Q5 Merge fit with F-kea-dhcp-relay (after it lands; you asked me to fold)
Both branches declare `desired.ServicesImplemented` / `desired.ServicesUnsupported` (mine: `desired/dns_services.go`,
same shape; delete my file and keep kea's), the `Services` const and `Domains[Services]` (keep one entry:
`append(kea's names, servicesDescriptors...)`), and `'services'` in `nav.ts` BUILT_DOMAINS / `nav.test.ts`. Both
`HostServices` and kea's builder call `ServicesUnsupported` — only one call may stay (else each warning appears twice).
I will do this fold as soon as F-kea is on main.

## Q6 Fake agent `action` handler (P5) vs F-vrf-static-ecmp
My spread `...unboundChronySyslogFake(this)` provides `action` (dns_lookup; everything else UNIMPLEMENTED) under my anchor,
after the original `action:`. F-vrf-static-ecmp replaces that line with its own spread (ping/traceroute). Whichever lands
second must chain them (mine answers only `dnsLookup`); otherwise the later spread wins for every action.

## Q7 Shared lines outside the anchors (listed in the status file)
`server.go`: the `Action` stream parameter `_` became `stream` (needed for the dns_lookup case). `service_test.go`:
the two implemented-domain assertions are registry-derived (D-129 F5 says the first wave-A branch does this).
`projection_test.go`: valid schema examples may carry `agent.secret-channel-pending` errors (the example
`services-snmp-lldp-ipfix-ntp.json` has an NTP keyRef). `management.ts`: one import + one spread line
(`SyslogServerSchema` had no seeded anchor).

## Q8 Whose daemons (decision taken, please ratify)
Only the globals owner renders into /etc/{unbound,chrony,rsyslog.d} and controls the units (RF-4's `systemctl restart
rsyslog`). Every other agent, including tools/app's main stack on this host (`VRX_GLOBALS_OWNER=0`), renders into
`/run/vrx-test/<owner>/…` with loopback-only listeners and never starts, restarts or signals a daemon. Without this gate,
the main stack would have rewritten the host's syslog export and restarted `rsyslog.service` on the first commit that
carries `management.syslog`.
