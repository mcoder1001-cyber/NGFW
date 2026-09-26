# nftables renderer — host firewall ↔ `table inet vrx`

Package `apps/agent/internal/renderers/nftables` (F-host-acl-nftables, WBS D5.3, D-057: the single owner of the host
firewall; F-hardening-lite consumes it). nftables 1.1.6 (`/usr/sbin/nft`). One table, `table inet vrx` (test slots
`vrx_<prefix>`), family `inet` (IPv4 and IPv6 in one table). The renderer never flushes the ruleset and never touches
another table: P10's static base policy (its own table) and anything else loaded on the box stay as they are.

## Mapping

| configuration | nftables |
|---|---|
| nothing under `acl.host*` | no table (a present one is deleted) |
| `acl.host`, `acl.hostSettings` without any enabled attachment | no table (the configuration is still applied and retrieved) |
| `acl.hostAttachments[] {list, chain, priority}` (enabled) | one base chain `in_<list>` / `out_<list>` / `fwd_<list>`: `type filter hook input\|output\|forward priority <priority>` |
| chain policy | input chains: `acl.hostSettings.defaultInput` (`accept` default, or `drop`); output and forward: `accept` |
| — (every chain, first) | `ct state established,related counter accept` |
| — (input / output) | `iif "lo" counter accept` / `oif "lo" counter accept` |
| `acl.hostSettings.allowIcmp` (input; default true) | `meta l4proto icmp counter accept` + `meta l4proto ipv6-icmp counter accept`; false: only `icmpv6 type { nd-router-solicit, nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert } counter accept` |
| `acl.hostSettings.antiLockout {enabled, sources[], interfaces[], ports[]}` (input; default on, any source, any interface, 22 + 443) | `[iifname { … }] [ip saddr { … }] tcp dport { … } counter accept`, one rule per source family, after the preamble and before every list rule |
| `acl.host.<list>.rules[]` (enabled, in `sequence` order) | one or more rules per configured rule (below); disabled rules render nothing |
| `rule.action` accept / drop / reject | `accept` / `drop` / `reject` (nftables' default reject: `icmpx port-unreachable`) |
| `rule.log` | `log prefix "vrx:<list>:<sequence> "` before the verdict |
| — (every rule) | `counter` (packets/bytes → `HostAclState`, `GET /api/v1/state/host-acl`) and `comment "vrx:<id>/<n>:<hash8>"` (identity) |
| `rule.interface` | `iifname "<if>"` (input, forward) / `oifname "<if>"` (output) |
| `rule.source` / `destination` `{kind: prefix}` | `ip saddr <prefix>` / `ip6 daddr <prefix>` (masked) |
| `{kind: object}` (address object or group) | named interval sets `a4_<object>` (`ipv4_addr`) and `a6_<object>` (`ipv6_addr`) holding the expansion (`objects.Expand`: aggregated, canonical; ranges → CIDR sets; FQDN → the resolver's answers) and `ip saddr @a4_<object>` / `ip6 saddr @a6_<object>` |
| `{kind: any}` | no match |
| `rule.ipVersion` | which families are rendered: an address match of a family implies it; with no address match and `ipv4`/`ipv6`, `meta nfproto ipv4\|ipv6`; `any` with no address match = one rule for both |
| `rule.service` `{kind: object}` / `{kind: inline}` | `objects.ExpandService` / `ExpandServiceSpec` → one clause per protocol: `tcp\|udp\|sctp [sport <ranges>] [dport <ranges>]` (destination ports of one protocol + source range + TCP flags merged into one set), `tcp flags & <mask> == <value>`, `icmp\|icmpv6 type <t> [code <c>]` or `meta l4proto icmp\|ipv6-icmp`, `meta l4proto <n>` (other), nothing (any) |
| a rule that expands to several families × protocol clauses | that many nftables rules, `<n>` = 0, 1, … in the comment |
| `description` (lists, rules, attachments), `tags` | not rendered (never reach the file) |

Rule identity: `vrx:<list>:<sequence>/<n>:<hash8>` for list rules, `vrx:@established|@loopback|@icmp|@anti-lockout/<n>:<hash8>`
for the fixed ones; `hash8` = the first 8 hex digits of sha256 of the rendered rule text (without the comment).

## Validation (the agent's DryRun; issues carry JSON pointers)

| rule | severity | when |
|---|---|---|
| `acl.host-anti-lockout` | error | the anti-lockout rule is **off** and a management probe (a new TCP connection to each `antiLockout.ports` from each `sources` prefix — any IPv4 / any IPv6 source when empty — on each `interfaces` — any when empty) would be dropped: at the dropping rule (`/acl/host/<list>/rules/<i>`) or at `/acl/hostSettings/defaultInput` (policy drop). The API answers the commit with **400** problem+json |
| `acl.host-anti-lockout-shadow` | warning | the anti-lockout rule is on and a rule (or the drop policy) would have dropped management traffic without it |
| `acl.host-object`, `acl.host-expansion-limit` | error | unknown/invalid object, group cycle, more than 10 000 entries (`objects.MaxEntries`) — at the rule's `…/name` or `…/spec` |
| `acl.host-fqdn-unresolved` | warning | an FQDN object without answers matches nothing yet |
| `acl.host-rule-empty` | warning | a rule whose families never meet (e.g. an IPv6-only object in an IPv4 rule) renders nothing |
| `acl.host-attachment` (priority) | error | an enabled **output** attachment at priority ≤ −200 (`NF_IP_PRI_CONNTRACK`): before conntrack `ct state established,related` never matches and a drop cuts management replies (fix round 1, H1; also the schema rule `acl.host-output-priority`) |
| `acl.host-list-name`, `acl.host-rule`, `acl.host-attachment`, `acl.host-settings` | error | defence in depth behind the schema (names, enums, ranges, the same list twice on a chain) |
| `agent.unsupported-field` | warning | `acl.lists`, `acl.macip`, `acl.attachments`, `acl.macipAttachments` (F-acl) until F-acl lands |

The probe walk follows nftables: base chains on the input hook run in priority order; in each, the first matching rule
decides; `accept` ends that chain only (the next base chain still sees the packet); `drop`/`reject` is final; no match →
the chain policy. A rule matches a probe fully, partly or not at all; an `accept` lets the probe through only when it
matches fully, a `drop`/`reject` counts when it matches even partly (conservative: split accepts that together cover a
source are reported, naming the rule to widen). Matches through FQDN-bearing objects count as partial whatever the
resolver answers, and a rule whose FQDN object has no answer yet still takes part (not rendered): the check is a pure
function of the configuration, so a config accepted at commit can never fail a later resync on DNS (fix round 1, M2).

## Apply, Retrieve, restart

- File `<state dir>/host-acl-<owner>.nft` (0600): `add table inet vrx` · `delete table inet vrx` · `table inet vrx { … }`,
  checked with `nft -c -f` on a staged copy, loaded with one `nft -f` (atomic: all or nothing).
- Store `<state dir>/host-acl-<owner>.json` (0600): the value last applied (configuration + rendering).
- Retrieve: `nft -j list table inet vrx` → sets (elements normalised to canonical prefixes), chains in evaluation order,
  rules by comment; the configuration and rule annotations come from the store entry the kernel still matches — same
  comment, same verdict, and the same rule body (sha256 of the kernel's `expr` JSON without counter values, recorded in
  the store right after each `nft -f`). The table's `flags dormant` is read too. A lost, dormant or edited table (rules
  added, removed, reordered, their verdict or body changed; chains, hooks, priorities, policies, sets) differs from the
  desired value, so the next Apply/resync (agent start, VPP reconnect) re-renders it (fix round 1, M1).
- Counters are state, not configuration: `HostAclState` (proto.md §11) → `GET /api/v1/state/host-acl`.

## Modes (`VRX_HOST_ACL_MODE`, `VRX_HOST_ACL_NETNS`)

| agent | mode | where |
|---|---|---|
| product (`VRX_OWNER=vrx`) and globals owner (D-071) | `apply` | `table inet vrx` in the agent's (root) network namespace |
| product owner with `VRX_GLOBALS_OWNER=0` (tools/app on the shared host) | `check` | `nft -c` only (fix round 1, H2) |
| test slot, `VRX_HOST_ACL_NETNS=ns-<prefix>-<name>` | `netns` | `table inet vrx_<prefix>` inside that namespace: every nft call runs on a thread that entered it with `setns(2)` |
| test slot without a namespace | `check` | `nft -c` only; nothing is loaded; Retrieve returns the stored value |
| any, `VRX_HOST_ACL_MODE=check` | `check` | a product stack on a shared host |

`VRX_HOST_ACL_MODE=apply` is refused for any owner but `vrx` and without the globals owner: a test slot, or a product stack
that is not the host's globals owner, never loads into the root namespace.

CLI equivalent: none yet (the API routes `PATCH /api/v1/config/acl/…`, `GET /api/v1/state/host-acl` are in
`docs/user/cli/reference.md` once the CLI binds them).
