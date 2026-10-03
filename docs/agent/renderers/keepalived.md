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

## Wired by F-vrrp-config-sync

`internal/subsystems/keepalived.go` registers the renderer as the singleton stage `keepalived.config/vrx` (domain
`ha`, pattern of F-snmp's `snmpd.config`). The projection (`internal/desired/vrrp.go`) puts every `engine: keepalived`
instance whose interface has a linux-cp pair (`interfaces.<if>.lcp`, P12) into the stage's value together with those
pairs; the stage sets P12's `lcpmap.Mapper` from that value before every render, so the renderer's `InterfaceMapper`
is the linux-cp mapping (no longer `NoMapper`). An instance without a pair is skipped with the DryRun warning
`ha.vrrp-keepalived-no-lcp`; one in a non-default VRF with `ha.vrrp-keepalived-vrf`. With `VRX_TEST_PREFIX` the stage uses
`TestPaths(prefix, $VRX_KEEPALIVED_BIN_DIR, $VRX_KEEPALIVED_NETNS)` and a pidfile controller
(`<conf dir>/keepalived.pid`). No secret resolver is passed (no API→agent secret channel; the D-086 stand-ins are not
contract fields yet). No change to the renderer package itself.

contract fields yet). The renderer exposes the read-only daemon readiness check used by the stage.

## Shared-host engine gates (S-rva-agent-gates, RV-A R4 M1/M2)

Both ha.vrrp engines are gated in `internal/subsystems/vrrp.go` (`vrrpVPPGate`, `keepalivedGate`), resolved once at
registration and handed to the projection through `subsystems.VrrpEnv()`. The owner name is never used: `VRX_OWNER`
defaults to `vrx`, which is also the tools/app agent on the shared host.

| variable | values | unset | effect when off |
|---|---|---|---|
| `VRX_VRRP_VPP` | `on` \| `off` | on only with `VRX_VPP_ID_RANGE=all` | the vrrp VPP descriptors and vrrp.meta are not registered (nothing dumps or writes `vrrp_vr_*`); each `engine: vpp` instance → WARNING `ha.vrrp-vpp-disabled` at `/ha/vrrp/<n>/engine` |
| `VRX_KEEPALIVED` | `on` \| `off` | on with `VRX_TEST_PREFIX` (TestPaths + pidfile controller) or `VRX_VPP_ID_RANGE=all` (ProductPaths + `keepalived.service`) | the stage is not registered; each `engine: keepalived` instance → WARNING `ha.vrrp-keepalived-disabled` |

`VRX_KEEPALIVED=on` without a slot prefix and without `VRX_VPP_ID_RANGE=all` is refused with a start-up warning (the
engine stays off): only the product agent on a box of its own ever writes `/etc/keepalived` or reloads the unit.
`VRX_VRRP_VPP=on` is the explicit opt-in of a manager window (VPP idle, V22b). `tools/app` sets both to `off`.

Turning an engine from on to off does **not** remove what it already applied: with the VPP engine off the vrrp
descriptors are not registered, so VRs created earlier stay in VPP (and vrrp-meta keeps their names) until the engine is
on again and a commit without them deletes them — or the product owner removes them by hand. Likewise a keepalived.conf
written earlier stays in place. Switch an engine off only on an agent with no VRs of that engine.

## Daemon readiness and slot lifetime

TD-13 Validator: `keepalived -t` on a staged copy with `dynamic_interfaces`; Stage = daemon.

In slot mode, the harness owns the daemon's lifetime and starts it before DryRun or Apply, using the slot namespace and pidfile. The stage reads daemon readiness without starting or signaling it. If no process is running, it reports: `keepalived is not running for this agent; slot harnesses start it`. Apply checks readiness before writing configuration or requesting reload. Whether the product agent may start a daemon remains the owner decision in `docs/decisions/PENDING-agent-privileges.md`.
