# Packaging resume — independent bounded R1/R2 review

Reviewed local HEAD `88079ec6048171abf8a2cb9140ad6c6a23bd2bdb`, tree `e79288282cba27082fe511d173e3bfd9187beb1b`; inspected delta from previously reviewed `aa76368a49f55e1dfa6e3198c02fdc9509ffc58a`. Own branch `codex/packaging-review-r1-r2-20261002`, worktree `/workspace/scratch/de92de7d9874/NGFW-packaging-r1-r2`. No product edits, service execution or publication. Manager identifies remote PR63 head `952cd663` as product equivalent; that remote object is absent from local git, so remote equality is not independently asserted here.

Read shared/R1/R2 rules, current R4/R8 findings and verify-round evidence, actual storage postinst/unit changes, shared-host-rules section 12, and ResolveIDScope implementation.

## R1 correctness/tests

Ordinary non-symlink storage reconfiguration preserves existing backup bytes, mode, owner and group in the supplied regression. Creation is idempotent and retains fail-fast shell behavior. The redirected fixture avoids real host accounts and paths. Nine packaging tests pass independently. However symlink-containing pre-existing storage is not covered and is unsafe as the R2 blocker below demonstrates; overall checkpoint cannot merge as-is.

Appliance `VRX_VPP_ID_RANGE=all` matches the pre-existing explicit dedicated-appliance contract in shared-host-rules section 12. Unit adds no conflicting table base; ResolveIDScope rejects missing, malformed and contradictory settings. The unit comment/docs forbid shared-host execution. No new capability or writable-path scope is introduced by that ID declaration. Existing unresolved daemon-ownership and identity parent-directory privilege work remains unfinished, not waived by this review.

R1 bounded ordinary-path verdict: **APPROVE**, subject to R2 blocker; no complete quick/installed appliance acceptance claimed.

## R2 security — BLOCKER

**Root package reconfiguration follows attacker-controlled storage symlinks**, `deploy/debian/vrx/debian/vrx-api.postinst:6,10`.

Line 6 makes `/data` owned by vrx; API sandbox explicitly allows writes to `/data`. A compromised vrx process can replace a child such as `/data/backups` with a symlink to an external root-owned directory. New line 10 runs root `install -d -m 0750 -o vrx -g vrx` on that symlink. GNU install follows it and changes the target's owner/mode. On the next root package configure, a link to `/etc` would grant vrx ownership of that directory. This crosses the API-to-root privilege boundary despite service sandbox restrictions because the maintainer script executes outside the service sandbox.

Safe reproduction executed only under a fresh temporary directory: create `data/backups -> outside`, outside initially mode 0700; execute a redirected copy of the actual postinst (account lookup replaced by fixture no-op, owner/group replaced by current test UID/GID, paths all redirected into the temporary root). Actual output:

```text
redirected actual postinst exit: 0
outside directory mode: before0700 after 0o750
backups symlink retained: True
```

This demonstrates target metadata mutation without touching `/etc` or any host configuration. The owner-changing flags are unchanged in product; non-root fixture proves mode traversal, not a live privilege escalation.

Required correction: provision storage without following symlinks or allowing a check/use race in the service-writable parent. A simple shell `test -L` followed by install is insufficient while vrx can rename children. Use an appropriate no-follow directory-descriptor approach (including safe ancestry) or another demonstrably race-safe design that retains required data/permissions and does not silently delete operator content. Add negative symlink regression and preserve the existing ordinary reconfigure test. Do not broaden privileges or resolve unrelated pending ownership policies as a workaround.

R2 verdict: **BLOCK** until this confirmed privileged path traversal is fixed and independently verified.

## Actual independent commands

```text
python3 deploy/debian/vrx/tests/test_packaging.py
.........
Ran 9 tests in 0.764s
OK
sh -n deploy/debian/vrx/debian/vrx-api.postinst
bash -n deploy/debian/vrx/prepare.sh
git diff --check aa76368a HEAD
```

Tests and syntax/diff commands returned success. Additional safe Python fixture reproduction is documented above. No complete quick gate, real package install, service start, VPP operation, root-owned target manipulation, or appliance acceptance run.

**Combined bounded verdict: BLOCK.** Existing R4/R8 approvals do not supersede the new R2 finding.
