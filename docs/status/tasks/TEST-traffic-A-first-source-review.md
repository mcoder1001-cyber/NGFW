# TEST-traffic-A first source review

**R1/R2/R5: BLOCK** frozen `6661c5cc`, independently inspected 2026-10-02. First source support checkpoint only; whole composed traffic acceptance is NOTIMPLEMENTED.

## MAJOR: timeout can leave descendants running

`commands.run_command` sends SIGTERM to its new process group but sends SIGKILL only if waiting for the leader times out. If the leader terminates while a descendant ignores SIGTERM, leader wait succeeds and the descendant is never killed. This contradicts bounded whole-process cleanup and can leak future traffic/capture helpers on the shared host.

Independently reproduced using only private temporary files and Python processes: the helper leader spawned a child that installed SIGTERM ignore, both slept; a 0.3-second command timeout terminated the leader, returned TimeoutExpired, and the child still existed. The reviewer killed only that created child afterward. Require bounded cleanup of the entire created process group regardless of leader exit, and an actual ignoring-descendant regression. No live network/SSH/rig/capture command was run.

## Other source findings

Seven stages explicitly retain NOTIMPLEMENTED, no whole-chain proof. Read-only plan and live refusal precede lease/host reads and external commands. Slot formulas agree with shared-host conventions including high-slot ports, database/table/prefix/socket bounds and CI12/13 exclusions; global ownership flags refuse. D-128 banned VPP trace commands are not introduced. Fixed argv avoids shell command interpolation; exclusive0600 output creation refuses existing files and symlinks. Manager lease contract is bounded/root-owned0600 and is only read, not self-issued or renewed. Lease validation is not exercised by the current six tests and remains future review/test scope before activation.

Actual `python3 -B test/topology/traffic-a/test_foundation.py`: **6 PASS**, 0.228 s. These tests do not cover the reproduced descendant leak; successful tests do not supersede that finding. No packet outcome, capture, lab acceptance or full hosted quick was executed/claimed. Developer may continue unrelated source work while fixing this concrete MAJOR.

Workspace full checkout hit No space left on device; review continued in a sparse isolated snapshot. Only reviewer-owned clean committed old worktrees were removed during recovery; no developer product tree was modified. Existing shared disk exhaustion must be addressed for subsequent build/integration work.
