# Prepared correlation composition recovery checkpoint

Branch `task/traffic-correlation-prepared-20261002`; isolated
`NGFW-traffic-correlation-prepared`; prepared foundation final base
`2cacb15c8951bebf8fea9bc9245597c6bc51c4d3`. Durable remote publication of this
new checkpoint awaits manager. Foundation PR82 full CI/merge is pending; base
is not yet claimed actual merged main. Exact final main/full hosted proof pending.

Imported approvedff794 correlation/compat bytes, independently reviewed reports
and exact1427 source inventory. Producer/transaction/cleanup files absent. All
current base apps/packages/scripts/tools/deploy/plan paths remain unchanged,
including current packagechecker/appliance/provision/CI/identity work. Existing
foundation metadata kept; shared WIP prefix retained; arbitration diff exactly
one added correlation row with zero removals. No board/main/other branch writes.

Actual local source checks on prepared tree 2026-10-02:

```
python3 .github/scripts/traffic-foundation-fixtures.py --check-policy
foundation named-contract policy checks: 11 PASS (not source/lab acceptance)
python3 .github/scripts/traffic-foundation-fixtures.py
Ran 16 tests in 0.443s — OK
original foundation source fixtures: tests=16 failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0
python3 .github/scripts/traffic-correlation-fixtures.py --check-policy
correlation gate-policy checks: 13 PASS (not source/lab acceptance)
python3 .github/scripts/traffic-correlation-fixtures.py
Ran 33 tests in 2.513s — OK
correlation source fixtures: tests=33 failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0
```

Independent byte/file-inventory check against1427 source PASS; base production
path diff empty; arbitration1 addition/0 deletions; whitespace PASS. Local cases
are source-only, not actual forwarding/lab or full quick. Hosted exact-current-main
quick/16/33 and independent final composition review: NOT RUN. No source fixture
capability means live authority; seven composed executors remain NOTIMPLEMENTED,
false proof flags remain and whole task stays open.

Exact next manager step: after foundation merge, `git fetch origin main` and
verify prepared base tree vs actual main; independently assess any new overlaps.
Preserve checkpoint history before final one-commit current-main publication.
Then run actual exact-head unchanged complete hosted quick and both source gates,
merge with expected head only after approvals/green, verify main. No final PR now.
