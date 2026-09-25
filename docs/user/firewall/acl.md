# Access lists — L3/L4 ACLs, MACIP ACLs, attachments, hit counters

**Screen:** *Firewall → ACL* (`/firewall/acl`, tabs `?tab=lists|rules|attachments|macip`, a list's rule editor at
`?tab=rules&list=<name>`). **REST:** the generic configuration routes under `/api/v1/config/acl/…`, plus
`GET /api/v1/state/acl/lists`, `GET /api/v1/state/acl/lists/{name}/rules`, `GET /api/v1/state/acl/attachments`,
`POST /api/v1/actions/acl/import`, `GET /api/v1/actions/acl/export.csv`, `POST /api/v1/actions/acl/lists/{name}/rules/bulk`.
**CLI:** `vrx set|merge|delete acl …`, `vrx show configuration acl`, `vrx commit` (`docs/user/cli/reference.md`); the
state and CSV routes have no dedicated CLI command yet — use REST (below).

An access list is an ordered list of rules evaluated by **sequence** (lowest first, first match wins). The box renders
each list into one VPP ACL (plugin `acl`) and binds it to interfaces in the direction you choose.

| part | what it is |
|---|---|
| `acl.lists.<name>` | L3/L4 rules: `action` `permit`, `deny` or `reflect`; `ipVersion` `ipv4`, `ipv6` or `any`; `source` / `destination` = `any`, a prefix, or an address object / address group by name; `service` = `any`, a service object / group by name, or an inline protocol with ports / ICMP type and code; optional `schedule` (a schedule object); `enabled`; `description` |
| `acl.macip.<name>` | L2 **MACIP** rules: `permit` / `deny` frames by **source MAC** (with a mask) and optional **source prefix** (omitted = any address, IPv4 and IPv6) |
| `acl.attachments[]` | a list on an **interface** or a **zone** (every interface of `objects.zones.<zone>`), direction `in` or `out`, and a `sequence` that orders several lists on the same interface and direction |
| `acl.macipAttachments[]` | one MACIP list per interface, inbound |

`acl.host` / `acl.hostAttachments` (the box's own management-plane firewall, nftables) are a different feature
(*host ACL*); this page is about the data plane.

## Stateless vs reflect

- `permit` / `deny` are **stateless**: each packet is matched on its own. A permit for `lan → web:443` does not let the
  replies back in on another interface's input list — write a rule for the return direction too.
- `reflect` = **permit and remember the 5-tuple session**: the return traffic of a session opened by a reflect rule is
  permitted on the same interface in the other direction without a rule of its own (stateful ACL). Sessions live in the
  acl plugin's connection table (size and timeouts are VPP defaults in this release).
- **Implicit default:** VPP **denies** everything that no rule of the interface's lists matches — on an interface that has
  at least one list in that direction. The box adds no hidden rule; end a list with an explicit `permit` if you want
  "everything else allowed". Non-IP frames (ARP) are not filtered by L3/L4 lists.

## How a rule becomes VPP rules

One configuration rule expands to **sources × destinations × services per address family**:

- an address group becomes its prefixes (ranges become the smallest prefix set, groups are flattened, siblings merged);
- `ipVersion: any` produces IPv4 and IPv6 VPP rules — only for the families that both sides have (a rule with an IPv4
  prefix is IPv4 only; `icmp` exists only in IPv4, `icmp6` only in IPv6);
- `tcp-udp` becomes a TCP and a UDP rule; several port ranges become one rule each;
- **disabled** rules and rules whose **schedule is not active now** are left out (the box re-checks schedules every 60 s
  and re-applies when one turns on or off; schedules use the box's time zone);
- an **FQDN** object uses the addresses the box resolved last; a change of the answer re-applies the lists that use it
  (a name that never resolved matches nothing — a warning at validation);
- limits: **10 000** VPP rules for one configuration rule, **100 000** for one list — above that the commit is refused at
  the rule's (or list's) pointer.

The rule editor shows, per configuration rule, how many VPP rules it became and its status (*applied*, *disabled*,
*schedule inactive*, *empty*).

`log: true` is accepted but **not honoured**: VPP's acl plugin cannot log per rule. Validation reports it as a warning
(`acl.log-unsupported`); use the hit counters instead.

## Hit counters (caveat)

Each configuration rule shows **packets / bytes**: the sum over the VPP rules it expanded to, from VPP's stats segment
(`/acl/<index>/matches`). They count only while the VPP-wide switch *acl.stats-enable* is on. That switch belongs to the
product box's agent (the "globals owner"); test agents never set it. VPP has no way to switch it off from the API and no
getter (tracked as V7) — the box reads it from `show acl-plugin tables mask`. When it is off, the screen says *counters
unavailable* with the reason. Counters restart at 0 when a list is re-created (e.g. after a VPP restart), not when it is
changed in place. There are no per-interface ACL counters in VPP.

The screen refreshes the counters of the rows you see every 30 s and on *Refresh* (only the stats segment is read; a
100 000-rule list is never dumped for a refresh).

## The rule editor at scale

The editor never loads a whole list: it pages on the server (up to 1 000 rows per page, sequence order, quick search
over sequence, action, addresses, service, schedule and description) and asks the agent for the counters of exactly
the visible rules. Bulk actions change the candidate in one edit: enable, disable, delete, **move to sequence** (the
selected rules get the target sequence and the following ones, rules already there move up), **renumber** (10, 20, …).
Dragging a row onto another moves it in front of that row. Nothing reaches the data plane until you **Commit**.

## CSV import / export

```
sequence,action,enabled,ipVersion,source,destination,service,schedule,log,description
10,permit,true,any,10.3.1.0/24,object:web-servers,tcp:80|443,office-hours,false,"web, public"
20,reflect,true,ipv4,any,any,tcp:22;src=1024-65535,,false,
30,deny,true,any,any,any,icmp:8,,false,no ping
40,permit,true,ipv6,any,any,icmp6:128/0,,false,
50,permit,true,any,object:lan,any,proto:47,,false,GRE
```

- `source` / `destination`: `any`, a prefix (a bare address is a host prefix) or `object:<name>`.
- `service`: `any`, `object:<name>`, `<tcp|udp|tcp-udp|sctp>[:<ports>][;src=<ports>][;flags=<value>/<mask>]` (ports
  `80|443|8000-8080`), `icmp[:<type>[/<code>]]`, `icmp6[:<type>[/<code>]]`, `proto:<number>`.
- `enabled` / `log`: `true` / `false` (empty = the default), `ipVersion` empty = `any`; only `sequence` and `action` are required.
- Text is RFC 4180 CSV (quotes for commas, quotes and line breaks). A cell starting with `= + - @` is exported with a
  leading `'` so spreadsheets do not run it as a formula; import strips it again.
- **Import** is a dry run first: it shows the rows, the errors with line and column, the object names the candidate does
  not define (the commit would refuse them) and a preview; *Import* then writes the rules into the candidate (replace the
  list, or append — the sequences must not collide). At most 100 000 rows; the file is read as a stream.

## Examples

A web list on the LAN zone, echo only to the web servers during office hours, everything else of IPv4 denied, and a
MACIP guard on the WAN port:

```json
{
  "acl": {
    "lists": {
      "lan-in": {
        "description": "LAN ingress",
        "rules": [
          { "sequence": 10, "action": "permit", "destination": { "kind": "object", "name": "web-servers" },
            "service": { "kind": "object", "name": "https" }, "schedule": "office-hours" },
          { "sequence": 20, "action": "reflect", "destination": { "kind": "object", "name": "web-servers" },
            "service": { "kind": "inline", "spec": { "protocol": "tcp", "destinationPorts": ["80"] } } },
          { "sequence": 30, "action": "deny", "ipVersion": "ipv4" }
        ]
      }
    },
    "macip": {
      "wan-l2": { "rules": [ { "sequence": 10, "action": "permit", "sourceMac": "02:00:00:00:00:01", "sourcePrefix": "10.3.2.2/32" } ] }
    },
    "attachments": [ { "list": "lan-in", "target": { "kind": "zone", "zone": "lan" }, "direction": "in", "sequence": 10 } ],
    "macipAttachments": [ { "list": "wan-l2", "interface": "host-w3w0" } ]
  }
}
```

### The same with REST

```
B=https://vrx-a/api/v1; T=<access token>
curl -s -X PATCH -H "authorization: Bearer $T" -H 'content-type: application/merge-patch+json' "$B/config/acl" -d @acl.json
curl -s -X POST -H "authorization: Bearer $T" "$B/config/commit?comment=acl"
curl -s -H "authorization: Bearer $T" "$B/state/acl/lists"                                   # lists + live status
curl -s -H "authorization: Bearer $T" "$B/state/acl/lists/lan-in/rules?page=1&pageSize=100&source=running"  # counters
curl -s -H "authorization: Bearer $T" "$B/state/acl/attachments"                             # bindings as VPP holds them
curl -s -X POST -H "authorization: Bearer $T" -H 'content-type: text/csv' --data-binary @rules.csv \
     "$B/actions/acl/import?list=lan-in&mode=append"                                          # dry run
curl -s -X POST -H "authorization: Bearer $T" -H 'content-type: text/csv' --data-binary @rules.csv \
     "$B/actions/acl/import?list=lan-in&mode=append&dryRun=false"                             # into the candidate
curl -s -H "authorization: Bearer $T" "$B/actions/acl/export.csv?list=lan-in" -o lan-in.csv
curl -s -X POST -H "authorization: Bearer $T" -H 'content-type: application/json' \
     "$B/actions/acl/lists/lan-in/rules/bulk" -d '{"op":"move","sequences":[30],"to":5}'
curl -s -X POST -H "authorization: Bearer $T" "$B/config/rollback/1?comment=undo"
```

### The same with the CLI

```
vrx merge acl lists '{"lan-in":{"rules":[{"sequence":30,"action":"deny","ipVersion":"ipv4"}]}}'
vrx merge acl '{"attachments":[{"list":"lan-in","target":{"kind":"zone","zone":"lan"},"direction":"in","sequence":10}]}'
vrx set acl lists lan-in rules 0 enabled false
vrx show configuration acl
vrx validate
vrx commit comment "acl"
vrx show drift
```

## What happens on the box

- Each list is one VPP ACL tagged `<owner>:<name>`; updates replace it **in place** (the index stays, bindings and
  policy-based routing that use it stay valid). Validation errors from the expansion point at the rule
  (`/acl/lists/<name>/rules/<i>`), e.g. a rule that names an empty address group:
  `400 … "pointer":"/acl/lists/lan-in/rules/4/destination/name","message":"address group 'none' has no members; the rule would match nothing"`.
- An interface's list is shared: ACLs of **other owners** on the same interface are kept, first, in their order; the box
  only adds and removes its own (`GET /state/acl/attachments` shows both, the foreign ones marked).
- After an agent restart, or when VPP lost the lists, the agent rebuilds them from the stored configuration within
  seconds (measured on the lab host: 0.2 s after the agent start).
- `GET /api/v1/state/drift` compares the running configuration with what VPP holds; a list changed behind the box's back
  shows up there as a difference.
- The equivalent VPP commands (read-only, for support): `vppctl show acl-plugin acl`, `show acl-plugin interface`,
  `show acl-plugin macip acl`, `show acl-plugin macip interface`, `show acl-plugin tables mask` (counters switch).

Access-list based **ADL** (allow-list) and **Auto-SDL** are configured with the interface security settings and the
services (feature *uRPF / ADL / PBR*); policy-based routing (ABF) matches traffic with these lists by name.
