# MPLS LDP

LDP (Label Distribution Protocol, RFC 5036, IPv4) distributes MPLS labels between routers. On NGFW, FRR `ldpd` runs the
protocol and the box syncs the resulting labels into the VPP MPLS FIB (the V5 agent-side sync). The result is a
transit LSR / penultimate-hop router. Configure it under **Config → Routing → MPLS** (`routing.mpls.ldp`); the live
state is the **LDP** tab of **Routing → MPLS**.

## Configuration

```
routing.mpls:
  interfaces: [TenGigabitEthernet0]      # MPLS must be enabled on the LDP interfaces
  ldp:
    routerId: 10.0.0.1                    # usually a loopback
    transportAddress: 10.0.0.1            # must be a configured local address (default VRF)
    interfaces: [TenGigabitEthernet0]     # each must be in routing.mpls.interfaces and the default VRF
    neighbors:
      10.0.0.2:                           # keyed by the peer LSR-ID
        passwordRef: password/ldp-peer    # optional TCP-MD5, a secret reference (never inline)
    labelRange: { min: 16000, max: 24000 }  # optional; the dynamic block LDP allocates from
```

Rules enforced at commit (with a JSON pointer to the offending entry):

- LDP interfaces must exist, be MPLS-enabled (listed in `routing.mpls.interfaces`), and be in the default VRF.
- `transportAddress` must be an address configured on a default-VRF interface or loopback.
- No static label route or SR binding-SID in table 0 may fall inside the LDP dynamic label range (LDP owns that block).

## Live state (LDP tab)

- **Neighbours** — the LDP adjacencies and their state (e.g. OPERATIONAL) (uptime reporting requires the remaining FRR time-format adapter).
- **Label bindings (LIB)** — the label ↔ FEC bindings, paged.
- **FRR → VPP sync** — the last sync, how many routes were installed, any label conflicts, and the source in use.

## What is and is not synced (V5)

- The agent reads FRR's in-use IPv4 label bindings, neighbors, link discovery and active RIB next hops, then reconciles a separate owned MPLS route scope through the scheduler. Linux interfaces must have configured LCP mappings.
- The supported limit is **256 distinct dynamic label routes**. An oversized or failed read retains the previous set for 60 seconds, then flushes it. A successful withdrawal removes its routes immediately.
- **EOS IPv4** label switching and remote implicit-null (PHP/pop) are supported by the implementation. Non-EOS label stacks and explicit-null remain unsupported pending host verification; do not use this build for stacked service-label transit.
- **Ingress label imposition** (pushing LDP labels onto IP routes at the head end) is outside this release. IP prefixes remain owned by the kernel/linux-cp route programmer.
- Renderer, synchronization, neighbor events and the live state RPC are wired in this build. The installed count comes from the source's current owned VPP routes, including configuration-triggered withdrawals. Live FRR/VPP acceptance remains deferred; no operational lab session or canonical configuration verification is claimed here.
- Kernel MPLS requirements and linux-cp delivery of LDP hellos still require the designated host probes. No kernel modules, sysctls or FRR daemon configuration are changed by this task.

## CLI equivalent (FRR)

```
mpls ldp
 router-id 10.0.0.1
 address-family ipv4
  discovery transport-address 10.0.0.1
  interface host-wan0
  exit
 exit-address-family
exit
```

The CLI uses the **Linux LCP name** (`host-wan0` in this example), while the NGFW
configuration uses the VPP logical name (`TenGigabitEthernet0`). Configure the
interface's `lcp.hostIfName` mapping accordingly.

## Out of scope

LDP IPv6 (RFC 7552), targeted LDP, pseudowires (VPWS/VPLS), mLDP, LDP in non-default VRFs, LDP–IGP sync, session
protection, and RSVP-TE.
