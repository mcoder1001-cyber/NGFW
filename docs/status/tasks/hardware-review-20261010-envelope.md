# Hardware installation independent review envelope

- Task: `hardware-review-20261010`; role: independent R8 installation/operability review with management-path checks.
- Assigned by manager `/root`; reviewed source: `d2d55984d74fa1d06c32e8271886f11f16375407` (`origin/main` at assignment).
- Branch: `codex/hardware-review-20261010`; isolated worktree: `/root/ngfw-wt/hardware-review-20261010`.
- Owned files: `docs/status/tasks/hardware-review-20261010*` only. No product-code ownership, target mutation, main merge, development-host VPP operation or agent worktree edits.
- User authorized NGFW package install/test on `root@172.30.126.37` and `root@172.30.110.211`, import all non-management physical NICs and reboot if required, provided management and routing remain available.
- Reviewer authorization: local source inspection and host-independent tests; read-only SSH target inspection. Host workers own target writes. Manager owns current-source artifact preparation/integration.
- Deliverable: independently verify safe installation prerequisites, package activation order, network exclusions, actual evidence and remaining acceptance; publish receipts immediately.
- Current host prerequisite: offline repair of corrupted mounted root filesystems and verified console/rescue access before install/reboot. No repair is authorized to this reviewer.

Required instructions read: `AGENTS.md`, `prompts/00-CONTEXT.md`, `docs/contributing.md`, `docs/decisions/decision-policy.md`, `prompts/REVIEW-PROMPT.md`, `prompts/reviewers/R8-operability-packaging.md`.
