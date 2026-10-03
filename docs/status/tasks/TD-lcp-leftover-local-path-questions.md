# TD-lcp-leftover-local-path — current integration boundaries

The reviewed leftover API local-path fix does not repair a missing default-namespace `plugin-low` Accept. The independent ISIS/RIP restart topology captured a recreated LAN pair with correct tap IPv4/multicast membership but no LAN Accept in the plugin-owned `(*,224.0.0.0/24)` paths.

Adding only that LAN path through `ip_mroute_add_del` would create the higher-priority API source, shadow the remaining plugin-low WAN paths and any other owner's default pairs; MFIB does not inherit paths across sources. D-217 deliberately restricts this API ownership to tables without default-namespace pairs. Such a workaround is unsafe and was not added.

Pinned VPP `lcp_router.c` updates IPv4 Accepts only from Linux address notifications; its pair-delete callback removes IPv6 paths only, and the registered callback has no pair-add reconciliation. Missing plugin-low paths require a separately reviewed plugin lifecycle repair or a new explicit whole-table ownership design. The exact restart trigger still needs per-source MFIB and IPv4-enabled-state evidence; no C fix or root-cause certainty is claimed here.

This separate upstream/runtime issue does not invalidate the passing local-path cleanup, mixed-table refusal, error recovery or desired Drift rejection tests. Current integration review, finishing CI and actual merge remain outstanding.
