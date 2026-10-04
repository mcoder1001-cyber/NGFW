# Independent running-task security/contract review

Verdict: APPROVE commit 40e2ae1a9eebb00b1f0955dd0f6b4e39d066d36d,
tree f9a9cb37e28d42be90918891910c57df7470ca90, against main 4c8d1b247.

Reviewer owns only this report and its WIP/envelope files on
codex/running-security-review-20261004 in the isolated Native-review worktree.
No product edits or host mutations; no duplicate broad suites. Read the repository
context, contributing rules and decision policy. Inspection supplements focused
verification recorded in complete-running-20261004-wip.md; it does not claim a
complete quick gate or live laboratory acceptance.

Inspected production IS-IS password references through SecretDeliveryService,
version-pinned ciphertext lookup, sealed agent snapshot selection, transient
validation, confirmed revert and FRR adapter. Adapter reads a copied active cache
value, clears bytes after conversion, and masks source errors. Desired documents
remain reference-only. Existing FRR token validation prevents newline/pipe CLI
injection; config is secret-marked and area/domain-password patterns plus literal
resolved values redact state, diff and daemon errors. Scheduler rollback retains
rendered old configuration; confirmed revert activates its saved secret snapshot.
No new secret exposure found.

Inspected native watcher startup/shutdown, bounded per-read context, cancellation,
owner-prefix filtering and safe-state capability. Failed readback retains baseline;
first successful snapshot is silent; counter/uptime changes do not produce events.
The new watcher calls read-only VPP APIs and publishes public identities only.
The snapshot count guard occurs after the existing SAs reader allocation; it is
not a pre-allocation memory bound. This is an inherited reader characteristic,
not evidence of an authorization leak or new merge blocker.

Inspected tunnel interface ownership filtering, descriptor/FIB readback and stats
join, cancellation-aware walk gate, explicit absent fields, protobuf presence and
REST string encoding of uint64 counters. GET is authenticated by global AuthGuard
with readonly minimum; Protected documents authentication rather than installing
it. No candidate-derived substitute for unavailable live endpoints/FIBs is exposed.

Native certificate follow-up remains explicitly incomplete and fail-closed:
internal/desired/ikev2.go rejects cert authentication because peer trust mapping
and plugin-global private-key provisioning are unavailable. The strict current
IpsecAuth cert branch has local certificate and optional remoteCa; it admits no
peer pin. An additive explicitly named peer certificate/pin field can preserve the
meaning of remoteCa, but choosing/provisioning trust and global key ownership is
still the pending security-boundary decision. This approval does not authorize
that implementation or removal of its fail-closed validation.
