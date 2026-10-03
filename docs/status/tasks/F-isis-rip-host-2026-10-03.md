# F-isis-rip-host acceptance, 2026-10-03

**Final strict topology PASS154.20 s; implementation and host acceptance ready for review, no merge performed.** The two original host worktrees were inspected read-only; relevant strict harness was imported into this shared checkout. Root authorized and reviewed the minimal FRR corrections required to finish acceptance.

Command: `eval "$(tools/lab env 6)"; VRX_ISOLATED_TEST_RUN=1 VRX_ISOLATED_PLUGIN_PATH=$PWD/.scratch/lcp-multicast-netlink/plugin:/usr/lib/x86_64-linux-gnu/vpp_plugins unshare --net python3 test/topology/hardware-smoke/isolated-vpp.py test/topology/isis-rip/run.sh topology`

The dedicated mount and network namespaces isolate both VPP and root-netns zebra routes. Shared VPP remains PID1014, NRestarts0; system FRR remains inactive/disabled. No plugin/package was installed on the shared appliance.

- Fresh live RIP **PASS48.05 s**: 20 prefixes learned, withdrawal100ms, clean removal. FRR10.7.1 unsupported RIP JSON commands are recorded.
- Fresh live IS-IS **PASS58.70 s** after corrections: live adjacency parser, 20-prefix learning/withdrawal, full golden Apply/DryRun/removal.
- Focused `tools/heavy.sh go -C apps/agent test ./internal/renderers/frr/isis ./internal/renderers/frr`: **PASS**. Regressions preserve non-default metric20, omit canonical defaults, and require full observed convergence for circuit removal; residual circuits still fail and roll back.
- Real production API, signed slot-local test licence: incompatible L1 IS/L2 circuit validate and commit return400 application/problem+json with the circuit pointer; L1L2 control validates200. Running config unchanged; candidate, DB/role, processes, Valkey prefix and private licence keys cleaned. [API output](F-isis-rip-host-2026-10-03-evidence/api-400.txt).
- Final complete agent topology: 40 RIP routes in FRR and VPP FIB; Retrieve equals desired; unchanged apply is a no-op; IS-IS applies/removes through the production agent; OSI punt remains disabled with observed drop evidence.
- Agent restart after native pair loss recovers **2.8 s** (target30s). Post-restart LAN withdrawal **300ms**, reannouncement **800ms**, FRR/VPP restore all40 routes. Rollback **300ms** to0/0 routes, Retrieve has no RIP, rendered/running FRR configuration has no RIP. Cleanup leaves0 owned dynamic routes and removes all owned pairs/interfaces/processes/namespaces. Disposable VPP stops normally.

[Final strict output](F-isis-rip-host-2026-10-03-evidence/topology.txt); [binary/patch/harness hashes](F-isis-rip-host-2026-10-03-evidence/builds.sha256).

Fixes and diagnosis:

- FRR omits metric10, level1-2 and metric-stylewide from running-config. Renderer now emits their canonical omission, retaining strict diff checks. Complete IS-IS removal can make FRR reject redundant circuit deletions after it already removed the circuit; recovery requires an independently empty full DryRun, scoped to removing all prior IS-IS. Root reviewed this correction.
- Stock Linux-NL loses the LAN plugin-low224/24 Accept after pair recreation. Both tap addresses/multicast membership and physical SPECIAL224.0.0.1/32+224.0.0.2/32 paths remain valid. Stock strict failures453.33s/454.52s and diagnostic-only SKIP73.63s are retained. Diagnostic-only mode never counts as acceptance.
- Reviewed private Linux-NL patch0003/V27 reconciles per-interface plugin-owned multicast lifecycle. Native agent's dedicated stock-fail/patched-pass IPv4/IPv6/multiple-address/last-address/pair/foreign-path proof complements this final real RIP forwarding proof. No API-source shadow or fixture CLI Accept was added.
- First patched lifecycle run proved recovery/rollback but failed an obsolete fixture expectation after a direct-FRR workaround. Fixture now removes IS-IS through the agent and verifies a subsequent unchanged apply is a no-op. Final complete strict run passes.

Recommendation: `F-isis-rip-host` → review; no merge claim. Full IS-IS adjacency through VPP still requires the separately gated OSI-punt decision; scoped direct FRR adjacency is proven, and the requested disabled-punt alternative evidence is complete. Integrated screenshots/broad CI are not claimed.
