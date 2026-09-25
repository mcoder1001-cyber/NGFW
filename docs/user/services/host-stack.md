# Host stack (advanced, T3)

VPP's host stack is the TCP/UDP session layer inside the data plane. A router rarely needs it, so this product exposes
only the part VPP's binary API can configure. Screen: **Services → Host stack**. Config path:
`/api/v1/config/services/hostStack`. Live view: `GET /api/v1/state/host-stack`.

## What you can configure

| Leaf | Meaning | Notes |
|---|---|---|
| `enabled` | The session layer must be on (rule-table engine) | Only the **globals owner** agent turns it on, and only when it is off. Other agents only check it. The agent **never** turns the session layer off or changes its engine. |
| `namespaces.<id>` `{vrf, interface?, secretRef?}` | Application namespace | `secretRef` is a `key/<name>` reference only. It is **refused** until the API→agent secret channel exists, so leave it out. VPP cannot list namespaces: the agent re-adds them on every resync (write-only). |
| `sessionRules[]` `{tag, scope, transport, local, localPort?, remote, remotePort?, action, redirectAppIndex?, appNamespace?}` | Session rules: the host-stack "firewall" | Prefixes must be canonical and of one family. Tags must be unique. `action: redirect` needs `redirectAppIndex`. These are the only objects the agent can read back from VPP. |
| `tcpSourceAddresses` `{first, last, vrf}` | TCP source-address pool of one VRF | VPP has **no delete**. The pool stays until VPP restarts. |
| `httpStatic` `{enabled, wwwRootPath, uri, cacheSizeMb}` | VPP's built-in static web server | This is **opt-in on the agent** (`VRX_HOSTSTACK_HTTP_STATIC=1`, globals owner only). VPP cannot disable it or change it once it runs, so turning it off takes a VPP restart. `wwwRootPath` must be under `/var/lib/vrx/www/`, with no `..` and no control characters. |

Rules the API enforces. A failure returns 400 problem+json with a `pointer` to the failing leaf.
- An inline secret, `..` in `wwwRootPath` or a path outside `/var/lib/vrx/www/`, a non-canonical prefix, or mixed families.
- A missing VRF, interface or rule namespace, or an interface in a different VRF from its namespace.
- Session rules and `services.autoSdl.enabled` (Auto-SDL) in the same document. VPP has one session-rule engine: rules need
  `rule-table`, and Auto-SDL needs `sdl`.

## Not supported (have-not list)

- VCL and LD_PRELOAD applications.
- TLS engine tuning. `tls_openssl_set_engine` exists in the API but is not exposed.
- QUIC/quicly, HTTP/3, CONNECT and UDP proxying, SRTP, HSI.
- TCP/UDP buffer sizes, congestion-control algorithm, and `session { evt_qs_memfd_seg … }`. These are startup.conf-only
  settings and belong to the start-up generator (F-startup-gen). They are not generated today.
- SDL and Auto-SDL knobs (F-rpf-adl-pbr), the Prometheus exporter over http_static, and the load balancer (F-lb).
- Reading namespaces, the TCP pool or http_static back from VPP. VPP has no dump for them, so the live view lists only the
  namespaces this agent applied since the last VPP start.
