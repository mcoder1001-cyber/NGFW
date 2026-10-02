# Multi-WAN monitor independent R4 review

Reviewed local `f5962281289941ffd2c0b7a1c30e0165740e3436` (tree `0e7dbc5af6a287e13607a69a544b9ced494615bb`) against `c76774e8d2e04abee8ea7a301e64acbb3f794373`, in isolated `review/wan-r8` worktree. Read R4/shared-host and pending-handover instructions. No product edits or shared-host mutations.

No R4 blocker identified for this bounded monitor-only slice:

- No VPP API binding changes, invented API messages, VPP C code, data-plane object creation/deletion, global settings, packet tracing, or VPP/daemon restart.
- Production watcher consumes immutable last-authoritatively-saved WAN/interface snapshots; Apply persistence failure does not activate speculative monitoring. Old probe generation captures original device mapping while draining; new mapping invalidates old observations.
- Probes bind their socket and DNS resolution to the member's default-namespace Linux LCP device; missing interfaces and unsupported namespaces/VRFs do not fall back to unbound routes.
- RPC remains owner-scoped. It reports observed health only with empty Active; projection emits an unsupported-forwarding warning. No route/failover/NAT/ABF functionality is certified.
- Unit tests use fake VPP and net.Pipe peers, temporary state directories, context cancellation and cleanup. No new slot formulas, host interfaces, namespaces, shared databases or system services are created by the diff. Reviewer socket-construction check created no connection and sent no packets.

R8 separately blocks packaging compatibility: raw ICMP requires a capability absent from the installed service. See `multiwan-monitor-review-R8.md`. This R4 approval does not override that blocker or approve any privilege expansion.

Real LCP/VRF/namespace binding, VPP/agent restart integration, and forwarding packet-level acceptance were not run. Since this change installs no forwarding objects, absence of such deferred lab-only proof does not block this development slice. Final unchanged hosted quick remains mandatory.

Verdict: **APPROVE** for R4's monitor-only runtime integration scope; combined merge remains **BLOCK** under R8.
