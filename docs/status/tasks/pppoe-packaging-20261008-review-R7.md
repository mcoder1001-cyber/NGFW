# PPP carrier packaging — independent R7 review

Reviewed seven-path packaging delta `e8dbe38a..152cc08f0eb5edef04d4f2bb077167ef1a8159dd`, local tree `f7d5c237c2f14b6852d4c28a075d207c5fc2e5f1`. Manager publication receipt associates remote `f413116214b3508acb693e60bce1b071e2c106e7`; R7 did not independently query GitHub. Helper/assets are read-only untracked inputs from a separate author, not part of this packaging commit. Actual helper SHA256 inspected: `26bc1254029bc5dd4ecbb652eb86a0745ac325e8a93a59592b85ce22d70a35a3` (documented helper checkpoint `d5504b30`).

No blocking finding in this scoped packaging delta. prepare.sh stages the fixed Python helper at 0644, four dispatch hooks at 0755, two dormant unit templates and tmpfiles configuration at 0644. Manifest destinations match their fixed runtime paths. Python invocation does not require executable permission on the helper. Missing helper/assets cause install under set -e to fail; the unit uses the exact helper path with no fallback. Runtime tool dependencies match fixed ip/nft/nsenter/sysctl/setpriv/pppd/run-parts paths. Units remain without Install sections, and dh_installsystemd retains --no-enable --no-start. Package source preparation copies tests and identity assets; asset tests explicitly select staged output in the prepared tree and independently pass there through the prepare fixture.

The existing WAN probe and digest sidecar remain installed. The corrected Debian strip/dwz rules exclude the attested WAN executable in addition to prior RA helpers; actual rule fixtures demonstrate attested bytes unchanged while an unrelated native file undergoes both transformations. This is transformation-contract evidence, not a real built/installed Debian package.

Independent focused verification (private fixtures, no CI or host activation):

```text
$ python3 deploy/debian/ngfw/tests/test_prepare.py -v
 test_build_mutation_refuses_release_staging ... ok
 test_failed_build_does_not_stage_stale_dist ... ok
 test_stale_dist_rebuilt_before_staging ... ok
Ran 3 tests in 0.681s
OK
[exit 0]

$ python3 deploy/debian/ngfw/tests/test_pppoe_assets.py -v
 test_debhelper_transforms_preserve_attested_bytes ... ok
 test_dormant_units_and_fixed_execution_graph ... ok
 test_exact_private_tmpfiles_and_hook_dispatchers ... ok
Ran 3 tests in 0.076s
OK
[exit 0]

$ bash -n deploy/debian/ngfw/prepare.sh
[exit 0; no output]

$ git diff --check
[exit 0; no output]
```

Six selected tests ran with zero skips. The prepare fixture runs the actual staging script with private harmless build/dispatcher boundaries and also executes the packaged asset tests from prepared output. The previously reported existing packaging fixture UID65534 fchown EINVAL remains an environment error and is not converted to PASS; R7 did not rerun or waive it.

MINOR — WIP checkpoint recovery: its latest source receipt still records earlier local bd96476a and omits the final manager-reported remote SHA. Update that recovery record to the frozen local/remote/tree above, retaining earlier checkpoints as history. This review records the exact association so no final-source identity is inferred from old prose.

Integration condition: the packaging branch cannot stand alone because helper source/assets are untracked external inputs. Final integration must include the separately reviewed helper's latest recovery fix at the same paths and rerun applicable focused checks on the combined tree. This source approval does not establish unit startup, live PPP/network behavior, package installation, native acceptance or final CI. Owner's postponed CI instruction remains intact.

Verdict: **APPROVE** scoped packaging/evidence. R7 wrote only this report; no product edits or commits.
