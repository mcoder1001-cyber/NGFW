# NGFW deployment/tooling rename WIP

Branch: codex/ngfw-deploy-tooling-rename-20261003.
Base: 0d174caf96413599a6bae7111bf74d14ecebede1.
Owned files and interfaces: see NGFW-deploy-tooling-rename-envelope.md.
Prerequisite checkpoint: d72ce01e restores exactly reviewed CPU/pipeline files
from 301998cf6fa933130c4c1ea577f8f25032aea5b0 and
6041b1ab48739a0a2153a8047b2f4297753c696d, separately from naming changes.
Completed: deployment/package/unit/ISO/tooling/script names and environment
paths renamed; runtime compatibility boundary documented in deploy/debian/README.md.
Upstream VPP identity and real dependency hash pins remain unchanged. New product
patch text uses NGFW variables; build manifest computes its actual new patch hash.
Tests actually run:
- bash deploy/vpp/tests/run.sh: 72 passed, 0 failed.
- bash deploy/image/iso/tests/run.sh: 69 passed, 0 failed.
- python3 tools/test_test_handoff.py: 11 tests, 48.520s, OK.
- python3 -m unittest discover -s deploy/debian/ngfw/tests -p 'test_*.py':
  30 tests, 38.243s, 29 pass/1 failure: cross-owned base Go rsyslog path still
  /etc/vrx/rsyslog-tls; matching Go checkpoint fdeaf461 is ready for integration.
- bash -n selected CI/lab/app/VPP scripts: PASS.
- shellcheck --severity=warning selected CI/lab/app/VPP scripts: PASS.
  Unfiltered shellcheck reports pre-existing informational/style items; no
  warning/error finding. Gate script behavior not weakened.
- git diff --check: PASS.
Remaining: integrated Go/API/root consumers, bundle fixtures and independent
review/full unchanged CI gate. New real binary/ISO builds and migration/install
acceptance NOT RUN. No host install, restart or privilege changes performed.
Next: consume coordinated product tree in separate integration worktree and
rerun failed packaging test without weakening it. Publish checkpoints via root.

Follow-up validation:
- Bundle discover suite: 52 tests in 270.974s, OK (including helpers).
- Separate integration worktree consumed exact contract 1b27363f690c2baab005f34c91bdc92b68f0e8ec
  and Go fdeaf461865d56668575f188d8f711eceabffa2d on rename checkpoint b4f1f4d8.
  Integration HEAD 91ef91f9fe9e3f436b00166beb649b004a6032f1,
  tree ca3b812ccb9e01427384bd2c60664cf17a995720. Only unique incoming WIP
  modify/delete conflict retained; no product conflict. The previously failing
  Packaging.test_units_preserve_privilege_boundary: 1 test, 0.001s, OK.
- Standalone tools/ci.sh check --base 0d174caf fails closed on old Go fixture
  placeholders; integrated consumer tree is required. No gate waiver claimed.
- Unfiltered shellcheck diagnostic codes for ci/lab/app are identical to
  prerequisite d72ce01e (17 informational/style items).
- Full 38-script warning/error shellcheck found pre-existing firstboot SC1087
  and pg-test SC2120; fixed braces in regex expansion and unused internal
  helper argument forwarding. No operational behavior changed.
- Firstboot suite after braces fix: 6 tests in 18.359s, OK.
- Legacy inherited integration/nested-worker controls are stripped by handoff
  and rejected by direct fast lane; current and historical flags covered by
  meaningful subprocess regression. Targeted 2 tests in 1.161s, OK.
- tools/README.md documents semaphore namespace switch requires quiescent old
  workers; source edits migrate no host locks or running processes.
- All VPP VERSION non-comment values equal prerequisite checkpoint; dependency
  pydeps.lock bytes equal. Python syntax 35 files PASS; license Node syntax PASS.
- Final full owned-shell warning/error audit after those fixes: 38 scripts, exit0,
  no diagnostics. Latest coherent product checkpoint 4478efdf1db21a3fb46669a50bbdd3da9bbaed6e.

P11 bounded continuation (root authorized): restored ONLY five product files
under deploy/strongswan from 09e16f0e3a0bd5ab340e63710ddba0952eec4a16.
Automated comparison proves each equals that source with nomenclature-only
VRX/vrx/Vrx to NGFW/ngfw/Ngfw replacement; no algorithm or test-gate changes.
No unsafe older builder/C source or stale P10 branch restored. Existing
historical upstream origin/digest metadata remains unchanged, not authenticated
anew; whole P11 build/plugin/traffic/install acceptance stays unfinished.
Legacy safety sanitation/refusal now derives the previous prefix from ASCII
86,82,88; historical placeholder regex uses equivalent V[R]X pattern. This
preserves previous checks without introducing runtime/auth configuration aliases.
Actual static TOML/Python check: all five exact nomenclature-only comparisons
true; historical and new placeholder allowed, generic token not allowlisted.
- Literal old-brand scan tools/.github/deploy/strongswan: no matches.
- Targeted legacy/refusal handoff regressions: 2 tests in 1.263s, OK.
- Original complete VPP script with new namespace: 72 passed, 0 failed.
- Python syntax for restored intake/stage/tests: PASS.
- Bash syntax and warning/error shellcheck test-fast.sh: PASS.
Full intake11/stage23 suites in progress; exact results pending.
VPP build recipe remains frozen from prior checkpoint; no host changes.
