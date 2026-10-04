# Accepted PIM/MFIB scale debt

Accepted by Codex manager after R5 review, 2026-10-04.
Owner: Codex manager. Due: 2026-10-11.

Existing MFIB Create/Delete performs full-table conflict/existence checks for each route. Keep those checks and persistent ownership isolation intact in this delivery. The runtime now supports at most 256 observed routes; 257+ records reject the whole snapshot before replacing cache. The independent parser input guard remains 10,000 records / below 4 MiB. This cap bounds feature-induced churn, not unrelated shared-table entries, and makes no throughput claim.

Follow-up: design per-family/per-transaction safe batched conflict and existence snapshots, preserving refusal of foreign prefixes, fresh identity/ownership checks, correct concurrent change detection and rollback/restart behavior. Bound shared-table retrieval in a way that rejects incomplete observations without silently adopting/deleting routes. Add adversarial tests with foreign rows and concurrent shared-table changes before widening support. Do not weaken MFIB ownership dumps merely to accelerate tests.

Acceptance for this scoped cap: 256-record snapshot accepted; 257-record parser input accepted by input guard but runtime refresh rejected; prior cache route keys unchanged. Unit race evidence recorded in task report.
