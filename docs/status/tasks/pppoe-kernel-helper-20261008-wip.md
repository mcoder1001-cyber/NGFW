# PPP kernel carrier helper checkpoint

Branch: `codex/pppoe-kernel-helper-20261008`. Base: `db7eb9d4`.
Owned: `scripts/pppoe-kernel-carrier.py`, its script test, source assets under
`scripts/pppoe-carrier-assets`, and this receipt. No deploy file or host changed.

This is opt-in source, not activated functionality or acceptance. Independent R4
review is in progress. Twelve fake-executor tests pass (0.002 seconds):
`python3 scripts/tests/pppoe-kernel-carrier.py -v`. Python compilation and diff
whitespace checks passed. No native namespace/route/service tests or CI ran.

## Owner-controller interface

Proposed reviewed installation: `/usr/libexec/ngfw/pppoe-kernel-carrier`.
Root-only, fixed executables and argument vectors, no shell or new sudo/broker
permission. Parent must invoke through the approved existing privilege boundary.

Token: `ngp-` + first 12 lowercase hex SHA256(UTF8(owner + NUL + logical)). Full
owner/logical comparisons also prevent truncated-hash collision adoption.

- `provision OWNER LOGICAL --spec-json JSON`: exact fields owner, logical,
  parent, mtu, host4, peer4, host6, peer6. MTU1280..1492; distinct usable same
  /30 IPv4 or /126 IPv6 host/peer. Spec immutable. Known transit overlap rejected.
- `list OWNER`/`inventory OWNER`: actual verified receipts, immutable spec,
  generation, boot ID, namespace dev/inode, current links/addresses/rules/firewall.
- `prepare TOKEN GEN --physical-mac MAC --local4 CIDR --peer4 IP --local6 CIDR
  --peer6 IP`: endpoint match to spec; optional paired raw-index/transit-index;
  fixed TAP names/kinds/indices, empty or owned alias, no foreign links. Alias
  becomes TOKEN:GEN:NAME; raw MAC set; raw IPv6 disabled and IPv4 addresses removed.
- `configure TOKEN GEN`: bound TAPs plus up PPP required; namespace-only routes,
  ingress lookups before relocated local rule, scoped IPv6 control exceptions,
  default-deny nft permitting only transit/PPP data pair and control/PMTU. No NAT.
- `withdraw TOKEN GEN`: persist configured=false before closing forwarding.
- `delete TOKEN GEN`: additionally refuses any link other than lo; parent must
  remove VPP TAPs and stop PPP first. `inspect TOKEN GEN` returns live evidence.
- `run TOKEN`: verify ledger/namespace/prepare binding then exec fixed
  `pppd call carrier nodetach unit 0`. Flock descriptor closes on exec while the
  namespace fd remains inherited by nsenter.

Root-controlled no-symlink ancestor directories protect `/run/ngfw-pppoe-carrier`
and `/run/netns`. Boot+generation+device/inode and NS_GET_NSTYPE verify netns.
Commands use pinned FD, not reopened namespace paths. Crash between namespace
creation and ledger save leaves an orphan refused until explicit recovery.
`configured` is an operation-completion flag, NEVER forwarding readiness.

## Packaging and remaining integration

Fixed unit source `ngfw-pppoe-carrier@TOKEN.service` binds per-session
`/var/lib/ngfw/pppoe-carrier/TOKEN/ppp` read-only onto private `/etc/ppp`.
Renderer supplies peers/carrier, options, credentials and hook fragments together
with four fixed dispatchers. Private /run prevents cross-session ppp0 pidfiles;
only ledger, namespace handles and dedicated hook state are bound back. No
writable /etc or generated systemd files. No Install/WantedBy and no package
activation in this checkpoint.

Parent owns VPP exclusivity/preflight, deterministic unique transit allocation,
normal logical-interface ACL/NAT ownership, TAP/xconnect rollback, private renderer
switch, hook session generations and actual joint kernel/VPP readback/readiness.
Native acceptance must exercise ULA neighbor discovery, DHCPv6/RA, NAT replies
before local delivery, PMTU, failed steps, replacement, restart and multi-session.

R4 early findings addressed: transit ND accepts owned-peer ULA sources;
namespace-origin PMTU errors explicitly allowed; transit allocation is caller
supplied immutable spec rather than same fixed address pair for every session.

Next: rerun focused script test after independent review fixes. Full CI remains
deferred by owner; no source completion or lab-only remainder claim is made.
