# F-ha-state-sync fresh independent R8 review

Frozen inspected source: `48e5af12fd6a42323f3c83d04129eb6563cef650`. Reviewer owned only these reports and private RAM fixtures; no product edits or shared host mutation.

Concrete probe maintains one challenged TCP flow, uses bounded 10-second reads to accommodate normal VRRP master-down delay, compares exact translated session identity and forwards no credentials to its namespace worker. Priority transitions use unconfirmed commits and ownership-checked cleanup; evidence is exclusively created mode0600 before mutation and successful evidence follows restoration. Legacy driver now shares HTTPS-origin and redirect refusal. Fault helper requires explicit dedicated appliance markers and post-handover identity; no real fault was executed. No new service or runtime daemon dependency. No BLOCKER or MAJOR findings. Real two-appliance failover remains deferred, not proven by these offline tests.

Commands run in `/dev/shm/ngfw-review-r8-final-20261005/.scratch/ha`:
```text
python3 -m unittest discover -s test/topology/ha-state-sync -p 'test_*.py'
Ran 16 tests in 3.239s
OK
Expected argparse refusal diagnostics were emitted by negative cases.
```

Verdict: **APPROVE** for inspected R8 scope. Complete unchanged quick CI and other mandatory panel reviews remain required.
