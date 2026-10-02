# TEST-traffic-A producer test-only recheck — R2

2026-10-02. **BLOCK** narrow test recheck at exact
`862d6b97f4b598abf80f12029fd9822cd212f1d0`; no new producer-source defect inferred.
Own task/traffic-producer-minor-recheck branch/worktree; this report only authored.
Original4dceefd7 source approval remains preserved, not silently withdrawn.

MAJOR test failure: renamed sleeping-leader case still checks child-written PID
against outer /proc without birth or namespace identity. Actual targeted run:
new confirmed-unreaped-leader case PASS; old renamed sleeping-leader case FAIL
'owned ignored-TERM descendant survived'; 2 tests in1.825s, failures1.
A justified diagnostic rerun of only failing case captured its actual stat reads:
child identity PID7 led to /proc/7/stat, comm sites-preview, state S, birth148.
This is an unrelated outer-namespace process, not proof the spawned child survived.
Do not signal that numeric PID. Fix fixture identity/verification to account for
PID namespace and birth identity while still meaningfully verifying cleanup of
only the group actually created. Re-run both affected cases after correction.

New waitid WNOWAIT/WNOHANG positively observes correct parent exited0 without
reaping, child readiness, returncode0 after producer cleanup and final
ChildProcessError reaping check. Its birth comparison prevents naive recycled/
foreign-PID assertions, but mismatch alone must not be advertised as direct
proof of the actual child having died. Add a namespace-consistent owned-group
or outer child identity assertion if claiming direct descendant cleanup proof.

Producer/evidence/commands/correlation/run/scenario diffs against4c are empty.
Delta changes tests/report metadata only. No redundant full40 run performed.
Author strict40PASS5.030 claim is historical and attributed; this independent
fresh two-case failure must remain explicit until fixed. No real capture,
network/VPP/nft/SSH or product writes. No whole live-producer or task completion.

Earlier reviewer-only supplemental probes had invalid readiness/reaped-parent
preconditions and attempted cleanup by child-written numeric PID before this
namespace mismatch was understood. They are not acceptance evidence; further
numeric child PID signalling is prohibited in this review. All subsequent
verification uses only actual spawned process ownership/identity.
