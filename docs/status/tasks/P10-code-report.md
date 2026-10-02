# P10 packaging foundation code report

Status: **RUNNING — bounded source foundation, not complete appliance acceptance**.
Source frozen at local `2892785f1006eb990f977ebcb8609d084c7a7a28`, published as
`1faec2e1609476bc98f6d36f5a87f3e3945f3748` on
`task/P10-packaging-finish-20261002`. This document adds evidence only.

## Implemented source

- Debian source and four binary package layouts, build staging and fail-closed
  consumption of the original verified seven-package VPP runtime manifest.
- Product helper paths, agent/API systemd units, HTTPS nginx configuration,
  protected idempotent TLS generation and explicit product service ordering.
- Firstboot PostgreSQL/API migration and existing-admin bootstrap verification,
  canonical runtime key/environment validation, durable completion marker and
  subsequent credential cleanup. Tests exercise failure retention and retry.
- Early explicit-interface static base firewall generation, separated from
  database/auth initialization to preserve early nftables ordering. No wildcard
  dataplane interface acceptance and no live host firewall loading.
- Local signed APT publication code with signing-home/output isolation and exact
  VPP dependency validation. No release repository was published.
- Explicit appliance installer preflight, maintainer-script no-start policy,
  same-filesystem atomic guard/restoration and protected persistent recovery
  evidence. Interrupted installation requires operator recovery before retry;
  this is not automatic recovery or proof of power-loss durability.

## Actual checks

At source `2892785f`, `python3 -m unittest discover -s
 deploy/debian/vrx/tests -p 'test_*.py'` ran **23 tests in 4.892 seconds:
22 PASS, 1 SKIP**. The skip is real GPG signing because the isolated environment
cannot start/connect its GPG agent; signing is not reported as passed. Checks
include real TLS generation, Debian fixture inspection, canonical firstboot
failure paths, typed firewall generation and installer no-start/restore/SIGKILL
fixtures. They do not run host package installation, real PostgreSQL bootstrap,
VPP, nftables loading or appliance services.

Shell syntax, Node bootstrap syntax and whitespace checks passed. Offline
systemd 255.4 security analysis measured API **3.0** and agent **5.0**, satisfying
source score targets; this is not target-OS boot acceptance. Startup driver
rebind writes belong to the operator/transient apply unit, not an agent child.
Independent review reports retain their original BLOCK findings and subsequent
resolution/approval records alongside this report.

The unchanged local full quick gate at `8b761430` did **not pass**: 34 of 35
Turbo tasks succeeded before API unit tests failed on environment restrictions
(Unix socket listen EPERM and foreign-owner chown EINVAL, with cleanup cascades).
Go steps were not reached. Evidence: `.scratch/p10-quick.log` and
`/tmp/vrx-ci/NGFW-packaging-finish-20261002-165449-5/08-turbo.log` in that run.
No assertions or CI gates were weakened. An unchanged hosted full quick gate on
the final integration commit is required before merge; prior main success is
not evidence for this feature commit.

## Remaining implementation and decisions

1. Dynamic LCP/punt-interface membership synchronization is **UNBUILT**. The
   static base DROP chain can affect later host ACL acceptance; appliance
   functionality is incomplete until the typed runtime handoff is implemented.
2. Agent ownership operations need a security/architecture decision: existing
   atomic writers require foreign UID ownership while approved capabilities
   omit CAP_CHOWN, and some `/etc` atomic writers require parent-directory
   mutation incompatible with narrowly writable file paths. See
   `../../decisions/PENDING-P10-agent-file-ownership.md`. No capabilities
   or broad `/etc` write access were added to bypass this boundary.
3. License/copyright metadata needs an authoritative owner decision before
   release. No license was invented.
4. Actual VPP artifacts, real signed publication, fresh-VM/chroot installation,
   package maintainer guards, real DB/auth bootstrap and crash boundaries,
   nftables ordering/boot, service activation and renderer acceptance remain
   **NOT RUN**. The single campaign is `../DEFERRED-ACCEPTANCE.md`; lab-only
   deferrals are not a merge blocker under the user's instruction, but real
   unbuilt functionality and security decisions are not labeled lab passes.

## Next action

Publish this evidence-only checkpoint, finish independent review of the frozen
installer durability delta, integrate the bounded foundation onto current main
and run the unchanged hosted full quick gate. Merge only that reviewed bounded
scope when green; keep P10 RUNNING and retain the centralized deferred campaign
and unresolved functional/security work. No release activation is claimed.
