# Independent R7 BUG-vpn-capability-warning review

Source `033ac00ca` (product `dfa993405`). Own branch `codex/bug-vpn-capability-r7-20261005`, isolated worktree `/root/ngfw-wt/bug-vpn-capability-r7-20261005`. Only reviewer report/envelope/WIP owned; no product/board edits or new test implementation.

## Finding

**BLOCKER — mandatory task evidence report missing**, `docs/status/tasks/BUG-vpn-capability-warning.md`. Source contains envelope/WIP and reviewer R2 evidence, but no task status file required by R7 rule1. WIP mentions private before/after log paths without pasted command/output, and has no complete built/scope/open-question/decision summary. Create a bounded task report now with actual regression commands and pasted output, explicit remaining independent reviews/fullquick/hosted gates, and no Done claim. Final full-gate evidence can be added when observed. Manager notified; recheck only this docs gap once committed.

**MINOR — envelope owned-file declaration incomplete**, `docs/status/tasks/BUG-vpn-capability-warning.envelope.md`. It lists only the prompt rather than the two actual Go files and authorized append-only board/status paths in the task prompt/row. Reconcile declaration for recovery.

## Independently checked evidence and scope

Actual commands run from the isolated reviewer worktree:

`python3 tools/board.py --help` (tool validates board and prints progress even for this option):

```text
board ok: 212 tasks; progress 96.2% by hours, 199/212 merged; ready=0 running=4 parked=9
```

Actual Python YAML comparison of parent dfa993405^ versus current plan (complete objects, not only IDs):

```text
original rows: 211 current: 212 unique: 212 unchanged original rows: True
```

Actual before/after logs read at `/root/ngfw-wt/logs/two-ready-vpn-warning-before.log` and `two-ready-vpn-warning-after.log`:

```text
--- FAIL: TestWireguardOtherVpnCapabilities (0.01s)
implemented VPN capability wrongly rejected: W agent.unsupported-field /vpn/ipsec
implemented VPN capability wrongly rejected: W agent.unsupported-field /vpn/pki
W agent.unsupported-field /vpn/remoteAccess
FAIL ngfw/agent/internal/desired 0.119s
ok ngfw/agent/internal/desired 0.115s
ok ngfw/agent/internal/subsystems 0.182s
```

Regression provenance in WIP distinguishes before-base40fa9fe and relevant unchanged code on d6e1646; after productdfa has only removed two obsolete blanket warnings and retains remoteAccess. Independent R2 report has actual guard/overlay/secret-scan commands/output. This reviewer does not infer fullquick completion from focused results. Native REST packet acceptance remains separate TEST-traffic-B.

Decision policy inspected: deleting false capability warnings is a direct correction of implemented registered validators, not a nontrivial new implementation choice. No schema/framework/privilege/licensing/security-boundary change, destructive operation or new WBS item is introduced; a small necessary bug board row is not a whole plan WBS item. No new D-number/PENDING needed. The task prompt scope matches the actual minimal product diff. Existing backup closeout metadata was acknowledged by manager as stale until its real main gate completes, so it is not treated as verified live developer inventory here.

## Required final docs-only closeout verification

After actual results: preserve archive checkpoints; record exact final single-commit head and unchanged complete quick/hosted checks on that head (or permitted documented unchanged-product carry-forward), all mandatory reviewer/tester reports, expected-head merge PR/SHA and successful resulting main CI. Then set only this task row merged with finished date and evidence notes, keeping all original211 rows; regenerate PROGRESS and verify212 unique rows/counts, resolve relative report/prompt links and ensure task report states what was built, pasted real outputs, out-of-scope, decisions and open questions. Reconcile backup metadata only against its separately observed actual main CI; do not imply whole traffic acceptance from this bugfix. Any final product/main dependency delta needs appropriate revalidation, not a docs-only assumption. No Done before these conditions.

Verdict: **BLOCK** (1 documentation BLOCKER, 1 MINOR). Narrow recheck can close this gap without rerunning unchanged product tests. No active reviewer commands or fixtures remain.
