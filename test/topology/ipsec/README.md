# Topology test — route-based IPsec site-to-site

Current scope: [DEC-ipsec-route-based](../../../docs/decisions/DEC-ipsec-route-based.md), 2026-10-03. Implement only route-based IPsec. This procedure replaces the former kernel-vpp policy-tunnel test.

## Rig

`LAN namespace → VPP route → protected IPIP interface → ESP/NAT-T underlay → IKEv2 peer → remote LAN namespace`, with traffic in both directions.

NGFW side uses VPP native IKEv2 and the agent (`engine: vpp-ikev2`, `routeBased.ipipInterface`). The peer may use stock strongSwan with kernel XFRM or another disposable VPP. Use a disposable VPP, dedicated sockets, slot-prefixed interfaces/VRFs and test-only secrets. Configure matching IKEv2 proposals, identities and traffic selectors. Keep overlay routing separate from the underlay route to the peer. Never restart the shared VPP, alter management NICs or install peer packages as host services.

## Required executable checks

1. Preflight the test interfaces, create the rig and record the shared VPP PID/NRestarts. Declare the tunnel, VRFs, interface addresses and static routes through the configuration API; validate rejects policy-only, transport or missing interface binding.
2. Negotiate IKEv2 and CHILD_SAs. Show the tunnel interface, the IKE profile's interface binding, inbound/outbound protection/SAs, route/FIB and counters. Observe plugin-owned SA IDs rather than allocating competing agent SAs.
3. Send ICMP and TCP in both directions between inner networks; compare TCP payload. Capture the underlay: protected data is ESP or NAT-T, never inner plaintext; ordinary IKE/ARP/ND control traffic is allowed.
4. Remove an inner route and prove that prefix no longer enters the tunnel; restore it and prove recovery. Check overlay and underlay VRF separation.
5. Exercise rekey and peer loss/recovery. Restart only the test agent and verify declarative recovery without manual SA repair; record interruption and negotiated SA/protection state.
6. Roll back: remove owned routes, IKE profile, protection/SAs and tunnel objects in dependency order. Verify no owned objects, processes or namespaces remain; stop only PIDs started by the test. Confirm shared VPP PID/NRestarts are unchanged.
7. Check state/API/CLI/UI and redact all secret material. Report unavailable product secret delivery as a blocker; a fixture does not prove the product secret channel.

## Status

The native route-based production-agent campaign is executable through `run.sh`; its PSK packet, peer-loss, restart and rollback acceptance passed on a disposable VPP. Full quick and independent review remain merge gates. P11-pkg/kernel-vpp is no longer a prerequisite for this topology.

## Native route-based production agent proof

The native module uses VPP IKEv2, with stock strongSwan only as the peer. It
requires product patch 0002; the shared host's unpatched plugin is deliberately
refused. Never replace the shared plugin or restart the shared VPP for this test.

With the pinned reference source/build available at `/root/vpp`, build a copied
API source and disposable plugin without modifying that reference:

```bash
tools/heavy.sh python3 test/topology/ipsec/build-native-plugin.py \
  --output "$PWD/.scratch/native-proof-plugin"
tools/heavy.sh go -C apps/agent build -o "$PWD/.scratch/native-product-agent" ./cmd/ngfw-agent
eval "$(tools/lab env 8)"
NGFW_INTEGRATION=1 NGFW_NATIVE_INITIATOR=1 NGFW_NATIVE_PEER_LOSS=1 \
NGFW_NATIVE_AGENT_BIN="$PWD/.scratch/native-product-agent" \
NGFW_NATIVE_EVIDENCE_DIR="$PWD/.scratch/native-proof-evidence" \
NGFW_ISOLATED_PLUGIN_PATH="$PWD/.scratch/native-proof-plugin/plugin:/usr/lib/x86_64-linux-gnu/vpp_plugins" \
tools/heavy.sh python3 test/topology/hardware-smoke/isolated-vpp.py \
  go -C apps/agent test ./internal/desired -run '^TestIKEv2NativePackets$' -v -count=1 -timeout 4m
```

The topology uses only slot-8 namespaces, veths and private VPP sockets. Stock
peer packages must already be extracted for the swantest harness (the stock-root
instructions above); the test currently also accepts the existing slot-10 stock
root. Set `NGFW_NATIVE_INITIATOR=0` for native responder and peer-triggered rekey.
Omit `NGFW_NATIVE_PEER_LOSS` for the faster packet/reconciliation proof.

`NGFW_NATIVE_AGENT_BIN` selects the production path: agent startup, sealed secret
cache, gRPC Apply with a separate secret bundle, declarative host/IPIP interfaces
and routes, native runtime actions and safe state. It proves bidirectional ICMP,
exact 1 MiB TCP, counters, foreign-SPI refusal, rekey, active-SA preservation
across actual agent restart/initial Resync, route withdrawal/recovery and
fail-closed admin state before SA installation and after deletion. Underlay
capture must contain ESP and no plaintext IPIP throughout. The optional hard peer
crash phase exercises unmodified VPP liveness defaults and peer recovery.

Without `NGFW_NATIVE_AGENT_BIN`, the test is a descriptor/interoperability fixture
and does not establish production secret delivery or reconciliation support.
Product package builds use `deploy/vpp/build.sh`, not the disposable linker helper.

## Reproducible P11-host campaign

The former placeholder `run.sh` now runs `host-acceptance.py`. Set
`NGFW_INTEGRATION=1` and use `run.sh --peer-loss` for both native responder and
initiator with production sealed-secret delivery, default peer-loss recovery and
explicit owned rollback before disposable VPP shutdown. The driver builds its own
plugin and agent, uses private namespaces, refuses existing slot8 resources and
checks shared VPP MainPID/NRestarts. No stock peer packages are installed as host
services; the already extracted `swantest` peer root is required.

For a repeat using the exact same product build, `--skip-build --agent PATH
--plugin-directory PATH` selects those artifacts explicitly. This does not prove
a different agent build. Raw failure logs stay0600 in worktree `.scratch`; share
only the safe JSON summary and redact any manually inspected peer/SA output.
Skipped tests cannot pass the driver. The committed PSK campaign summary is in
`docs/status/tasks/P11-host-evidence/native-production-summary.json`.
