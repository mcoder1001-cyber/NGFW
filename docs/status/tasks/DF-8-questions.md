# DF-8 — questions for the manager (written while continuing; nothing waits on them)

## Q1 — missing plugins / binapi (manager-owned, D-014)
Verified on vrx-a 2026-09-24 (`/usr/share/vpp/api`, `/usr/lib/x86_64-linux-gnu/vpp_plugins`, `apps/agent/binapi`):

| DF-8 object | needs | state on vrx-a |
|---|---|---|
| trace-filter (`trace_set_filters`, `trace_set_filter_function`, `trace_filter_function_dump`, `trace_v2_dump`, `trace_capture_packets`, `trace_clear_capture`) | `tracedump` plugin (`src/plugins/tracedump/tracedump.api`) | not built/installed: no `.so`, no `.api.json`, no binapi package |
| tracenode-interface (`tracenode_enable_disable`) | `tracenode` plugin (`src/plugins/tracenode/tracenode.api`) | not built/installed |
| trace-path | 26.06 Trace Path (`src/plugins/tracepath`) | no `.api` at all — CLI only |
| prom-exporter | `prom` plugin | loaded, but **no `.api` file** in 26.06 — CLI/startup.conf only |

Options: (a) build + install `tracedump`/`tracenode` (vpp-plugin-devtools or the CMake targets), regenerate binapi on main,
then DF-8 follow-up adds `trace.filter` + `trace.tracenode-interface` (≈ 2 h); (b) drop these object types from D8.2 and
keep BPF filter + pcap (built). Recommendation: (a) for tracedump (trace_v2_dump is the only way to read packet traces
over the API), (b) for trace-path. For prom: (a) render `prom { … }` into startup.conf via F-startup-gen, (b) add a
prom .api on the VPP code track (V-item), (c) serve metrics from the agent's own stats-segment reader (P05/F-*).
Recommendation (c) now, (a) later if VPP's own exporter is wanted.

## Q2 — VPP bugs found (for docs/vpp-code-track.md; not crashes, D-064 does not apply)
1. `ipfix_classify_stream_details` / `ipfix_classify_table_details` are sent without `REPLY_MSG_ID_BASE`
   (`src/vnet/ipfix-export/flow_api.c` lines 335, 438) → clients receive an unrelated message id; the dump is empty.
   Host log: `No subscription found for the notification message. msgId=12`. DF-8 made both objects write-only.
   Fix: add `REPLY_MSG_ID_BASE` (two-line patch).
2. `lcp_itf_pair_get_v2` with sw_if_index ~0 replies with the v1 reply id (`lcp_api.c`) → the v2 client rejects it;
   DF-8 uses v1 `lcp_itf_pair_get`.
3. `sflow_interface_details` carries only `hw_if_index` and no API maps hw → sw index; DF-8 learns/probes (sflow.md).
   Fix: add `sw_if_index` to the details.
4. `show dns servers` prints the IPv6 list from the IPv4 vector (CLI only; API state correct).

## Q3 — http_static host run (decision needed, default = not run)
`http_static_enable_v5` cannot be undone (no disable API) and enables the session layer on the shared VPP until the
next restart. The full host run is gated behind `VRX_DF8_HTTP_STATIC=1`; by default only message compatibility is
checked. Options: (a) run it once during the next manager-owned VPP restart window; (b) keep it opt-in forever.
Recommendation (a).

## Q4 — DHCPv6 DUID host run
`dhcp6_duid_ll_set` changes a VPP-global without getter or reset; gated behind `VRX_DF8_DUID=1` (unit-tested with the fake).

## Q5 — shared helper package `internal/descriptors/dfkit`
The envelope lists `descriptors/<plugins of DF-8>/**`. Nine packages need the same codec/globals/boot-store/error
helpers, so they live in one new package `descriptors/dfkit` (+ `dfkit/dfkittest` for tests, `dfkit/restarttest` for the
restart simulation) instead of nine copies. Interface resolution and claims delegate to DF-1's `iface` (D-069/D-075).
No existing file is touched. Options: (a) keep as DF-8's package; (b) P05 promotes it to a shared `descriptors/kit`. Recommendation (a) now, (b) when a second factory wants it.

## Q6 — scheduler sentinel
`dfkit.ErrRetrieveUnsupported` has the same text as P05's `scheduler.ErrRetrieveUnsupported` (recognised by
`scheduler.IsRetrieveUnsupported`); once P05 is on main, DF-8 aliases it in a one-line follow-up.

## Q7 — dhcp relay ownership (D-071 wording)
The manager's message lists "dhcp proxy/global" among VPP-global settings. DF-8 keeps `dhcp.proxy`/`dhcp.proxy-vss` as
per-owner objects scoped by rx VRF (`dhcp.WithVRFScope`; production: every VRF, tests: a slot sub-range), because a relay
is configured per VRF table and the VRF range is the documented ownership unit; only the DHCPv6 DUID is in
`dhcp.RegisterGlobals`. Options: (a) keep per-VRF (current); (b) move the relays to `RegisterGlobals`. Recommendation (a).

## Q8 — persisted stores for P05/P08
Untagged-interface claims (`iface.Claims`, shared with DF-1) and the D-076 boot records (`dfkit.Boot`) are in memory by
default; P05/P08 install persisted ones: `iface.SetClaimStore(owner, …)`, `dfkit.SetBootStore(owner,
dfkit.NewFileBootStore(<state dir>/df8-boot.json))`.
