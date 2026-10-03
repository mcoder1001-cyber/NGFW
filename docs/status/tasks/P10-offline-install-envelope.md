# P10 offline installation increment

Branch: `codex/p10-offline-install-20261003`; base `a044cf1d`.
Owned product files: `deploy/debian/bundle/install.py`, `test_install.py`, `docs/user/install/bundle.md`, additive installer-fixture step in `.github/workflows/provisioning-fixtures.yml`.
Owned recovery documents: this envelope and `P10-offline-install-wip.md`.

Scope: trusted external manifest, pinned private snapshot, default plan, explicit root Ubuntu 26.04 amd64 local-file APT simulation/install. No host installations, service operations, network fetches or lab execution in development/tests. No new task, schema, privilege-boundary deviation, artifact authenticity claim or P10 completion claim. Manager owns publication/review/merge. Actual install belongs only on an explicitly authorized fresh target.
