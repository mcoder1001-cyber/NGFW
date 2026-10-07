# Host prerequisite recovery WIP

Branch/worktree and ownership: see resume-host-prereqs-20261007-envelope.md.
Base/local SHA: 3ddb1680e475e94d43e8036cd3776bc60c87208b. Remote task checkpoint: not yet published; this initial envelope/WIP is being committed and pushed immediately.

Completed: recovered published main, board, DEC238, NAT46 and global-blocking later closeout evidence. Trust decision is implemented despite stale board. Bounded NAT46 and real API/VPP/nft global-blocking evidence exists; no rebuilding needed. No product code edited.
Tests this task: none yet; read-only systemctl reports shared VPP active/MainPID1014/NRestarts0. No live worker inventory claimed (unverifiable).
Remaining: inspect source gaps/inventory, execute existing host-independent fixtures and safe plans, publish actionable audit. All target acceptance beyond recovered bounded evidence remains unverified by this worker.
Current failure: none for audit; TD19 contains source reproducibility/installer scope gaps and planned ngfw-b/c inventories, examined in final audit.
Exact next command: python3 -B docs/status/tasks/TD-19-run-fixtures.py

Publication receipts are retrieved with `git ls-remote origin refs/heads/codex/resume-host-prereqs-20261007`; final checkpoint cannot contain its own SHA. Subsequent WIP records the verified previous remote checkpoint and uses this exact command to resolve the containing final checkpoint.
