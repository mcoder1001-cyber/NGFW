# Host ACL — protecting the box itself (local-in, management plane)

**Screen:** *Firewall → Host ACL* (`/firewall/host-acl`: tabs *Lists*, *Attachments*, *Settings*, *Rendered*).
**REST:** the generic configuration routes under `/api/v1/config/acl/host/<list>`, `/api/v1/config/acl/hostAttachments`,
`/api/v1/config/acl/hostSettings`, and `GET /api/v1/state/host-acl` (the rendered firewall with per-rule counters).
**CLI:** `vrx configure set|merge|delete acl host …`, `… acl hostAttachments …`, `… acl hostSettings …`, then `commit`;
`vrx show configuration acl` (`docs/user/cli/reference.md`). There is no `show` command for the rendered table yet — use
`GET /api/v1/state/host-acl`.

Host ACLs filter traffic **to and from the appliance's own host stack**: SSH, the HTTPS UI/API, SNMP, BGP/OSPF sessions
punted to Linux, DNS/NTP the box serves. They are not the data-plane ACLs of the VPP interfaces (*Firewall → ACL*); they
are rendered into the Linux kernel's nftables as one table, `table inet vrx`, which the box owns completely: every
commit replaces the whole table in one atomic step, and nothing else in the kernel ruleset is touched (the static base
policy the package installs, and anything else, stays).

## Building blocks

| part | what it is |
|---|---|
| **host list** (`acl.host.<name>`) | an ordered list of rules. Each rule: `sequence` (order), `action` accept / drop / reject, `ipVersion` ipv4 / ipv6 / any, `source` and `destination` (any, a prefix, or an address object/group), `service` (any, a service object/group, or inline protocol + ports), optional `interface` (Linux name, e.g. `ens192`), `log`, `enabled`, `description` |
| **attachment** (`acl.hostAttachments[]`) | puts a list on a hook: `input` (to the box), `output` (from the box) or `forward` (through the Linux stack), with a `priority` −500…500 (lower runs first). A list that is not attached does nothing |
| **settings** (`acl.hostSettings`) | `defaultInput` (accept / drop: what happens to input traffic no rule matched), `allowIcmp`, and the **anti-lockout** rule |

Every chain starts with *established/related → accept* (replies to connections that were allowed are never cut), then
loopback, then ICMP (if allowed; IPv6 neighbour discovery always), then — on input — the anti-lockout rule, then your
rules in `sequence` order. Every rule has a counter; rules with *log* write `vrx:<list>:<sequence>` to the kernel log.

## Default policy

Out of the box nothing is filtered: without an attachment there is no table at all, and an attached input chain has the
policy **accept** unless you set `defaultInput: drop`. With `drop`, only traffic a rule accepts reaches the box — build
the accept rules first, then switch the policy.

## Anti-lockout

A commit can never cut the management connection you are using, in two layers:

1. **The anti-lockout rule** (on by default) accepts new TCP connections to the management ports (`22` and `443` unless
   you change `antiLockout.ports`) from `antiLockout.sources` (empty = anywhere) on `antiLockout.interfaces` (empty =
   any interface), at the top of every input chain — before any of your rules.
2. **The commit check.** If you turn the rule off, *Validate* / *Commit* simulate a new management connection from each
   source, on each interface, to each port through your input chains. If any of them would be dropped, the commit is
   refused with a **400** validation problem that points at the rule that drops it (or at `defaultInput`), e.g.
   `/acl/host/mgmt-in/rules/1: management TCP 443 from 10.0.0.0/24 on ens192 would be dropped by this rule …`.
   The check is conservative: an accept rule must cover the whole source; two accepts that only together cover it are
   reported — widen one of them.

With the rule on, a rule that *would* have dropped management traffic gets a warning (`acl.host-anti-lockout-shadow`):
the rule still works for everybody else. The *Settings* tab shows the anti-lockout state as a banner.

**Narrow it to your management network.** With the defaults, SSH and HTTPS stay open from anywhere; set `sources` (and
`interfaces`) to the addresses you manage the box from.

## Example: SSH only from 10.0.0.0/24

Allow SSH and HTTPS from the management network `10.0.0.0/24` on `ens192`, keep SNMP from there, drop SSH/HTTPS from
everyone else:

```json
{
  "acl": {
    "host": {
      "mgmt-in": {
        "description": "management plane",
        "rules": [
          { "sequence": 10, "action": "accept", "source": { "kind": "prefix", "prefix": "10.0.0.0/24" },
            "service": { "kind": "inline", "spec": { "protocol": "tcp", "destinationPorts": ["22", "443"] } } },
          { "sequence": 20, "action": "accept", "source": { "kind": "prefix", "prefix": "10.0.0.0/24" },
            "service": { "kind": "inline", "spec": { "protocol": "udp", "destinationPorts": ["161"] } } },
          { "sequence": 30, "action": "drop", "log": true,
            "service": { "kind": "inline", "spec": { "protocol": "tcp", "destinationPorts": ["22", "443"] } } }
        ]
      }
    },
    "hostAttachments": [{ "list": "mgmt-in", "chain": "input", "priority": 0 }],
    "hostSettings": { "antiLockout": { "sources": ["10.0.0.0/24"], "interfaces": ["ens192"] } }
  }
}
```

CLI (configuration mode):

```text
merge acl {"host": {"mgmt-in": {"rules": [ … as above … ]}}, "hostAttachments": [{"list": "mgmt-in", "chain": "input"}]}
set acl hostSettings antiLockout sources 10.0.0.0/24
set acl hostSettings antiLockout interfaces ens192
commit
```

What the box renders (from `GET /api/v1/state/host-acl`, *Rendered* tab):

```text
chain in_mgmt-in { type filter hook input priority 0; policy accept;
  ct state established,related counter accept
  iif "lo" counter accept
  meta l4proto icmp counter accept
  meta l4proto ipv6-icmp counter accept
  iifname "ens192" ip saddr 10.0.0.0/24 tcp dport { 22, 443 } counter accept        # anti-lockout
  ip saddr 10.0.0.0/24 tcp dport { 22, 443 } counter accept                         # rule 10
  ip saddr 10.0.0.0/24 udp dport 161 counter accept                                 # rule 20
  tcp dport { 22, 443 } counter log prefix "vrx:mgmt-in:30 " drop                   # rule 30
}
```

Rule 30 drops SSH/HTTPS from everywhere else; the counters in the *Lists* tab show how many packets each rule matched.

## Objects

Sources and destinations can name address objects and groups (*Firewall → Objects*); each becomes a named set in the
table (`a4_<object>` for IPv4, `a6_<object>` for IPv6), so editing the object and committing updates every rule that uses
it. FQDN objects use the addresses the box last resolved; one that has not resolved yet matches nothing (warning).
Services can be service objects/groups; a service with several protocols becomes one rule per protocol.

## Troubleshooting

- *Rendered* tab: **present** = the table exists in the kernel; **in sync** = it is exactly what the last commit rendered.
  If someone changed or deleted the table by hand, the next commit (or an agent restart) puts it back.
- Rules of a list that is not attached are validated but not rendered.
- `reject` answers with an ICMP *port unreachable* (nftables' default); `drop` stays silent.
- Descriptions are kept in the configuration only; they never reach the kernel ruleset.
