# WAN integration verification, 2026-10-04

Integration preserves PR151 tunnel/IS-IS/native events and adds the independently reviewed WAN tree through local fb8bb4780. Native reviewer approves WAN and integration delta through 3d914914a; report wan-independent-review-20261004.md.

Actual focused integrated Go race: agent5.124s, multiwan1.217s, natcommon1.108s PASS. An initial filter matched no core/nat44ed tests; complete core/natcommon/nat44ed packages subsequently PASS1.413/1.124/1.178s. Final optional priority fix independently tested by developer (multiwan race2.136s). Dependency API build8/8 PASS1m46s; normal API-client generation PASS with no tracked drift. Final optional-priority regression PASS1.132s in integration. Source guard PASS11s, 52.31KB scanned, no leaks. Public certificate checksum wording correction avoids a scanner false positive without suppressing scanner rules.

Supported forwarding/probing scope and actual source limitations remain in docs/user/network/multi-wan.md: static gateway, default namespace/defaultVRF probes; unsupported VRF/netns refuse observation and forwarding; DHCP/PPPoE gateway handoff excluded with explicit warning. Lab weighted-flow distribution/affinity, packet failover/restart/rollback NOTRUN. No shared VPP or host policy changed. Existing owner full/hosted CI waiver retained.
