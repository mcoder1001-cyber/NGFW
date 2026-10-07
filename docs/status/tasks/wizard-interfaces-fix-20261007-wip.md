Branch: codex/wizard-interfaces-fix-20261007
Base/local parent: 3ddb1680e475e94d43e8036cd3776bc60c87208b
Remote checkpoint: pending publication of this commit; resolve branch head with git ls-remote.
Owned files: see task envelope.
Completed: picker merges running config with live parent interfaces; excludes local0/host-owned/subinterfaces; translated missing selection and discovery feedback; API rechecks missing interfaces through agent and includes additions in exact preview before atomic stage.
Tests: targeted API run initially failed before collection because fresh worktree dependencies had not been built (@ngfw/proto entry missing). Web test started. Full unchanged quick gate will build dependencies and run checks.
Remaining: independent reviews, complete quick gate and hosted CI, integration only after approval and green checks.
Current failure: unbuilt workspace dependency, not yet a product test result.
Exact next command: tools/ci.sh --base origin/main > /tmp/wizard-quick.log 2>&1
