# F-dashboard-prom-alarms-host — independent review plan

Date: 2026-10-01. Baseline: origin/main at 5561d041.

Changed areas: Prometheus stats/collector/listener, desired state, API state integration, metrics/docs.

Mandatory panel: R1 correctness/tests; R2 security; R3 contracts/API; R6 UI/user documentation; R7 evidence/scope; R8 operability; R4 agent/data-plane safety; R5 timers/reconcile/scale. Each report records the exact reviewed commit. Findings require a developer fix and focused independent verification. No completed-task or live-acceptance claim follows from mock/unit success.

Testers: T1 quick gate/generated-contract verification; T2 API/auth/transaction end-to-end; T4 en/fa light/dark UI where web changed. T3 real data-plane/daemon/host acceptance is required for P11, dashboard, multiWAN and setup forwarding defaults. Notifications needs real management routing verification.

Environment: hosted GitHub runner is being established for the unchanged complete quick gate; local AF_UNIX/chown restrictions prevent full API tests. Lab host is unreachable and no browser binary is available. T2/T3/T4 must report BLOCKED-ENV until the actual required environment is available; focused tests do not replace those scenarios. Merge requires combined APPROVE, complete quick gate PASS and applicable acceptance PASS.
