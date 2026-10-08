# PPP DHCPv6 delegated LAN integration

Branch `codex/pppoe-pd-20261008`, base `db7eb9d`.

Contract checkpoint: additive explicit `delegationTargets` with LAN interface and
uint32 subnet ID; DHCPv6-only, enabled LAN, VRF, static IPv6/RA and duplicate owner
validation. Generated Go/TS use the repository generator with pinned CI versions.
A numeric uint64 design failed the JSON roundtrip test (ts-proto correctly emits
uint64 strings); uint32 preserves the existing numeric configuration contract.
Schema-driven interface form exposes targets, with nested English/Persian labels.

Focused schema tests: 10 PASS. Wire roundtrip test being rerun after uint32 fix.
No aggregate CI or host operation. This contract is not operational PD completion.

Remaining: enforce kernel carrier parent topology semantic rules; authoritative
lease generation/deadline parsing and tests; dynamic desired projection; durable
separate descriptor ownership/static exclusion; live readiness callback integration;
withdraw/renewal/rollback/restart regression tests; independent review.

DHCPv6 export names verified from upstream maintainer example:
https://github.com/NetworkConfiguration/dhcpcd/discussions/309
`new_dhcp6_ia_pd1_prefix1_pltime` and `_vltime` are used with existing new-event prefix.
Missing/invalid metadata must remove old PD, not extend an old lease. The existing
IPv6 admission token is 64 hex characters. The refresh generation alone does not
identify the configured carrier generation.
