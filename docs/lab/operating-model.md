# Operating model (D-015)

Until Claude Code is authenticated on the host, the **manager is the Claude session on the operator's desktop**;
worker agents are spawned there too. All git state lives on the host:

- Manager creates worktrees on the host: `tools/operator/wt.sh new <id>`.
- A worker edits in a local copy (`wt.sh pull <id>` → edit → `wt.sh push <id>`) and runs everything on the host
  (`wt.sh run <id> 'tools/ci.sh --base main'`), commits on the host (`wt.sh commit <id> "feat: …"`).
- Alternatively a worker edits directly over SSH (heredocs / scp). Either way: **no local git, no push/pull.**
- The manager merges on the host (`git -C /root/ngfw merge --no-ff task/<id>`), runs `tools/ci.sh`, updates the board
  (`tools/board.py --set <id> merged`), writes status, commits.

When the product owner logs Claude in on the host (`claude` once as root), the same prompts run there under
`tools/manager-supervisor.sh` (systemd `ngfw-manager.service`) and this document is updated.
