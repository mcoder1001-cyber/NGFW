# keepalived renderer — desired state ↔ rendered directives (RF-4)

Code: `apps/agent/internal/renderers/keepalived` (README there: script policy, VRRPv2 auth, dump redaction, tests).
File: `/etc/keepalived/keepalived.conf`, `root:root 0640`, written atomically; validated with `keepalived -t -f
<staged> [-s <netns>]`; applied with `systemctl reload keepalived` (SIGHUP, same PID); convergence and state from
keepalived's JSON dump (SIGJSON) and the notify helper's `<instance>.state` files.

Only `ha.vrrp` entries with `engine: "keepalived"` and `enabled` ≠ false are rendered (`vpp` → DF-7).
Stand-ins (D-055): `ha.keepalived.*` and `ha.vrrp.<name>.keepalived.*` (RF-4-questions.md Q2).

| desired state (JSON path) | rendered | validation |
|---|---|---|
| — | `global_defs { enable_script_security; script_user root; vrrp_version 3 }` | fixed |
| `ha.keepalived.routerId` *(stand-in)* / `system.hostname` | `router_id <id>` (default `vrx`) | `[A-Za-z0-9_.-]{1,32}` |
| `ha.keepalived.garpMasterRefresh` *(stand-in)* | `vrrp_garp_master_refresh <s>` | 1–86400 |
| `ha.keepalived.scripts.<name>.{check,interval,weight,fall,rise}` *(stand-in)* | `vrrp_script <name> { script "<ChecksDir>/<check>" interval … weight … fall … rise … }` | `check` ∈ shipped allow-list (`WithChecks`); never a path or script text; interval 1–3600, weight −253…253, fall/rise 1–255 |
| `ha.vrrp.<name>` | `vrrp_instance <name> { … }` | name `[A-Za-z0-9_.-]{1,32}` |
| `.interface` | `interface <linux if>` and `dev <linux if>` of each VIP | VPP → Linux via the interface mapper (product default `NoMapper`: rejected until F-vrrp); Linux name `[A-Za-z0-9_.-]{1,15}` |
| `.vrId` | `virtual_router_id <n>` | 1–255, unique per (interface, family) |
| `.priority` | `priority <n>`; `state MASTER` when 255 else `state BACKUP` | 1–255 (default 100) |
| `.advertisementIntervalMs` | `advert_int <seconds>` (`500` → `0.5`, `1230` → `1.23`) | 10–40950, multiple of 10 |
| `.addressFamily` ipv6 | `native_ipv6` | VIPs/peers of the same family |
| `.preempt` false | `nopreempt` | not with priority 255 |
| `….keepalived.preemptDelay` *(stand-in)* | `preempt_delay <s>` | 0–1000, only with preempt |
| `.acceptMode` true | `accept` | false renders nothing (keepalived's non-strict default accepts; enforcing needs firewall rules — Q4) |
| `.unicast.peers[]` | `unicast_peer { <ip> … }` | same family, not unspecified/multicast |
| `….keepalived.unicastSrcIp` *(stand-in)* | `unicast_src_ip <ip>` | same family, unicast instances only |
| `.addresses[]` + `….keepalived.prefixLength` *(stand-in)* | `virtual_ipaddress { <ip>/<len> dev <if> }` | 1–32 addresses, same family, no duplicates; len default /32 or /128 |
| `….keepalived.virtualRoutes[].{prefix,via,interface}` *(stand-in)* | `virtual_routes { <net> [via <gw>] dev <if> }` | prefix masked, same family; gateway unicast; interface mapped |
| `.track[].{interface,priorityDecrement}` | `track_interface { <if> weight -<n> }` | mapped; not the instance's own interface; 1–253 (default 10) |
| `….keepalived.trackScripts[]` *(stand-in)* | `track_script { <name> }` | defined in `ha.keepalived.scripts` |
| `….keepalived.authRef` *(stand-in, psk/…)* | `version 2` + `authentication { auth_type PASS auth_pass <key> }` | IPv4, whole-second advert; key 1–8 chars `[A-Za-z0-9_.,:;@%+=/~^*-]`; file marked Secret |
| — | `notify_master` / `notify_backup` / `notify_fault` / `notify_stop` `"<helper> <state dir> INSTANCE <name> <STATE>"` | helper and state dir from `Paths` |
| `ha.keepalived.syncGroups.<name>[]` *(stand-in)* | `vrrp_sync_group <name> { group { <instance> … } }` | members are rendered instances, each in one group |
| `.vrf` | — | only `default` |
| `.description` | not rendered | — |

Never rendered: `vrrp_strict`, `use_vmac`, `no_accept`, `include`, `$VAR`, `@…`. Retrieve per instance: notify
`state`/`since` and the whitelisted dump view (`state`, `interface`, `vrid`, `version`, `basePriority`,
`effectivePriority`, `vipsSet`, `vips`, counters) — never `auth_data`.
