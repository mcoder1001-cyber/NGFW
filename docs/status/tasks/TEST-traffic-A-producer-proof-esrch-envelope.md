# Producer-proof postcleanup ESRCH robustness phase

Own task/TEST-traffic-A-producer-proof-esrch-20261002 in isolated
NGFW-traffic-producer-proof-esrch, basebc1bbe39a2aef93b5a76d78b3e9bda2b962b06ce.
Diagnostic491c6752 preserved verbatim: deterministic ESRCH injection exposed only
post-gone fixture robustness, not an observed live failure/product leak or invalid
prior PASS. OldPR84/1016/rulings/refs immutable; prior A3 not extended to this phase.

Only producer test postcleanup helper explicitly accepts ProcessLookupError next
to FileNotFoundError. Readiness/positive live PID+birth checks and other errors
remain unchanged. Existing two separate control methods now cover both foundation
and producer helper contexts with every foundation negative retained; still2.
Original47 source discovery,17/13 CI policy counts and product/gates unchanged.
Separate controls2PASS0.005s. Required validation: two actual producer cases5each
with numericPID signals forbidden, original47, bothexisting policy checks once.
Fresh independent applicable review and eventual coherent integration/full gates
required; no live namespace issuer/producer activation, board/main/ref changes.

Actual final evidence: existing2control methodsPASS0.005s, both producer cases5
repetitions each10PASS4.120s with os.kill numericPID signalling forbidden, original
strict47PASS4.046s with zero failures/errors/skips/expected failures/unexpected
success. Existing cleanup17 + transaction13 gate-policy checksPASS once; they are
policy controls, not source/lab acceptance. git diff --check clean. Producer test
source delta is exactly one except clause; no live source behavior/gate/count or
readiness changes. New independent review pending, not inferred from authorPASS.
