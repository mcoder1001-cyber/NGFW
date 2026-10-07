# P12 independent source review — BLOCK

Reviewed exact PR203 source c7ef8faa0 against actual main f6ae6e555, independently of historical receipts. Applicable R1/R2/R4/R7. Product edits: none.

## BLOCKER R2/R4 — proc-root namespace inventory lacks process identity bracket

`test/topology/frr-linuxcp/private-fib.py:81–115` snapshots members(current), then reads `/proc/<pid>/root/run/netns` and retains namespace descriptors without verifying saved immutable starttime and network-namespace identity on the successful read path. A PID can be reused or leave the owned outer namespace between enumeration and open. A foreign mount tree's allowed `ns-w14-lan` handle then becomes retained cleanup membership; later exact-PID signal checks cannot correct the incorrectly widened namespace ownership. NS_GET_NSTYPE proves nsfs type, not the inventory producer's identity.

Independent host-independent reproduction: saved PID123 identity `(old-start,net:[777])`; actual identity changes to `(new-start,net:[888])`; proc-root directory mock supplies ns-w14-lan nsfs inode999. Current code outputs `retained foreign netns {net:[999]: 42} observed {ns-w14-lan}` and calls process_identity(123) **zero times**. No real processes/namespaces/mounts were opened or signaled.

Required fix: bracket proc-root inventory and accepted descriptors with the saved immutable process identity, before inventory and before retaining/observing each descriptor; close and reject changed identity without widening cleanup scope. Include reuse and same-starttime namespace migration controls, including identity change during open.

## Actual checks

- `python3 test/topology/frr-linuxcp/private-fib.py --self-test`: Ran10 tests0.023s, OK.
- `TMPDIR=/asr GOMAXPROCS=4 GOPROXY=off go test -race -count=1 ./internal/agent -run 'Test(P12CleanupInventoryRequiresSuccessfulRead|PageRemoteAccessSessions)'`: `ok ngfw/agent/internal/agent 1.343s` on own temporary combined P12+RA review tree.
- `git diff --check`: no output, exit0.
- Initial cleanup test attempt overlapped my own RA checkout and failed transient compilation while files were being updated; discarded that run and reran after the tree stabilized as above. This is not attributed to product source.

Cleanup failed reads/residue and rollback running-config errors now reject false success, defer agentStop is present, actual main Wave-B preflight guards preserved. Strict immutable fallback checks do not accept active/custom links. Native mgmtd30s failure remains unknown and200route evidence is outstanding. No fullCI, host acceptance or VPP/daemon operation performed.

Verdict: BLOCK until the identity inventory issue is corrected and independently rechecked.
