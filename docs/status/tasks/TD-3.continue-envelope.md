# TASK ENVELOPE — TD-3 (CONTINUE, respawned by host manager cycle 12, 2026-09-24 07:2x)
id: TD-3   branch: task/TD-3   worktree: /root/ngfw-wt/TD-3
CONTINUE: the previous worker died mid fix round 1 at ~05:45. Its uncommitted edits are salvaged as the top commit d5116b8 ("wip(TD-3): salvage …") — review that diff first, keep what is right, finish the fix round; do not restart.
read first: docs/status/tasks/TD-3-review.md (BLOCK 46fafdb: H1 L2 ACL/policer bits = second crash path, H2 invisible bindings, H3 delete must clear bindings first, M1; reviewer's safe behaviour adopted: force L3 → resurrect table index → quarantine; V23 folded in), docs/status/tasks/TD-3.md, docs/tech-debt.md row D-095, docs/vpp-code-track.md V19/V23
merged deps you can rely on: DF-1, DF-2, DF-5, DF-7, DF-8, P05, TD-1
slot: 2 → eval "$(tools/lab env 2)" (VRX_TEST_PREFIX=w2, ports 3200/5200/9121, tables 2000-2999, DB vrx_w2). V19 SAFETY: never send a packet through an interface you have not dumped+asserted clean; check systemctl show vpp -p NRestarts before/after every host run — stop and write it down if it rises.
files you own exclusively: apps/agent/internal/descriptors/{interface,classify,policer,ipfix,adl}/** apps/agent/internal/vpp/ifsanitize/** apps/agent/cmd/vrx-vpp-preflight/** plus the minimal wiring calls already in the salvaged diff (af_packet, bond, core/loopback, df6, dfkit fake, lcp, memif, mpls, tapv2) · tools/ci.sh v19_preflight hunk · docs/agent/descriptors/{classify,interface}.md · docs/status/tasks/TD-3*
coordination: P08 (slot 1) is running and wires interface creation in apps/agent/internal/{agent,subsystems,desired} — do not edit those.
time box: 4 h
finish by: fix-round section appended to docs/status/tasks/TD-3.md answering every review finding with pasted real output (unit + host test on slot 2 + NRestarts before/after) · tools/ci.sh --base main green in this worktree · everything committed · final message = 10-line summary
rules on main to honour: prompts/00-CONTEXT.md; docs/decisions/LOG.md (D-039…D-097); docs/lab/shared-host-rules.md
commit WIP at least every 45 min and keep docs/status/tasks/TD-3-wip.md current — you may be killed by a usage limit at any time and respawned with a CONTINUE envelope
questions: docs/status/tasks/TD-3-questions.md — write and keep going; never wait for a human
never: merge to main · touch /root/ngfw (main) or other worktrees · restart/kill VPP · edit /etc/vpp, startup.conf, vpp.service · Docker · pkill · secrets in files/logs/fixtures · install packages
