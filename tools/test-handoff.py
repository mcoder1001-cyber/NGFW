#!/usr/bin/env python3
"""Hand finite validation to the coordinator without blocking a developer.

submit --cwd <frozen clean worktree> --head <SHA> --lane fast|heavy -- <argv...>
status <job.json>
Results/logs live on disk; developers continue in a different worktree.
This starts a finite local worker, not a persistent AI supervisor.
"""
import argparse
import datetime
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
import uuid

TOOLS = Path(__file__).resolve().parent
JOBS = Path('/root/.cache/ngfw-test-jobs')


def save(path, data):
    temp = path.with_suffix('.tmp')
    temp.write_text(json.dumps(data, indent=2) + '\n')
    os.replace(temp, path)


def clean_head(cwd, head):
    actual = subprocess.check_output(['git', '-C', cwd, 'rev-parse', 'HEAD'], text=True).strip()
    clean = not subprocess.check_output(
        ['git', '-C', cwd, 'status', '--porcelain', '--untracked-files=normal'], text=True,
    ).strip()
    return actual == head and clean


def run(path):
    job = json.loads(path.read_text())
    job.update(pid=os.getpid(), state='checking', started=datetime.datetime.now(datetime.timezone.utc).isoformat())
    save(path, job)
    process = None

    def interrupted(signum, _frame):
        raise InterruptedError(f'Worker interrupted by signal {signum}')

    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    try:
        if not clean_head(job['cwd'], job['head']):
            job.update(state='stale', error='Worktree must remain clean at submitted SHA; create a new worktree for development.')
            return
        wrapper = TOOLS / ('test-fast.sh' if job['lane'] == 'fast' else 'heavy.sh')
        env = os.environ.copy()
        env.pop('VRX_HEAVY_HELD', None)
        # Host tests belong to the exclusive laboratory queue, never this worker.
        env.pop('VRX_INTEGRATION', None)
        job['state'] = 'queued_or_running'
        save(path, job)
        with open(job['log'], 'w') as log:
            process = subprocess.Popen(
                [str(wrapper), *job['command']], cwd=job['cwd'], env=env,
                stdout=log, stderr=subprocess.STDOUT,
                start_new_session=True,
            )
            code = process.wait(timeout=job['deadline_seconds'])
        job['exit_code'] = code
        job['state'] = 'passed' if code == 0 else 'failed'
        if not clean_head(job['cwd'], job['head']):
            job.update(state='stale', error='Tracked tree changed during validation; result does not validate submitted SHA.')
    except subprocess.TimeoutExpired:
        job.update(state='failed', error='Worker exceeded queue + execution deadline')
    except Exception as error:
        job.update(state='failed', error=str(error))
    finally:
        if process is not None:
            # Even after a wrapper exits, descendants can retain its lock FD.
            # Kill only this job's dedicated process group, never a shared PID.
            try:
                os.killpg(process.pid, signal.SIGTERM)
                deadline = time.monotonic() + 10
                while time.monotonic() < deadline:
                    process.poll()  # reap the wrapper; descendants still matter
                    try:
                        os.killpg(process.pid, 0)
                    except ProcessLookupError:
                        break
                    time.sleep(0.1)
                else:
                    os.killpg(process.pid, signal.SIGKILL)
                process.wait()
            except ProcessLookupError:
                pass
        job['finished'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
        save(path, job)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='action', required=True)
    submit = commands.add_parser('submit')
    submit.add_argument('--cwd', required=True)
    submit.add_argument('--head', required=True)
    submit.add_argument('--lane', choices=['fast', 'heavy'], default='fast')
    submit.add_argument('--deadline-seconds', type=int, default=1200)
    submit.add_argument('command', nargs=argparse.REMAINDER)
    status = commands.add_parser('status')
    status.add_argument('job', type=Path)
    worker = commands.add_parser('_run')
    worker.add_argument('job', type=Path)
    args = parser.parse_args()
    if args.action == '_run':
        run(args.job)
        return
    if args.action == 'status':
        data = json.loads(args.job.read_text())
        if data['state'] in ('submitted', 'checking', 'queued_or_running'):
            try:
                pid = data.get('pid')
                if pid is None:
                    pid = int(args.job.with_suffix('.pid').read_text())
                os.kill(pid, 0)
            except ProcessLookupError:
                data.update(state='lost', error='Worker exited without final result; resubmit from frozen checkpoint.')
            except FileNotFoundError:
                data['state'] = 'starting'
        print(json.dumps(data, indent=2))
        return
    command = args.command[1:] if args.command[:1] == ['--'] else args.command
    if not command or args.deadline_seconds < 1:
        parser.error('a finite command and positive deadline are required')
    cwd = str(Path(args.cwd).resolve())
    head = subprocess.check_output(['git', '-C', cwd, 'rev-parse', args.head + '^{commit}'], text=True).strip()
    if not clean_head(cwd, head):
        parser.error('submit a clean worktree at the exact checkpoint SHA')
    JOBS.mkdir(parents=True, exist_ok=True)
    path = JOBS / (uuid.uuid4().hex + '.json')
    job = dict(cwd=cwd, head=head, lane=args.lane, command=command,
               deadline_seconds=args.deadline_seconds, state='submitted', log=str(path.with_suffix('.log')))
    save(path, job)
    # A finite worker survives this chat turn and owns its child process group.
    process = subprocess.Popen(
        [sys.executable, str(Path(__file__).resolve()), '_run', str(path)],
        stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        start_new_session=True,
    )
    # Worker owns later state writes. Store launcher separately to avoid races.
    path.with_suffix('.pid').write_text(str(process.pid) + '\n')
    print(path)


if __name__ == '__main__':
    main()
