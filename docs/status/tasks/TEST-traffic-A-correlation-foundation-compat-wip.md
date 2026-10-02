# Correlation/foundation named-contract compatibility WIP

Branch `task/traffic-correlation-foundation-compat-20261002`; isolated worktree
`NGFW-correlation-foundation-compat`; reviewed base9b2b1e8d. Publication pending,
no new durable remote SHA claimed yet. Source remains exact1427; producer freeze
unchanged. Existing correlation runner/workflow and all base metadata unchanged.

Real integration failure prevented: original module loader expected16, while
expanded Foundation10+Evidence12 contain22. Instead of dropping any original
case or claiming22 results as16, retain exact original16 named method contract
extracted from fda AST. All16 names still exist in current source. Additional six
Foundation/Evidence cases remain included in unchanged mandatory full33 workflow,
and later full40; original gate is complementary, never sufficient for expanded
source integration. Labels state original16 accurately; guards reject missing,
duplicate, zero/reduced and every failing/skip/xfail/xpass/error outcome.

Actual 2026-10-02 validation:

```
AST comparison fda original Foundation9+Evidence7: all16 names present in1427
python3 .github/scripts/traffic-foundation-fixtures.py --check-policy
foundation named-contract policy checks: 11 PASS (not source/lab acceptance)
python3 .github/scripts/traffic-foundation-fixtures.py
Ran 16 tests in 0.406s — OK
original foundation source fixtures: tests=16 failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0
python3 .github/scripts/traffic-correlation-fixtures.py
Ran 33 tests in 2.456s — OK
correlation source fixtures: tests=33 failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0
```

Policy executes actual16-case unittest outcome fixtures, a genuine missing-method
loader error and missing/duplicate contract refusal; zero/reduced otherwise-success
results fail. Policy counts are separate from real source results. Source and
approved workflow identity/whitespace checks PASS. New independent review, hosted
gates and final current-main unchanged quick: NOT RUN. No producer suite rerun,
source change, packages, real network/lab/ip/tcpdump/VPP/nft/SSH operation. Seven
composed stages/causal live lifecycle remain source gaps and all proof flags false.
Next: publish checkpoint for independent compatibility review before final33 merge.
