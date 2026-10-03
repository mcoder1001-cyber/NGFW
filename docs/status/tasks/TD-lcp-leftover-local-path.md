# TD-lcp-leftover-local-path — current integration, 2026-10-03

The shared bbd9 worktree now contains the reviewed leftover-local-path recovery. Only LCP-owned changes were ported from `3d4099d2` (the newer integrated hardening of the historical `6bc0fceb` fix); the desired-state Drift rejection regression was recovered from `6bc0fceb`. No other worktree was changed and no branch was merged.

A new default-namespace pair checks its table's `(*,224.0.0.0/24)` entry before creation. A lone local path is removed from the API source; remaining non-default-namespace Accepts still refuse mixed ownership. Moved-interface Accepts are handled without falsely refusing a reused index. Cleanup restores the local path after a failed verification read or a concurrent non-default pair, preserves both recovery errors, and retains deletion metadata across a default-namespace change. Retrieve-only Drift remains forbidden in desired configuration.

Current validation:

```text
$ tools/heavy.sh bash -c 'cd apps/agent && go test -race -count=1 ./internal/descriptors/lcp/... ./internal/desired/...'
ok ngfw/agent/internal/descriptors/lcp     1.426s
ok ngfw/agent/internal/desired           33.086s
```

The live check used disposable VPP, slot 5, its own table 5224 and loop590. Before adding the default pair, CLI showed `src:API` with a local Forward path. The descriptor removed it before creating `w5-lcp90`; afterward the table contained only its default drop entry and no API source. `TestLeftoverLocalClearedOnHost` passed in 0.62 seconds. Cleanup completed and the disposable VPP stopped. Full before/after output is [live-leftover.txt](TD-lcp-leftover-local-path-2026-10-03-evidence/live-leftover.txt); package evidence is [own-race.txt](TD-lcp-leftover-local-path-2026-10-03-evidence/own-race.txt).

No shared VPP configuration, process, NIC or other table was changed. Shared VPP PID 1014 was not restarted.

Status: ready for review of the current integration. Historical reviewed work is reused, but current changes remain uncommitted in this shared workspace; a current finishing CI gate and an actual review/merge are still outstanding. An API Accept whose pair is gone remains indistinguishable from stale plugin-owned Accepts through the available dump, so unrelated Accepts are deliberately preserved.
