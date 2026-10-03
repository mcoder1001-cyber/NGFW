#!/usr/bin/env python3
"""Index retained Go test events; counts include subtests and repeated executions."""
import collections
import json
from pathlib import Path
ROOT = Path(__file__).resolve().parents[3]
OUT = ROOT / 'docs/status/tasks/full-test-2026-10-03-evidence'
rows = []
for path in sorted(OUT.iterdir()):
    if path.suffix not in {'.log', '.txt', '.jsonl'} or not path.is_file(): continue
    events = []
    outputs = collections.defaultdict(list)
    for line in path.read_text(errors='replace').replace('\0','').splitlines():
        try: event = json.loads(line)
        except (ValueError, TypeError): continue
        if not isinstance(event, dict): continue
        test = event.get('Test')
        if test and event.get('Action') == 'output': outputs[test].append(event.get('Output',''))
        if test and event.get('Action') in {'pass', 'fail', 'skip'}:
            events.append({'package':event.get('Package'), 'test':test, 'result':event['Action'], 'elapsed':event.get('Elapsed')})
    if not events: continue
    counts = collections.Counter(e['result'] for e in events)
    failures = [dict(e, context=''.join(outputs[e['test']][-10:])) for e in events if e['result']=='fail']
    skipped = [dict(e, context=''.join(outputs[e['test']][-3:])) for e in events if e['result']=='skip']
    rows.append({'log':path.name, 'counts':dict(counts), 'failures':failures, 'skipped':skipped})
(OUT/'test-summary.json').write_text(json.dumps({'counting':'Go test events include subtests and duplicate reruns; not a unique test total', 'runs':rows}, ensure_ascii=False, indent=2))
lines=['# Go test evidence index', '', 'Counts include subtests; repeated runs are separate. See test-summary.json for failures and skip reasons.', '', '| Log | Pass | Fail | Skip |', '|---|---:|---:|---:|']
for row in rows:
    c=row['counts']; lines.append('| [%s](%s) | %s | %s | %s |' % (row['log'],row['log'],c.get('pass',0),c.get('fail',0),c.get('skip',0)))
(OUT/'index.md').write_text('\n'.join(lines)+'\n')
print('Indexed',len(rows),'Go runs')
