#!/usr/bin/env python3
'''Validate plan/tasks.yaml and write docs/status/PROGRESS.md (progress by estimated hours and by count).
Usage: tools/board.py [--set ID STATE [--note TEXT]]   (run from the repo root)'''
import sys, re, pathlib, datetime
try:
    import yaml
except ImportError:
    sys.exit("python3-yaml missing: apt-get install -y python3-yaml")
ROOT = pathlib.Path(__file__).resolve().parents[1]
BOARD = ROOT / "plan" / "tasks.yaml"
d = yaml.safe_load(BOARD.read_text())
tasks = d["tasks"]; by = {t["id"]: t for t in tasks}
args = sys.argv[1:]
changed = False
if "--set" in args:
    i = args.index("--set"); tid, state = args[i+1], args[i+2]
    if tid not in by: sys.exit(f"unknown task {tid}")
    by[tid]["state"] = state
    if "--note" in args: by[tid]["notes"] = (by[tid].get("notes") or "") + " | " + args[args.index("--note")+1]
    changed = True
# auto-ready runs on every invocation
for t in tasks:
    if t["state"] == "todo" and all(by[x]["state"] == "merged" for x in t["deps"]) and not t.get("parked_on"):
        t["state"] = "ready"; changed = True
# validation
ids = [t["id"] for t in tasks]
dup = {x for x in ids if ids.count(x) > 1}
bad = [(t["id"], x) for t in tasks for x in t["deps"] if x not in by]
def cyc():
    seen, stack = set(), set()
    def visit(n):
        if n in stack: return True
        if n in seen: return False
        stack.add(n)
        for x in by[n]["deps"]:
            if x in by and visit(x): return True
        stack.discard(n); seen.add(n); return False
    return [t for t in ids if visit(t)]
cycles = cyc()
if dup or bad or cycles:
    sys.exit(f"BOARD INVALID duplicates={dup} unknown_deps={bad} cycles={cycles[:5]}")
if changed:
    d["updated"] = str(datetime.date.today())
    BOARD.write_text(yaml.safe_dump(d, sort_keys=False, allow_unicode=True, width=200))
# progress
def pct(a, b): return f"{(100*a/b):.1f}%" if b else "n/a"
stages = sorted({t["stage"] for t in tasks})
lines = ["# Progress", "", f"Updated {datetime.date.today()} from plan/tasks.yaml (estimated hours are the plan's, not actuals).", ""]
tot_h = sum(t["est_hours"] for t in tasks); done_h = sum(t["est_hours"] for t in tasks if t["state"] == "merged")
counts = {s: sum(1 for t in tasks if t["state"] == s) for s in ("merged","review","running","ready","parked","failed","todo")}
lines += [f"**Overall: {pct(done_h, tot_h)} by hours ({done_h}/{tot_h} h), {pct(counts['merged'], len(tasks))} by tasks ({counts['merged']}/{len(tasks)})**", "",
          "| state | tasks |", "|---|---|"] + [f"| {s} | {n} |" for s, n in counts.items()] + ["", "| stage | merged h / total h | % | tasks merged/total | running | ready | parked |", "|---|---|---|---|---|---|---|"]
for s in stages:
    ts = [t for t in tasks if t["stage"] == s]
    th = sum(t["est_hours"] for t in ts); dh = sum(t["est_hours"] for t in ts if t["state"] == "merged")
    lines.append(f"| {s} | {dh} / {th} | {pct(dh, th)} | {sum(1 for t in ts if t['state']=='merged')}/{len(ts)} | {sum(1 for t in ts if t['state']=='running')} | {sum(1 for t in ts if t['state']=='ready')} | {sum(1 for t in ts if t['state']=='parked')} |")
lines += ["", "## Running / review", ""] + [f"- {t['id']} — {t['title']} ({t['state']}, {t.get('owner') or 'unassigned'})" for t in tasks if t["state"] in ("running","review")] or ["- none"]
lines += ["", "## Parked", ""] + ([f"- {t['id']} — parked_on: {t.get('parked_on')}" for t in tasks if t["state"] == "parked"] or ["- none"])
prog = ROOT / "docs" / "status" / "PROGRESS.md"
body = "\n".join(lines) + "\n"
strip = lambda x: re.sub(r"^Updated \S+ ", "", x, flags=re.M)
if not prog.exists() or strip(prog.read_text()) != strip(body):  # a date-only change is not a change
    prog.write_text(body)
print(f"board ok: {len(tasks)} tasks; progress {pct(done_h, tot_h)} by hours, {counts['merged']}/{len(tasks)} merged; ready={counts['ready']} running={counts['running']} parked={counts['parked']}")
