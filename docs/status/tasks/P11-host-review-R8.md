# P11-host independent R8 review

Inspected source: `f59c3ea7`.

Review role: fresh independent R8 operability and packaging. Product code was not edited. Shared packages, services, boot configuration and VPP were not changed. This aspect review does not replace R1/T1 complete quick CI, R2 security, or R4 data-plane acceptance. Source hashes below are the inspected frozen trees, not an assertion that GitHub has merged them.

No BLOCKER or MAJOR findings. The wrapper reserves the fixed slot-8 packet fixture, takes its exclusive fixture lock plus the shared lab lock, refuses namespace/veth collisions and targets a disposable VPP. It retains diagnostics privately and publishes only sanitized completion/hash evidence. Production agent builds and native-plugin outputs stay in private scratch. Required test markers include owned rollback before disposable VPP teardown; the wrapper rejects skip output and verifies shared VPP identity and slot cleanup. Heavy commands use the shared limiter. Existing isolated-VPP finally cleanup targets only its spawned PID.

Commands run in `/root/ngfw-wt/ready-p11-host-20261005`:
```text
python3 -m unittest discover -s test/topology/ipsec -p 'test_host_acceptance.py'
Ran 2 tests in 0.000s
OK
shellcheck test/topology/ipsec/run.sh
exit 0, no output
```
I did not rerun the full privileged packet campaign. Existing sanitized packet evidence is reviewed as earlier tester evidence, not a run attributed to this reviewer. Root quick CI and mandatory R4/T3 reports remain separate merge prerequisites.

Verdict: **APPROVE** for the inspected R8 scope.
