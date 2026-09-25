# MPLS and SR-MPLS

VRX switches labelled packets in the data plane (VPP): static label routes (LSP hops), label ↔ IP-prefix bindings,
MPLS tunnels (head-end LSPs) and SR-MPLS policies with steering. Everything lives under `routing.mpls` of the
configuration and goes through the usual candidate → diff → commit → rollback. LDP (dynamic label distribution) is a
separate feature (F-mpls-ldp) and adds `routing.mpls.ldp` later.

Web UI: **Routing → MPLS** with the tabs *Interfaces & tables*, *Label routes*, *Tunnels*, *SR-MPLS* and *MPLS FIB*.

## The model

```jsonc
"routing": {
  "mpls": {
    "interfaces": ["TenGigabitEthernet0/0/0"],         // MPLS enabled: these interfaces accept labelled packets
    "tables": { "10": {} },                           // additional MPLS tables (table 0 is the default table, never listed)
    "labelRoutes": [                                  // local label → paths
      { "table": 0, "label": 1001, "eos": true,        // eos: the label is the bottom of the stack
        "payload": "ip4",                               // EOS payload: ip4 (default; ip6 when a next hop is IPv6), ip6, ethernet
        "paths": [{ "nextHop": "192.0.2.2", "interface": "TenGigabitEthernet0/0/0",
                    "outLabels": [2001], "weight": 1 }] }
    ],
    "ipBindings": [{ "label": 3001, "vrf": "default", "prefix": "198.51.100.0/24" }],
    "tunnels": { "lsp1": { "paths": [{ "nextHop": "192.0.2.2", "interface": "TenGigabitEthernet0/0/0",
                                       "outLabels": [4001] }], "l2Only": false } },
    "sr": {
      "policies": { "5001": { "segmentLists": [{ "labels": [16001, 16002], "weight": 1 }], "spray": false } },
      "steering": [{ "vrf": "default", "prefix": "203.0.113.0/24", "bsid": 5001, "vpnLabel": 6001 }]
    }
  }
}
```

- **Labels** are 16–1048575 everywhere (0–15 are reserved, RFC 3032). An out-label stack has at most 16 labels.
- **A path** sends the packet to `nextHop` (on `interface`) pushing `outLabels` — one label = swap, several = swap and
  push, none = pop. `interface` may also be an MPLS tunnel of `tunnels`. A path with only `vrf` pops the label and looks
  the IP packet up in that VRF (end-of-stack routes only: the termination of a simple L3 VPN). Several paths share the
  traffic by `weight`.
- **MPLS table 0** is the default MPLS table: labelled packets that arrive on an MPLS interface are looked up there.
  Additional tables (`tables`) hold label routes that a path elsewhere looks up explicitly.
- **Bindings** install a local label for an IP prefix in MPLS table 0.
- **Tunnels** are interfaces: the name is the tunnel interface's name (it must not be the name of an `interfaces`
  entry), so a label route can send packets into it.
- **SR-MPLS policies** are keyed by their binding SID (a label); each segment list is a label stack, first segment
  first. **Steering** sends the traffic to a prefix of a VRF into a policy, optionally with a VPN label under the
  segments.

Validation (HTTP 400, `application/problem+json` with a `pointer` to the offending value): label range, stack depth,
`payload` and lookup paths only on end-of-stack routes, one next-hop family per route, a label route's table is 0 or
declared, (table, label, end of stack) unique, MPLS interfaces and path interfaces exist, VRFs exist, a tunnel name is
not an interface name, a binding SID is not also a label route of table 0, a bound label is not used elsewhere in
table 0 and each prefix is bound once, steering names an existing policy and each (VRF, prefix) once, the segment lists
of a policy differ.

## Example: a static LSP transit hop and an SR-MPLS policy

```
vrx configure
merge /routing {"mpls": {"interfaces": ["TenGigabitEthernet0/0/0"], "labelRoutes": [{"label": 1001, "paths": [{"nextHop": "192.0.2.2", "interface": "TenGigabitEthernet0/0/0", "outLabels": [2001]}]}]}}
merge /routing/mpls/sr {"policies": {"5001": {"segmentLists": [{"labels": [16001, 16002]}]}}, "steering": [{"prefix": "203.0.113.0/24", "bsid": 5001}]}
set routing mpls interfaces TenGigabitEthernet0/0/1
compare
commit confirm 120 comment "LSP 1001 and SR policy 5001"
confirm
```

(`merge` is an RFC 7386 merge patch at a path; `set` on the scalar list `interfaces` appends one name; `show
configuration candidate routing mpls set` prints the section as `set` commands.)

The same with the REST API (a merge patch of `routing`; arrays are replaced whole, `null` removes a member):

```
curl -X PATCH https://vrx/api/v1/config/routing -H 'content-type: application/merge-patch+json' \
  -d '{"mpls": {"interfaces": ["TenGigabitEthernet0/0/0"],
               "labelRoutes": [{"label": 1001, "paths": [{"nextHop": "192.0.2.2", "interface": "TenGigabitEthernet0/0/0", "outLabels": [2001]}]}],
               "sr": {"policies": {"5001": {"segmentLists": [{"labels": [16001, 16002]}]}},
                      "steering": [{"prefix": "203.0.113.0/24", "bsid": 5001}]}}}'
curl -X POST 'https://vrx/api/v1/config/commit?comment=mpls'
```

Removing everything MPLS: `delete routing mpls` (CLI) or `{"mpls": null}` (merge patch), then commit. The agent removes
label routes before their tables and steering before its policy.

## Live state

- `GET /api/v1/state/routing/mpls/fib?table=0&label=&page=1&pageSize=100` — the live MPLS FIB of one table, paged by
  the agent: each entry's label, end-of-stack flag, payload and paths (next hop, interface, lookup table, out labels,
  weight). Table 0 shows every entry, including VPP's reserved labels and other features' labels; other tables must be
  this system's. Every read walks the whole table inside VPP, so the UI reads on demand (Refresh) and never polls.
- `GET /api/v1/state/routing/mpls/tunnels` — the MPLS tunnels with their VPP interface (`mpls-tunnel<N>`), index and
  paths.
- The CLI has no `show mpls` command yet; `show configuration routing mpls` shows the configuration and `show drift`
  compares it with what the agent retrieves.

## What the system does not report back

VPP 26.06 has no way to read label bindings and SR-MPLS policies/steering back. The agent applies them write-only: they
are re-applied once per VPP start (after a VPP restart the agent re-creates them) and never appear in a Retrieve, so
`show drift` lists `routing.mpls.ipBindings` and `routing.mpls.sr` as missing on the data-plane side. Interfaces, tables,
label routes and tunnels are read back and compared.

## MPLS table 0 and the globals owner

MPLS table 0 is global to the data plane. Enabling MPLS on an interface, label bindings, SR-MPLS policies and label
routes of table 0 need it. The product agent (the *globals owner*) creates it whenever the MPLS configuration needs
it; any other agent (lab test slots) only requires it and fails the commit with a clear error when it does not exist.

## Notes

- Do not enable MPLS on an interface that has a Linux-CP pair unless the host kernel has MPLS (VPP's linux-cp then also
  enables MPLS on the host tap).
- LDP, L3VPN with BGP labels, RSVP-TE, SR-TE color-based steering, MPLS multicast and EXP marking are not part of this
  feature.
