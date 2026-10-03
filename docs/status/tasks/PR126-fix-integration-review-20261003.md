# PR126 current-main consumer integration review

Immutable `fbe3fcd2a6add4a1234fb636a751dad49b111386`, scoped vs ee288a57 and
current main6db19a513. Verdict: APPROVE WITH LIMITS for the new integration.

Current OSPF feature files and module registration survive the composition. Protected
GET/fixed reader/public bounded projection remain intact. Error tests use
https://ngfw.dev/problems/unavailable, matching composed PROBLEM_TYPE_BASE; the
actual ProblemFilter test remains present. No auth downgrade or invented healthy
state is introduced by integration.

License wrapper consistently targets ngfw-license.mjs, its real fixed key filenames,
NGFW_SIGNING/NGFW_LICENSE environment names, .ngfwlic output and ngfw-api.service.
The issuer keygen names match defaults; API licensing.config reads
NGFW_LICENSE_PUBLIC_KEYS and expands escaped PEM newlines. Trust verifies the selected
license against the public key before changing settings, writes a public-key-only
EnvironmentFile drop-in for the actual service, reloads/restarts that service and
checks active status. Existing default all-features behavior and explicit issuer
options remain. No new private key/password logging appears; printed environment
contains public key material, not signing private material.

This is read-only consumer wiring review, not the full mass rename. Existing trust
command host-write/backup/restart semantics are inherited from merged main; no host
operation was performed here. Root's separately regenerated canonical outputs and
full gate results need exact-head review before merge. No compilation/tests/product
edits or author/root worktree writes were performed.
