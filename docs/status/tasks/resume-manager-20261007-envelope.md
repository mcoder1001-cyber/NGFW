# Remaining branch integration envelope

Owner request (2026-10-07): continue the remaining GitHub branches until reviewed code is merged. Manager branch `codex/resume-manager-20261007`, isolated worktree `/root/ngfw-wt/resume-manager-20261007`, initial main `0ec397e327123cadfd5d278a9a1cda37532fdc2c`.

Manager owns this envelope/WIP, narrowly edited task-board rows and acceptance/status documentation. Developers own separate PPPoE, P12 and RA worktrees. Reviewers never edit product code. No shared host daemon, privilege or VPP boundary changes. Laboratory-only acceptance may be deferred explicitly; real defects and mandatory complete quick gate remain merge blockers. Preserve reviewed history, single-commit final integration, sequential merges with current-main checks.

Remote branches removed during owner-authorized cleanup remain recoverable in `refs/archive/github-cleanup-20261007/*` and verified `/root/NGFW-github-branches-20261007.bundle` (59 MiB). Do not recreate obsolete remote branches merely to run this queue.
