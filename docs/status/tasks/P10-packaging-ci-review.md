# Independent packaging fixture workflow review

Frozen `1cd9ae5176b8f5d269a9e06a50c21abece367feb`. Scope R7 workflow/evidence and independent guard correctness/security inspection. Reviewer changed only this report.

**MAJOR strict-gate gap:** packaging-fixtures.py relies on result.wasSuccessful and no skipped tests, but unittest counts expectedFailures as successful. Independent executable fixture returned exit 0 for an actual failing test decorated expectedFailure. Thus a future failed signing fixture can be masked while the wrapper reports a successful gate. Explicitly reject expectedFailures, summarize them and cover guard status branches. No current fixture uses expectedFailure; the finding is in the new strict guard contract, not a claim existing suite already masks a failure.

Actual independent wrapper guard experiment imported production wrapper and replaced only unittest discovery with private suites; no product edits:

```text
pass -> exit 0
failure -> exit 1
error -> exit 1
skip -> exit 1
zero tests -> exit 1
expected failure -> exit 0 (finding)
```

R7 BLOCK until strict failure guard fixed. Remaining workflow scope is sound: Ubuntu24 runner, exact existing checkout/setup-node action SHAs, Node22.23.2, read-only contents permission, checkout credentials not persisted, no secrets, bounded timeout and canceled superseded runs. Required fixture tools checked before execution. OS utilities/Python/GPG/OpenSSL remain runner-provided versions, not falsely pinned tools; actual hosted proof still required.

Path filters cover existing fixture files and shipped source dependencies they inspect/execute: package assets/metadata, systemd units, VPP validator/startup script, APT/runtime scripts and rsyslog/capture default paths. Pure tests do not invoke real product DB/bootstrap/VPP/nft/netlink/service/publication. Temporary-key signing uses real GPG in isolated directories and fixture reprepro/VPP commands; it is not actual repository provenance or release publication. Existing fixtures capture private-material output and clean isolated key-agent homes. Whole quick CI and frozen PR67 are unchanged.

Local existing signing environment restriction is a legitimate SKIP, which correctly causes new wrapper to fail; hosted 26/26 proof has not yet occurred. No actual signing/host PASS claimed. Workflow source addition is not full P10 completion or appliance acceptance.
