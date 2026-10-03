# T1 independent local fixture evidence — P10 bounded archive slice

Tester: /root, independent of developer.
Exact tested local SHA `71cee90b2a28421480dfd67a6d64ca7f47f37200`, tree538878ff020cae5e4752bb81d1d1c6d8ed95cc87.
Worktree: work/NGFW-p10-resume. Manager reports published e53bc20b tree equality; exact hosted gate required separately.

Commands and actual output:
```text
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/debian/bundle -p 'test_*.py'
............................................
----------------------------------------------------------------------
Ran 44 tests in 769.730s

OK
git diff --check
(no output, exit0)
git rev-parse HEAD
71cee90b2a28421480dfd67a6d64ca7f47f37200
```

| Scenario | Expected | Observed | Result |
|---|---|---|---|
| Real dpkg metadata and valid changed payload | Same fields, different hash | Asserted by test_real_deb_metadata_hash_and_architecture | PASS |
| Malformed truncated archive | InvalidBundle | Asserted by new test_malformed_archive_rejected | PASS |
| Whole transport checksum and archive size | Equal independently read saved bytes | Asserted by test_export suite | PASS |
| All verifier/installer/exporter fixtures |44 successful cases |44 successful cases, no skip reported | PASS |

Only local isolated source/package fixtures ran. No real appliance package installation, VPP restart, root firewall changes, hardware forwarding or signing provenance claim. Slow run coincided with concurrent generation/build load; timeout/strict assertions were not weakened.

Focused fixture verdict: PASS.
Overall T1 integration verdict: PENDING — unchanged complete hosted quick and applicable independent panel must still certify final integration tree. No merge authorization inferred from fixture success.

## Exact published full quick evidence
Root independently queried PR98 head e53bc20bda899ebe5b12d12233e75086fb212edb. Mandatory quick gate37110162496 completed SUCCESS on2026-10-03T08:51:08Z (12:21:08Asia/Tehran), duration16m36s. Both provisioning fixture runs37110162481/37110135578 SUCCESS. Published tree equality to the independently tested source was separately verified by root GitHub git/commits API.
T1 gate/unit verdict: PASS for this bounded candidate. Integration remains blocked on missing mandatory fresh R1/R7/R8/R6 panel and any current-main rebase/new-tree testing required before merge. No wholeP10 completion claim.
