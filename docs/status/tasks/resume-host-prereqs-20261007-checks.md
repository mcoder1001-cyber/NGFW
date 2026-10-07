# Fresh read-only/offline verification receipt

2026-10-07, worktree /root/ngfw-wt/resume-host-prereqs-20261007, product source frozen at base3ddb1680e475e94d43e8036cd3776bc60c87208b. This worker modifies only its four task status files. No actual target installation, packet, performance, shared-service or security mutation.

`python3 -B docs/status/tasks/TD-19-run-fixtures.py` exit0:

```text
TD19 fixtures: tests=44 failures=0 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0
Ran 44 tests in 102.475s
OK
```

`tools/ci.sh check --base origin/main` before initial checkpoint exit0:

```text
no contract files changed in the 0 commit(s) of HEAD since origin/main (3ddb1680e)
ok: no secret-shaped strings
ok: gitleaks — scanned ~0 bytes (0) in 766ms no leaks found
board valid: 212 tasks; read-only validation
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m14s)
```

Syntax commands exit0:

```bash
bash -n scripts/00-add-repos.sh scripts/20-install-build.sh scripts/40-install-lab.sh tools/lab
bash -n docs/status/tasks/F-nat46-host-evidence/host.sh
shellcheck -x -P SCRIPTDIR scripts/00-add-repos.sh scripts/20-install-build.sh scripts/40-install-lab.sh tools/lab
git diff --check
```

Initial plain shellcheck reported SC1091 for lib.sh/os-release; -x alone still reported relative lib.sh. Existing provisioning workflow's `-x -P SCRIPTDIR` resolved both and passed, including tools/lab. No suppressions/source edits.

`bash scripts/20-install-build.sh --check-config` exit0:
`Go=1.26.0 protoc-gen-go=v1.36.12 protoc-gen-go-grpc=v1.6.2 govpp=v0.13.0`.
`bash scripts/40-install-lab.sh --check-config` exit0: pinned containerlab0.79.0 digest reported. This confirms current pins only, not their permitted platform scope.

`tools/lab provision ngfw-b` and `tools/lab provision ngfw-c` each exit3. Selected transcript:

```text
REMOTE MODE (Ubuntu ubuntu-26.04 + our .debs from /root/vpp/build-root + DPDK dev <pci> on vmxnet3)
1. ssh root@tbd
2. verify the published vpp.source manifest with the original install gate; transfer only its seven shipping runtimes
ngfw-b is 'planned' — create the VM first
ngfw-c is 'planned' — create the VM first
```

No --apply was supplied. The tool prints plans without contacting those targets.
`python3 -B test/topology/security-host-acceptance/run.py --dry-run` exit0:

```text
slot8: real API/agent, isolated VPP; expiry/remove/restart/rollback; global block forward/reverse/unlisted/unselected/replay; private netns nft local-input/rollback
```

Read-only inventory: stat found /run/vpp/api.sock and cli.sock as sockets; /srv/ngfw-artifacts/vpp and apt missing (stat exit1). systemctl show vpp: ActiveState=active/MainPID=1014/NRestarts=0. Handover document says pending; no responsiveness probe or VPP mutation performed.

Published source recovery: both ancestry commands exit0:

```bash
git merge-base --is-ancestor 5e2d08c7f4bfdbd330ed0ffcde2bc231aafe8d87 origin/main
git merge-base --is-ancestor e3517a98353dd081558465adcf2bac04eaae3aad origin/main
```

Initial checkpoint push succeeded; exact ls-remote receipt:

```text
2fe5e6422a510232e0693c4b9dcb80857c11cdef refs/heads/codex/resume-host-prereqs-20261007
```

Remote main rechecked and remained3ddb1680e475e94d43e8036cd3776bc60c87208b. Historical main complete hosted quick37590128647 passed on that SHA. No new-branch complete quick or independent audit review claimed. Final publication SHA is the branch ref containing this document (cannot be self-embedded); verify with `git ls-remote origin refs/heads/codex/resume-host-prereqs-20261007`.

Final precommit `tools/ci.sh check --base origin/main`: exit0, `check PASSED (0m24s)`; gitleaks scanned initial checkpoint2.99KB, no leaks. Working-tree forbidden-pattern checks included the new audit/receipt. Final commit history secret scan follows publication, using the same unchanged check mode.
