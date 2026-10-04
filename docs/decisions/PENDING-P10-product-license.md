# PENDING: authoritative NGFW product license metadata

- raised: 2026-10-04 by P10
- decision: pending authoritative owner license text/name or existing source document
- affected work: authoritative release/distribution license metadata; P10 installed-development-bundle task is complete and is not parked

## Context and alternatives

No authoritative product LICENSE or source-package copyright text was located in the reviewed repository. The package must not invent the owner's product license or silently borrow a dependency's license. The old strongSwan packaging scope is superseded by DEC-ipsec-route-based and does not resolve NGFW product metadata.

1. Owner identifies an existing authoritative license document; derive package copyright metadata from that document.
2. Owner selects and supplies the product license text; review third-party component notices and package metadata against it.

Recommendation: use established authoritative metadata if it exists. No license has been selected, and no source license or dependency terms were changed. Isolated builds, staging regression fixes, artifact verification and unrelated approved source merges continue. This requires the product owner's hands; decision policy always-PENDING items5/6 apply as relevant to the supplied terms.
