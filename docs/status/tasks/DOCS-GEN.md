# DOCS-GEN implementation and validation

Implemented deterministic docs/user/README.md guide navigation and
source-reference.md from actual API controllers and schema sources. Generator
uses Python standard library only, checks drift and resolves every generated
relative file link inside the checkout. Root-level guides are included.
No endpoint or appliance acceptance is inferred from source paths.

Actual verification:

```
python3 tools/docs/generate-reference.py
python3 tools/docs/generate-reference.py --check
docs reference OK: 58 guides, 63 controllers, 113 schema sources; all generated links resolve
```

A temporary extra root guide caused --check to refuse drift; the probe was
removed and the unchanged checkout passed again. git diff --check passed.
The full unchanged tools/ci.sh --base origin/main gate is not green:
@ngfw/ui-kit SchemaForm.test.tsx:82 timed out at 30000ms (1 failed, 89 passed).
Manager requested stopping the owned gate after the known timeout to reduce
shared-host load. Its process tree was terminated; logs are preserved.
Remaining gate checks are NOT RUN and require a sequential retry.
No test configuration or product source was changed to hide the failure.
Actual logs: /root/ngfw-wt/logs/ci/docs-gen-20261005-20261005-153524-3820172.

CLI push returned HTTP403. Authorized GitHub connector successfully published
remote 23ee3289608b8b37c5676e434e0c098965984bd4; draft PR183:
https://github.com/mcoder1001-cyber/NGFW/pull/183.
A later checkpoint extends that remote tip; git remote branch is authoritative.
Independent review, green complete local/hosted gates and final manager
integration remain required. Source documentation is not release acceptance.
