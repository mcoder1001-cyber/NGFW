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

## BLOCKER B3 — mandatory focused fixture regression

`docs/status/tasks/TD-19-test-repo-keys.py:108-119`: copied actual repository entry was not adapted to include its new shared helper. Independent unchanged strict runner exit1:45 tests,1failure,0errors,0skips in115.017s. Expected `realpath:` error is replaced by `/tdr/tmpirjmzfuv/setup.sh: line 247: /tdr/scripts/install-common.sh: No such file or directory`. Fix the fixture copy/layout and rerun complete strict45, preserving authority assertions.

```
Ran 45 tests in 115.017s
FAILED (failures=1)
TD19 fixtures: tests=45 failures=1 errors=0 skipped=0 expectedFailures=0 unexpectedSuccesses=0
```

Independent new safe-root command finished:

```
Ran 9 tests in 11.320s
OK
```

Full strict and safe9 output preserved alongside report as reviewer-owned log text. These passing nine checks do not cover confirmed B1/B2.

 Original trust routines/constants and SHA validation order remain in source; pnpm derives exact packageManager. Python lock requires explicit exact version/hash direct-package input, pip enforces dependency closure during apply; no authoritative default release closure provided. Dry-run branches are read-only in source and do not claim real artifact verification.

Verdict: **BLOCK** at exact545f; independent reproduction confirms unsafe fixture execution boundary. No actual host install/network effects were exercised.

## Repair verification in progress (not final approval)

Source664f3f7f9 imported in own report branch. Original harmless B1/B2 controls now both refuse exit1, external markers false, no effect calls. Deterministic shipped recorder bytes are checked using trusted absolute Python before effect execution; alternate PATH reset to validated stubs plus system utilities; no fake-root bin prepend; fake pip dispatch fixed. New10 suite PASS10.844s; syntax/ShellCheck pass. Strict45 rerun pending. Remaining shell-source of fake /etc/os-release identified independently; developer replacing with strict read-only data parser. Approval withheld until updated frozen source and verification.

## Final863d9c8eb verification checkpoint

Final frozen source imported; product/test paths equal exact863d9c8eb (git diff exit0). Preceding664 strict45 independently PASS146.964s zero failures/errors/skips; this is not the final863 result. Final original B1/B2 controls refuse1/noeffects; new10 independently PASS11.083s. Four additional controls PASS: valid recording build and lab apply plus repository dry-run ignore PYTHONPATH/sitecustomize/PYTHONSTARTUP (exit0/nooutside startup marker); nonexecutable fake os-release command substitution refuses1/nooutside marker/noeffect calls. The os-release test bypasses only artifact preflight exactly like historical trust fixtures, proving metadata parser refusal separately from real signed artifacts. Syntax/ShellCheck and product identity checks pass. Final strict46 pending34389 /tdr/final-strict46.log; approval remains pending this run.

## Final863 focused result and native metadata finding

Independent exact863 strict46 PASS155.563s, failures/errors/skips/expectedFailures/unexpectedSuccesses all0. New10 PASS11.083s and owncontrols above. Developer separately advanced0489 to bind verifier TMPDIR; not yet imported/approved.

BLOCKER B4 (native correctness), scripts/00-add-repos.sh os_release assignment / shared ngfw_install_path: default root `/` still refuses standard `/etc/os-release` symlink. Read-only host observation: `/etc/os-release -> ../usr/lib/os-release`; target regular. Pure helper invocation `bash -c 'source scripts/install-common.sh; NGFW_INSTALL_ROOT=/; ngfw_install_path /etc/os-release'` exit1 prints `REFUSED: installation path cannot traverse symlinks`. Native repository installer therefore cannot consume standard OS metadata. No actual native installer invoked. Developer notified to choose trusted regular canonical OS metadata fallback/read parser in native mode while preserving alternate-root symlink rejection. Approval withheld pending correction and focused verification; native install/boot acceptance remains deferred separately.
