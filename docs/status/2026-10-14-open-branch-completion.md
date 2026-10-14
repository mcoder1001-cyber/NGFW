# NGFW open branch completion — 2026-10-14

The audit started at main `4f70b3315b9641738f9d193bbad6fb271c9a3d3f`, with no
open PRs/issues and 212 source tasks marked merged. That count does not certify
all deferred appliance acceptance.

| Original branch | Audit result | Disposition |
|---|---|---|
| `claude/status-check-report-aa44ab` | An installed-host deployment helper had not reached main | Complete its validation, backup, version-order and health controls with focused tests |
| `codex/appliance-half-resource-defaults` | Operational records and one-off recovery artifacts, no unfinished product source | Preserve the existing branch; its artifacts are historical evidence, not supported replay tooling |
| `codex/restore-ethernet-display-dual-host` | Product code already integrated by PR236; only six status lines remained | Do not replay the product merge; preserve the existing closeout branch |

This integration publishes source and a compact audit. Historical appliance
addresses, inventories, startup configuration and operational artifacts are not
copied into this change. Automatic approval review rejected the proposed aggregate
operational-record publication; that portion remains unmerged in main. Existing
source branches are preserved, including the reviewed deployment checkpoint
`72fe819b9e4d4e31be1b70e6d05125d1cae8d4a9` (tree identical to local worker `0810ec94`).

## Changes

The helper validates the complete four-package set, checksums, metadata and cache
provenance before use. It uses private staging, refuses invalid/downgrade/dependency
changes, requires configuration and PostgreSQL backups, checks each required
service independently before and after installation, preserves default routes,
and verifies a fresh SSH connection. The fleet stops on the first failure.
The lock is validated and held in a private directory. Preparation emits
[ordered development versions](../decisions/DEC-248-development-package-ordering.md).
The existing strict packaging workflow discovers the new fixtures and triggers
on helper-only changes; mandatory CI additionally runs ShellCheck for the helper.

## Actual verification and independent reviews

The product/test/operator-guide files in this integration are byte-identical to
the independently reviewed worker source `0810ec94ce30b42dc102133934a5a1a3451f3975`.
R1 correctness, R2 security, R4 shared-host safety, R8 operability and R7 evidence
reviews approved their scopes. Independent results on that source:

```text
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_deploy_hosts.py -v
Ran 20 tests in 45.037s
OK
python3 -m unittest discover -s deploy/debian/ngfw/tests -p test_prepare.py -v
Ran 4 tests in 0.818s
OK
bash -n deploy/deploy-hosts.sh deploy/debian/ngfw/prepare.sh
exit 0
```

R1 additionally exercised seven version/service failure probes. R2 repeated the
lock-symlink and master-key controls. The manager's unchanged check with gitleaks
passed on the complete pre-publication integration: 102.56 KB scanned, no leaks;
212-task board valid. These fixtures used disposable command seams and minimal
real Debian archives; no live host, credentials or services were accessed.

The local full quick attempt stopped at generation because uncached Go modules
could not be downloaded (`connect: network is unreachable`); this is BLOCKED-ENV,
not PASS. An unchanged packaging baseline test also hit EINVAL because the local
namespace maps only UID 0 and cannot chown to UID 65534. No check was weakened.
Complete hosted quick, ShellCheck and strict packaging must pass on the final PR
head before merge. Exact PR/head/run/merge receipts are recorded in the PR body.

## Remaining acceptance

The final PPPoE/RA and seven primary Traffic-B receipts supersede older OPEN
statements only within their tested scope. The existing central ledger still
contains historical statements requiring reconciliation; the proposed aggregate
rewrite is retained separately because its publication was rejected.

Current-source signed ISO inputs/build/install/boot, full appliance reboot and
recovery, HA and unproven feature-specific native cases need actual target
campaigns. PR233 retains a TD2 PostgreSQL setup timeout before product logic and
14 skipped tests, with no final successful rerun. Product license metadata needs
the owner's authoritative text under `PENDING-P10-product-license.md`; no license
or password policy is selected by this recovery. Local PostgreSQL, Valkey, VPP and
agent runtime are absent. Fresh-host installation and runtime acceptance remain
distinct from this source completion.
