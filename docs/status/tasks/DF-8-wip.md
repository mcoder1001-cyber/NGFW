# DF-8 — WIP log

- 00:36 start (worker runs directly on the host). Read 00-CONTEXT, WORKER-OPS, DF-8 prompt, envelope, template,
  shared-host rules, host-vrx-a, LOG (D-055, D-060), descriptors README, scheduler/descriptor.go, vpp/{client,fake,vpptest},
  DF-4 as style reference. Enumerated binapi: dhcp, dhcp6_{ia_na,pd}_client_cp, dns, flowprobe, ipfix_export, sflow,
  http_static, bpf_trace_filter, lcp, interface (pcap_*). Missing: prom, tracenode, tracedump (trace_set_filters /
  trace_v2_dump), tracepath → questions file. Read VPP C handlers for semantics (dns enable needs a name server,
  flowprobe uses exporter 0 only, pcap file forced under /tmp, sflow dump returns hw_if_index, http_static has no disable).
