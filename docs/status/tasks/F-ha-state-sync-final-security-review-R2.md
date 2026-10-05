# HA state sync final independent R2 security review

Exact local source: `075c062b7945fcb504e5a010a382f294bda8128b` (published source `d4ecbddab46ff074ae968c737c1ac51a1b16015c`, manager-provided). Reviewer owns reports only on `codex/review-r2-final-security-20261005`. Source extracted unchanged to `/dev/shm/r2f-ha`; dependency links are read-only source-external fixture dependencies, not modified product code.

Provenance: whole-feature security review of `c42307e12d124aea01640b60a454c652270154ae`, published `1c71a8959640d5ace6d42acf5bbca8e6ffe34f6d`, originally BLOCK solely on driver Bearer redirects. Independent closure at `48e5af12fd6a42323f3c83d04129eb6563cef650`, published review `5a8c01bb49013566db1746911330ad95fa7e1dce`, established real urllib redirect refusal. Current git comparison 48→075 is empty for agent implementation, topology acceptance and HA API controller. c423→48 changes besides the security driver/tests are bounded same-socket timeout adjustments (3→10 seconds socket, 4→11 seconds parent); inspected here, no reconnect or altered privilege boundary. Current generic gRPC permission mapping, real transport regressions and HA UI changes independently reviewed.

PERMISSION_DENIED now yields HTTP 403 with distinct `agent-permission-denied` kind and generic detail, withholding both raw gRPC details and message. No change affects the startup globals-owner check or REST admin requirement. runAction rejects errored streams even after partial lines; controller refuses missing/failed completion and only adds completed audit after verified success. Existing FAILED_PRECONDITION/ABORTED 409, UNAVAILABLE 503 and INTERNAL 502 remain intact. HA UI suppresses cached failed/observationError status and disables resync while observation is unavailable or refreshing, as well as retaining role/active-kind/actionsAllowed gates. UI does not replace backend authorization.

Actual independent unchanged-source tests:

```
env TMPDIR=/dev/shm/r2f-t ./node_modules/.bin/vitest run src/agent/action-transport.test.ts src/features/ha-state-sync/controller.test.ts --maxWorkers=2
2 files, 11 tests PASS; duration 16.70s (Unix-socket real AgentClient/gRPC transport, controller; no shared DB).
env TMPDIR=/dev/shm/r2f-t PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s test/topology/ha-state-sync -p 'test_*.py' -v
16 tests PASS in 3.242s.
```

Transport cases verify permission redaction, partial-stream failure, failed audit, empty completion and unchanged 409/503/502 mappings. Acceptance cases retain foreign/downgrade redirect refusal, private fault-helper refusal and same established socket. No real HA fault or two-appliance execution, shared service/database/VPP modification, browser acceptance or full quick gate was run. Native NAT HA UDP remains unauthenticated and requires the documented trusted isolated deployment network; no hostile-network certification implied.

Original redirect BLOCKER is closed; no new BLOCKER, MAJOR or MINOR security findings in reviewed feature scope. Verdict: **APPROVE**, whole-feature R2 grade by explicit unchanged-path provenance plus independent affected-security review. Exact integration quick and other applicable aspects remain prerequisites.
