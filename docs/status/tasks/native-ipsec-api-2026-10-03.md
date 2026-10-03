# Native route-based IPsec — production API secret lifecycle

The real API and product agent passed native IKEv2 profile commit, restart with simulated VPP profile loss, PSK rotation and revision rollback against a disposable VPP using the reviewed native readback plugin.

The fixture creates a private Ed25519 test licence/trust root for one IPsec tunnel, a throwaway slot-6 database, two random PSKs and clean API/agent process environments. The production entitlement guard is exercised: an unlicensed initial attempt was rejected. No licence bypass or external signing key is used. All fixture keys, sealed cache, process logs, database and the run's random Valkey prefix are removed after the run. Shared Valkey databases are never flushed.

## Verified production path

- `POST /secrets` creates `psk/site` version 1. API commit revision 1 delivers its actual value through the production secret resolver and encrypted agent cache. An in-memory VPP profile dump confirms the exact generated PSK bytes were installed.
- The agent stops and its owned native profile is deleted behind its back. Restart reopens the durable secret cache, recreates the profile with the exact original PSK and preserves the opaque snapshot identity.
- Replacing the secret creates version 2. Commit revision 2 installs the new PSK, pins version 2 and changes the active opaque snapshot.
- Rollback to revision 1 creates revision 3, reactivates secret version 1 and reinstalls the exact original PSK and snapshot identity.
- Safe API native-state reads succeed and drift is empty after each step. API responses, agent/API logs, audit and sealed-cache bytes contain none of either PSK in raw, hex-encoded or base64-encoded form; final shutdown logging is checked too.

Raw PSKs and CLI profile dumps stay in process memory. Evidence contains structural metadata and booleans only. No PSK values or fingerprints are retained.

This test proves profile configuration and secret lifecycle, and does not claim a negotiated SA or encrypted packet path. The separate native/strongSwan topology owns those tests. The IPIP interface has a configured transit address, logical name `site` and runtime instance `ipip6001`. MTU is omitted intentionally; the real creation default 9000 is now normalized without false drift.

An earlier attempt with omitted MTU was rolled back by verification (`interface.mtu/ipip6001 still present`); [sanitized failure](native-ipsec-api-2026-10-03-evidence/optional-mtu-failure.json) is retained. The drift agent fixed the specific IPIP creation default; the final production API run with omitted MTU passed. A separate real binapi probe established that a newly created IPIP interface reports `LinkMtu=65516` with creation vector `[9000,0,0,0]`; the existing nonzero-link default comparison expects `[65516,0,0,0]`. [Raw creation evidence](native-ipsec-api-2026-10-03-evidence/ipip-default-mtu.txt).

The final fixture also commits overlay route `10.6.20.0/24` with next-hop interface `site`. API running configuration and both routing-only and routing+tunnels agent Retrieve preserve this logical name. Real VPP FIB readback reports `198.18.63.2 ipip6001 (p2p)` at all four lifecycle phases. There is no duplicate generic `/interfaces/ipip6001` entry. Since this fixture intentionally has no peer or negotiated SA, its forwarding stack remains unresolved; this is configuration/readback proof, not packet proof.

## Reproduce

Build the product agent with `tools/heavy.sh go -C apps/agent build -o "$PWD/.scratch/restart-socket/vrx-agent" ./cmd/vrx-agent` and the current API dist. Build the reviewed IKEv2 plugin using the native agent's patch workflow, then run from the repository root:

```bash
eval "$(tools/lab env 6)"
export VRX_ISOLATED_PLUGIN_PATH="$PWD/.scratch/ikev2-patched/plugin:/usr/lib/x86_64-linux-gnu/vpp_plugins"
export VRX_IPSEC_API_AGENT_BIN="$PWD/.scratch/restart-socket/vrx-agent"
export VRX_IPSEC_API_EVIDENCE="$PWD/docs/status/tasks/native-ipsec-api-2026-10-03-evidence/lifecycle.json"
python3 test/topology/hardware-smoke/isolated-vpp.py python3 test/topology/ipsec/api-flow.py
```

[Lifecycle evidence](native-ipsec-api-2026-10-03-evidence/lifecycle.json) · [current tested-file hashes](native-ipsec-api-2026-10-03-evidence/builds.sha256) · [disposable VPP lifecycle](native-ipsec-api-2026-10-03.log).

Final extended run exited 0. The initial passing explicit-MTU secret lifecycle is preserved in `psk-lifecycle-explicit-mtu.json`; final `lifecycle.json` adds actual route text, selected Retrieve checks, omitted-MTU proof and exact-prefix Valkey cleanup. Shared system VPP stayed PID 1014, NRestarts 0. Slot 6 is free; its fixture database, private runtime and native API processes were cleaned. `python3 -m py_compile` and `git diff --check` passed.

Final revalidation also passed after removing unsupported native DPD/IKE settings and adding the native counter capability guard. The lifecycle evidence and tested artifact hashes were refreshed using the final product-agent and API/schema builds.
