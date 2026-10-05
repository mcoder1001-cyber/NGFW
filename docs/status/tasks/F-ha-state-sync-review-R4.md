# Independent R4 HA acceptance

SHA c42307e12d124aea01640b60a454c652270154ae. 2026-10-05; reviewer branch codex/review-r4-seven-20261005; owned reports only. Exact-SHA git archive snapshots under this reviewer worktree. Envelope: independent R4/T3, no product changes, no real process/service kills, no shared VPP/host config mutation.

Inspected acceptance.py, flow.py, driver.py and kill_vpp_lab.py. Concrete evidence uses one TCP connection with challenged echoes, exact server-observed translation on both EI endpoints, no truncated dumps, MASTER/BACKUP transition and increased backup packets. Priority mutation has dedicated lease/candidate digest checks and 60-second unconfirmed revert; foreign pending/lease/candidate changes refuse cleanup. Output reserved exclusively mode0600 before mutation.

Optional fault helper requires explicit handover/VM boot/PID/nonce and root-owned nonwritable marker/parents, executable/unit identity, pidfd and starttime recheck. Only its verify() ran against filesystem fixtures; no signal helper main or remote SSH invoked. Two-node appliance/UDP capture and real fault execution remain deferred. No R4 blocker found in these guarded probe changes; no runtime HA completeness claim.

Actual command `tools/heavy.sh python3 -m unittest discover -s .review-fixtures/ha/test/topology/ha-state-sync -p 'test_*.py' -v`:
```
Ran 13 tests in 0.028s
OK
```
Expected argparse errors from refusal tests appeared; suite exit0. Nonroot initial runuser command could not traverse /root and returned Permission denied (environment, not product). Copied unchanged Python files into reviewer-owned /tmp/ngfw-r4-ha-CJFVUm, then actual `tools/heavy.sh runuser -u nobody -- python3 -m unittest discover -s /tmp/ngfw-r4-ha-CJFVUm -p test_acceptance.py -v`:
```
Ran 10 tests in 0.016s
OK
```
Verdict: APPROVE for guarded acceptance probe R4 scope.
