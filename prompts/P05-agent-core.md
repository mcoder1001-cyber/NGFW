# Task P05 — vrx-agent core: govpp + reconciler   (prepend 00-CONTEXT.md)

## Goal
The Go agent that owns everything privileged: connects to VPP, exposes the P03 gRPC
service, and converges VPP to a desired state with a Kubernetes-style reconciler.
Reference design: Ligato vpp-agent's KVScheduler (read `docs/01-architecture.md` AD-3) —
**learn from it, do not vendor it**.

## Read first
`apps/agent/gen/vrx/v1` (P03 stubs), `apps/agent/binapi/` (P04 bindings), `docs/01-architecture.md`.

## Precondition
P05a is merged: `internal/scheduler/descriptor.go`, `internal/vpp/{client.go,fake}`, `internal/renderers/renderer.go` and the READMEs are **frozen, read-only for you** (changes = `contract/<id>` branch). Start at step 1.

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
3. Descriptors: **only the minimum to prove the scheduler end to end**, under `internal/descriptors/core/`: `loopback`
   (create_loopback_instance), `interface-ip` (sw_interface_add_del_address), `vrf` (ip_table_add_del), `static-route`
   (ip_route_add_del). Everything interface-family (host-interface/af_packet, subinterface, admin-state, MTU, rx-mode,
   neighbor/ND) is owned by DF-1/DF-2 — do not create it here. Every descriptor implements `Retrieve`.
3b. **Ownership scoping (mandatory on the shared host):** the agent takes `VRX_OWNER` (default `vrx`) and tags every object
   it creates with it (interface tag via `sw_interface_tag_add_del`; tables/routes via an owner-prefixed name or an owner
   table in the state dir). Resync and rollback act **only on owned objects**; `Retrieve` filters by owner. Two agents with
   different owners on one VPP must never delete each other's objects — write the test.
4. gRPC server on `VRX_AGENT_SOCKET` (product default `/run/vrx/agent.sock`, 0660, group from `VRX_SOCKET_GROUP` — if the group does not
   exist, log a warning and use the process's primary group; **never `groupadd` on the host**); the agent itself runs as root
   (VPP sockets are root:vpp 775 — see `docs/lab/host-vrx-a.md`). Tests use their slot's socket path.: `Apply`, `DryRun`
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
8. Tests: unit tests for the scheduler with a fake descriptor set (ordering, rollback, idempotency, dependency failure);
   integration tests against the host VPP (`/run/vpp/api.sock`, `VRX_INTEGRATION=1`, shared lab lock, your `VRX_TEST_PREFIX`):
   apply two prefixed loopbacks with IPs in your VRF range and a static route → `Retrieve()` == desired and `vppctl show ip fib table <n>`
   reflects it → **simulate loss**: stop your agent, delete the objects via binapi, start the agent → everything back within 30 s
   (`Retrieve`) → rollback removes them. No VPP restart (D-012); no ping (that is P08).

## Acceptance
- [ ] `go test ./...` green; integration suite green against the P04 lab
- [ ] `grep -r "vl_api_\|_reply" internal/ | grep -v binapi` finds nothing hand-typed
- [ ] Applying the same desired state twice produces an empty plan (idempotent)
- [ ] `kill -9 <your agent's PID>` (never by name) → restart → no VPP changes (state already converged)

## Out of scope
NAT, ACL, IPsec, FRR/strongSwan renderers, DPDK interfaces, packet capture. The Node API.
