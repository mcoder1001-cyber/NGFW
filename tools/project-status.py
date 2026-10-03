#!/usr/bin/env python3
"""Read-only board and local Git inventory. Does not fetch or infer live workers."""
import argparse
from collections import Counter
import json
import math
from pathlib import Path
import subprocess
import sys

import yaml

STATES = ('merged', 'review', 'running', 'ready', 'parked', 'failed', 'todo')


def board_summary(document):
    tasks = document.get('tasks') if isinstance(document, dict) else None
    if not isinstance(tasks, list):
        raise ValueError('board tasks must be a list')
    counts = Counter({state: 0 for state in STATES})
    total_hours = merged_hours = 0
    ids = set()
    for task in tasks:
        if not isinstance(task, dict) or not isinstance(task.get('id'), str):
            raise ValueError('each task must have a string id')
        if task['id'] in ids:
            raise ValueError('duplicate task id: ' + task['id'])
        ids.add(task['id'])
        state = task.get('state')
        if not isinstance(state, str) or not state:
            raise ValueError('each task must have a nonempty state')
        hours = task.get('est_hours')
        if isinstance(hours, bool) or not isinstance(hours, (int, float)) or not math.isfinite(hours) or hours < 0:
            raise ValueError('est_hours must be a finite nonnegative number')
        counts[state] += 1
        total_hours += hours
        if state == 'merged':
            merged_hours += hours
    return {'total_tasks': len(tasks), 'state_counts': dict(sorted(counts.items())),
            'unknown_states': sorted(set(counts) - set(STATES)),
            'estimated_hours': {'merged': merged_hours, 'total': total_hours,
                                'progress_percent': 100 * merged_hours / total_hours if total_hours else None},
            'task_progress_percent': 100 * counts['merged'] / len(tasks) if tasks else None,
            'live_developers': {'count': None, 'status': 'unverifiable',
                                'reason': 'Board running tasks and Git authors do not establish live workers.'}}


def contributors_summary(log):
    authors = {}
    for line in log.splitlines():
        name, separator, email = line.partition('\t')
        if not separator or not email.strip():
            continue
        key = email.strip().casefold()
        authors.setdefault(key, set()).add(name.strip())
    return {'count': len(authors), 'identity_basis': 'case-insensitive Git author email; local reachable HEAD history',
            'authors': [{'email': email, 'names': sorted(names)} for email, names in sorted(authors.items())]}


def git_read(root, *args):
    result = subprocess.run(['git', '-C', str(root), *args], capture_output=True, text=True, check=False)
    return result.stdout.strip() if result.returncode == 0 else None


def git_summary(root):
    head = git_read(root, 'rev-parse', '--verify', 'HEAD')
    comparison = git_read(root, 'rev-list', '--left-right', '--count', 'HEAD...refs/remotes/origin/main')
    ahead = behind = None
    if comparison:
        parts = comparison.split()
        if len(parts) == 2 and all(part.isdigit() for part in parts):
            ahead, behind = map(int, parts)
    authors = git_read(root, 'log', '--format=%an%x09%ae', 'HEAD')
    return {'head': head, 'branch': git_read(root, 'symbolic-ref', '--quiet', '--short', 'HEAD'),
            'origin_main': {'ahead': ahead, 'behind': behind,
                            'status': 'known' if ahead is not None else 'unverifiable',
                            'basis': 'local origin/main ref; no fetch performed'},
            'contributors': contributors_summary(authors) if authors is not None else {'count': None, 'status': 'unverifiable'}}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--json', action='store_true', help='print machine-readable JSON')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    try:
        report = board_summary(yaml.safe_load((root / 'plan/tasks.yaml').read_text()))
        report['git'] = git_summary(root)
    except (OSError, ValueError, yaml.YAMLError) as exc:
        print('project-status: ' + str(exc), file=sys.stderr)
        return 1
    if args.json:
        print(json.dumps(report, ensure_ascii=False, indent=2))
    else:
        print(f"Tasks: {report['total_tasks']}; " + ', '.join(f'{state}={count}' for state, count in report['state_counts'].items()))
        hours = report['estimated_hours']
        percent = f"{hours['progress_percent']:.1f}%" if hours['progress_percent'] is not None else 'n/a'
        print(f"Estimated-hours progress: {hours['merged']}/{hours['total']} ({percent})")
        print(f"Live developers: {report['live_developers']['status']} (running tasks are not a worker inventory)")
        git = report['git']
        divergence = git['origin_main']
        print(f"Git HEAD: {git['head']}; branch: {git['branch'] or 'detached/unknown'}")
        print(f"Local origin/main: ahead={divergence['ahead']} behind={divergence['behind']} ({divergence['status']}; no fetch)")
        print(f"Historical contributor emails: {git['contributors']['count']} (not live developers)")
        if report['unknown_states']:
            print('Unknown board states: ' + ', '.join(report['unknown_states']))
    return 0


if __name__ == '__main__':
    sys.exit(main())
