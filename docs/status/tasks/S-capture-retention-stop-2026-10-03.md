# S-capture-retention-stop — 2026-10-03

Implementation complete in the integration worktree; acceptance is ready for review, with no merge or appliance deployment claimed.

- Retention protects the newest capture. Oversized packet plans fail before VPP mutation. File imports reject FIFO/symlink/foreign ownership without exposing directory paths.
- Startup, Read and Delete recover interrupted captures. Failed metadata saves, VPP stop or filter restoration retain recoverable ownership and cannot falsely complete a capture. Pending filter restoration is boot-bound; an old boot never overwrites new globals. Interrupted file moves preserve evidence on retry. Foreign kept files persist a terminal error record; failed refusal metadata writes remain retryable and never serve or modify the target. Release is guarded by record identity so an older request cannot clear a newly running capture.
- Administrative `POST /actions/capture/:id/stop` cancels the owned live stream. Missing/completed/foreign-process captures return bounded errors. Stream shutdown and late errors are handled safely. The UI confirms Stop and displays localized fixed errors.
- Capture uses the resolved service globals role, shared boot store and isolated state directory. The API client and CLI operations were regenerated.

Validation: final full capture package race tests passed (13.697s), focused agent capture RPC race tests passed (1.370s), final focused API capture-stream/route-guard tests passed (12), web tests passed (3), API/web type checks passed and CLI generated build passed. Evidence is in [the evidence directory](S-capture-retention-stop-2026-10-03-evidence/). Stable-source command evidence is in `agent-race-final.txt`; source hashes are in `source-sha256.json`. Independent review found no remaining known stop/recovery blocker after the fixes above. Integrated compile validation is recorded separately in the task closure report.

The pinned binary API has no read-only packet-count getter. VPP enforces recording limits; agent finalization uses timeout or explicit Stop, as permitted by the task. Real capture host acceptance/screenshots, buffered download and temporary-file permission follow-ups remain separate tasks. See [remaining capability notes](S-capture-retention-stop-questions.md).
