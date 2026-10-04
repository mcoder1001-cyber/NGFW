# Independent completion source review

Verdict: **APPROVE bounded source integration** at frozen commit
`40e2ae1a9`, tree `f9a9cb37e28d42be90918891910c57df7470ca90`, relative to
origin/main `4c8d1b247`. Reviewer owns only this report and did not edit manager
product source. Applicable concerns R1–R7 examined; no merge-blocking defect
identified. This is not product laboratory acceptance or whole P11 completion.

Reviewed F-tunnels live-state enrichments and protected REST/UI mapping:
owner-filtered initial interface selection, serialized readback, descriptor
readback endpoint joins, optional FIB/counter data and honest unavailable notes.
SixRD does not substitute configuration as observed state. API counters remain
decimal strings; interface-index reuse checks preserve nonempty counter names.

Reviewed F-isis-rip contract/render/state/event wiring and the production sealed
password adapter. API selects only the two allowed IS-IS routing references,
requires password kind, uses existing transaction-pinned secret versions and
sealed delivery. Store.Text returns an independent active-snapshot copy; FRR
zeroes the byte copy and passes plaintext through its existing redacting resolver.
Existing RenderContext validates strict single-token CLI values, tracks resolved
secrets, and masks area/domain-password output and errors. Desired documents
retain references. Dry-run and runtime render share the resolver wiring.

Reader commands are fixed registered strings, existing output bounds retained;
RIP text parsing rejects malformed/incomplete peer rows, protocol/family mismatch
and invalid counters. REST query paging and public-field parsing are bounded,
with separate unavailable state. IS-IS global OSI enable retains globals-owner
and explicit irreversible-enable opt-in; it does not claim rollback revocation.
Actual packet/FRR/VPP propagation remains deferred and unverified here.

Reviewed native SA events: safe owner-filtered observation, bounded SA count,
public transition attributes, stable identity ignoring uptime/counters, sorted
child SPIs, rekey detection, retained last-good baseline across read errors,
first-snapshot suppression and cancellation-bound lifecycle. Certificate trust
semantics remain explicitly pending; this event delta does not resolve them.

Validation assessment: inspected the manager's frozen status containing focused
Go/race checks, API28, UI8, schema4 tests, dependency builds/typechecks and actual
forbidden-pattern/gitleaks checks. These are manager evidence, not independent
reruns; no duplicate heavy tests were necessary for a specific unresolved
concern. Full CI waiver belongs to the owner's recorded campaign authority.
`git rev-parse 40e2ae1a9^{tree}` independently confirmed the exact tree above.

Minor follow-up: LiveState.tsx uses `as never` for regenerated endpoint paths
and a hand-written Observed shape. Prefer the generated client response type
and typed endpoint calls now that generation includes those routes. This does
not change observed runtime semantics and is not a blocking finding.

Before final merge preserve reviewed history, verify final integration composition
on current main, and retain honest unfinished P10/P11 and deferred acceptance.
