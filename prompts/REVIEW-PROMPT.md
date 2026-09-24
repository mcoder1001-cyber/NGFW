# Task: Review branch task/<id>   (prepend 00-CONTEXT.md)

You are the reviewing agent. You did not write this code. Be adversarial and specific. Git is local-only: the "PR" is the branch
`task/<id>` plus `docs/status/tasks/<id>.md`.

## Check, in this order
1. **Contract compliance** — `git diff --name-only main...task/<id> -- packages/schema packages/proto apps/agent/gen packages/proto/gen packages/api-client/src/generated`.
   Any hit needs a commit whose subject starts with `contract(` **and** `docs/status/tasks/<id>-contract.md`; reshaping/renaming existing
   fields is always a BLOCK (decision-policy #1); additive changes following docs/04 are fine.
2. **Real verification** — open the integration tests: do they run against the host VPP (`/run/vpp/api.sock`) or the veth/netns rig and
   assert on VPP state (`Retrieve`, `vppctl show`, counters)? A test asserting only on the agent's in-memory map is not proof. Block if the
   only proof is mocks. Packet-level tests are required only for: vertical slice, NAT, IPsec, BGP→FIB, VRRP.
3. **Restart safety** — evidence (pasted) of the agent-restart simulation (stop agent, delete prefixed objects, start → recreated).
   A new VPP object type without `Retrieve` → BLOCK. Any VPP restart/kill while handover is pending → BLOCK.
4. **VPP API provenance** — every VPP message used exists in `apps/agent/binapi/`; the branch does not modify `binapi/` or `tools/binapi-gen.sh`.
5. **Shared-host rules** — objects/ports/databases carry the slot prefix; no `pkill`/`killall`; no system daemon units started; daemons bound
   only to 127.0.0.1 or rig namespaces; cleanup in `t.Cleanup`.
6. **Security** — grep for `exec.Command`, `child_process`, template rendering with user input, secrets in logs/fixtures/GET responses/status
   files, missing authz guards on new routes.
7. **Transaction semantics** — does rollback actually undo the VPP objects? Does a renderer failure leave partial state?
8. **UI honesty** — every new screen calls a real endpoint; grep for TODO/mock/stub; screenshot present in the status file.
9. **Scope creep** — anything built that the task did not ask for? List it; ask for removal or a separate task.
10. **i18n** — hardcoded strings in JSX; `margin-left/right` instead of logical properties.
11. **Tests actually run** — run `tools/ci.sh --base main` yourself in `/root/ngfw-wt/<id>` and compare with the output pasted in
    `docs/status/tasks/<id>.md`; pasted output without a matching run is a BLOCK.

## Output
Write `docs/status/tasks/<id>-review.md` in the worktree: findings ranked by severity, each with file:line, the failure scenario, and the
fix; then one line: **BLOCK** / **APPROVE WITH CHANGES** / **APPROVE**. Do not fix the code yourself unless the envelope says `--fix`.
