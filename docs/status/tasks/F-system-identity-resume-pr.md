# PR draft: Complete system identity observations and pre-login banner

The System screen previously showed committed settings without observing installed
identity, and the configured login notice never reached the login page. This
continuation recovers the existing implementation and its independently reviewed
fixes onto main `53a43ce5`.

- Add an additive, owner-checked `SystemIdentityState` RPC and preserve the existing
  `/api/v1/state/system` health response while adding installed identity facts.
- Observe bounded files under the descriptor's scoped paths. Slot agents do not
  disclose the shared host kernel hostname or resolver. Resolver file readability
  is explicitly distinct from service health; no restart or privilege is added.
- Serve only committed `banner.login` through the public no-store endpoint, capped
  at 4096 UTF-16 units. Render literal text on login without blocking authentication
  on banner failures. Candidate configuration and other fields remain private.
- Show operational identity separately from candidate/running settings with en/fa
  labels and existing number-format settings. Keep older-agent absence explicit.
- Regenerate protobuf, OpenAPI/client and CLI contracts; add meaningful renderer,
  owner-isolation, public-route, literal-rendering and compatibility regressions.

## Evidence and merge conditions

Historical independent R1/R2/R3/R4/R6/R7 approvals and their original findings are
preserved in `identity-finish-independent-review.md`. New checkout targeted Go
race tests pass. The unchanged full quick gate is running; final result belongs
in `F-system-identity-finish-wip.md`. An independent integration review must verify
the final branch head. No merge readiness is claimed until that gate and review
pass. Appliance topology/restart/browser acceptance remains explicitly NOT RUN
and deferred by the owner; runtime DNS restart remains the existing privileged
handoff.

Publication currently blocked by automatic approval review; this file is a local
reviewable draft, not an existing GitHub PR.
