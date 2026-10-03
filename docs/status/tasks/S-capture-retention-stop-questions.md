# Capture remaining capability and release checks

The pinned interface binary API exposes pcap_trace_on/off and filter configuration,
without a read-only capture-count/status getter. Packet-limit recording is enforced by
VPP, but early agent finalization at that limit cannot be implemented through this API.
The timer and explicit Stop remain; this is the fallback allowed by prompt item 3.

Capture temporary-file permissions (TD-H25), buffered download (TD-H24), the real
capture host/API acceptance and screenshots remain separate tasks. Main appliance
configuration was not changed. Full merge CI and actual merge are not inferred from
focused tests on this dirty integration worktree.
