# F-ikev2-native — route-based PSK milestone (2026-10-03)

Status: **running overall; native route-based PSK implementation and production packet milestone verified in disposable VPP**. Certificate/PKI acceptance remains unavailable. No patched plugin/package was deployed to the shared appliance.

## Implemented behavior

The product schema accepts only `engine: vpp-ikev2` with a required protected IPIP binding. Desired-state projection, owned profile metadata, reconciliation, native IKE/CHILD state, measured per-SA forwarding counters and explicit runtime actions are wired into the production agent. Referenced PSKs travel separately through the API→agent secret channel and sealed cache; scheduler values contain keyed references. Production API commit, rotation, restart and rollback installed the expected PSK in VPP without exposing material.

Native profile creation lowers its bound IPIP before negotiation. Native IKE raises it only after installing protection and lowers it before deleting protection. The generic admin descriptor refuses writes to live bound IPIP interfaces and excludes their runtime admin status from Retrieve, so ordinary reconciliation cannot lower a working tunnel or raise an unprotected one. Full desired context is retained for partial Apply.

Native state uses direction-correct addresses/SPIs and counters joined to the owned native SAD index. SAD/IKE key buffers are wiped immediately after dump receipt. Owned-profile/SPI checks refuse foreign runtime actions. The administrator-only API, generated client, CLI SA/actions and web SA controls use native state; the original-responder local rekey control is disabled with a reason.

## Actual validation

- Production packet topology: agent startup and sealed cache, gRPC Apply with a separate SecretBundle, owned WAN/LAN/IPIP interfaces and static route, native explicit initiation/rekey/deletion; bidirectional ICMP and exact 1 MiB TCP passed.
- Native state returned positive inbound/outbound byte and packet counters with no key echo. Foreign CHILD/IKE SPI actions were refused.
- Repeated full Apply preserved the active SA, protected interface and traffic. Actual agent process restart reopened sealed secrets; its initial Service.Resync retained identical IKE/CHILD SPIs and forwarding.
- Declarative route withdrawal stopped traffic; restoration recovered it. Tunnels-only partial Apply retained fail-closed admin state before SA installation and after SA deletion while the route remained.
- Underlay capture contained ESP and no plaintext IPIP across those lifecycle phases. Retained capture: `native-ipsec-2026-10-03-evidence/initiator-underlay.pcap`. [Structured result and artifact hashes](native-ipsec-2026-10-03-evidence/result.json); [default-timer packet log](native-ipsec-2026-10-03-evidence/production-default-dpd.log).
- Default VPP DPD (30 seconds, three retries) removed a hard-crashed peer SA and lowered IPIP. Peer restart recovered automatically to exactly one new owned SA and bidirectional encrypted traffic. The full production default-timer run passed in 108.93 seconds. Accelerated diagnosis used a disposable 1-second/three-retry override and is labeled separately.
- Independent production API lifecycle: [report](native-ipsec-api-2026-10-03.md), including final omitted-MTU/default-9000 round trip, PSK rotation and version-pinned rollback.
- CLI generation, documentation, full race tests and build passed (`.scratch/native-cli-final.log`). Product VPP static verification passed 66 tests (`.scratch/native-vpp-static.log`). Parent report records broad Go/schema/API/web validation.

## Required VPP patch and reproduction

[Product patch 0002](../../../deploy/vpp/patches/0002-ikev2-safe-native-state.patch) guards incomplete-negotiation profile lookups in v2/v3 dumps and fixes action SPI byte order. The agent requires exact plugin capability major 1/minor `0x56525801`. An unpatched plugin is refused before dangerous native state polling. The product patch series now produces `26.06-release+vrx2`.

The reviewed [disposable builder](../../../test/topology/ipsec/build-native-plugin.py) copied API source from the pinned reference, applied the patch and compiled/linked only private outputs; the upstream source, its existing objects and shared installed plugin remained untouched. [Topology instructions](../../../test/topology/ipsec/README.md) show the production-agent and descriptor-only modes separately.

## Remaining limits

PSK, fixed IPv4 peer, default underlay VRF and one selector per direction form the verified milestone. Certificate authentication awaits an explicit peer-certificate/CA-trust mapping and plugin-global private-key provisioning; PKI file delivery alone does not supply native CA-chain verification. See [certificate integration audit](F-pki-native-audit-2026-10-03.md). IPv6 underlay, non-default underlay VRFs, wildcard/hostname peers, multiple selectors, MOBIKE, ESN, explicit fragmentation, reauthentication, separate ESP PFS, packet lifetimes, per-tunnel DPD and IKE lifetime controls are refused. IPv6 overlay and NAT traversal are unverified. Original IKE responders use peer-triggered CHILD rekey; local responder rekey is unavailable in this VPP implementation.

Agent restart with an active SA was verified; a full VPP process restart with autonomous negotiation has not been tested. UI build validation is recorded, but no browser screenshot or full `tools/ci.sh --base main` run is claimed. Full certificate/PKI feature acceptance remains open.

## Shared integration hunks

Native hooks were added to agent projection and subsystem registration; the generic admin descriptor is wrapped for protected IPIP ownership. Existing IPsec RPC/product API state now uses native VPP state while historical renderer fixtures remain explicitly separate. ActionRequest gained the coordinated native action; root regenerated protobuf/OpenAPI clients and implemented the sealed secret channel. API feature registration, administrator route coverage, the existing IPsec drawer, locales and CLI operation table/reference were updated. Root/drift own route alias and default IPIP MTU reconciliation fixes.

Cleanup confirmed: slot-8 namespaces, veths and peer/agent processes are gone; disposable VPP stopped. Shared VPP PID 1014 remained running with `NRestarts=0`.
