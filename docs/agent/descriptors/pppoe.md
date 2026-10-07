# pppoe descriptors (DF-6, WBS D6.6)

Package `apps/agent/internal/descriptors/pppoe`. Messages only from `apps/agent/binapi/pppoe`. Shared rules: [df6.md](df6.md).

| Object type | Descriptor / key | Create / Delete | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| PPPoE session | `pppoe.session` · `pppoe.session/<client_mac>/<session_id>` | `pppoe_add_del_session` + owner tag | `pppoe_session_dump` + tag | `ErrRecreate` | `vrf/<decap_vrf_id>` |
| PPPoE control-plane interface (**VPP-global**, globals owner only) | `pppoe.cp` · `pppoe.cp/global` | `pppoe_add_del_cp` once per VPP boot (the enable stacks the `pppoe-input` feature) | **write-only** | set | `interface/<interface>` |

Model `pppoe.Session`: `session_id` (1–65535), `client_ip`, `client_mac`, `decap_vrf_id`.

Limitation: VPP creates a session only for a client MAC that `pppoe-input` has learned from PPPoE discovery packets;
otherwise `pppoe_add_del_session` returns INVALID_SW_IF_INDEX, mapped to `pppoe.ErrClientNotLearned`. The host has no
PPPoE clients and DF-6 sends no packets, so the host test verifies that typed error and skips the create/retrieve part
(Q3). PPPoE client/server daemons are out of scope.

`pppoe_add_del_cp` sets VPP's single `cp_if_index` (review M2), so it is a global singleton under D-071; its host test is
opt-in (`NGFW_DF6_PPPOE_CP_HOST=1`) and never runs on the shared VPP.

## Client desired-state descriptor and hook lifetime

`pppoe.client.config/ngfw` is a singleton `StageDaemon` object carrying a subset of `DesiredState` with PPPoE references and parent LCP mappings. It depends on the logical client interfaces and parent LCP objects. The `Validator` resolves available secrets, stages mode-safe files in a private temporary tree and runs exactly one renderer structural checker; pppd has no offline checker. Missing material is a projection warning during dry-run and an apply-time `passwordRef` error. Checker output is masked with `rfkit.Redactor`.

`ClientConfig.Retrieve` compares real files against the private reference-only manifest, reports file drift, and hydrates process-local session metadata after restart. The runtime retains no resolved password in its applied session registry. Product supervision uses an allowlisted argv-only runner. Slot apply writes only its own renderer paths and performs no supervision. `SetPppoeSecrets` injects the existing sealed versioned socket cache; no additional secret transport or API secret store was introduced.

The `pppoe-watch` source runs a one-second poll under the agent lifetime context. `Env.Exclusive` serializes mirror I/O with config commit/resync/rollback. The combined runtime-address classifier keeps PPPoE-assigned addresses and accepted VRRP VIPs out of static-address reconciliation. Mirroring validates the full negotiated record before writes; additions and withdrawals dump first to remain idempotent. An attempted partial mirror is tracked before I/O, so later down/removal can withdraw it. Route operations carry one path and `IsMultipath=true`; only an exact `api.NO_SUCH_ENTRY` on withdrawal is benign. Globals-owner exit observation deduplicates unit `ExecMainStatus`/`NRestarts` snapshots and resets on up.

Exit messages map pppd statuses 1–8, 10, 11, 15, 16 and 19; other nonzero statuses receive a numeric message. Source: [upstream pppd manual, EXIT STATUS](https://github.com/ppp-project/ppp/blob/master/pppd/pppd.8). No daemon output or credential is copied into state errors. Credentials-only edits receive an explicit unit restart when the existing renderer's peer/unit comparison would otherwise miss them. Clamp-only edits preserve hook state; removed/changed dial sessions delete their owned state file.

IPv6 (`ipv6` = `slaac` | `dhcpv6`): the poll also reads `<hostif>.state6` (ipv6-up/ipv6-down hook, see the renderer README) and the mirror adds every global IPv6 address as a /128 on the WAN interface and, with `defaultRoute`, the single `::/0` path via the RA router (usually the ISP's link-local, `FIB_API_PATH_NH_PROTO_IP6`) in the interface's own IPv6 table (`sw_interface_get_table` with `is_ipv6`), policy-checked like the IPv4 table before any write. Withdrawal removes the routes before the addresses. An IPv6 down withdraws only the IPv6 part (the poll re-mirrors the remaining IPv4 record), and a renumbering replaces the address. The runtime-address classifier covers the IPv6 addresses too. State for a session with IPv6 off is ignored. The DHCPv6-PD prefix is reported in `PppoeSessionState.ipv6` (`delegated <prefix>`) but not routed or assigned: the schema has no downstream target for it.
