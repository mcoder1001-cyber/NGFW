# Independent final installer durability verification

Frozen local `2892785f1006eb990f977ebcb8609d084c7a7a28`, remote `1faec2e1609476bc98f6d36f5a87f3e3945f3748`. Scope bounded to follow-up of installer crash durability MINOR. Prior findings/reviews preserved; reviewer changed only this report.

MINOR resolved for documented manual-recovery behavior: recovery directory now protected 0700 under /usr/sbin beside policy, with original regular file/symlink and had-policy metadata persisted before replacement. Guard publication/restoration use same-filesystem renames. Trap is armed before publication; flag is set before rename so interrupted/failed rename retains evidence and refuses unexpected policy instead of deleting recovery data. Sync calls persist filesystem state before APT. Stale record blocks another installation before mutating APT. Normal success/failure restores exact prior policy; SIGKILL leaves no-start guard and durable recovery evidence for operator review.

R1/R2/R7/R8 APPROVE bounded installer/source-foundation checkpoint. No unresolved findings in this delta. This is manual recovery after a terminated process, not proven automatic recovery or simulated machine power loss. Whole P10 remains partial: privilege boundary decisions, dynamic punt synchronization, licensing and actual package/appliance acceptance are not waived.

Independent actual checks:

```text
python3 deploy/debian/vrx/tests/test_runtime_profile.py
Ran 2 tests in 0.594s
OK
bash -n scripts/10-install-runtime.sh
exit 0
git diff --check
exit 0
tools/ci.sh check --base origin/main
ok: gitleaks — scanned ~131589 bytes (131.59 KB) in 204ms no leaks found
check PASSED (0m03s)
```

Nine policy-flow combinations cover absent/regular/symlink original against success/APT failure/owned installer-process SIGKILL. Fixtures verify denial 101, restoration metadata, protected recovery evidence after SIGKILL and fresh retry rejection without new APT calls. SIGKILL targets only the fixture process spawned by that test; no unrelated process, host policy, package installation or service was touched. Full final hosted quick and real appliance acceptance are still required before corresponding merge/release claims.
