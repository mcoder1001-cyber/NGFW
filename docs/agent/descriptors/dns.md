# dns plugin descriptors (DF-8, WBS D7.3)

Package `apps/agent/internal/descriptors/dns` — VPP's caching DNS resolver/proxy: the enable switch and the upstream
name servers, plus resolve action helpers. Message names only from `apps/agent/binapi/dns`.
`dns.RegisterGlobals(registry, client)` registers both (name servers first).

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `dns.name-server` | `dns.name-server/<ip>` | `dns_name_server_add_del` is_add=1 / 0 | **write-only** (`ErrRetrieveUnsupported`) | re-apply (address is the key) | — |
| `dns.enable` (singleton) | `dns.enable/global` | `dns_enable_disable` enable=1 / 0 | **write-only** | in place | — (see ordering) |

Actions (not desired state): `ResolveName(ctx, client, name)` (`dns_resolve_name`; name validated to DNS characters,
never a shell), `ResolveIP(ctx, client, addr)` (`dns_resolve_ip`).

## Notes and limitations
- **No dump, no getter** in `dns.api` (enable_disable, name_server_add_del, resolve_name, resolve_ip only): both
  descriptors are write-only (D-063). Create is idempotent (VPP ignores a duplicate server; enable twice is fine);
  `NAME_SERVER_NOT_FOUND` on delete = already gone.
- **Ordering is reversed vs. the DF-8 prompt:** VPP refuses `dns_enable_disable(1)` with `NO_NAME_SERVERS` while no
  server is configured. `dns.enable` therefore has no dependency and `RegisterGlobals` puts `dns.name-server` first (the
  scheduler breaks ties by registration order); deletes run in reverse (disable first).
- `dns_resolve_name` replies only after the upstream answers or VPP's retries give up: pass a ctx with a deadline
  (a host run without one blocked for minutes against unreachable upstreams).
- VPP CLI bug seen in the evidence: `show dns servers` prints the IPv6 list from the IPv4 vector (`fd00:5::53` is shown
  as `a05:3501::`, the bytes of `10.5.53.1`); the API state is right.
- Ownership: servers have no tag; production owns the resolver. The singleton is VPP-global; nobody else on the
  shared host uses VPP's resolver (Unbound is RF-3's), the host test is **opt-in** (`VRX_DF8_GLOBALS=1`,
  manager window): without a getter the previous state cannot be restored (review M3).

## Registration, ownership and restarts (D-069, D-071, D-074, D-076)
- There is no per-owner `Register` (everything here is VPP-global); `dns.RegisterGlobals(...)` registers the
  VPP-global singletons (`dns.name-server`, `dns.enable`) constructed as **globals owner** — P08 calls it only in the designated globals
  owner's agent (D-071). A descriptor constructed without the role (`dfkit.GlobalsOwner(false)`)
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
