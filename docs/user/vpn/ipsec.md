# Route-based IPsec site-to-site VPN

The product IPsec module uses native VPP IKEv2 and ESP with a protected IPIP interface. Overlay routes select traffic for the tunnel; the underlay routes encrypted packets to its peer. Policy-based VPN, transport mode, IKEv1 and a NGFW-side strongSwan fallback are outside the implementation. StrongSwan is used only as an interoperability peer in the tests.

## Current implementation

Native desired-state projection, profile reconciliation, owned SA state and runtime actions are wired into the agent and API. The API delivers referenced PSKs through the agent secret channel; desired configuration and state contain references, never secret material. A sealed cache restores the selected secret snapshot after agent restart and configuration rollback. Independent disposable tests proved actual VPP PSK installation, rotation, restart and rollback through the production API and agent.

Disposable VPP packet tests proved bidirectional ICMP, exact 1 MiB TCP transfer, peer-triggered CHILD rekey, route withdrawal and recovery, and SA deletion. Underlay capture contained ESP and no plaintext IPIP before negotiation, during forwarding/rekey or after SA deletion. The plugin raises the protected interface after SA installation and lowers it before removing protection; declarative interface administration cannot override that ownership.

This requires [product patch 0002](../../../deploy/vpp/patches/0002-ikev2-safe-native-state.patch). It prevents a VPP SA dump crash during incomplete negotiation and corrects SPI byte order in runtime actions. The agent checks its exact capability revision and refuses native state/configuration on an incompatible plugin. The patch was tested in disposable VPP instances; it has **not been installed on the shared appliance**. Build and deployment remain separate work.

## Configuration

Use `vpn.ipsec.proposals.<name>` and `vpn.ipsec.tunnels.<name>`:

- `engine: "vpp-ikev2"`, IKEv2, tunnel mode and ESP; PSK references for both identities.
- Fixed local/remote underlay IP addresses and supported IKE/ESP transforms.
- Required `routeBased.ipipInterface`, referencing an existing `tunnels.ipip` entry with matching source, destination, overlay VRF and underlay VRF.
- At least one IPv4 or IPv6 address on the IPIP interface. An addressless interface drops decrypted IP traffic. Configure the overlay routes through this interface.
- At most one CIDR traffic selector per direction. The peer must agree with the selectors.
- `startAction: "none"`; initiation is an explicit runtime action when the local underlay interface can be resolved.

Prefer FQDN IKE identities such as `@branch.example`. VPP's current identity API cannot represent an IP identity containing embedded zero bytes; those identities fail validation. This restriction concerns the identity, not the underlay IP address. Disable MOBIKE on the interoperability peer for the verified fixed-address flow.

The final production API lifecycle test omitted IPIP MTU and passed using the VPP default 9000; omission is reconciled correctly. The packet topology tested IPv4 underlay and IPv4 overlay. IPv6 underlay and non-default underlay VRFs are refused. IPv6 overlay and NAT traversal have not been established by this evidence; wildcard peers are refused.

## State and actions

`GET /api/v1/state/ipsec/{tunnels,sas}` and `GET /api/v1/state/ipsec/ikev2/sas` report owned native profiles and IKE/CHILD SAs, direction-correct SPIs, algorithms and lifetimes. The CLI SA command and web SA drawer use this state. Administrator-only `POST /api/v1/actions/ipsec/ikev2/{tunnel}/{operation}` accepts `initiate`, `rekey` or `delete-sa`. SPI handles are checked against the named owned profile.

An original IKE responder cannot locally initiate CHILD rekey with this VPP implementation. Its local rekey action fails explicitly and its web rekey control is disabled; peer-triggered rekey is supported and tested. Certificate authentication is unavailable: native VPP verifies a pinned peer public key and uses one plugin-global local private key. Mapping the product CA-trust contract and provisioning that global key remain required; PKI file delivery alone is insufficient. Multiple selectors, MOBIKE, ESN, explicit fragmentation, reauthentication, per-tunnel DPD, IKE lifetime and separate ESP PFS are refused.

Per-SA byte/packet counters are measured from VPP's stats segment and joined to the owned CHILD SPI and native SAD index. An unavailable counter source returns an explicit unavailable error. Production packet tests also verified that restarting the agent reopens the sealed secret cache and completes initial reconciliation without changing active IKE/CHILD SPIs or interrupting protected forwarding. Hard peer crash, default-timer DPD expiry and automatic native-initiator recovery were verified: VPP removed the lost SA, lowered IPIP, then restored one new owned SA and bidirectional protected traffic after peer restart. Per-tunnel DPD and IKE lifetime configuration are unavailable; VPP uses its global liveness defaults (30 seconds, three retries).

Authority: [DEC-ipsec-route-based](../../decisions/DEC-ipsec-route-based.md). Evidence: [production API secret lifecycle](../../status/tasks/native-ipsec-api-2026-10-03-evidence/lifecycle.json).
