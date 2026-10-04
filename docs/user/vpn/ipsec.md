# Route-based IPsec site-to-site VPN

The product IPsec module uses native VPP IKEv2 and ESP with a protected IPIP interface. Overlay routes select traffic for the tunnel; the underlay routes encrypted packets to its peer. Policy-based VPN, transport mode, IKEv1 and a NGFW-side strongSwan fallback are outside the implementation. StrongSwan is used only as an interoperability peer in the tests.

## Current implementation

Native desired-state projection, profile reconciliation, owned SA state and runtime actions are wired into the agent and API. The API delivers referenced PSKs through the agent secret channel; desired configuration and state contain references, never secret material. A sealed cache restores the selected secret snapshot after agent restart and configuration rollback. Independent disposable tests proved actual VPP PSK installation, rotation, restart and rollback through the production API and agent.

Disposable VPP packet tests proved bidirectional ICMP, exact 1 MiB TCP transfer, peer-triggered CHILD rekey, route withdrawal and recovery, and SA deletion. Underlay capture contained ESP and no plaintext IPIP before negotiation, during forwarding/rekey or after SA deletion. The plugin raises the protected interface after SA installation and lowers it before removing protection; declarative interface administration cannot override that ownership.

This requires [product patch 0002](../../../deploy/vpp/patches/0002-ikev2-safe-native-state.patch). It prevents a VPP SA dump crash during incomplete negotiation and corrects SPI byte order in runtime actions. The agent checks its exact capability revision and refuses native state/configuration on an incompatible plugin. The patch was tested in disposable VPP instances; it has **not been installed on the shared appliance**. Build and deployment remain separate work.

## Configuration

Use `vpn.ipsec.proposals.<name>` and `vpn.ipsec.tunnels.<name>`:

- `engine: "vpp-ikev2"`, IKEv2, tunnel mode and ESP; sealed PSK references or explicit RSA certificate public-key pins.
- Fixed local/remote underlay IP addresses and supported IKE/ESP transforms.
- Required `routeBased.ipipInterface`, referencing an existing `tunnels.ipip` entry with matching source, destination, overlay VRF and underlay VRF.
- At least one IPv4 or IPv6 address on the IPIP interface. An addressless interface drops decrypted IP traffic. Configure the overlay routes through this interface.
- At most one CIDR traffic selector per direction. The peer must agree with the selectors.
- `startAction: "none"`; initiation is an explicit runtime action when the local underlay interface can be resolved.

Prefer FQDN IKE identities such as `@branch.example`. VPP's current identity API cannot represent an IP identity containing embedded zero bytes; those identities fail validation. This restriction concerns the identity, not the underlay IP address. Disable MOBIKE on the interoperability peer for the verified fixed-address flow.

The final production API lifecycle test omitted IPIP MTU and passed using the VPP default 9000; omission is reconciled correctly. The packet topology tested IPv4 underlay and IPv4 overlay. IPv6 underlay and non-default underlay VRFs are refused. IPv6 overlay and NAT traversal have not been established by this evidence; wildcard peers are refused.

## State and actions

`GET /api/v1/state/ipsec/{tunnels,sas}` and `GET /api/v1/state/ipsec/ikev2/sas` report owned native profiles and IKE/CHILD SAs, direction-correct SPIs, algorithms and lifetimes. The CLI SA command and web SA drawer use this state. Administrator-only `POST /api/v1/actions/ipsec/ikev2/{tunnel}/{operation}` accepts `initiate`, `rekey` or `delete-sa`. SPI handles are checked against the named owned profile.

An original IKE responder cannot locally initiate CHILD rekey with this VPP implementation. Its local rekey action fails explicitly and its web rekey control is disabled; peer-triggered rekey is supported and tested. Certificate authentication uses the bounded peer public-key pin described below. Multiple selectors, MOBIKE, ESN, explicit fragmentation, reauthentication, per-tunnel DPD, IKE lifetime and separate ESP PFS are refused.

Per-SA byte/packet counters are measured from VPP's stats segment and joined to the owned CHILD SPI and native SAD index. An unavailable counter source returns an explicit unavailable error. Production packet tests also verified that restarting the agent reopens the sealed secret cache and completes initial reconciliation without changing active IKE/CHILD SPIs or interrupting protected forwarding. Hard peer crash, default-timer DPD expiry and automatic native-initiator recovery were verified: VPP removed the lost SA, lowered IPIP, then restored one new owned SA and bidirectional protected traffic after peer restart. Per-tunnel DPD and IKE lifetime configuration are unavailable; VPP uses its global liveness defaults (30 seconds, three retries).

Authority: [DEC-ipsec-route-based](../../decisions/DEC-ipsec-route-based.md). Evidence: [production API secret lifecycle](../../status/tasks/native-ipsec-api-2026-10-03-evidence/lifecycle.json).


## Native RSA certificate public-key pin

Set `auth.method: "cert"`, `auth.certificate` to the local object in `vpn.pki.certificates`, and `auth.peerCertificate` to an imported public leaf object in the same collection. Only the local object needs `privateKeyRef`; the peer object needs `certificateRef` without the peer private key. Do not set `remoteCa`: CA-chain trust is explicitly rejected for this mode. The normal candidate, validate, commit and confirmed-commit APIs apply; use the same fields with the CLI configuration editor.

Every enabled certificate tunnel shares exactly one local certificate, RSA private key and local IKE identity. Conflicting configuration fails before application. Local and configured peer leaf certificates must currently be valid, contain RSA keys of at least 2048 bits, permit digital signatures, and match the configured IKE identities through SANs. The local private key must match its certificate. ACME and HSM material are unavailable in this path. Certificate configuration is accepted only by the designated VPP globals owner; a foreign RSA profile prevents provisioning the shared key.

VPP verifies IKE AUTH against the public key from the configured peer leaf. It does not validate an incoming certificate chain, compare the incoming certificate DER, enforce an incoming certificate's dates, or exchange the configured local certificate. A peer using a reissued certificate with the same public key can still authenticate. Configure the equivalent explicit trusted peer public key on an interoperability peer. This native RSA-SIG AUTH uses the pinned VPP implementation's legacy RSA/SHA-1 signature operation; changing the IKE proposal's PRF does not change that signature algorithm.

The agent obtains material from its encrypted sealed reference cache and keeps immutable generations under its private state directory (`native-ikev2-<owner>`, mode 0700; PEM snapshots mode 0600). Desired state, metadata and RPC output contain reference fingerprints and paths only. Files are applied before the shared key, and the shared key before certificate profiles. Old generations are retained for rollback and confirmed revert; removing configuration does not securely erase historical generations. A later lifecycle cleanup may remove them only after retained revisions no longer need them.

Key or peer-pin rotation recreates the certificate profiles and retires their old IKE/CHILD sessions. A configuration rollback restores previous key/profile references but cannot restore retired negotiated sessions; explicitly initiate a fresh session (`startAction: "none"`). On a failed key load, the agent restores the previous immutable key; if it cannot establish compensation it reports a degraded/uncertain transaction. After the last certificate profile is removed, the private key remains loaded but inert in VPP because its API has no key unset. Agent/VPP reconciliation re-provisions the key before profiles after restart. RSA certificate packet interoperability remains subject to the recorded disposable/laboratory acceptance evidence; the prior PSK packet results do not establish RSA interoperability.
