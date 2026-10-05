# D-236: Remote-access capability readiness

Manager decision, 2026-10-05. The product owner already approved the independent engine and full security/test scope. This clarifies existing additive capability semantics; no protocol shape, privilege or authentication change.

`operational` means the installed engine can safely activate its first profile. Requiring an already-active generation would create a cycle because the UI correctly disables activation while the engine is unavailable. Active generation and connected sessions are separate live observations.

Read-only preflight must verify the actual pinned engine artifact/ABI, root helper and fixed unit configuration, sealed credential resolver, required private-namespace/kernel capabilities, verified VPP connection/boot identity, compatible owned TAP IDs, and complete registered route/VRF/policy/descriptor lifecycle. A compiled callback alone is insufficient. Missing or unverified prerequisites yield false with bounded static reason; no daemon starts during capability lookup. A deployment with zero active profiles may be ready. Shared-host absence of the installed isolated engine remains false; disposable fixtures may inject their explicitly owned supervisor and prove first activation.

Activation still requires full observed outer and inner ingress/egress ACL handoff before credential snapshot/unit startup, exact daemon namespace/PID/start/VICI ownership, real configuration load and readback. Failure, rollback, restart and VPP boot changes stop the observed owned generation before repair. Source completion and independent security/full tests remain mandatory.

Alternatives considered: require active generation; report ready merely when callbacks exist; verified installed preflight. Selected preflight removes first-use circularity while preserving fail-closed activation. Reversal cost is one capability implementation and corresponding tests, within task scope.
