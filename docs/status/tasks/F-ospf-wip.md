# F-ospf source completion checkpoint

Branch codex/complete-ospf-20261004; local contract4b9c268cf; remote contract
b2945f9cbb81c0b35819edb75ca5c42fd3a4a279, exact tree b3536e174ac9b4b47863b826c5d5ba30d29ea005.
Own files listed in F-ospf-envelope.md. source checkpoint publication follows.
Completed source: OSPFv3 schema/proto/rendering/interface hooks and desired FRR/retrieve projection;
MD5 interface auth resolver plus redaction; Event20 mapping for v2/v3 neighbors;
bounded observed v3 array parser, family-selectable authenticated API state, fixed interface/LSDB readers and
paged bounded public projection; UI v3 configuration tab and poll-based observed neighbor grid.
Tests: schema OSPF+existing routing 24 passed; existing Go ospf/desired/frr passed before new tests.
Current verification: new renderer/API/UI tests and typechecks running. API first typecheck blocked only
missing fresh-worktree @ngfw/yang build, being built; not declared passed.
Remaining: finish focused verification, resolve review findings, docs/runtime-boundary wording.
MD5 production resolver unavailable until approved PENDING-secret-channel; fixture renderer is tested;
no production password delivery silently added. Host FIB/restart/withdraw/rollback/browser acceptance deferred.
Next command: pnpm --filter @ngfw/api exec vitest run src/features/ospf; focused UI/Go checks.

## Final source verification (2026-10-04)

Previous source checkpoint remote61f7f10d132b02a5b2994bfef48392b27049e541 exactly matched local8c99cedfe tree4983197cbf7fab59aadfacb70abca12a0f418420.
Final corrections: separate Ospf6Interface wire message, optional auth.type per D039, validated shared Redistribute
exclusion, OnDemand readers excluded from automatic Retrieve with focused regression test,
accurate bilingual auth/runtime warnings, observed-neighbor/v3 UI test, and user docs.
Schema+existing routing:24 passed. API OSPF:24 passed. Web OSPF:6 passed (including real providers/query polling fixtures).
Go tests: internal/contracttest, renderers/frr/ospf, renderers/frr, desired, agent, subsystems passed.
Go vet touched packages exit0. API+web typechecks exit0; API/web/schema focused eslint exit0.
Current source gitleaks dir scan: no leaks. git diff --check: exit0.
History check found a generic-api-key false positive on the public literal StateReader Key "ospf6Neighbors";
changed to named constants. No actual credential exists and no allowlist was weakened. Standard D112 final single
commit integration must check its own clean history (checkpoint archive preserves recovery history).
Independent review requested from complete_setup. No host mutation, live-FIB acceptance or real MD5 delivery claimed.
Next command: independent review final HEAD, root integration/client generation and focused checks on final tree.

Independent review complete_setup identified malformed v3 poll snapshots causing false removal events; c2460d469 rejects these and adds regression cases. Reviewer APPROVE on c2460d469; see F-ospf-completion-review.md for durable exact SHA/tree evidence.
