# OSPF FRR compatibility successor review

Immutable `fad3717c`, compared with `ae282009`.
Verdict: APPROVE WITH LIMITS for the two mapping fixes previously requested.

The state allowlist now preserves FRR point-to-point `Full/-` and other known
adjacency states with dash role. The precise `neighbor` placeholder is recognized
without inventing an IPv4 router ID; its object rows are omitted while healthy known
neighbors remain visible and routing-observation-partial is explicit.
Placeholder entries still consume the inspection budget and require object shape.
Arbitrary non-IPv4 keys remain invalid. All-placeholder observations report partial
instead of silently healthy empty inventory. Existing byte/instance/output limits
and raw diagnostic exclusion remain intact.

Meaningful fixtures cover point-to-point state, mixed healthy/placeholder rows,
all-placeholder partial observation and over-budget placeholder arrays. No product
edits or tests were run by this reviewer. Root owns generated outputs, registration
and validation. The separately reported controller error-propagation test failure
is not resolved or approved by this compatibility-only review; its successor needs
independent assessment before the whole feature is considered green.
