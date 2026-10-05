#!/usr/bin/env python3
"""Reproducible freeze checks; offline checks never certify appliance acceptance."""
import argparse
import datetime
import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[3]
CASES = {
    'build-contracts': ('.', ['pnpm', 'exec', 'turbo', 'run', 'build', '--filter=@ngfw/schema', '--filter=@ngfw/proto']),
    'reachability-contract': ('test/integration/reachability', ['go', 'test', '-json', '-count=1', './...']),
    'commit-engine': ('apps/api', ['pnpm', 'exec', 'vitest', 'run', 'src/commit/commit.engine.test.ts', 'src/commit/commit.service.test.ts']),
    'auth-boundaries': ('apps/api', ['pnpm', 'exec', 'vitest', 'run', 'src/auth/route-guard.test.ts', 'src/auth/mfa-fail-closed.test.ts']),
}
LIVE = {
    'forwarding': 'Owned disposable VPP: real af_packet smoke, commit/readback/rollback and agent recovery.',
    'traffic-B': 'Run TEST-traffic-B product campaign with native route-based IPsec; old kernel-vpp phase superseded.',
    'traffic-C': 'Run test/topology/traffic-c in an owned manager window; require packet and rollback evidence.',
    'browser': 'Run owned API/agent/browser en/fa flows; preserve real commit, rollback and console-error checks.',
    'appliance': 'Install clean target, boot, full-stack recovery and authorized HA peer acceptance.',
}


def save_report(directory, report):
    with tempfile.NamedTemporaryFile(mode='w', dir=directory, delete=False) as stream:
        temporary = Path(stream.name)
        json.dump(report, stream, indent=2)
        stream.write('\n')
    os.replace(temporary, directory / 'summary.json')


def stop_owned(child):
    try:
        os.killpg(child.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        child.wait(timeout=10)
    except subprocess.TimeoutExpired:
        pass
    # The group leader can exit while an owned grandchild still holds the scheduler
    # lock. Reap the entire isolated group even when the leader exited gracefully.
    try:
        os.killpg(child.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    child.wait()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run-offline', action='store_true')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    os.chmod(args.output, 0o700)
    report = {'source_sha': None, 'dirty': None, 'time_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'scope': 'offline cross-component regression; no real appliance acceptance', 'cases': [],
              'live': {name: {'status': 'NOT RUN', 'required': detail} for name, detail in LIVE.items()},
              'release_accepted': False, 'offline_passed': False, 'status': 'RUNNING'}
    save_report(args.output, report)
    try:
        report['source_sha'] = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
        report['dirty'] = bool(subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT, text=True))
    except (OSError, subprocess.CalledProcessError):
        report['status'] = 'FAIL'
        save_report(args.output, report)
        return 1
    env = dict(os.environ)
    env.pop('NGFW_INTEGRATION', None)
    failed = False
    for name, (directory, command) in CASES.items():
        result = {'name': name, 'cwd': directory, 'command': command, 'status': 'NOT RUN'}
        report['cases'].append(result)
        save_report(args.output, report)
        if args.run_offline:
            log = args.output / (name + '.log')
            # Raw output stays private: test diagnostics can contain fixture credentials.
            with log.open('w') as stream:
                os.chmod(log, 0o600)
                child = None
                result['status'] = 'RUNNING'
                save_report(args.output, report)
                try:
                    child = subprocess.Popen([str(ROOT / 'tools/heavy.sh'), *command], cwd=ROOT / directory,
                                             env=env, stdout=stream, stderr=subprocess.STDOUT, start_new_session=True)
                    code = child.wait(timeout=1800)
                    result['exit_code'] = code
                    result['status'] = 'PASS' if code == 0 else 'FAIL'
                except subprocess.TimeoutExpired:
                    stop_owned(child)
                    result.update(status='FAIL', exit_code=124)
                except KeyboardInterrupt:
                    if child is not None:
                        stop_owned(child)
                    result.update(status='FAIL', exit_code=130)
                    report['status'] = 'INTERRUPTED'
                    save_report(args.output, report)
                    return 130
                except OSError:
                    result.update(status='FAIL', exit_code=127)
            failed |= result['status'] == 'FAIL'
        save_report(args.output, report)
        print(name + ': ' + result['status'], flush=True)
    report['offline_passed'] = args.run_offline and not failed
    report['status'] = 'FAIL' if failed else ('PASS' if args.run_offline else 'NOT RUN')
    save_report(args.output, report)
    return int(failed)


if __name__ == '__main__':
    def interrupted(signum, frame):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, interrupted)
    raise SystemExit(main())
