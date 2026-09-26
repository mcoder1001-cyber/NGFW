# dns plugin descriptors (DF-8, WBS D7.3)

Package `apps/agent/internal/descriptors/dns` — VPP's caching DNS resolver/proxy: the enable switch and the upstream
name servers, plus resolve action helpers. Message names only from `apps/agent/binapi/dns`.
`dns.RegisterGlobalsReady(registry, client) *Readiness` registers both (name servers first) and returns the live
readiness fact the lookup action checks (below).

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `dns.name-server` | `dns.name-server/<ip>` | `dns_name_server_add_del` is_add=1 / 0 | **write-only** (`ErrRetrieveUnsupported`) | re-apply (address is the key) | — |
| `dns.enable` (singleton) | `dns.enable/global` | `dns_enable_disable` enable=1 / 0; enable=1 refused with `ErrNoIPv4Upstream` (nothing sent) unless an IPv4 server was added on the running VPP | **write-only** | in place (Value carries the upstream set: a changed set re-enables after the new servers exist) | — (see ordering) |

Actions (not desired state): `ResolveName(ctx, client, name, ready)` (`dns_resolve_name`; name validated to DNS
characters, never a shell), `ResolveIP(ctx, client, addr, ready)` (`dns_resolve_ip`). **Precondition (D-137):** call them
only after an IPv4 `dns_name_server_add_del` and `dns_enable_disable(1)` succeeded on the **running** VPP instance, and
say so with `ready` — take it from `(*Readiness).Ready(ctx)`, never from the stored document; without it they return
`ErrResolverNotReady` and send nothing (unit test `TestResolveHelpersRefuseWithoutReady`).

**Readiness (`readiness.go`, review H2).** A live fact kept by the two descriptors of the globals owner: an IPv4 server
add and a `dns_enable_disable(1)` that VPP accepted are recorded together with the VPP boot identity (kernel boot_id,
VPP main PID, start time — `internal/vpp/bootid`) seen at that moment. `Ready(ctx)` re-reads the identity (one
control_ping) and is true only on the same instance, while enabled and with at least one IPv4 server. A disable, a
delete of the last IPv4 server, an unreadable identity and any VPP restart/crash/reconnect clear it until the resync
applies both objects again (`TestReadinessDoesNotSurviveAVPPRestart`). A nil `*Readiness` (not the globals owner) is
never ready. IPv6 servers are applied but never count.

## Notes and limitations
- **No dump, no getter** in `dns.api` (enable_disable, name_server_add_del, resolve_name, resolve_ip only): both
  descriptors are write-only (D-063). Create is idempotent (VPP ignores a duplicate server; enable twice is fine);
  `NAME_SERVER_NOT_FOUND` on delete = already gone.
- **Ordering is reversed vs. the DF-8 prompt:** VPP refuses `dns_enable_disable(1)` with `NO_NAME_SERVERS` while no
  server is configured. `dns.enable` therefore has no dependency and `RegisterGlobals` puts `dns.name-server` first (the
  scheduler breaks ties by registration order). A dependency on the servers would be wrong: the scheduler deletes and
  re-creates dependents around a delete, re-enabling before the replacement server exists.
- **Never enabled without a name server (F-unbound-chrony-syslog, V-item):** VPP 26.06 crashes on a DNS request (API or
  UDP 53 packet) while enabled with no server, and the scheduler runs deletes before creates. So the globals owner's
  `dns.name-server` Delete sends `dns_enable_disable(0)` first, and `Enable.Upstreams` makes a changed server set an
  update of the switch that re-enables it after the new servers were created. Fake-VPP model test
  `TestUpstreamChangesNeverLeaveAnEnabledResolverWithoutServers` drives the real scheduler through server replacements.
- The host test `TestDNSOnHost` needs `VRX_DNS_VPP_HOST=1` **and** `VRX_DF8_GLOBALS=1` (D-064: it can crash VPP).
- `dns_resolve_name` replies only after the upstream answers or VPP's retries give up: pass a ctx with a deadline
  (a host run without one blocked for minutes against unreachable upstreams).
- **VPP 26.06 crashes on any DNS request when no IPv4 name server was added since it started** (`is_enabled` does not
  matter on the API path): SIGSEGV in `ip4_sas` from `vnet_send_dns4_request` on the NULL `ip4_name_servers` vector —
  `dns_resolve_name`, `dns_resolve_ip`, an IPv4 UDP-53 request to a VPP address while enabled, and IPv6-only servers
  alike. The shared VPP went down on 2026-09-25 04:27:21 from a `dns_resolve_name` (V-item in
  `docs/vpp-code-track.md`, D-137). Guards: `dns.enable` needs an IPv4 server (`ErrNoIPv4Upstream`,
  `TestIPv6OnlyUpstreamsAreNeverEnabled`); the schema and the projection refuse an enabled cache without an IPv4
  upstream; the agent's `dns_lookup` action (`internal/actions/unbound-chrony-syslog`) needs the globals owner, a
  non-DEGRADED agent and `Readiness.Ready` (FAILED_PRECONDITION otherwise), and nothing else calls the helpers.
- `dns_name_server_add_del` and `dns_enable_disable` are safe in every state (vector operations; disable of a
  never-enabled plugin returns early; enable without servers → `NO_NAME_SERVERS`).
- VPP CLI bug seen in the evidence: `show dns servers` prints the IPv6 list from the IPv4 vector (`fd00:5::53` is shown
  as `a05:3501::`, the bytes of `10.5.53.1`); the API state is right. With **IPv6-only** servers the same CLI crashes
  VPP (NULL IPv4 vector): never run it on the shared VPP.
- Once enabled, VPP answers UDP 53 on every VPP address in every FIB (the ports are registered globally and stay
  registered after a disable); the plugin has no client ACL. The projection warns (`services.dns-vpp-cache-exposure`).
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
