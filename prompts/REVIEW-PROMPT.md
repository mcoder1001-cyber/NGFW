# Task: Review PR <number/branch>   (prepend 00-CONTEXT.md)

You are the reviewing agent. You did not write this code. Be adversarial and specific.

## Check, in this order
1. **Contract compliance** — did the PR change anything in `packages/schema`,
   `packages/proto`, or generated code? If yes and the PR is not labelled `contract`, block it.
2. **Real data-plane proof** — open the integration tests. Do they run against the VPP
   container and assert on VPP state (`Retrieve`, counters, `trace`)? A test that asserts
   on the agent's own in-memory map is not proof. Block if the only proof is mocks.
3. **Restart safety** — is there a test (or pasted evidence) of reconcile after
   `tools/lab restart-vpp vrx-a`? If the feature adds a new VPP object type and has no
   `Retrieve`, block.
4. **VPP API provenance** — every VPP message used must exist in `apps/agent/binapi/`.
   Grep for it. Anything hand-typed is a hallucination until proven otherwise.
5. **Security** — grep for `exec.Command`, `child_process`, template rendering with
   user input, secrets in logs/fixtures/GET responses, missing authz guards on new routes.
6. **Transaction semantics** — does rollback actually undo the VPP objects? Does a
   renderer failure leave partial state?
7. **UI honesty** — every new screen must call a real endpoint. Grep for TODO/mock/stub.
8. **Scope creep** — anything built that the task did not ask for? List it; ask for removal
   or a separate PR.
9. **i18n** — hardcoded strings in JSX; `margin-left/right` instead of logical properties.
10. **Tests actually run** — check CI logs, not the PR text.

## Output
A findings list ranked by severity, each with file:line, the failure scenario, and the
fix. Then a one-line verdict: **BLOCK** / **APPROVE WITH CHANGES** / **APPROVE**.
Do not fix the code yourself unless the task explicitly says `--fix`.
