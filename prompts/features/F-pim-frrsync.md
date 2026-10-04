# F-pim-frrsync — FRR PIM to VPP multicast

Read 00-CONTEXT.md, AGENTS.md, contributing and decision policy. This prompt specializes FEATURE-TEMPLATE for the exact existing board row.

Reuse existing routing.multicast.pim proto/schema and FRR framework; render static RP group ranges and mapped PIM interface commands. Read bounded fixed show ip mroute json command, validate FRR group/source/OIL observations and translate enabled logical LCP interfaces to mFIB accept/forward paths. Cache successful snapshots, preserve prior state on read/parse failure, and synchronize through seam S1 only. Register an exclusive descriptor outside config Domains so static multicast entries cannot be overwritten/adopted by dynamic routes.

Tests must cover unsafe tokens, deterministic rendering, source-specific/wildcard groups, failure preservation, successful withdrawal, configuration dependency deletion, owned descriptor isolation, and global-table scope. Add opt-in child pimd harness using frrtest. Document default-VRF limits and deferred actual topology/forwarding/restart acceptance.

Out of scope: new contracts, UI/API changes, BIER, IPv6 PIM, VRF PIM, direct VPP writes in poller, real-host daemon ownership changes.
