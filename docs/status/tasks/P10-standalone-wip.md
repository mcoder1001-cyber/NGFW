# P10 standalone WIP

Branch: `codex/p10-standalone-20261003`; base `71cee90b2a28421480dfd67a6d64ca7f47f37200`.
Implementation local `4daa46c5`; published exact-tree counterpart `107de244c496810c60f514ddd2b1f2ebf4bb2b0c` (connector publication, different commit ancestry). Initial envelope published counterpart `bd55353cc23610cd042dfb52b47247304cbad301`.
Owned files: `deploy/debian/bundle/**`, `docs/user/install/bundle.md`, `docs/status/tasks/P10-standalone-*`.

Completed: canonical helper exporter and isolated recipient launcher. Launcher is externally authenticated BEFORE execution; separately authenticated expected helper-report SHA256 authorizes report, report authorizes entire archive BEFORE tar parsing, exact inventory authorizes every member BEFORE installer execution. External runtime manifest remains required. Unknown delivery helpers are never imported. Full VPP gate and all its tests retained; deterministic `/nonexistent` HOME fixes the previous sanitized-environment omission. Actual distro dependencies documented (Python >=3.12, Bash, dpkg/APT, Git, patch, coreutils/text tools; optional shellcheck). New fixtures are included by the existing hosted test_export.py entrypoint without workflow changes.

Actual results:

```text
bash deploy/vpp/verify.sh
ok tests/run.sh: 66 passed, 0 failed
verify.sh: OK
python3 -m py_compile deploy/debian/bundle/helpers.py deploy/debian/bundle/recipient.py
PASS
 tools/ci.sh check --base 71cee90b2a28421480dfd67a6d64ca7f47f37200
check PASSED (0m10s)
python3 -m unittest discover -s deploy/debian/bundle -p test_helpers.py -v
Ran 7 tests in 104.695s
OK
```

The 7 fixtures use actual small Debian archives and the unchanged real full VPP gate from delivered helpers outside the checkout, with no gate stubbing or host installation. Synthetic metadata consistency is NOT real upstream build provenance. Tests refuse modified archive/report, unsafe members, omitted dependencies, changed runtime manifest, oversized/nonregular input; outside-checkout read-only preflight passes.

Currently running: complete test_export.py (existing 11 export fixtures + new 7 helper fixtures), test_install.py, test_verify.py and unchanged `tools/ci.sh --base 71cee90b2a28421480dfd67a6d64ca7f47f37200`; logs `/tmp/p10-standalone-{export,install,verify,quick}.log`. No failure observed yet.

Remaining: freeze final result, publication/PR, unchanged hosted quick gate and provisioning fixture results, independent review. Signing/bootstrap delivery ownership/security provenance policy, genuine product/VPP build provenance, clean Ubuntu install/remove/reinstall, firstboot and hardware release acceptance remain open P10 gaps. No all-P10-DONE claim.

Next command: inspect running logs, complete focused/quick gates, commit final status and request independent review of the published exact head.
