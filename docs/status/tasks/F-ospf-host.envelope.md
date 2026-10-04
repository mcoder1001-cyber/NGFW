TASK ENVELOPE
id: F-ospf-host   branch: task/F-ospf-host   worktree: /root/ngfw-wt/F-ospf-host   base: main@79fff64a   started: 2026-09-28T18:52
title: F-ospf host runs: frrtest ospfd + rig FIB evidence (R7/R4/R1 owed lists)
prompt: prompts/features/F-ospf-host.md   wbs: —
scope: owed checklist = "Owed on the host" sections of docs/status/tasks/RV-A-review-R7.md, RV-A-review-R4.md and RV-A-review-R1.md for F-ospf; daemon-owner frr; FRR host rows one at a time (launch queue §3)
merged deps you can rely on: F-ospf — plus everything on main@79fff64a
slot: 11 → source of truth `eval "$(tools/lab env 11)"`:
  NGFW_SLOT=11
  NGFW_TEST_PREFIX=w11
  NGFW_HTTP_PORT=4100
  NGFW_WEB_PORT=6100
  NGFW_METRICS_PORT=9211
  NGFW_AGENT_SOCKET=/run/ngfw-test/w11/agent.sock
  NGFW_PG_DATABASE=ngfw_w11
  NGFW_VALKEY_DB=11
  NGFW_VPP_TABLE_BASE=11000
  NGFW_LAB_LOCK=/run/lock/ngfw-lab.lock
files you own exclusively: docs/status/tasks/F-ospf-host* test/topology/ospf/**
files you must not touch: everything else; /root/NGFW (main) and every other worktree; /etc/vpp, /root/vpp, vpp.service (handover pending, D-012); apps/agent/binapi/**; tools/binapi-gen.sh
Manager constraints: HOST-FOLLOWUP-TEMPLATE applies (evidence .txt, NRestarts before/after every host step, D-174 anchors). FRR host rows run ONE AT A TIME — you are the only FRR host row now; FRR daemons only inside your slot netns/instance, never the system frr unit. Owed checklist = the F-ospf sections of docs/status/tasks/RV-A-review-R7.md, R4.md, R1.md. Close every lock fd in background children (`8>&- 9>&-`) — see the F-vrrp-config-sync-host review. Screenshots owed to T4 after merge.

shared-host rules: docs/lab/shared-host-rules.md; never `vppctl delete host-interface` by hand; reset ip4/ip6 classify on every interface you create before any address add (D-185); globals only under the globals lock inside the shared lab lock (D-167); evidence as .txt, NRestarts pasted, CI tail pasted after merging current main (D-175); no pkill/killall/pattern kills
how you run: on the host itself (ignore the ssh/$WT mechanics of WORKER-OPS.md; its rules apply); first line `cd /root/ngfw-wt/F-ospf-host`; `pnpm install --prefer-offline` once; CI `TMPDIR=/tmp/g-w11 tools/ci.sh --base main`
commit WIP at least every 45 min and keep docs/status/tasks/F-ospf-host-wip.md current
time box: 5 h — when exceeded, stop, commit WIP, write docs/status/tasks/F-ospf-host.md with what is left
finish by: committing on your branch, writing docs/status/tasks/F-ospf-host.md (what, how verified with pasted real output, Shared hunks, out of scope, decisions, open questions), all checks green; never merge, never touch main
questions: docs/status/tasks/F-ospf-host-questions.md; keep going on everything not blocked
