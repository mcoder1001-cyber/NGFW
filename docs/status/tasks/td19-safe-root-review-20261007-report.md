# Independent TD19 safe-root review

Frozen product source: `545f118af511a3903f03a5ade5fff9c0b928364b`, based on `f6ae6e555`. R1/R2/R4/R7/R8 bounded review; no product edits or real install/network/service actions. Full aggregate CI not run under current owner waiver. This scope does not finish TD-19 or prove native install/boot.

## BLOCKER B1 — unchecked executable escapes fixture-only command seam

`scripts/20-install-build.sh:86,98-106`: helper validates initial PATH commands, then the installer executes an existing fake-root `/usr/local/go/bin/go` directly and prepends both fake-root Go bin and GOBIN to PATH. Neither executable source is required to be the approved recording stub. A valid symlink-free root containing an arbitrary regular Go executable therefore bypasses checked recording commands and can run real Go downloads/builds or unrelated effects.

Independent harmless reproduction uses the existing Fixture recording stubs unchanged and only substitutes an existing rooted Go script that writes a sentinel outside `NGFW_INSTALL_ROOT` and prints the accepted pinned version. Actual result: script exit0; external sentinel true; recorder calls only apt-get twice and corepack twice; all three generator installs bypassed the recorder. Reproducer committed alongside this report; it creates no real download or install.

Fix: alternate-root execution must never directly execute unverified rooted binaries or prepend unchecked rooted bins. Dispatch effects only to reviewed recording commands; validate every executable used, including any generated venv pip. Preserve real-root Go digest/platform/version checks separately.

## BLOCKER B2 — regular executable path is not recording-stub recognition

`scripts/install-common.sh:26-35`: any regular file named apt-get/curl/etc at caller-provided NGFW_INSTALL_STUB_DIR passes. Nothing verifies it is a recording stub or rejects a copied real package-manager binary/delegating wrapper. Independent harmless apt-get substitution writes an outside-root sentinel, then returns success; the installer exits0 and sentinel exists. Thus the advertised refusal boundary does not prevent real global effects merely by requiring explicit environment flags and a directory.

Fix: trusted reviewed fixture harness/recognizable fixed content contract for effect executables (and bounded trusted interpreter/coreutils handling), or refuse alternate-root apply entirely outside that harness. Do not claim arbitrary user executables are sandboxed.

## Verification so far

`TMPDIR=/tdr PYTHONDONTWRITEBYTECODE=1 python3 docs/status/tasks/td19-safe-root-review-20261007-adversarial.py`:

```
{"case": "existing-go-shadow", "returncode": 0, "external_marker": true, "calls": ["apt-get", "apt-get", "corepack", "corepack"]}
{"case": "regular-nonrecording-command", "returncode": 0, "external_marker": true}
```

`bash -n scripts/{install-common,00-add-repos,20-install-build,40-install-lab}.sh` and `shellcheck -x -P SCRIPTDIR` on those four files: exit0, no diagnostics.

Strict fixed inventory45 fixture run in progress; already observed failure in `RepoKeys.test_actual_repository_entry_uses_authorized_pins_and_refuses_differing_override`, traceback pending run completion. New safe-root9 queued in same shell after strict run. Original trust routines/constants and SHA validation order remain in source; pnpm derives exact packageManager. Python lock requires explicit exact version/hash direct-package input, pip enforces dependency closure during apply; no authoritative default release closure provided. Dry-run branches are read-only in source and do not claim real artifact verification.

Verdict: **BLOCK** at exact545f; independent reproduction confirms unsafe fixture execution boundary. No actual host install/network effects were exercised.
