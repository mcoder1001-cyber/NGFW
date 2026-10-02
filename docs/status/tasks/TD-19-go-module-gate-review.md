# Go-module installer fixture gate independent review

**R1/R2/R4/R7/R8: APPROVE bounded new gate** exact `b17b7f5db172d2364e976fbaba38b84ba3ed06af`, independently checked2026-10-02 against frozen source basebe646973. This does not replace separately reviewed installer security scope or establish target installation/lab acceptance.

Complete base-to-head delta is only new runner/workflow/WIP. Original script/helper/six cases and original provisioning36 runner/workflow are byte-identical. Explicit inventory requires precisely six named ModuleVersion methods, then executes exactly those names and requires actual completed6. Failure/error/skip/xfail/xpass, zero/reduced completion and empty/missing/extra inventory refuse. Import/source errors fail nonzero. Policy controls use genuinely executed unittest suites and real loader method inventories, not fabricated result objects. Constant policy exec strings contain no external input. No test assertions or existing gate were weakened.

Workflow has pinned existing checkout, contents:read, no persisted credentials, Ubuntu24/five-minute bound and path triggers for source script, module, exact source tests, runner/workflow. Actual ShellCheck availability/version/source check runs before policy and six source cases; no package install/network/host setup or secrets. Original provisioning36 remains complementary, not replaced or incorrectly recounted as six acceptance. WIP explicitly says local ShellCheck NOT RUN and hosted/full/current-main gates pending.

## Independently executed evidence

- `python3 .github/scripts/go-module-version-fixtures.py --check-policy`: **12 controls PASS** (six real outcome suites, zero/reduced suites, four method inventories).
- `python3 .github/scripts/go-module-version-fixtures.py`: **6 source tests PASS**,0.125s, all non-success counts zero; actual inventory completed6.
- `bash -n scripts/20-install-build.sh`: PASS. `git diff --check`: PASS. Source/original36 identity diff: empty/PASS.

No heavy build, actual Go/installer/network/APT/host/live traffic, main/PR86/board edit or publication performed. ShellCheck remains unavailable locally / NOT RUN, so no hosted lint PASS inferred. Final current-main composition, unchanged full quick and actual hosted ShellCheck/six gate required before integration. Separately approved source/provenance report remains separate evidence; whole TD-19 is incomplete.
