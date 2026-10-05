# F-images fresh independent R8 review

Frozen inspected source: `6bd58ecbe10efb45a486139777655c6c826093cd`. Reviewer owned only these reports and private RAM fixtures; no product edits or shared host mutation.

The repaired common mutation-root preflight now covers configure, put and GRUB before kernel reads or writes. It refuses the host root, aliased root ancestors and boot/grub/output symlinks; regression fixtures preserve outside sentinels. Existing target-only mount/GRUB operations and cleanup remain unchanged. No new runtime dependency or host service activation. No BLOCKER or MAJOR findings. Full signed appliance build and VM/cloud boot acceptance remain separate deferred execution, and this review does not certify those.

Commands run in `/dev/shm/ngfw-review-r8-final-20261005/.scratch/images`:
```text
python3 -m unittest discover -s test/topology/images -p 'test_*.py'
Ran 13 tests in 0.665s
OK
shellcheck -x deploy/image/vm/*.sh
exit 0
```

Verdict: **APPROVE** for inspected R8 scope. Complete unchanged quick CI and other mandatory panel reviews remain required.
