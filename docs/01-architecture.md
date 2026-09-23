# Architecture decisions

## AD-1 — Node.js must not talk to VPP

VPP's control interface is a shared-memory **binary API** (plus a stats segment).
The maintained client libraries are C, Python (`vpp-papi`) and Go (`go.fd.io/govpp`).
There is no maintained Node.js binding, and writing one over `ffi-napi` puts a
blocking, memory-unsafe FFI in your event loop — it will deadlock under load and
crash the API when VPP restarts.

**Decision:** a Go agent (`vrx-agent`) owns everything privileged. Node.js owns the
product logic. gRPC over a UNIX socket between them.

**Consequences:** you need Go skills on the team. You gain: the agent can be tested,
restarted and upgraded independently; the API layer stays crash-safe; the same agent
can later be driven by a CLI, by RESTCONF, or by an orchestrator.

**Rejected alternative:** Ligato `vpp-agent` (Apache-2.0) already models interfaces,
routes, ACLs, NAT44, IPsec and L2 with protobuf + a gRPC northbound and would save
months. But upstream activity is thin, it lags current VPP releases, and you would
inherit a dependency at the very bottom of your stack. **Use it as a reference
implementation — read its `plugins/vpp/*` descriptors — but own your agent code.**

## AD-2 — Routing lives in FRR, not in our code

Never implement BGP. FRR (GPLv2) is a separate process; we render `frr.conf` and drive
`vtysh`/`frr-reload.py`, and read state from FRR's JSON output (`vtysh -c "show bgp summary json"`).
The glue that makes FRR's routes reach VPP is VPP's **`linux-cp`** plugin (plus `linux-nl`),
which mirrors VPP interfaces into Linux TAPs and syncs the Linux FIB into VPP.

**This is the most fragile part of the whole system.** Budget real time for it:
interface-pair lifecycle, MTU/state sync, IPv6 RA, ARP/ND, multicast, VRF mapping,
and route churn performance at 900k routes (full table). Test with a full BGP feed.

## AD-3 — Declarative reconciler, not imperative scripting

The agent receives desired state and converges. Structure it like a Kubernetes
controller:

```
DesiredState (protobuf)
   → per-subsystem Descriptor { Create, Update, Delete, Retrieve, Dependencies }
   → dependency graph / topological order
   → diff(desired, retrieved) → plan → apply → verify
```

`Retrieve` (dump actual VPP state) is what makes restart-safety, drift detection and
`config diff running-vs-actual` possible. Implement `Retrieve` for every object type,
even when it is tedious. Ligato's `KVScheduler` is the canonical design here.

## AD-4 — Transactions across heterogeneous backends

A single commit may touch VPP (atomic-ish), FRR (file + reload), strongSwan (file +
`swanctl --load-all`) and Kea (file + reload). There is no distributed transaction.
Approach:

1. Validate everything (schema → semantic → dry-run) before touching anything.
2. Order backends so the risky ones go last.
3. Snapshot every config file before writing; on failure, restore files, reload, and
   revert VPP objects via the reconciler by re-applying the previous desired state.
4. If a rollback itself fails, mark the system `DEGRADED`, alarm loudly in the UI, and
   offer "reload last good config and restart data plane".
5. Confirmed-commit (auto-revert timer) is the backstop for unreachable-after-commit.

## AD-5 — Telemetry path

VPP's **stats segment** is a shared-memory ring of counters; the agent polls it at 1 Hz
(cheap, no API round-trip) and pushes deltas to Node.js over a gRPC stream. Node.js:

- fans out to browsers over one WebSocket with topic subscriptions,
- writes downsampled series to a time-series store for the dashboard history.

For history, use **Prometheus + a VPP exporter** (or your own `/metrics` from the agent)
rather than storing series in PostgreSQL. Grafana optional for power users; the product
dashboard is your own React charts.

## AD-6 — Process & privilege layout

| Unit | User | Restart policy | Notes |
|---|---|---|---|
| `vpp.service` | root | on-failure, agent reconciles after | hugepages, isolcpus, `startup.conf` generated |
| `vrx-agent.service` | root | always | `After=vpp.service`, holds the gRPC socket |
| `vrx-api.service` | `vrx` | always | no `CAP_NET_ADMIN`, no raw sockets |
| `frr.service` | frr | always | config rendered by agent |
| `strongswan.service` | root | always | `swanctl.conf` rendered by agent |
| `kea-dhcp4/6`, `unbound`, `chronyd`, `snmpd`, `keepalived` | own users | always | rendered by agent |
| `postgresql`, `redis` | own | always | local only, unix sockets |
| `nginx` | www-data | always | TLS termination, serves the built React SPA, proxies `/api` |

## AD-7 — Why not just fork VyOS / pfSense / OPNsense?

- **VyOS** (GPLv2) gives you a mature config engine and CLI but a Linux-kernel data
  plane — it cannot reach TNSR performance. Its config semantics are worth studying.
- **pfSense/OPNsense** are FreeBSD/pf based, PHP UI, same performance ceiling.
- **DANOS** (Vyatta's successor, AT&T) was VPP-based and is effectively dead, but its
  YANG models are a useful reference.
- A GPLv2 fork also constrains your commercial licensing.

Building on VPP + your own control plane is more work and is the only route to
TNSR-class numbers.
