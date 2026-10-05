# RA production controller work in progress

Branch: codex/ra-controller-20261005. Worktree: /dev/shm/ra-controller-20261005.
Base: engine f0651aae295b86e8bd5387fe018bbd77f4608464 plus API/UI 8ec352cf584bd17a7ce2a3558169592260fb66b8.

Owned: new internal/ra_vpn controller/runtime/engine_descriptor/handoff_verifier/unit_supervisor and tests; desired/ra_vpn and tests; agent/rpc_ra_vpn plus minimal projection/assembly/agent hooks; new subsystems/ra_vpn_controller plus minimal registration hooks. Existing helpers, renderers, packaging, private packet/security fixtures belong the engine author.

Completed: material-free EngineSpec and sealed snapshot preparation contract. Secret bytes never enter scheduler records or generic formatting. Activation and controller consumers remain unfinished; enabled profiles still fail closed. Compile and meaningful lifecycle tests pending. No shared host service, VPP or configuration mutations.

Next: pair snapshot adapter contract with engine owner, implement verified transport readback and stopped-before-repair controller, production projection and RPC, then targeted lifecycle/security tests. Full quick and independent review remain mandatory.

Checkpoint: lifecycle, fixed-unit supervisor and shared transport readback consumers drafted. New contract preserves actual immutable hmac:<64hex> refs. Initial new transport tests failed due reviewer fixture decoding camelCase with encoding/json; corrected fixture to normal protojson. Targeted retest running. Production descriptor/store/projection/RPC/preflight and restart/failure lifecycle tests remain unfinished. No operational readiness claimed.
