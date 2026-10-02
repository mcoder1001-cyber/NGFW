# P10 firstboot third-round arbitration

Case: mandatory fresh third-round verification (D156), security/process arbiter delegated by CI delivery manager. Developer and original reviewer agree on prior findings; no factual dispute is invented. Arbiter wrote no product code or tests and did not start services or alter host configuration.

Reviewed local HEAD `f5184cd123842a459104f1cf7dbb135c21ffbb29`, remote checkpoint `649292f70ca784095a7fe43c68db28bc48ac8719` (manager reports same product tree); full firstboot delta from `940e6d81`. Applicable rules: AGENTS owner instruction (real failures block; laboratory acceptance may defer), decision-policy security boundary, ARBITER-PROMPT third-round finish/split/reassign, contributing unchanged quick gate.

## Ruling: finish with one bounded correction; do not merge this head

Prior marker cleanup MAJOR is resolved: shipped unit no longer suppresses script execution when the durable completion marker exists; cleanup branch removes remaining credentials without reprovisioning. Prior JWT/random MAJOR is resolved for the generated/existing JWT assignment: generation failure aborts; exact assignment count precedes extraction, including whitespace-shaped duplicates; missing, empty, valid-empty, empty-valid and valid-valid all refuse completion. No unresolved issue was found in these two specific fixes.

**New MAJOR: persisted environment can still diverge from bootstrap runtime.** firstboot.sh checks DB/master-key assignment presence using grep, then exports canonical constants for bootstrap-db.mjs. vrx-api.service instead loads the complete file with EnvironmentFile, whose later assignments can override those validated lines. A file containing a correct DB assignment followed by a different DB assignment, or a correct secret-key assignment followed by a different secret-key assignment, is accepted; firstboot publishes completion and removes credentials despite runtime API using a different database/key. Root-only file permissions do not make this an idempotent recovery invariant: malformed persisted inputs must fail closed just as duplicate JWT inputs do.

Independent reproduction used the existing private fixture, first failed at nginx to generate persisted state, appended one conflicting duplicate, retried without the failure. Both cases returned exit 0, completion true, credentials retained false. No live appliance was used. Settings values/secret contents are intentionally omitted from this report.

Developer next: enforce exactly one canonical DB assignment and exactly one canonical master-key assignment, rejecting whitespace-shaped competing assignments before parsing; audit incompatible runtime overrides (notably VRX_JWT_KEY_FILE precedence) so bootstrap and deployed API use identical verified inputs. Add failure-retention regressions for both duplicate orderings and whitespace forms. Preserve existing legitimate runtime configuration or document any deliberately refused firstboot override; do not silently rewrite persisted operator configuration. This is validation of existing designed secret/DB boundaries, not authorization to reshape those boundaries.

Reviewer next: independently verify narrow fix and failure retention. Manager next: retain whole P10 running; publish checkpoint, obtain unchanged hosted full quick on current integration tree, then merge coherent approved scope. No additional arbiter round is needed merely because this bounded correction is made; escalate genuine disagreement or additional major scope.

## Actual evidence

- `python3 deploy/debian/vrx/tests/test_firstboot.py`: 4 tests, 0.845s, OK. Fixture flow only.
- `python3 deploy/debian/vrx/tests/test_packaging.py`: 7 tests, 0.661s, OK. Private temporary paths.
- `node --check deploy/debian/vrx/assets/bootstrap-db.mjs`: exit 0.
- `bash -n deploy/debian/vrx/assets/firstboot.sh`: exit 0.
- `git diff --check`: exit 0.
- Additional private fixture reproduction: conflicting DB duplicate and conflicting master-key duplicate both exit 0 / completed true / credentials retained false; MAJOR upheld.
- Source inspection: bootstrap-db.mjs uses deployed migration/AuthService/argon2 confirmation and sanitized failure reporting; shipped API unit loads api.env, TokensService gives JWT_KEY_FILE precedence over JWT_SECRET.

Hosted full quick, real PostgreSQL/Valkey/bootstrap execution, installed systemd parsing/sandbox, nginx/appliance boot, package/APT activation and lab acceptance: **NOT RUN by arbiter**. This ruling does not mark all P10 done. Deferred appliance acceptance belongs in the single DEFERRED-ACCEPTANCE campaign; unfinished firstboot activation/APT remain code work.

Manager should append this bounded ruling to ARBITRATION-LOG when integrating; arbiter ownership in this assignment is this report only.
