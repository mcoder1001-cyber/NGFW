# P02b — WIP

- 13:35 read 00-CONTEXT, WORKER-OPS, P02 prompt, docs/04, LOG D-017..D-024, vdom.md, WBS D4/D5, F-nat44-ed-sessions;
  pulled worktree, `pnpm install` on host (zod 4.6.5, vitest 3.2.7). Probed Zod 4: enum discriminators, record key
  patterns, refine (ignored in JSON Schema), nested `.prefault({})` — all fine.
- 13:50 design fixed: `nat` top level = NAT44 (docs/04 + F-nat44 shape: mode ed|ei, inside/outside, pools, staticMappings,
  timeouts, sessionLimit) + siblings nat64/nat66/nptv6/det44/dslite/map/cnat/ipfix; `objects` = addresses, addressGroups,
  services, serviceGroups, schedules, zones, tags; `acl` = lists, macip, host, attachments, macipAttachments, hostAttachments.
  Local primitives (ipPrefix, l4Port, l4PortRange, ipv4AddressRange, timeOfDay …) live in domains/objects.ts / nat.ts
  because P02a owns primitives.ts. Writing domains next.
