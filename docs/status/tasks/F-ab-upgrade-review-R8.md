# F-ab-upgrade independent R8 review

Inspected source: `be1448118afd246f39b0d6a7db01d2f986448482`.

Review role: fresh independent R8 operability and packaging. Product code was not edited. Shared packages, services, boot configuration and VPP were not changed. This aspect review does not replace R1/T1 complete quick CI, R2 security, or R4 data-plane acceptance. Source hashes below are the inspected frozen trees, not an assertion that GitHub has merged them.

No BLOCKER or MAJOR findings. Trial confirmation requires the actual agent Health RPC and API response; missing/non-executable probes are retried to the deadline and roll back. Pre-migration PostgreSQL backup is ordered and required before firstboot/API and fails closed. State uses root-owned shared storage, locking and atomic fsync/rename. Slot activation leaves the previous default in place until confirmation; interrupted staging invalidates the inactive slot before writing. Shared database automatic restore is deliberately absent and only backward-compatible migration policy is accepted. Firmware boot, crypt-volume unlock and real populated database migration remain documented appliance acceptance.

MINOR: `deploy/debian/ngfw/debian/control:12` declares new runtime tools zstd, grub2-common, e2fsprogs, mount and util-linux, but `docs/09-os-packages.md` has not been aligned. Add the tool list and their A/B purpose to that existing package reference. This does not block installation: Debian dependencies are declared. Manager notified.

Commands run in `/root/ngfw-wt/ready-ab-20261005`:
```text
deploy/upgrade/tests/run.sh
Ran 17 tests in 3.293s
OK
python3 -m unittest discover -s deploy/debian/ngfw/tests -p 'test_*.py'
Ran 35 tests in 38.347s
OK
```
These runs included unsigned/tampered bundle denial, extraction ownership, grubenv lifecycle, failed health rollback/reboot, missing/non-executable probe, storage privileges and staging behavior. They do not boot firmware or migrate a production database.

Verdict: **APPROVE** for the inspected R8 scope.
