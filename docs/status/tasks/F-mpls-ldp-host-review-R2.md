# F-mpls-ldp-host — R2 security review

Reviewed exact head: `339f400c1a137dcc0e988223d368b4ba97704420` (original source checkpoint `99218b6d`) against `main`. Independent reviewer; no product edits.

Findings: no remaining BLOCKER, MAJOR, MINOR or NIT security findings.

The preliminary input-budget concern is resolved: each show response has a 4 MiB limit, parsed RIB/LIB/neighbor/interface/adjacency/hop sets and translated bindings are limited to 10,000, adjacency lookup is indexed by peer, loops check cancellation, and dynamic reconciliation supports at most 256 distinct label routes. Oversized/unsupported reads fail instead of becoming an empty snapshot; cached forwarding has a bounded 60-second failure hold-down.

All daemon reads are constant argv show commands; router/peer/interface configuration tokens use existing FRR validators and the framework validates resolved passwords as a single WORD token. New state RPC checks the service owner and reports no password material. Named route records use the durable current-boot store; route/IP-bind mutation refuses LDP-held labels, and retrieval excludes the competing static scope. Logical interface removal/remap filters stale cached paths. Read-only lab helper validates pathspace, table and label; table-zero exception requires explicit read-only opt-in and documented shared locks. No new privilege/socket/auth boundary or dependency.

Verification executed on final product content:

```text
cd /workspace/scratch/e4f791ef53f7/ldp/apps/agent
/workspace/scratch/e4f791ef53f7/go/bin/go test -count=1 ./internal/frrsync/ldp ./internal/descriptors/mpls ./internal/subsystems -run 'Ldp|LDP|Named|Cache|Read'
ok ngfw/agent/internal/frrsync/ldp 0.188s
ok ngfw/agent/internal/descriptors/mpls 0.051s
ok ngfw/agent/internal/subsystems 0.144s
```

Gitleaks 8.30.1 with repository `.github/gitleaks.toml` scanned all changed tracked and untracked files copied to `/tmp/ngfw-r2-final/ldp`: exit 0, 105.90 KB, no leaks found. Final source-to-head delta adds only documented shared-lock commands with public example addresses. No live VPP/FRR acceptance was executed by R2 or implied by unit evidence.

Verdict: **APPROVE**.

Final delta verification: prospective labelRange filters cached labels; `go test -count=1 ./internal/subsystems -run TestLdpRegistrationAndConfigDependencies` executed at final source and passed (0.029s). No security boundary weakened.
