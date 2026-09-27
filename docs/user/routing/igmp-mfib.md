# Multicast (IGMP, mFIB, PIM)

IPv4 multicast in FAST MODE: IGMPv3 host/router interfaces, static multicast routes in the VPP multicast FIB (mFIB),
and PIM-SM where FRR `pimd` builds the state and the box programs the mFIB. Configure it under **Config → Routing →
Multicast** (`routing.multicast`); the live state is shown under **Routing → Multicast**.

## IGMP interfaces

Each interface runs in one of two modes:

- **router** — the box is the IGMPv3 querier and snoops membership reports on the segment.
- **host** — the box joins groups itself, via **static joins**.

Because VPP 26.06's IGMP plugin is **INCLUDE-only**, every static join must name at least one source (an EXCLUDE join is
rejected). Groups must be routable multicast (224.0.0.0/4) and not the 224.0.0.0/24 link-local control block.

```
routing.multicast.igmp.interfaces:
  TenGigabitEthernet0:
    mode: host
    joins:
      - group: 239.1.1.1
        sources: [10.0.0.5]
```

`ssmRanges` (default `232.0.0.0/8`) marks the source-specific ranges; a static route to a group inside an SSM range must
name its source.

## Static multicast routes (mFIB)

A static route has one **accept** (incoming / RPF) interface and one or more **forward** (outgoing) interfaces; the
accept interface cannot also forward. Omit the source for a `(*,G)` route or set it for `(S,G)`.

```
routing.multicast.mroutes:
  - group: 239.2.2.2
    source: 10.0.0.9        # omit for (*,G)
    paths:
      - { interface: TenGigabitEthernet0, flags: accept }
      - { interface: TenGigabitEthernet1, flags: forward }
```

## PIM-SM

List the PIM interfaces and the static rendezvous points. FRR `pimd` runs over the linux-cp pairs and builds the
multicast state; the agent mirrors the resulting `(S,G)`/`(*,G)` entries into the VPP mFIB.

```
routing.multicast.pim:
  interfaces: [TenGigabitEthernet0, TenGigabitEthernet1]
  rp:
    - { address: 10.0.0.1, groups: [239.0.0.0/8] }
```

## Live state

**Routing → Multicast** shows, refreshed on demand:

- **IGMP group memberships** learned on router-mode interfaces.
- **Multicast FIB** — the mFIB entries (group, source, accept, forward).
- **PIM neighbours** from FRR pimd.

## Limitations (V5, and VPP's IGMP plugin)

- VPP's IGMP plugin consumes IGMP for all local delivery and handles only IGMPv3, so FRR pimd never sees membership
  reports directly — receivers come from VPP router-mode IGMP events or from static joins.
- `linux-nl` does not sync multicast, so the agent programs the mFIB from pimd's state (V5).
- The data-plane programming of the mFIB and the FRR pimd sync arrive with the host build; the contract, the API and
  this page are active now.

## Out of scope

IPv6 multicast / MLD / pim6d, MSDP, multicast over tunnels, MPLS multicast, and BIER.
