# Live agents, checkpoints and serial integration

Observed during the 2026-10-02 CI delivery stream:

- A board row marked RUNNING does not prove a worker is executing. Query the
  live agent inventory and report active development, testing and independent
  review separately. A completed worker counts as zero active workers.
- `send_message` delivers context but does not start a completed worker's next
  turn. Use `followup_task` with a concrete next assignment; verify the worker
  changes to running and reports its current milestone. This was the cause of
  a completed TD19 checkpoint remaining idle despite queued messages.
- Every developer and reviewer uses its own branch/worktree. Commit completed
  increments frequently and publish each checkpoint remotely; a local commit
  alone is not a durable cloud handoff. Preserve development and superseded
  integration heads before rewriting the manager's integration branch.
- The manager consumes actual checkpoint SHAs, independent review verdicts and
  logs. Review fixes against frozen commits; preserve genuine initial BLOCK
  reports. Resume development after concrete findings and request a fresh
  independent recheck. Do not turn a passing fixture suite into an unreviewed
  security approval.
- Serialize main merges. A final one-commit integration must be based on actual
  current main, preserve its existing entries, pass unchanged full hosted quick
  plus applicable fixture gates, merge with the expected head SHA, and verify
  post-merge main. A green run for an older base/head is historical evidence.
  Main changed twice during PR67 and required fresh composition/gates.
- `unittest discover` returned zero tests for TD19's hyphenated filenames. Its
  explicit import runner rejects empty modules, zero total tests, skips and
  non-success outcomes. Quote actual executed counts, not a successful exit
  from a command that discovered nothing.

These are execution rules for available agents, not a permanently running
24-hour supervisor. When live activity cannot be observed, say that it cannot
be verified. Scheduled reporting does not itself keep development workers alive.
Laboratory acceptance stays in the single DEFERRED-ACCEPTANCE.md campaign;
NOT RUN, environment SKIP and real failures are distinct results.
