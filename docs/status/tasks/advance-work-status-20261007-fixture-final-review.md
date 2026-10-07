# Independent TD19 fixture compatibility review

**R1/R2/R7/R8 APPROVE** exact `f745de9c3e6344cdf4a459b824f96722743c42c0`, based on actual merged safe-root main `4fcdc4557e857efb8d079bf2c52c0e2f0fedd9db`. Bounded fixture compatibility only; no product, runner or workflow changes. Own branch codex/td19-fixture-compat-review-20261007; reviewer owns report/control/log artifacts only, imports frozen source without writing product code.

Independent baseline confirmed module reader-negative failure: exact6 runner had5pass1fail0.980s; trusted absolute AWK bypassed old PATH mock, then copied entry missed shared helper. Selected FRR actual-entry case failed both subcases because expected GPG gates never ran. Baseline module output preserved separately; FRR selected run1test2subcasefail0.042s, assertion `any(gate in args for args in self.calls())` false. No native effects executed.

The repaired module fixture copies both actual installer helpers. Reader-negative fault injection replaces exactly one owned `/usr/bin/awk` subprocess location in a disposable entry, guarded by count1; AWK program, canonical module selection and actual failure branch are retained. Diagnostic assertions, unsupported-version, malformed/duplicate/directive, wrong-CWD/environment and symlink checks remain intact, exactsix inventory unchanged.

The repaired FRR entry fixture copies byte-identical helper/recording contract and builds exact trusted recording PATH plus disposable root metadata. Controlled mock stays outside effect PATH. Exactlyeight owned GPG call locations in disposable source are explicitly fault injected (count8); actual selection/key authorization, failure handling and private-home cleanup remain. Original artifact preflight bypass and authorized controlled identities stay fixture-only, never affect shipped source. Existing reached-gate, nonzero, no unexpected-mutation, authorization and cleanup assertions retained. Added node.key-specific show-key/error diagnostic, FRR-import abort before NodeSource and zero APT/keyring/list writes strengthen coverage. Private real certificate tests are unchanged.

Independent exact frozen-source commands:

```
TMPDIR=/tdr PYTHONDONTWRITEBYTECODE=1 python3 .github/scripts/go-module-version-fixtures.py
Ran 6 tests in 0.965s
OK
Go module source fixtures: tests=6 failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0

TMPDIR=/tdr PYTHONDONTWRITEBYTECODE=1 python3 docs/status/tasks/TD-19-run-frr-selection.py
Ran 13 tests in 25.082s
OK
selector fixtures: started=13 completed=13 failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0 accepted=True

python3 .github/scripts/go-module-version-fixtures.py --check-policy
Go module gate policy: 12 PASS (not source/lab acceptance)

python3 docs/status/tasks/TD-19-frr-selector-ci-policy.py
Ran 5 tests in 0.023s
OK
```

Policy controls reject skips, expected failures, unexpected successes, failures/errors, missing/extra/duplicate/replaced inventories and incomplete/setup-failed suites. Exact runners unchanged; no skip/count/assertion weakening. `git diff --exit-code 4fcdc455..f745de9c -- scripts .github docs/status/tasks/TD-19-run-frr-selection.py docs/status/tasks/TD-19-frr-selector-ci-policy.py`: exit0. Exact repaired two fixture paths match frozen source: exit0. `git diff --check` and ShellCheck two installer source checks: exit0/no diagnostics. Independent full runner outputs preserved in reviewer-owned logs.

No actual installs, downloads, repository/keyring writes, VPP/service operations, native or shared-host acceptance exercised. No aggregate CI run under owner waiver; automatically triggered hosted run history remains parent-owned evidence and is not claimed passed here. Actual native installation/boot and authoritative Python release lock remain outstanding; fixture repair does not finish whole TD-19.
