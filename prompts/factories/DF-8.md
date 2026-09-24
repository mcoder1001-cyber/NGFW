# Task: DF-8 — Descriptors for VPP plugins: dhcp, dns, flowprobe, sflow, prom, pcap/tracenode, lcp [skip-unless-loaded]   (prepend 00-CONTEXT.md)

## Goal
Write the reconciler **descriptors** (Create/Update/Delete/Retrieve/Dependencies) for VPP's host-service and observability object types —
DHCP relay/client (WBS D7.2), the VPP caching DNS plugin (D7.3), IPFIX/flowprobe + sFlow (D7.6), the Prometheus `prom` exporter (D8.2/D8.3),
packet capture / trace (D8.2) and the linux-cp interface pairs (D3.1) — against the scheduler interface published by P05a and the generated
bindings in `apps/agent/binapi/`. No API, no UI — pure agent-side building blocks that P12 (linux-cp), RF-3 (Kea/Unbound) and the
observability F-* tasks wire up later. `linux_cp`/`linux_nl` are **not loaded on the host**: write the code, integration test = skip-unless-plugin-loaded.

## Inputs to read first
- `apps/agent/internal/scheduler/descriptor.go` (P05a — the interface; do not change it, request changes via a question file)
- `apps/agent/internal/descriptors/README.md` and one merged example (e.g. `interface/`) — key scheme `interface/<name>`, `vrf/<id>`
- `apps/agent/binapi/dhcp/`, `binapi/dns/`, `binapi/flowprobe/`, `binapi/ipfix_export/`, `binapi/sflow/`, `binapi/prom/`, `binapi/http_static/`, `binapi/pcap/`,
  `binapi/tracenode/`, `binapi/bpf_trace_filter/`, `binapi/trace/` (vlib trace), `binapi/tracepath` or similar (26.06 Trace Path — find the real name), `binapi/lcp/`,
  `binapi/linux_nl/` (if it has an API at all) — **the only source of message names and fields**; verify every name below, never guess
- `prompts/P12-frr-linuxcp.md` — the consumer of `lcp-itf-pair` (`lcp_itf_pair_add_del_v2`/newest) and of the skip pattern
- VPP 26.06 docs: https://s3-docs.fd.io/vpp/26.06/ (dhcp, dns, flowprobe, ipfix, sflow, prom, http_static, pcap/trace, tracenode, linux-cp)
- `docs/lab/host-vrx-a.md` — **not loaded:** `linux_cp_plugin.so`, `linux_nl_plugin.so` (also `ip6_dad_autoremove`, irrelevant here). Skip predicate: govpp
  `CheckCompatiblity` on the plugin's messages returns unknown-message → `t.Skip("plugin not loaded: …")`. Everything else here is loaded.
- `docs/lab/shared-host-rules.md` — prefix `w<N>`, addresses `10.<N>.0.0/16`, tap/netdev names `w<N>-*`, ports from your slot, files only under `/run/vrx-test/w<N>/`

## Scope — build exactly this
Enumerate the object types in `docs/status/tasks/DF-8.md` first, then build (estimate: 24 object types × Create/Update/Delete/Retrieve
+ unit test with the fake client + one integration check under the shared lock with your slot prefix):

### Object types per plugin (binapi package → messages to look for)
- **dhcp** (`binapi/dhcp`): `dhcp-proxy` (dhcp_proxy_config: server + src address, rx vrf, server vrf, ip4/ip6 — relay per vrf), `dhcp-proxy-vss`
  (dhcp_proxy_set_vss: vss type, vpn ascii/oui/id), `dhcp-client` (dhcp_client_config: interface, hostname `w<N>-*`, client id, set_broadcast_flag, pid/
  want_dhcp_event — Retrieve dhcp_client_dump; lease state via dhcp_compl_event → `StreamEvents`), `dhcp6-client` (dhcp6_client_enable_disable),
  `dhcp6-pd-client` (dhcp6_pd_client_enable_disable + dhcp6_duid_ll_set), `dhcp6-pd-address` (ip6_add_del_address_using_prefix: interface, prefix group,
  address, plen). Retrieve: dhcp_proxy_dump (ip4 + ip6), dhcp_client_dump; dhcp6 clients have no dump → `partial`, questions file. Events:
  want_dhcp6_reply_events / want_dhcp6_pd_reply_events. dhcp6_send_client_message / dhcp6_pd_send_client_message = action helpers.
- **dns** (`binapi/dns`): `dns-enable` (dns_enable_disable — global singleton; read/restore), `dns-name-server` (dns_name_server_add_del: ip4/ip6 upstream
  in `10.<N>…` for tests). Retrieve: no dump exists → `partial`, name it in `DF-8-questions.md`. dns_resolve_name / dns_resolve_ip = action helpers.
- **ipfix / flowprobe** (`binapi/ipfix_export`, `binapi/flowprobe`): `ipfix-exporter` (set_ipfix_exporter_v2 or ipfix_exporter_create_delete for multiple
  exporters — verify: collector `10.<N>…`:port, src address, vrf, path mtu, template interval, udp checksum; Retrieve ipfix_exporter_dump / ipfix_all_exporter_get),
  `ipfix-classify-stream` (set_ipfix_classify_stream + ipfix_classify_table_add_del — depends on DF-2 classify tables; Retrieve ipfix_classify_stream_dump,
  ipfix_classify_table_dump), `flowprobe-params` (flowprobe_set_params or flowprobe_params: record l2/l3/l4, active/passive timers — global singleton; Retrieve
  flowprobe_get_params), `flowprobe-interface` (flowprobe_interface_add_del: interface, which ip4/ip6/l2, direction rx/tx/both; Retrieve flowprobe_interface_dump).
- **sflow** (`binapi/sflow`): `sflow-global` (composite over sflow_sampling_rate_set, sflow_polling_interval_set, sflow_header_bytes_set, sflow_direction_set,
  sflow_drop_monitoring_set — global singleton; Retrieve the matching `*_get`), `sflow-interface` (sflow_enable_disable per interface; Retrieve sflow_interface_dump).
  Export to a collector is hsflowd's job — out of scope.
- **prom** (`binapi/prom`, `binapi/http_static`): `http-static-server` (http_static_enable_v4 or newest: listen `127.0.0.1:$((VRX_METRICS_PORT+1))` in tests, www root
  under `/run/vrx-test/w<N>/`, fifo sizes, cache, max age — **global singleton per VPP**: if already enabled by another slot, Retrieve reports it and the
  test skips with the reason; never disable a server you did not enable), `prom-exporter` (prom_enable_disable or the generated equivalent: stats patterns,
  scrape interval, used-only flag). Retrieve: check for dumps; if none, `partial` + questions file. The agent's own metrics (P05) stay separate.
- **pcap / trace** (`binapi/pcap`, `binapi/trace`, `binapi/tracenode`, `binapi/bpf_trace_filter`): `pcap-capture` (pcap_trace_on / pcap_trace_off: rx/tx/drop, interface or
  any, max packets, max bytes per packet, filter flag, filename under `/run/vrx-test/w<N>/` — **one global capture per VPP**: Retrieve first and skip if another
  capture is active), `trace-filter` (trace_set_filters / trace_set_filter_function; Retrieve trace_filter_function_dump), `bpf-trace-filter`
  (bpf_trace_filter_set_v2: cBPF/pcap expression compiled by VPP — expression is user input: validate charset and length, never shell), `tracenode-interface`
  (tracenode_enable_disable: interface, direction), `trace-path` (26.06 Trace Path plugin — find the binapi package; if none, questions file).
  Retrieve: pcap dump if present, else `partial`; trace_v2_dump = state (Retrieve-only). trace_capture_packets / trace_clear_capture = action helpers,
  clear only what you started.
- **lcp** (`binapi/lcp`; plugin **not loaded**): `lcp-default-netns` (lcp_default_ns_set / lcp_default_ns_get — global singleton), `lcp-itf-pair`
  (lcp_itf_pair_add_del_v2 or v3: sw_if_index, host if name `w<N>-*`, host if type tap/tun, netns; Retrieve lcp_itf_pair_get / lcp_itf_pair_get_v2),
  `lcp-replace` (lcp_itf_pair_replace_begin / _end as a transaction helper, not a descriptor). `linux_nl`: usually no API — document. Integration tests:
  skip-unless-plugin-loaded; unit tests with the fake are mandatory and must cover create/update/delete/Retrieve decoding.

### Dependencies (declare in `Dependencies()`, test ordering with the fake)
dhcp-proxy → `vrf/<id>` (rx + server) · dhcp-proxy-vss → dhcp-proxy · dhcp-client / dhcp6-client / dhcp6-pd-client → interface · dhcp6-pd-address → interface +
dhcp6-pd-client · dns-name-server → dns-enable · ipfix-exporter → `vrf/<id>` (Optional) · ipfix-classify-stream → ipfix-exporter + `classify-table/…` (DF-2 key;
fixture until merged) · flowprobe-interface → interface + flowprobe-params + ipfix-exporter · sflow-interface → interface + sflow-global (Optional) ·
prom-exporter → http-static-server · pcap-capture → interface (Optional, when not "any") · tracenode-interface → interface · bpf-trace-filter → none ·
lcp-itf-pair → interface + lcp-default-netns (Optional).

### Deliverables per object type
1. Descriptor in `apps/agent/internal/descriptors/<plugin>/<object>.go`: `KeyOf`, `Dependencies`, `Create`, `Update` (or `ErrRecreate`), `Delete`,
   `Retrieve` (full dump, decoded into the same proto type used for desired state, including metadata such as sw_if_index / exporter index).
2. Registration in the plugin's `Register(scheduler)` function; add to the descriptor registry list.
3. Unit tests with the fake VPP client (table-driven: create, idempotent re-apply, update, delete, dependency ordering, Retrieve decoding).
4. Integration test against the host VPP (`/run/vpp/api.sock`, `VRX_INTEGRATION=1`, `flock -s /run/lock/vrx-lab.lock`): create → Retrieve shows it →
   delete → Retrieve shows nothing. **Every object name/tag/table id carries your `VRX_TEST_PREFIX` / slot range**; Retrieve-based assertions filter
   by your prefix (other workers' objects exist on the same VPP). Use prefixed loopbacks/tables; never touch `local0`, `ens192` or anything unprefixed;
   clean up in `t.Cleanup`. Global singletons (dns enable, flowprobe params, sflow globals, http_static, pcap, lcp netns) are read first, restored
   after, and skipped with a reason when another slot holds them. Capture files and www roots live under `/run/vrx-test/w<N>/` and are removed.
5. `docs/agent/descriptors/<plugin>.md`: table object type ↔ VPP messages ↔ notes/limitations (every `partial`, every singleton, the skip rule).

## Rules
- Message names come from binapi. `apps/agent/binapi/` is **manager-owned** (generated for all plugins by P04, D-014): if a message is missing, write
  `docs/status/tasks/DF-8-questions.md` naming the plugin and continue with the rest; never edit `tools/binapi-gen.sh` or `apps/agent/binapi/` in your branch.
- Retrieve must decode *everything* the diff needs; a descriptor without Retrieve is not done — `partial` only where VPP has no dump, documented.
- No shelling out to `vppctl`. No C. No changes to `startup.conf` (enabling linux_cp/npt66 there is the manager's job after handover). No VPP restarts (D-012).

## Acceptance (paste the evidence)
- [ ] `go test ./internal/descriptors/{dhcp,dns,ipfix,flowprobe,sflow,prom,pcap,trace,lcp}/...` green (unit + integration on the host)
- [ ] `grep -rn "vppctl\|exec.Command" internal/descriptors/{dhcp,dns,ipfix,flowprobe,sflow,prom,pcap,trace,lcp}` is empty
- [ ] Applying the same desired state twice yields an empty plan (log excerpt)
- [ ] Object ↔ message table committed; `lcp` integration output shows `skip-unless-plugin-loaded` with the reason; lcp unit tests green with the fake
- [ ] `vppctl show dhcp proxy` / `show flowprobe params` / `show sflow` / `show ipfix exporter` / `show pcap trace` pasted for your prefixed objects, then clean after delete

## Out of scope (do not build)
API endpoints, UI screens, schema changes, F-*/P12 feature wiring, performance, startup.conf changes (plugin enable blocks). Not yours: Kea DHCP server,
Unbound, chrony renderers (RF-3), FRR/linux-nl route sync behaviour and LCP pair lifecycle policy (P12), hsflowd/collector setup, Grafana dashboards and
the agent's own Prometheus metrics (P05/F-*), G2/offline trace tooling, classify tables (DF-2), NAT IPFIX logging toggles (DF-3), interfaces (DF-1).
No binapi regeneration, no VPP restart, no `local0`, no global `trace clear`/`pcap` interference with another slot's capture.
