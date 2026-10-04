# Auto-block inactive maintenance gate correction

Branch fix/pbr-gate-isolation-20261004; isolated worktree gate-fix; base prior source
009bdda0. Root authorized narrow autoblock maintenance product fix and deterministic
regressions after unchanged hosted gate37177580917 exposed resync event contamination.
Own rpc_autoblock.go guard, new autoblock_maintenance_test.go and task recovery/report
files only. Original seams rate-limit test unchanged. No CI changes, skips or privileges.
Manager publishes and reruns complete hosted gate after independent R1/R2 review.
