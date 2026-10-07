# Global Blocking test recovery WIP

Branch/worktree: `codex/resume-global-blocking-tests-20261007`,
`/root/ngfw-wt/resume-global-blocking-tests-20261007`.
Local starting SHA `3ddb1680e475e94d43e8036cd3776bc60c87208b` from origin/main,
clean. Verified local/remote envelope checkpoint
`f755bbbfa497f7432bd1b42632b6f9704fa4fd58`; CLI push succeeded and ls-remote matched.
Exact owned paths in matching envelope. No production edits authorized.
Completed: required instructions, published audit, original prompt/status and
current service/fetcher/renderer/tests inspected. Existing integration retained.
Implemented only owned tests: service/parser retention14cases, transport fixture
failures (no real feed) and IPv6-only anti-lockout rendering3cases. No production edit.
Workspace frozen pnpm install PASS19.2s (all604 reused, no host package install);
schema/proto/yang prerequisite builds PASS; no tracked generated change.
First Go compile failed because the test used nonexistent HostAclChain; corrected
to existing nftables.Chain, no product defect. Initial focused Go race PASS1.276s.
API new14 + existing fetcher3 PASS17/17 in26.68s. Final exact rule constraint
assertion strengthened and race recheck PASS1.207s; 3subcases, no skip.
Focused ESLint exit0; emitted pre-existing root MODULE_TYPELESS_PACKAGE_JSON warning,
no lint findings. Check mode PASS22s. API typecheck exit0; scoped Go lint0issues.
Verified local/remote tested source checkpoint
`35f601ef67919ef5a34237751b7a0ca52f279149`; CLI push succeeded, ls-remote matched.
Current product failure: none reproduced. Remaining owned code: none.
Independent review/full hosted quick on final integration manager-owned; no merge
or full quick PASS claimed here. Final documentation receipt follows the tested
source SHA; resolve final tip with git rev-parse/ls-remote (cannot embed own SHA).
Exact next command: `git push origin codex/resume-global-blocking-tests-20261007`;
manager reads this WIP and reviews only the two scoped new test files.

## Actual command output

```text
tools/heavy.sh pnpm --filter @ngfw/api exec vitest run src/features/global-blocking/refresh-retention.test.ts src/features/global-blocking/fetch.test.ts
✓ src/features/global-blocking/fetch.test.ts (3 tests) 345ms
✓ src/features/global-blocking/refresh-retention.test.ts (14 tests) 110ms
Test Files  2 passed (2)
Tests  17 passed (17)
Duration  26.68s
cd apps/agent
../../tools/heavy.sh go test -race -count=1 -timeout 120s ./internal/renderers/nftables -run '^TestGlobalBlockingIPv6AntiLockoutConstraints$' -v
--- PASS: TestGlobalBlockingIPv6AntiLockoutConstraints (0.08s)
    --- PASS: TestGlobalBlockingIPv6AntiLockoutConstraints/explicit_IPv6_management (0.07s)
    --- PASS: TestGlobalBlockingIPv6AntiLockoutConstraints/no_sources_must_not_bypass_blocking (0.00s)
    --- PASS: TestGlobalBlockingIPv6AntiLockoutConstraints/disabled_must_not_bypass_blocking (0.00s)
PASS
ok ngfw/agent/internal/renderers/nftables 1.207s
tools/ci.sh check --base origin/main
no contract files changed in the 1 commit(s) of HEAD since origin/main (3ddb1680e)
board valid: 212 tasks; read-only validation
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m22s)
tools/heavy.sh pnpm --filter @ngfw/api typecheck
$ tsc -p tsconfig.json
(no errors; exit0)
cd apps/agent
../../tools/heavy.sh golangci-lint run ./internal/renderers/nftables/...
0 issues.
exit0
git diff --check
(no output; exit0)
Final pre-receipt tools/ci.sh check --base origin/main
no contract files changed in the 2 commit(s) of HEAD since origin/main (3ddb1680e)
ok: gitleaks — scanned ~17311 bytes (17.31 KB) in 646ms no leaks found
check PASSED (0m13s)
```

The API service uses real parser/refusal/state/event logic, fake transport and
in-memory persistence. Controls cover scheduled and manual network/HTTP failures,
empty/comment-only/garbage/partially-valid refusal; both IP families retained;
scheduled last-good validators/count retained, alarms/notification emitted, lease
released, later valid feed recovers;304 clears failure without commit; failed
systemCommit retains old list/metadata. Existing fetcher uses a private ephemeral
loopback HTTP fixture; no real remote feed or datastore/agent is exercised.
Go controls assert exact IPv6 management source/interface/custom-port accept before
the broad IPv6 drop, no-source/disabled bypass absence and overlap collapse.
No packets or actual nft execution; current protectHost all-host-interface limitation
is preserved, not certified as selected-interface enforcement.
Native/feed/browser acceptance NOTRUN; no full task completion claim.
This includes real scheduled URL refresh/retention against the deployed commit
engine, native IPv6 packets/anti-lockout behavior and selected-interface local-in
semantics, real en/fa browser/screenshots. Historical native IPv4 evidence from
the audit is preserved as historical; no fresh native claim. Performance/200k
execution not performed. No production fix proposed because no defect reproduced;
any future negative control requires a manager-authorized narrow production fix.
