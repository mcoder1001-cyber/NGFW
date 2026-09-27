# MPLS LDP

LDP (Label Distribution Protocol, RFC 5036, IPv4) distributes MPLS labels between routers. On VRX, FRR `ldpd` runs the
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

- **Neighbours** — the LDP adjacencies and their state (e.g. OPERATIONAL) and uptime.
- **Label bindings (LIB)** — the label ↔ FEC bindings, paged.
- **FRR → VPP sync** — the last sync, how many routes were installed, any label conflicts, and the source in use.

## What is and is not synced (V5)

- The agent reads FRR's LDP label state and programs the VPP MPLS FIB. On this build the kernel MPLS modules are not
  loaded, so zebra may not build its LFIB; the sync then uses ldpd's in-use remote bindings plus the LDP adjacency.
- **Ingress label imposition** (pushing LDP labels onto IP routes at the head end) is **not** in this release — IP
  prefixes belong to the kernel/linux-cp route programmer (one programmer per route). This box acts as a transit LSR
  and penultimate hop (implicit-null → pop).
- The FRR ldpd section rendering and the live FRR→VPP label sync arrive with the host build; the contract, the
  validation, the API and the LDP tab are active now.

## CLI equivalent (FRR)

```
mpls ldp
 router-id 10.0.0.1
 address-family ipv4
  discovery transport-address 10.0.0.1
  interface TenGigabitEthernet0
 exit-address-family
exit
```

## Out of scope

LDP IPv6 (RFC 7552), targeted LDP, pseudowires (VPWS/VPLS), mLDP, LDP in non-default VRFs, LDP–IGP sync, session
protection, and RSVP-TE.
