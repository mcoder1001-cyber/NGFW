#!/usr/bin/env python3
# Manager: create worktrees + envelopes for tasks and mark them running.
# Usage: tools/manager/prep.py ID:SLOT:HOURS[:daemon-owner] ...   (run from /root/ngfw, main clean)
import sys, subprocess, datetime, pathlib, yaml
ROOT = pathlib.Path("/root/ngfw"); WT = pathlib.Path("/root/ngfw-wt")
d = yaml.safe_load((ROOT/"plan/tasks.yaml").read_text()); by = {t["id"]: t for t in d["tasks"]}
base = subprocess.check_output(["git","-C",str(ROOT),"rev-parse","--short","HEAD"]).decode().strip()
now = datetime.datetime.now().strftime("%Y-%m-%dT%H:%M")
for spec in sys.argv[1:]:
    parts = spec.split(":"); tid, slot, hours = parts[0], int(parts[1]), parts[2]; daemon = parts[3] if len(parts) > 3 else "none"
    t = by[tid]
    if t["state"] not in ("ready","todo","failed","parked"): print(f"{tid}: state {t['state']} — skipped"); continue
    wt = WT/tid
    if not wt.exists():
        subprocess.check_call(["git","-C",str(ROOT),"worktree","add","-q",str(wt),"-b",f"task/{tid}","main"])
    merged = [x for x in t["deps"] if by[x]["state"] == "merged"]
    env = f"""# TASK ENVELOPE — {tid}
id: {tid}   branch: task/{tid}   worktree: {wt}   base: main@{base}   started: {now}
title: {t['title']}
prompt: {t['prompt']}   (template: {t.get('template') or '-'})   wbs: {', '.join(t.get('wbs') or [])}
scope: {t.get('scope') or '-'}
merged deps you can rely on: {', '.join(merged) or 'P01'}
slot: {slot} → VRX_TEST_PREFIX=w{slot}  VRX_HTTP_PORT=3{slot}00  VRX_WEB_PORT=5{slot}00  VRX_METRICS_PORT=91{slot}1  VRX_AGENT_SOCKET=/run/vrx-test/w{slot}/agent.sock  VRX_PG_DATABASE=vrx_w{slot}  VRX_VPP_TABLE_BASE={slot}000
daemon-owner: {daemon}
files you own exclusively: {t.get('files_owned') or '(see prompt)'}
files you must not touch: everything else; never /root/ngfw (main), other worktrees, /etc/vpp, /root/vpp, apps/agent/binapi (P04/manager-owned)
time box: {hours} h — when exceeded: stop, commit WIP, write docs/status/tasks/{tid}.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/{tid}-wip.md current
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/{tid}.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
questions: docs/status/tasks/{tid}-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
"""
    (wt/"docs/status/tasks").mkdir(parents=True, exist_ok=True)
    (wt/"docs/status/tasks"/f"{tid}.envelope.md").write_text(env); (WT/f"{tid}.envelope.md").write_text(env)
    subprocess.check_call(["git","-C",str(wt),"add","-A"]); subprocess.check_call(["git","-C",str(wt),"commit","-qm",f"chore({tid}): task envelope"])
    t.update(state="running", owner=f"desktop-agent slot{slot}", branch=f"task/{tid}", worktree=str(wt), started=now)
    print(f"prepared {tid} slot {slot} ({hours} h)")
yaml.safe_dump(d, open(ROOT/"plan/tasks.yaml","w"), sort_keys=False, allow_unicode=True, width=200)
subprocess.check_call(["python3", str(ROOT/"tools/board.py")])
