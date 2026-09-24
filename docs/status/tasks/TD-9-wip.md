# TD-9 WIP

- 23:41 read context, envelope, review items (s1 = section 1 of verify-results), TD-8 verify. Base `8a96a9c` (task/TD-8).
  Verified on base: 1.1 (conn.go no reply timeout; Resync/revert/rollback unbounded), 1.1b, 1.1c, 1.1d, 1.1e, 1.2, 1.3, 1.4,
  1.5a/b/c/e, ARCH-01 (service.go Apply: save error only logged) — all still present at the base line numbers
  (service.go:290-344 Apply, :398-431 applyLocked, :496-545 revert, :579-604 Resync; reconciler.go:621-735 ApplyWith;
  agent.go:186 grpc.NewServer(), :286 wiring.Connected(ctx), :296 watchLinks; main.go:29-30 log level).
- next: tests that fail on the base first, then the fix.
- 00:20 code done for 1.1 (conn.go reply bound + Bounded, txnContext, resync/revert/rollback deadlines, connect-hook
  deadline), 1.1b (owed-resync retry via Env.Resync path + drift gauge), 1.1c, 1.1d/e, 1.2, 1.3, 1.4, 1.5a/b/c/e,
  ARCH-01, owed-revert tech-debt; FlushClaims hook for TD-11c (manager's coordination note). Unit tests green (-race).
  next: base-first evidence (shims), proto.md §2, TD-9.md, CI.
