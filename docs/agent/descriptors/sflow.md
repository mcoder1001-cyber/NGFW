# sflow plugin descriptors (DF-8, WBS D7.6)

Package `apps/agent/internal/descriptors/sflow` — random packet sampling: global parameters and sFlow on an
interface. Export to a collector is hsflowd's job (out of scope). Message names only from `apps/agent/binapi/sflow`.
`sflow.Register(registry, client, owner, opts...)` (+ `RegisterGlobals` for `sflow.global`); option `WithInterfaceKey`.

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `sflow.global` (singleton) | `sflow.global/global` | `sflow_sampling_rate_set`, `sflow_polling_interval_set`, `sflow_header_bytes_set`, `sflow_direction_set`, `sflow_drop_monitoring_set`; delete = VPP defaults | the five `*_get`, reported while ≠ defaults (10000 / 20 s / 128 B / rx / off) | in place | — |
| `sflow.interface` | `sflow.interface/<ifname>` | `sflow_enable_disable` enable=1 / 0 | `sflow_interface_dump` + learned/probed mapping (below) | re-apply | `interface/<ifname>`, `sflow.global/global` optional |

Value fields: `Global{sampling_rate (0 = off), polling_interval, header_bytes (64..256, step 32 — VPP rounds other
values silently, so Validate rejects them), direction rx|tx|both, drop_monitoring}`; `Interface{interface}`. Meta:
`InterfaceMeta{SwIfIndex, HwIfIndex}`.

## The hw_if_index gap (VPP API)
`sflow_enable_disable` reads a **sw_if_index** from its field named `hw_if_index`, while `sflow_interface_details`
reports the real **hw_if_index**, and no VPP 26.06 API maps a hw_if_index back to a sw_if_index. The descriptor keeps a
hw → sw map bound to the D-080 VPP boot identity (cleared on a VPP restart, review M1) and learns entries only
deterministically, **in Create** (the write path):
1. after its own enable, when exactly one new hw index appeared in the dump;
2. otherwise — a concurrent change, or `VALUE_EXIST` on an interface that is ours (after an agent restart) — by
   disabling that interface, dumping and re-enabling it: the hw index that disappears is its own.
**Retrieve is read-only** (review M2): it reports learned entries that are still enabled. An interface it has not learned
(agent restart) is not reported, so the first resync after an agent restart plans one re-create, whose Create learns
the mapping (toggling only our own interface) — the restart simulation shows exactly that. Another owner's interface
is never toggled. All calls of the descriptor are serialised.
Proposed VPP fix (for `docs/vpp-code-track.md`): add `sw_if_index` to `sflow_interface_details`.

## Notes
- VPP answers a redundant enable/disable with `VALUE_EXIST`; Create and Delete treat it as success.
- Ownership: this owner's tagged interfaces or claimed untagged ones (below). The global singleton is VPP-global: the host test skips when not at defaults and
  restores the defaults in Cleanup.

## Registration, ownership and restarts (D-069, D-071, D-074, D-076)
- `sflow.Register(...)` registers the per-owner object types (`sflow.interface`); `sflow.RegisterGlobals(...)` registers the
  VPP-global singletons (`sflow.global`) constructed as **globals owner** — P08 calls it only in the designated globals
  owner's agent (D-071), before `Register`. A descriptor constructed without the role (`dfkit.GlobalsOwner(false)`)
  only *requires* the value: Create succeeds when VPP already has it (checked through the getter where one exists,
  otherwise `dfkit.ErrNotGlobalsOwner`), Delete is a no-op, Retrieve is write-only.
- Interfaces are named by their **logical name** and resolved with DF-1's `iface.ResolveName` (D-069): this owner's
  tag id first, then an untagged interface's VPP name; another owner's interface fails with
  `iface.ErrForeignInterface`, local0 never resolves. Objects on an **untagged** interface (a DPDK NIC) are recorded
  in the owner's ClaimStore (`iface.Claims`, shared with DF-1; P05/P08 install a persisted one) **only after VPP
  accepted the add**, released on Delete, and reported by Retrieve only while claimed (D-071 claim rule). An object that
  already exists on an untagged interface without our claim is **never adopted** (Create fails with
  `dfkit.ErrNotOurs`, nothing is claimed) and Delete never touches it (review H1). Claims are bound to the D-080 VPP
  boot identity (kernel boot_id, VPP main PID, VPP start time — `internal/vpp/bootid` via `dfkit.BootIdentity`) and the sw_if_index, so they
  expire when VPP restarts or the name moves to another interface.
- Deletes re-resolve the logical name right before acting by sw_if_index (never a Meta index — indexes are reused
  after a VPP restart) and first check that the object still exists (D-074); "already gone" is success.
- Retrieve never reports a key twice (`dfkit.Dedupe`).
- Restart simulation (fresh connection + fresh descriptors → empty plan; objects deleted via binapi → exactly their
  re-creation planned → empty plan again): `internal/descriptors/dfkit/restarttest`, output in `DF-8.md`.

## F-ipfix-sflow additions (gap-only, D-104)
- TD-11b declarations (`ownership.go`): `GlobalDescriptor.RecordsNoOwnership()` (VPP-global singleton);
  `InterfaceDescriptor.CheckPersistent()` = `dfkit.CheckClaims`. The learned hw→sw map is not ownership (re-learned in
  Create, V17). Found by `subsystems.TestRequirePersistentPerFamily`.
- Product wiring: `RegisterGlobals` only for the globals owner, `NewGlobal(c)` (requirement only) otherwise. The agent's
  restart test (`internal/agent` `TestIpfixSflowGlobalsOwnerLifecycle`) shows exactly one `sflow.interface` re-create
  on the first resync after a restart, then an empty plan.
