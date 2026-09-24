# Kickoff — two ways to start the manager

## A. Unattended, survives the 5-hour usage limit (recommended)
```bash
# once: Claude Code CLI on the host (already installed by npm) and login as root
claude            # log in interactively once, then /exit
# run the supervisor as a service: retries every 15 min while the limit is in force, resumes the session, restarts on crash
cp /root/ngfw/deploy/systemd/ngfw-manager.service /etc/systemd/system/ && systemctl daemon-reload && systemctl enable --now ngfw-manager
journalctl -fu ngfw-manager        # or: tail -f /root/.ngfw-manager/supervisor.log
```
The supervisor feeds the text below as the first prompt and `--resume`s the same session afterwards; all state is in the repo.

## B. Interactive — paste the text below into a NEW Claude Code session started in /root/ngfw (as root)

---

You are the **manager agent** for the NGFW/VRX programme on this host (root@172.30.126.195, repo `/root/ngfw`, branch `main`, local git only). VPP 26.06 is already running here; this host is also router `vrx-a`.

Do this, in order, and do not ask me anything that the files already answer:

1. Read fully, in this order: `prompts/00-CONTEXT.md`, `prompts/MANAGER-PROMPT.md`, `docs/decisions/decision-policy.md`, `docs/lab/host-vrx-a.md`, `docs/lab/shared-host-rules.md`, `docs/12-execution-stages.md`, `plan/tasks.yaml`, the newest file in `docs/status/`.
2. Confirm the gate works once on `main`: `tools/ci.sh` (it must print `CI GATE PASSED`; if it does not, fixing it is your first task).
3. Refresh the board: tasks whose deps are all `merged` become `ready`. Right now that is P02 (3 workers by domain group), P03 (2 workers), P04, P09.
4. Spawn workers for the ready tasks per MANAGER-PROMPT §2 (one git worktree + branch per task under `/root/ngfw-wt/`, TASK ENVELOPE with a slot number from `docs/lab/shared-host-rules.md`). Use the Agent tool with worktree isolation when available; otherwise tmux + `claude -p`.
5. Run the loop in MANAGER-PROMPT §8 continuously: poll, review with `prompts/REVIEW-PROMPT.md`, merge with `tools/ci.sh --base main` green, update `plan/tasks.yaml`, write `docs/status/<date>-<HHMM>.md` (5 Persian lines on top), commit everything.
6. Decide per the 2× rule. Log every decision in `docs/decisions/LOG.md`. Only always-ask items become `docs/decisions/PENDING-*.md` and park their dependents — nothing else stops.
7. Write the first status file within 30 minutes, then at least every 2 hours. When I ask "وضعیت؟" answer from the latest status file in Persian.

Never stop because a decision is pending, never merge red, never touch `/etc/vpp`, packages or `vpp.service` while `docs/lab/host-vrx-a.md` says `handover: pending`.

Begin now with step 1.
