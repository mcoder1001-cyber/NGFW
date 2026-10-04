# F-tunnels — source completion, 2026-10-04

Branch `codex/next-tunnels-20261004`, base `dfd001be4`. Recovery reused existing GRE/IPIP/VXLAN, sixrd, VXLAN-GPE/GTP-U/L2TPv3/PPPoE schema/projection/descriptors and owner-filtered TunnelState RPC. No existing advanced implementation was rebuilt.

Completed gaps:
- Additive protobuf optional live endpoints, underlay FIB, separate IPv4/IPv6 interface FIBs, nullable interface counters and readback availability notes.
- Agent enriches only owner-tagged interfaces from live descriptor dumps, FIB readback and stats. 6RD parameters remain unknown because VPP has no getter. Missing stats remain absent; stats with mismatched interface names are excluded.
- Protected GET `/api/v1/state/tunnels` through AgentClient; nullable fields retained and uint64 counters remain decimal strings.
- UI uses config-name plus kind to find actual engine allocations; all four primary kinds, advanced GTP-U/L2TPv3/PPPoE toggle, localized limits and live endpoint/FIB/counter columns. Candidate edits never determine live interface names.
- L2TP no-delete and GTP-U dump-first guards preserved. PPPoE session editor distinguished from ISP dialer.

Checks: focused agent/descriptor Go suites pass (no shared VPP mutation); API 4 tests pass; UI 6 tests pass; API/web typechecks pass; targeted lint and proto lint pass. Contract generation succeeds; combined API-client regeneration is owned by the manager. Full CI waived by user.

Source ready for independent review and integration; no merge claimed. Deferred laboratory acceptance: packet transfers, actual VPP restart persistence, provisioned appliance screenshots, GTP-U opt-in host mutation and irreversible L2TPv3 creation. These deferrals are not source failures.

Publication evidence and exact frozen commit are in F-tunnels-wip.md. Historic implementation and tests remain in Git history.
