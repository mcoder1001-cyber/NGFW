TASK ENVELOPE
id: F-nat46-host   branch: task/F-nat46-host   worktree: /root/ngfw-wt/F-nat46-host   base: main@4741ff86   started: 2026-09-29T07:52
title: F-nat46 host runs: TestNat46OnHost on a slot, vppctl show map domains, rollback, NRestarts, screenshot
prompt: prompts/features/F-nat46-host.md   wbs: —
scope: host evidence owed by F-nat46 (cloud session without VPP): the "Not built (remaining — for the host)" / "Not done / remains" section of docs/status/tasks/F-nat46.md
merged deps you can rely on: F-nat46 — plus everything on main@4741ff86
slot: 17 → source of truth `eval "$(tools/lab env 17)"`:
  NGFW_SLOT=17
  NGFW_TEST_PREFIX=w17
  NGFW_HTTP_PORT=11700
  NGFW_WEB_PORT=15700
  NGFW_METRICS_PORT=9271
  NGFW_AGENT_SOCKET=/run/ngfw-test/w17/agent.sock
  NGFW_PG_DATABASE=ngfw_w17
  NGFW_VALKEY_DB=17
  NGFW_VPP_TABLE_BASE=17000
  NGFW_LAB_LOCK=/run/lock/ngfw-lab.lock
files you own exclusively: docs/status/tasks/F-nat46-host*
files you must not touch: everything else; /root/NGFW (main) and every other worktree; /etc/vpp, /root/vpp, vpp.service (handover pending, D-012); apps/agent/binapi/**; tools/binapi-gen.sh
Manager constraints: HOST-FOLLOWUP-TEMPLATE applies (evidence .txt under -evidence/, NRestarts before/after every host step, D-174 anchors, lock fds closed in children `8>&- 9>&-`). Read D-185 and D-203 (table-0 sentinel is permanent). Any VPP-global NAT setting only under the shared lab lock + exclusive globals lock (D-167) and restored afterwards, else record as owed to a manager window. Slot 17 has no Valkey database: Go/agent-level only, no API stack. F-det44-map-dslite-cnat-host runs in parallel on slot 16 — coordinate on globals via the lock, never by hand.

shared-host rules: docs/lab/shared-host-rules.md; never `vppctl delete host-interface` by hand; reset ip4/ip6 classify on every interface you create before any address add (D-185); globals only under the globals lock inside the shared lab lock (D-167); evidence as .txt, NRestarts pasted, CI tail pasted after merging current main (D-175); no pkill/killall/pattern kills
how you run: on the host itself (ignore the ssh/$WT mechanics of WORKER-OPS.md; its rules apply); first line `cd /root/ngfw-wt/F-nat46-host`; `pnpm install --prefer-offline` once; CI `TMPDIR=/tmp/g-w17 tools/ci.sh --base main`
commit WIP at least every 45 min and keep docs/status/tasks/F-nat46-host-wip.md current
time box: 5 h — when exceeded, stop, commit WIP, write docs/status/tasks/F-nat46-host.md with what is left
finish by: committing on your branch, writing docs/status/tasks/F-nat46-host.md (what, how verified with pasted real output, Shared hunks, out of scope, decisions, open questions), all checks green; never merge, never touch main
questions: docs/status/tasks/F-nat46-host-questions.md; keep going on everything not blocked
