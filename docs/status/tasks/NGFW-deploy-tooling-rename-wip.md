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
