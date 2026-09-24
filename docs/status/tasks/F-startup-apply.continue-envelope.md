# TASK ENVELOPE — F-startup-apply (CONTINUE, respawned by host manager cycle 12, 2026-09-24 07:2x)
id: F-startup-apply   branch: task/F-startup-apply   worktree: /root/ngfw-wt/F-startup-apply
CONTINUE: the previous worker died mid fix round 2 (last round) at ~05:45. Its uncommitted edits are salvaged as the top commit 028eda4 ("wip(F-startup-apply): salvage …") — review that diff first, keep what is right, finish the round; do not restart.
read first: docs/status/tasks/F-startup-apply-rereview.md (APPROVE WITH CHANGES 3c0bafc: N1 SSH-session probe → rollback loop, N2 no systemd-run fallback, N3 early crash, N4 approval binding), docs/status/tasks/F-startup-apply.md, the original envelope docs/status/tasks/F-startup-apply.envelope.md (scope, owned files — still valid)
slot: 6 → eval "$(tools/lab env 6)". NEVER run --apply for real — fake-host harness only. D-060: linux_cp/linux_nl/npt66 are enabled but handover is still pending; the handover gate must stay enforced.
files you own exclusively: deploy/vpp/apply-startup.sh deploy/vpp/vpp-iface-check* deploy/vpp/test-apply-startup.sh apps/agent/cmd/vrx-vppcheck/** docs/agent/renderers/vppstartup.md docs/status/tasks/F-startup-apply*
time box: 3 h
finish by: fix-round-2 section in docs/status/tasks/F-startup-apply.md answering N1–N4 with pasted fake-host harness output · tools/ci.sh --base main green · everything committed · final message = 10-line summary
rules on main to honour: prompts/00-CONTEXT.md; docs/decisions/LOG.md (D-039…D-097); docs/lab/shared-host-rules.md
commit WIP at least every 45 min and keep docs/status/tasks/F-startup-apply-wip.md current — you may be killed by a usage limit at any time and respawned with a CONTINUE envelope
questions: docs/status/tasks/F-startup-apply-questions.md — write and keep going; never wait for a human
never: merge to main · touch /root/ngfw (main) or other worktrees · restart/kill VPP · edit /etc/vpp, startup.conf, vpp.service · Docker · pkill · secrets in files/logs/fixtures · install packages
