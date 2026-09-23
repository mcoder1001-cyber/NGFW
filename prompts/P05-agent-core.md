# Task P05 — vrx-agent core: govpp + reconciler   (prepend 00-CONTEXT.md)

## Goal
The Go agent that owns everything privileged: connects to VPP, exposes the P03 gRPC
service, and converges VPP to a desired state with a Kubernetes-style reconciler.
Reference design: Ligato vpp-agent's KVScheduler (read `docs/01-architecture.md` AD-3) —
**learn from it, do not vendor it**.

## Read first
`apps/agent/gen/vrx/v1` (P03 stubs), `apps/agent/binapi/` (P04 bindings), `docs/01-architecture.md`.

## P05a — first, within the first hours (merged alone to `main`)
Publish `internal/scheduler/descriptor.go` (the `Descriptor` interface, `KV`, `Dependency`, `ErrRecreate`, plan/result types) plus `internal/vpp/fake` (a fake VPP client for descriptor unit tests) and `internal/descriptors/README.md` (how to write one). Commit, tell the manager: the DF-*/RF-* factories start from this. Keep the interface stable afterwards; changes are contract tasks.

## Build exactly this
1. `internal/vpp`: govpp connection manager with reconnect/backoff, health, and an
   interface so tests can inject a fake. Stats-segment reader at 1 Hz.
2. `internal/scheduler`: the reconciler. Types:
   ```go
   type Descriptor interface {
     Name() string
     KeyOf(obj proto.Message) string
     Dependencies(obj) []Dependency         // keys that must exist first
     Create(ctx, obj) (metadata, error)
     Update(ctx, old, new, meta) (metadata, error)   // or return ErrRecreate
     Delete(ctx, obj, meta) error
     Retrieve(ctx) ([]KV, error)            // dump ACTUAL state from VPP
   }
   ```
   A transaction = desired KV set → topological order → diff against `Retrieve()` →
   plan (create/update/delete) → apply → verify by re-Retrieve. On any error: revert the
   applied operations of this txn in reverse order, return `ROLLED_BACK` with per-object results.
3. Descriptors for the P02/P03 domains: `interface` (host-interface/af_packet for now,
   generic for others later), `interface-ip`, `interface-admin-state`, `interface-mtu`,
   `subinterface (vlan/qinq)`, `vrf` (ip_table_add_del), `static-route` (ip_route_add_del),
   `neighbor` (read-only Retrieve for state). Every one implements `Retrieve`.
4. gRPC server on `/run/vrx/agent.sock` (0660, group `vrx`); the agent itself runs as root (VPP sockets are root:vpp 775 — see `docs/lab/host-vrx-a.md`): `Apply`, `DryRun`
   (plan only, no apply), `Retrieve`, `StreamStats`, `StreamEvents` (link up/down from
   `want_interface_events`, reconcile start/done), `Health`. `Action` returns
   UNIMPLEMENTED for now.
5. **Resync on start / VPP restart**: the agent keeps the last applied desired state in
   `/var/lib/vrx/agent/desired.pb`; on start and on VPP reconnect it runs a full
   reconcile automatically and emits `RECONCILE_DONE` with counts.
6. **Confirm timeout**: `Apply` with `confirm_timeout_sec > 0` starts a timer; unless
   `Apply` is called again with `confirm_txn_id`, the agent re-applies the previous desired
   state itself (it must not depend on the API being alive).
7. Structured logs (`slog`), Prometheus metrics on `127.0.0.1:9101/metrics`
   (reconcile duration, objects, errors, vpp_connected).
8. Tests: unit tests for the scheduler with a fake descriptor set (ordering, rollback, idempotency,
   dependency failure); integration tests against the lab VM's VPP: apply two interfaces with IPs
   and a static route → ping from host-lan VM → `tools/lab kill-vpp vrx-a` →
   assert everything is back within 30 s using `Retrieve`.

## Acceptance
- [ ] `go test ./...` green; integration suite green against the P04 lab
- [ ] `grep -r "vl_api_\|_reply" internal/ | grep -v binapi` finds nothing hand-typed
- [ ] Applying the same desired state twice produces an empty plan (idempotent)
- [ ] `kill -9` of the agent itself → restart → no VPP changes (state already converged)

## Out of scope
NAT, ACL, IPsec, FRR/strongSwan renderers, DPDK interfaces, packet capture. The Node API.
