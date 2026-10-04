# FRR PIM and mFIB synchronization

The existing `routing.multicast.pim` contract renders each enabled logical interface through its LCP host mapping, adding `ip pim` to the framework's single interface block. Static RP entries render `ip pim rp <address> <multicast-prefix>`; empty groups select 224.0.0.0/4. Unsafe addresses/interfaces, duplicate mappings, duplicate RP ranges and noncanonical/nonmulticast prefixes fail before applying files. The existing FRR stage runs its staged checker before VPP operations.

The production runtime registers dynamic source `pim` with exclusive descriptor `mfib.route.pim`, outside configuration Domains. Every second it reads the fixed shell-free `show ip mroute json` command from this agent's FRR instance, requiring a pimd vty socket. The adapter follows FRR 10.x `pimd/pim_cmd_common.c`: group/source keys, source/group fields, installed integer, iif, and oil objects with inboundInterface/outboundInterface/ttl. Parser input is bounded (4 MiB and 10,000 records). Runtime snapshots support at most 256 routes; larger valid snapshots are rejected before replacing the cache, preserving the last supported forwarding snapshot. This conservative limit applies to all observed records, including uninstalled ones. Errors preserve the cached snapshot, and a successful empty object withdraws prior routes. The daemon uses a separate process; no FRR code is linked.

Source-specific entries become (S,G), source 0.0.0.0 becomes (*,G). The IIF maps to an accept path and each active OIL interface maps to a forward path. Missing/ambiguous mappings or disabled/removed PIM interfaces suppress that route in the same configuration transaction. The cache does no VPP I/O; seam S1 runs the scheduler under its transaction lock. It retries sync on every successful poll, including unchanged snapshots, and existing reconnect reconciliation repairs lost routes. A separate named MFIB ownership record prevents static/dynamic adoption and overwriting.

The contract has no PIM VRF selector: synchronization targets table zero only. A product agent must explicitly own all IDs (`NGFW_VPP_ID_RANGE=all`). Numbered lab ranges register no PIM source and cannot mutate table zero. Do not widen a lab range to test a shared host; forwarding topology acceptance needs a dedicated box approved for global ownership.

Example configuration:

```json
{"routing":{"multicast":{"pim":{"interfaces":["uplink","lan"],"rp":[{"address":"10.0.0.1","groups":["239.0.0.0/8"]}]}}}}
```

Both interfaces need valid LCP mappings. Equivalent FRR diagnostics: `show running-config`, `show ip mroute json`; VPP diagnostic: `show ip mfib`. No user-provided show command reaches a shell.

Run unit verification with `go test -race ./internal/renderers/frr/pim ./internal/frrsync/pim ./internal/descriptors/mfib` from apps/agent. The opt-in `TestPimdScopedHarness` starts child pimd in an owned namespace, checks staged config/apply/read/rollback, and cleans its PIDs through frrtest. It is not a VPP forwarding test. Actual peer adjacency, packet forwarding and <=30s recreation after simulated loss require the real lab and remain explicitly deferred until recorded.


Scale debt: existing MFIB Create/Delete checks dump the shared table per route. The 256-route cap bounds feature-induced churn and is not a throughput guarantee; unrelated shared-table entries remain unbounded by this cap. The manager accepted follow-up work for safely batched conflict checks and bounded shared-table snapshots, owner Codex manager, due 2026-10-11. Ownership/existence safety is retained; see F-pim-frrsync-debt.md.

Operator diagnostics use the existing event stream: the first failed observation or scheduler sync produces EVENT_KIND_ERROR with a fixed readable message and component/source=pim, status=degraded. It also emits a structured WARN at normal log levels. Repeated failures within the same outage produce no additional source warnings/events; errors contain no raw daemon/parser payloads. Only a successful observation and scheduler sync clears the condition, emits a recovered routing event/INFO log, and rearms notification for a later failure. Shutdown cancellation is not reported as an outage. This covers daemon/socket, parser, runtime cap and scheduler failures; no new API contract is added.
