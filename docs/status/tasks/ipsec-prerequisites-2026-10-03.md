# Native route-based IPsec implementation — 2026-10-03

Owner-authorized scope: native VPP IKEv2 route-based IPsec, prerequisite repairs,
and hardware/topology validation using disposable VPP instances. The product
module has no policy-based or strongSwan fallback.

## Implemented and verified

- Schema-default drift normalization preserves genuine differences. Real API
  commit, rollback and confirmed revert passed. Native IPIP creation MTU defaults,
  logical tunnel route names and routing-only Retrieve/Apply now round-trip.
- Nine isolated topology scopes passed, including ACL, object model, VLAN, bonding,
  bridge, both NAT suites, loopback/GSO/LLDP/SPAN and router advertisements.
  RA expiry reconstruction checks the actual configured window and rejects external
  changes. See [restart/topology report](restart-socket-2026-10-03.md).
- The production API resolves enabled native PSK references, pins secret versions
  to revisions, and supplies a separate agent secret bundle. The agent seals it
  with owner-bound AES-GCM and atomically selects opaque current/confirmed snapshots.
  Restart, rotation, rollback, transient DryRun and secret-buffer clearing were tested.
- The final real API lifecycle passed commit, agent restart with missing VPP profile,
  PSK rotation and rollback, exact in-memory PSK installation, empty drift, logical
  overlay route readback, omitted MTU and no secret echo. See
  [API lifecycle report](native-ipsec-api-2026-10-03.md).
- Native projection, owned IKE/CHILD state, direction-correct SPIs, counter ownership,
  initiation/rekey/deletion actions, API, CLI and UI are implemented. Full and partial
  interface reconciliation cannot raise an unprotected IPIP interface.
- Disposable packet tests passed bidirectional ICMP, exact 1 MiB TCP transfer,
  local initiator and peer-triggered responder CHILD rekey, route withdrawal/recovery,
  SA deletion and no plaintext IPIP in underlay captures. Restarting the actual agent
  retained active IKE/CHILD SPIs through initial resync and forwarding continued.
- Final production-agent peer-loss/recovery test passed (108.93 seconds) with unchanged
  VPP default liveness: 30 seconds / three retries. Hard peer failure removed its SA
  and lowered IPIP; restarting the peer let native automatic retry negotiate a new
  owned SA and restore bidirectional encrypted traffic. The earlier failed fixture
  had sent a duplicate initiation during VPP automatic retry; its failure is retained
  in the detailed feature evidence.

## Validation and deployment boundary

Final schema suite: 78 files, 1623 tests passed. Final Go gate passed agent,
subsystems, desired, IKEv2, interfaces, core fixtures, secret cache and RA packages;
focused routing/cache race tests also passed. Generated protocol/API client build
and checks passed. Web IPsec tests (5), production build, bundle budget and no-dev-route
checks passed. CLI tests and build passed. The complete API suite passed 74 files / 435 tests,
followed by successful typechecking. Final evidence secret scanning found no leaks;
`git diff --check` passed and owned network namespaces were cleaned.

Native support requires [VPP patch 0002](../../../deploy/vpp/patches/0002-ikev2-safe-native-state.patch):
safe incomplete-SA readback and corrected runtime SPI byte order. The agent refuses
incompatible plugins. The reproducible patched plugin was built and tested only in
disposable instances; it was not installed on the shared appliance.

Verified scope is PSK, fixed IPv4 underlay/default outer VRF and IPv4 overlay.
Certificates, IPv6 overlay/NAT interoperability and advanced features need separate
work. Unsupported per-tunnel DPD/IKE lifetime, reauthentication, multiple selectors,
MOBIKE, ESN, fragmentation and separate PFS settings are refused. A native original
responder supports peer-triggered rekey; local responder rekey is explicitly unavailable.

Shared system VPP stayed PID 1014 with zero restarts. Test fixtures clean only their
owned processes, interfaces, namespaces, databases and Valkey keys. Earlier physical
hardware results and limitations remain in [hardware report](hardware-2026-10-03.md).
Detailed native proof and remaining acceptance: [feature report](F-ikev2-native.md).
Evidence: [directory](ipsec-prerequisites-2026-10-03-evidence/).
