# PPPoE lifecycle repair candidate

Exact local source checkpoint: 6be8034; branch codex/pppoe-lifecycle-20261008;
base 4d4723f. This source is unpublished and is not a merge-ready result.

Stop now fences IPv6 admission even when no PID exists, keeps process evidence
when its helper is missing, and verifies/stops writers before replacement. Hook
admission captured before action-lock waiting is checked against a rotated token;
a queued old hook cannot be admitted into the reopened generation. Parent pppd
lifetime uses an inherited pidfd, avoiding resurrection from numeric PID reuse.
Supervision stops old pppd while fenced, replaces/invalidates state, then opens the
next admission before unit start. Durable pending evidence survives daemon-reload
and restart failures, so identical retry cannot silently leave a session stopped.

Regression tests added: late up with/without existing PID; old token replay after
reopen; simulated reused parent numeric identity; missing helper with PID evidence;
daemon-reload/restart failure followed by identical retry and no-op convergence.

Actual verification:

```
git diff --check
exit 0

tools/ci.sh check --base main
check PASSED (0m02s)
WARN gitleaks not installed — built-in secret grep only

Rendered helper Python AST parse
PASS

Private lifecycle process fixture
FAIL: ipv6-parent-unavailable
```

Go/gofmt unavailable; Go tests and golden regeneration have not executed. Golden
was synchronized from templates and requires the existing renderer golden check.
Unchanged mandatory hosted quick and independent source/security reviews are owed.

CLI source publication was rejected by automatic approval review: repository
source payload/destination authorization and trust/privacy were not established.
No connector workaround or publication was attempted after rejection. Next action:
obtain owner authorization for this exact reviewed source PR payload to the verified
NGFW repository; only then publish and run the unchanged hosted quick gate.

No shared host, services, real sysctls, packages or VPP were changed. Product
PPPoE discovery, LAN encapsulation and real dial/reconnect/rollback acceptance
remain unsupported or unverified. This repair does not complete the whole feature.
