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
