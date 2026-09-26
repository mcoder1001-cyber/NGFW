# Anti-spoofing and policy routing: uRPF, ADL, PBR, Auto-SDL

**Screens:** *Routing → Policy routing* (`/routing/pbr`) and *Firewall → ADL / Auto-SDL* (`/firewall/adl`); uRPF and ADL
also appear in the interface drawer (*Interfaces → interface → Security*). **REST:** the generic configuration routes
(`/api/v1/config/interfaces/<if>/urpf`, `…/adl`, `/api/v1/config/routing/pbr`, `/api/v1/config/services/autoSdl`) and
`GET /api/v1/state/pbr` (live). **CLI:** `vrx configure set …` / `merge …` on the same paths, `vrx show drift`
(`docs/user/cli/reference.md`). **VPP:** `urpf`, `adl`, `abf` and `auto_sdl` plugins.

| feature | what it does | configuration | absent / off |
|---|---|---|---|
| **uRPF** (unicast reverse-path forwarding) | drops a packet whose source address is not routable (`loose`) or not routable back through the interface it arrived on (`strict`) | `interfaces.<if>.urpf { ipv4?: loose\|strict, ipv6?: loose\|strict, direction: rx\|tx }` | no mode for a family = no check |
| **ADL** (allow/deny list) | on a physical port, drops IPv4/IPv6 packets whose source is not in an *allow-list VRF* | `interfaces.<if>.adl { ipv4, ipv6, allowVrf, defaultAllow }` | `ipv4` and `ipv6` false |
| **PBR** (policy-based routing, VPP ABF) | packets a policy's ACL permits are forwarded over the policy's paths instead of the routing table | `routing.pbr { policies.<name> { acl, priority, paths[] }, attachments[] { policy, interface, family } }` | no `pbr` |
| **Auto-SDL** | the VPP host stack denies new TCP sessions from a source that exceeded a threshold, for a while | `services.autoSdl { enabled, threshold, removeTimeoutSec }` | not enabled |

Nothing reaches the data plane until you **Commit** in the pending-change bar (review the diff there).

## Example: source-based routing to a second uplink, strict uRPF on the WAN

Two uplinks: `GigabitEthernet0/8/0` (default route, main table) and `GigabitEthernet0/9/0` in VRF `wan2` (its own
default route). Hosts of `10.0.1.0/24` behind `loop0`/LAN must leave through the second uplink; everything else uses the
main table. The WAN interface drops spoofed sources.

```jsonc
{
  "vrfs": { "wan2": { "id": 10 } },
  "interfaces": {
    "GigabitEthernet0/8/0": { "enabled": true, "ipv4": ["198.51.100.2/30"], "urpf": { "ipv4": "strict" } },
    "GigabitEthernet0/9/0": { "enabled": true, "vrf": "wan2", "ipv4": ["203.0.113.2/30"] },
    "GigabitEthernet0/10/0": { "enabled": true, "ipv4": ["10.0.1.1/24"] }
  },
  "routing": {
    "static": [
      { "prefix": "0.0.0.0/0", "nextHops": [{ "address": "198.51.100.1" }] },
      { "prefix": "0.0.0.0/0", "vrf": "wan2", "nextHops": [{ "address": "203.0.113.1", "interface": "GigabitEthernet0/9/0" }] }
    ],
    "pbr": {
      "policies": {
        "lan-b-via-wan2": { "acl": "from-lan-b", "priority": 10, "paths": [{ "address": "203.0.113.1", "interface": "GigabitEthernet0/9/0" }] }
      },
      "attachments": [{ "policy": "lan-b-via-wan2", "interface": "GigabitEthernet0/10/0", "family": "ipv4" }]
    }
  },
  "acl": { "lists": { "from-lan-b": { "rules": [
    { "sequence": 10, "action": "permit", "ipVersion": "ipv4", "source": { "kind": "prefix", "prefix": "10.0.1.0/24" } }
  ] } } }
}
```

> **Until F-acl is merged** the agent does not apply `acl.lists` (the `acl` domain is not implemented yet), so no
> owner ACL exists in VPP and a commit with a PBR policy fails with `agent.dependency-missing` at the policy's
> pointer. After F-acl, the ACL is created with the same commit (ACL before policy, policy before ACL on delete).

Instead of a next hop, a path can hand the packet to another VRF's routing table: `{ "vrf": "wan2" }` (no address, no
interface) looks the destination up in `wan2`, so the policy follows whatever routes `wan2` has.

Rules the configuration is checked against (400 problem+json with a JSON pointer on violation):

| rule | pointer |
|---|---|
| a policy's ACL exists in `acl.lists` | `/routing/pbr/policies/<name>/acl` |
| path VRFs and interfaces exist; `vrf` only on a path without an interface | `/routing/pbr/policies/<name>/paths/<i>/…` |
| a policy does not mix IPv4 and IPv6 next hops; an attachment's family matches its policy's next hops | `…/paths`, `/routing/pbr/attachments/<i>/family` |
| an attachment names an existing policy and interface, once per (policy, interface, family) | `/routing/pbr/attachments/<i>/…` |
| the ADL allow-list VRF exists; it is required when a family is checked | `/interfaces/<if>/adl/allowVrf` |
| policy names: letters, digits, `_ . -`, at most 63 characters (no `#`) | `/routing/pbr/policies/<name>` |

The agent adds these at commit time (DryRun): `defaultAllow: false` is refused (VPP 26.06 cannot filter non-IP frames,
see below); strict uRPF on an interface that is one of several ECMP egress interfaces of a static route is a **warning**
(replies may arrive on another member and be dropped — use loose mode on ECMP uplinks).

## Policy routing screen

![Policy routing, policies and attachments](../../status/tasks/F-rpf-adl-pbr-screens/pbr-list-en.png)

- **Policies**: name, ACL, priority (lower first among the policies on one interface), paths, number of attachments, and
  the live **status** from `GET /api/v1/state/pbr`: *in sync* (VPP has the committed policy), *drift* (VPP differs),
  *not applied* (committed but not in VPP — e.g. its ACL does not exist in VPP yet), *unmanaged* (in VPP only; the next
  commit deletes it). A pending mark says the candidate adds, changes or removes it.
- **Add policy / edit**: the form is generated from the configuration schema; the ACL and path VRF are pickers filled
  from the candidate, the path editor adds rows. **Save to candidate**, then commit.
- **Attachments**: a policy acts only on the interfaces it is attached to (per address family).

![Policy editor](../../status/tasks/F-rpf-adl-pbr-screens/pbr-editor-en.png)

Per-policy ACL hit counters are not shown in this release (they need a counters RPC in the agent and the ACL plugin's
statistics, which only the globals owner switches on).

## ADL / Auto-SDL screen and the interface drawer

![ADL / Auto-SDL](../../status/tasks/F-rpf-adl-pbr-screens/adl-page-en.png)

The table shows uRPF and ADL of every interface of the candidate; the pencil opens both groups for one interface. The same
fields are in the interface drawer's **Security** group. An object that checks nothing is removed (absent = off).

- **ADL** works on physical ports (DPDK) only — VPP checks the source in the `device-input` path. The allow-list VRF must
  hold the allowed source prefixes as *local* (receive) entries. Non-IP frames (ARP, …) always pass: VPP 26.06's
  non-IP allow-list node is a stub, so `defaultAllow: false` is refused. The allow-list binding cannot be read back from
  VPP: `Retrieve` and the drift view show only whether ADL is on (the agent marks the other leaves `agent.write-only`).
- **Auto-SDL** is a VPP-global setting: only the agent that owns the globals applies it (a test agent reports it as not
  applied), and VPP needs the session layer's SDL backend (`session { enable rt-backend sdl }` in startup.conf) — without
  it VPP answers "feature disabled". Auto-SDL cannot be read back either.

The screens are available in Persian (right-to-left):

![Policy routing, Persian](../../status/tasks/F-rpf-adl-pbr-screens/pbr-list-fa-rtl.png)

## The same with REST and the CLI

```sh
# configuration (candidate), then commit
curl -s -X PUT -H "authorization: Bearer $T" -H 'content-type: application/json' \
  http://127.0.0.1:3000/api/v1/config/routing/pbr \
  -d '{"policies":{"lan-b-via-wan2":{"acl":"from-lan-b","priority":10,"paths":[{"address":"203.0.113.1","interface":"GigabitEthernet0/9/0"}]}},
       "attachments":[{"policy":"lan-b-via-wan2","interface":"GigabitEthernet0/10/0","family":"ipv4"}]}'
curl -s -X PATCH -H "authorization: Bearer $T" -H 'content-type: application/merge-patch+json' \
  http://127.0.0.1:3000/api/v1/config/interfaces/GigabitEthernet0~18~10 -d '{"urpf":{"ipv4":"strict"}}'
curl -s -X POST -H "authorization: Bearer $T" http://127.0.0.1:3000/api/v1/config/commit
# live policy routing view (attachments paged: ?page=&pageSize=)
curl -s -H "authorization: Bearer $T" http://127.0.0.1:3000/api/v1/state/pbr
```

```text
vrx configure merge routing pbr '{"policies":{"lan-b-via-wan2":{"acl":"from-lan-b","priority":10,"paths":[{"address":"203.0.113.1","interface":"GigabitEthernet0/9/0"}]}}}'
vrx configure set interfaces GigabitEthernet0/8/0 urpf ipv4 strict
vrx configure set interfaces GigabitEthernet0/10/0 adl '{"ipv4":true,"allowVrf":"allowed-sources"}'
vrx configure set services autoSdl '{"enabled":true,"threshold":5,"removeTimeoutSec":300}'
vrx commit
vrx show drift
```

VPP's own view (read-only, on the router): `vppctl show abf policy`, `vppctl show abf attach <if>`,
`vppctl show interface features <if>` (`ip4-rx-urpf-strict`, `abf-input-ip4`, `adl-input` on `device-input`),
`vppctl show auto-sdl`.
