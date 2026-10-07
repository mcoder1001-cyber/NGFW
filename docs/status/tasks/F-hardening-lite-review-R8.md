# F-hardening-lite independent R8 review

Inspected source: `726d92141402459505ee27fdc6fafc7aeef94295`.

Review role: fresh independent R8 operability and packaging. Product code was not edited. Shared packages, services, boot configuration and VPP were not changed. This aspect review does not replace R1/T1 complete quick CI, R2 security, or R4 data-plane acceptance. Source hashes below are the inspected frozen trees, not an assertion that GitHub has merged them.

No BLOCKER or MAJOR findings. Packaging ships profiles under `/usr/lib/ngfw/hardening` without enabling them or changing shared host controls. Offline staging refuses the live root and symlink ancestors before writes; repeat staging preserves optional controls. Exact staged-file inspection is distinct from effective kernel/SSH/service inspection. Agent and FRR retain mount/namespace operations and their configured write paths; Kea retains AF_PACKET; Node does not receive MemoryDenyWriteExecute. Existing daemon runtime compatibility is expressly unverified and profiles remain opt-in until appliance smoke tests. No systemd exposure score is represented as daemon acceptance.

Command run in `/root/ngfw-wt/ready-hardening-20261005`:
```text
deploy/hardening/tests/run.sh
Ran 10 tests in 0.820s
OK
```
Tests include repeat staging, management drift, offline escape refusal, exact signed snapshot use and signing key rotation. This approval covers offline hardening tooling and package staging, not enabling profiles on an untested appliance.

Verdict: **APPROVE** for the inspected R8 scope.
