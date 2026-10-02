# P10 packaging checkpoint

Branch: `task/P10-packaging-finish-20261002`; base: `53a43ce5`.
First published remote SHA: `c401f4efde321505cdd6a358cf93d044b1d215b4`. Local first checkpoint `ff6223db`; later code publication pending manager upload.
Owned: deploy/debian/vrx/**, deploy/debian/README.md, deploy/apt/**,
scripts/publish-apt.sh, scripts/10-install-runtime.sh, docs/09-os-packages.md,
docs/install/**, deploy/systemd/vrx-* (not manager unit or P11).

Implemented: single-source four-package control/install metadata; product VPP exact-version substitution;
no-start/no-enable package install; idempotent group/user creation and data-preserving upgrade handling;
base hardened API/agent units with explicit AF_NETLINK and approved capability boundary.
The build deliberately refuses missing staged binary/API/web/VPP inputs.

Implemented additionally: prepare.sh verifies product manifest/install gate before staging, builds static Go tools, pnpm deploy production API and web payload, copies units and generates SHA version. Driver has not yet built actual packages.

Verification: four Python host-independent checks PASS; bash -n prepare.sh PASS; git diff --check PASS; tools/ci.sh check PASS with missing-gitleaks warning (not full quick). systemd-analyze verify could not resolve absent vpp.service/product binaries; no unit syntax error reported, acceptance NOT RUN.

Next checkpoint: HTTPS-only nginx config with WS, RESTCONF and host-meta; overwrites forwarded client identity at the trusted local proxy. Real OpenSSL TLS bootstrap generates private 0600 key once, validates existing matching pair and preserves it on retry, refuses partial pairs. Six host-independent checks PASS (including actual TLS generation/retry/refusal). nginx syntax on target and integration NOT RUN.

Remaining: package copyright/license metadata (repository has no LICENSE; do not invent terms), firstboot, nginx enablement, base policy,
APT signing/publication, runtime installer correction, user install/upgrade docs.
This is a scaffold; task is RUNNING, not done. No package install, host config change,
daemon startup, VPP restart or laboratory acceptance has been performed.

Next command: python3 deploy/debian/vrx/tests/test_packaging.py

Independent review fix: explicitly writable/provisioned capture and rsyslog TLS directories; regression checks actual renderer defaults. Correct TLS runtime driver to rsyslog-openssl (renderer uses ossl, not gnutls). Six checks PASS again.
