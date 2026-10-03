# Debian real build task envelope

- Task: audit and attempt genuine unprivileged product Debian build.
- Branch: `codex/debian-real-build-20261003`.
- Base: `a237827811a4abd2293157d5e8ea0cebf61196fc`.
- Worktree: `/root/Documents/Codex/2026-10-03/check-out-latest-code-from-git/work/NGFW-debian-real-build`.
- Owned tracked paths: this envelope and `docs/status/tasks/debian-real-build-wip.md`. Build-code ownership will be declared before any justified edit.
- Owned outputs: `deploy/vpp/.build/`, worktree dependency/build outputs, private `/tmp/debian-real-build-*`.
- No helper/recipient/P11 scripts, other worktrees, host installs/services/configuration, Docker, verification bypass or fabricated product inputs.
- Root coordinator publishes checkpoints and handles independent review/merge. Git denial is recorded, never bypassed.

## Resume ownership, 2026-10-03

- Owned: deploy/vpp/build.sh, bounded CPU-affinity helper in deploy/vpp/lib.sh, deploy/vpp/tests/run.sh and these unique Debian status/envelope files.
- Resume HEAD: ae23d2dca4b69b93b99a76d36e3e55409b1d1397; root reports published checkpoint 76cf3a6915dcd62a89f21f63266d2473fc3e6c23.
- Coordinator owns publication, mandatory independent reviews, current-main integration and actual compiler rerun after committed source freeze. No full compile in this worker turn.
