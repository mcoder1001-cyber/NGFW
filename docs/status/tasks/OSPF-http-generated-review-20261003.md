# OSPF HTTP error and generated API contract review

Author immutable `d684e186` vs `619bca2a`: APPROVE WITH LIMITS.
The plain async adapter throws the actual ProblemError without Vitest rejection
cloning, while the test keeps class/status/exact-body assertions. Additional actual
Fastify HTTP injection with AuthGuard and ProblemFilter verifies HTTP503,
application/problem+json, exact public fields and route instance. The failure flag
resets before each test and deliberately bypasses successful observation mock calls.
No production code is changed by this author successor.

Root immutable `ca12ac25`: APPROVE SOURCE WIRING/GENERATED TYPES WITH LIMITS.
app.module imports ospfFeature once and adds its controller array once; no provider
is needed because AgentClient already belongs to the module. Generated path is
GET-only `/api/v1/state/ospf`, with no query/body or mutating methods. Response types
agree with strict DTO fields/nullability and explicit unavailable/partial enums.
401/403/502/503 problem responses agree with Protected/controller contract. The
100-row maximum and numeric priority bounds remain runtime schema constraints,
which TypeScript array/number types alone cannot express.

No edits to author/integration/generated worktrees and no test launches. Author
21/21 success was reported by coordinator, not rerun by reviewer. CLI regeneration
was running separately and its output is not approved by this pre-final checkpoint
review. Root should verify that final frozen successor retains this reviewed wiring
and generated response shape before publication.
