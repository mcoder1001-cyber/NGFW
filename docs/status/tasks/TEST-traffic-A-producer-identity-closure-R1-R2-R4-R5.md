# TEST-traffic-A producer namespace identity closure — R1 / R2 / R4 / R5

2026-10-02. **APPROVE** bounded correction at exact
`e1697d99788b7d3771eced0e2135a1e5893d9a24`; closes independent BLOCK400b8b16.
Own task/traffic-producer-identity-closure branch/worktree, only this report owned.
No product/test authorship or arbitration role. Original source approval and
historical failure report are preserved; no failed result was relabelled PASS.

Actual independent checks:

- Both affected producer cases, five repetitions each: 10 tests in4.117s PASS.
- `python3 test/topology/traffic-a/check.py`: 40 tests in4.001s PASS;
 tests=40 failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0.
- Producer/evidence/commands/correlation/run/scenario identity diff against
 original4c freeze: empty. `git diff --check`: clean.

R1/R2: child sets SIG_IGN before reading its own /proc/self/stat. It publishes
proc-mounted PID plus kernel start-time identity in closed JSON via rename.
Parent opens the corresponding /proc/<proc_pid>/stat and positively verifies PID,
matching birth and live S/R state BEFORE returning the executor process. Therefore
later disappearance, zombie state or changed birth is assessed only after a real
same-child identity was established; namespace mismatch alone cannot create PASS.
The previously observed outer sites-preview PID cannot satisfy this readiness.
No direct child-reported numeric PID is signalled. Cleanup remains exclusively
against the actual process group created by the injected Popen process.

Confirmed-parent-exit case additionally uses waitid WNOWAIT/WNOHANG to prove
correct parent exited0 without reaping before producer begins, with ready child
still holding pipes. Producer timeout/finally cleans owned group; same verified
child becomes nonrunning and final waitid raises ChildProcessError, proving
leader already reaped. Sleeping-parent case has accurate name and verifies its
same ready child stops. Ten actual executions prove both paths in this environment.

R4/R5: no producer source, capabilities, VPP API, ownership contract, live entry
or global operation changes. Tests operate on private temporary processes/files;
existing bounds and all false proof flags remain. No unsafe external process
selection or broad kill is introduced. Actual namespace/device lease authority,
transaction/synchronization and config/counter linkage remain source gaps, not
lab-only deferrals. No live capture or whole-task DONE claim follows.

This approval fulfills the bounded source/test failure closure required by the
conditional A3 ruling, not a new arbiter ruling or blanket integration approval.
Manager still needs remaining applicable source/docs panels, final actual-main
composition and unchanged complete hosted quick/strict40 on exact final head.
No real SSH, network, VPP/nft, ip/tcpdump, live lease or host installation occurred.
