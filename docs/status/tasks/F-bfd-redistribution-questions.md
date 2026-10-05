# Task decisions and remaining capability
- Per-box validation errors chosen for VPP/FRR BFD port conflicts (rather than warnings or unsupported glue). Admin-down VPP sessions still claim UDP ports.
- Current sealed secret channel supersedes historical PENDING-secret-channel note: BFD auth references are now selected, version-pinned and size-validated, and passed to existing agent sealed cache.
- Manager authorized minimal FRRDoc profile copy, event lifecycle calls, secret delivery filter and sealed-cache wiring, and shrink-only reachability maintenance.
- Superseded historical gap: multihop now uses successful-add durable complete boot identity/interface index/tuple ownership, restart Retrieve/Delete proof, PartialCreate compensation, globals-owner enable and batch native observations. Exact native multihop peer runtime remains NOT RUN; full integration gate and mandatory panel still required.
- No shared daemon, package, host VPP, service, interface, port, or boot state has been changed.
