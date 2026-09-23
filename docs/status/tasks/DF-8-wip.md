# DF-8 — WIP log

- 00:36 start (worker runs directly on the host). Read 00-CONTEXT, WORKER-OPS, DF-8 prompt, envelope, template,
  shared-host rules, host-vrx-a, LOG (D-055, D-060), descriptors README, scheduler/descriptor.go, vpp/{client,fake,vpptest},
  DF-4 as style reference. Enumerated binapi: dhcp, dhcp6_{ia_na,pd}_client_cp, dns, flowprobe, ipfix_export, sflow,
  http_static, bpf_trace_filter, lcp, interface (pcap_*). Missing: prom, tracenode, tracedump (trace_set_filters /
  trace_v2_dump), tracepath → questions file. Read VPP C handlers for semantics (dns enable needs a name server,
  flowprobe uses exporter 0 only, pcap file forced under /tmp, sflow dump returns hw_if_index, http_static has no disable).
- 00:42–00:56 dhcp, dns, ipfix, flowprobe, sflow, lcp, pcap, trace, prom descriptors with unit + host tests; evidence
  captured; found VPP bugs (ipfix classify details msg id, lcp get_v2 reply id, sflow hw index, dns CLI).
- 00:58 manager: D-063/D-064/D-065 → write-only via ErrRetrieveUnsupported (dropped the applied cache), NRestarts checks.
- 01:10 manager: D-069/D-071/D-074/D-076 → merged main (DF-1 iface), logical names + shared ClaimStore, RegisterGlobals,
  index re-verify before delete, boot-identity records (pcap/http_static), dedupe, restart simulation (host green).
- 01:25 docs + DF-8.md; lint 0 issues; CI running.
