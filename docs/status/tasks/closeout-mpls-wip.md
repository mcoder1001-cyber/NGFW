# MPLS rollback reference leak closeout

Branch `codex/closeout-mpls`, worktree `/root/ngfw-wt/codex-closeout-mpls`, base `664544349`. Initial code checkpoint: `7d4007727e6c89683ac3c52588f214d0bce25b9f`. Remote publication: pending manager connector; not yet published.

Owned files: MPLS descriptor and its tests; `docs/status/tasks/closeout-mpls*`.

## Observed defect and implemented correction

The manager's actual `TestMplsOnHost` on isolated unpatched VPP 26.06 converged before and after agent restart, but rollback left label 100040 in table 0. Upstream pinned source explains why: `fib_entry_src_mpls_set_data` calls `fib_table_entry_special_dpo_add` for unchanged binds; those calls increase SPECIAL source reference counts. The IP prefix is sourced once, whereas an unbind removes each label source only once. Repeated apply and resync therefore leak MPLS labels.

The descriptor now sends a matching prefix/label unbind before each bind. The API ignores an unbind when the current label differs, and normalizes only that exact binding; no arbitrary MPLS label route deletion is introduced. Existing LDP ownership checks run before normalization. This configuration-only workaround requires no VPP C patch, plugin installation or shared service restart. Existing leaked references from older software are not globally swept.

Added a reference-count regression model exercising repeated create with reconstructed descriptors and one rollback. The unchanged real host test already repeats apply and restarts the agent before checking zero surviving labels.

## Actual validation and recovery

- `tools/ci.sh check --base 664544349`: PASS, 10 seconds; recorded in `closeout-mpls-evidence/check.log`.
- Private slot-7 VPP command below: **PASS**, `TestMplsOnHost` 7.15 seconds, production agent commit → canonical Retrieve → idempotent apply → agent restart/simulated object loss → canonical Retrieve → rollback with no surviving owned label/tunnel/table. The original leak assertion is unchanged. Evidence: `closeout-mpls-evidence/host.log`.
- Shared VPP `NRestarts=0` before and after; owned disposable VPP stopped normally, no service restart.
- Targeted MPLS agent unit tests passed. `tools/heavy.sh go -C apps/agent test -race ./internal/descriptors/mpls -count=1`: PASS, 1.207 seconds; evidence `closeout-mpls-evidence/race.log`. This executes the complete descriptor suite including the new reference leak regression. No skipped host suite is counted as acceptance.

Exact live command used (from this worktree):

```
env $(tools/lab env 7 | sed 's/^export //') NGFW_INTEGRATION=1 NGFW_DF7_GLOBALS=1 NGFW_ISOLATED_TEST_RUN=1 tools/heavy.sh python3 test/topology/hardware-smoke/isolated-vpp.py go -C apps/agent test ./internal/agent -run '^TestMplsOnHost$' -v -count=1 -timeout 3m
```

Completed regression command:

```
tools/heavy.sh go -C apps/agent test -race ./internal/descriptors/mpls -count=1
```

Mandatory complete quick gate and independent product review remain required before integration. Normalization momentarily removes and recreates the exact binding during reapply, because the upstream API has no binding dump; it does not sweep old leaks or unrelated routes. No contract or generated-binapi changes.
