# TD19 bounded source resume WIP

Branch codex/resume-td19-source-20261007; worktree /root/ngfw-wt/resume-td19-source-20261007. Owned files explicitly in envelope. Base/local3ddb1680e475e94d43e8036cd3776bc60c87208b; remote task checkpoint not yet published.

Tests-first change: actual installer entries invoked only under recording stubs; APT update can succeed as a no-op, install always fails42, downloads/other dangerous commands fail91. Expected package argv and no continuation into venv/download are asserted. Lab legacy containerlab fixture name retained for unchanged fixed strict runner. This initial regression checkpoint intentionally fails until source removal is applied; not merge-ready.

Remaining source: remove build CONTAINER and lab containerlab download/installer/VIRT/Docker groups, update lab --check-config. Preserve native tools and strict pins. Wider safe-root/dry-run seam, reviewed Python/pnpm pins, target artifact inventory/local exact-version followups are not addressed here. No target provisioning/boot acceptance claimed.

Exact next commands: python3 -B docs/status/tasks/TD-19-test-build-preflight.py; python3 -B docs/status/tasks/TD-19-test-containerlab.py. Record actual regression output before first commit/push. Containing checkpoint SHA is resolved via git rev-parse HEAD and git ls-remote origin refs/heads/codex/resume-td19-source-20261007; later WIP records verified previous receipt.

Actual initial regression: build7tests,1FAIL (APT argv includes docker.io/docker-compose-v2); lab5tests,12failed assertions/subtests (old config, forbidden source and containerlab pre-download blocks reaching native APT). Zero host commands succeeded. Fixtures bypass only root UID gate in temporary copies to work on unprivileged hosted runners; build canonical go.mod remains the real source. Existing five Go/digest/PATH tests passed. Tests intentionally red, source not changed yet.
