#!/usr/bin/env python3
"""Real slot API acceptance; changes only the candidate, never commits/restarts."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import urllib.error
import urllib.request

sys.dont_write_bytecode = True


class Refused(RuntimeError):
    pass


def cores(text):
    values = set()
    for part in text.split(','):
        bounds = part.split('-')
        first, last = int(bounds[0]), int(bounds[-1])
        if first < 0 or last < first or last > 4095 or len(bounds) > 2:
            raise Refused('invalid online CPU inventory')
        values.update(range(first, last + 1))
    return sorted(values)


class Api:
    def __init__(self, slot, token):
        if slot not in (*range(1, 12), *range(14, 33)) or not token:
            raise Refused('allocated developer slot and access token required')
        port = 3000 + slot * 100 if slot < 12 else 10000 + slot * 100
        self.base = f'http://127.0.0.1:{port}/api/v1'
        self.token = token

    def call(self, method, path, body=None, want=200):
        data = None if body is None else json.dumps(body).encode()
        request = urllib.request.Request(self.base + path, data=data, method=method,
            headers={'Authorization': 'ApiKey ' + self.token, 'Content-Type': 'application/json'})
        try:
            response = urllib.request.urlopen(request, timeout=30)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            status = response.status
            raw = response.read(1048577)
            content_type = response.headers.get('Content-Type', '')
        if len(raw) > 1048576 or status != want:
            raise Refused(f'{method} {path}: HTTP {status}, expected {want}')
        result = json.loads(raw)
        if want == 400 and not content_type.startswith('application/problem+json'):
            raise Refused('validation response is not problem+json')
        return result


def assert_problem(result):
    errors = result.get('errors', [])
    if not any(issue.get('pointer', '').startswith('/dataplane/corelist') or
               issue.get('pointer', '').startswith('/dataplane/workers') for issue in errors):
        raise Refused('invalid corelist has no field pointer')


def restarts():
    return subprocess.run(['systemctl', 'show', 'vpp', '-p', 'NRestarts', '-p', 'MainPID', '-p', 'ActiveEnterTimestampMonotonic'],
        check=True, capture_output=True, text=True, timeout=10).stdout.strip()


def acceptance(api, snapshot):
    if api.call('GET', '/config/diff').get('changes'):
        raise Refused('candidate must be clean before acceptance')
    if api.call('GET', '/config/lock').get('locked'):
        raise Refused('candidate already locked; dedicated API key required')
    api.call('PATCH', '/config', {})  # no-op first edit takes the product lock atomically
    owner = api.call('GET', '/config/lock')
    if not owner.get('locked') or not owner.get('ownerKeyId'):
        raise Refused('candidate must be owned by a dedicated API key')
    baseline = api.call('GET', '/config/diff')
    if baseline.get('changes'):
        raise Refused('candidate changed during lock acquisition; preserve it for inspection')
    original = api.call('GET', '/config')['dataplane']
    before = snapshot()
    edited = True
    try:
        state = api.call('GET', '/state/dataplane')
        if state.get('error') or state.get('runtimeErrors') or not state.get('runtimeThreads'):
            raise Refused('observed VPP state unavailable')
        available = [cpu for cpu in cores(state['onlineCpus']) if cpu != state.get('mainCore')]
        if not available:
            raise Refused('no spare online CPU for candidate preview')
        candidate = dict(original, workers=1, corelist=available[:1])
        current_lock = api.call('GET', '/config/lock')
        if (current_lock.get('ownerKeyId'), current_lock.get('lockedAt')) != (owner['ownerKeyId'], owner['lockedAt']):
            edited = False
            raise Refused('candidate ownership changed; preserve candidate')
        api.call('PUT', '/config/dataplane', candidate)
        preview = api.call('POST', '/actions/dataplane/preview')
        if (preview.get('restartRequired') is not True or type(preview.get('applyAvailable')) is not bool
            or hashlib.sha256(preview.get('rendered', '').encode()).hexdigest() != preview.get('sha256')):
            raise Refused('preview gating or digest mismatch')
        if 'corelist-workers ' + str(available[0]) not in preview['rendered']:
            raise Refused('candidate CPU is absent from rendered startup')
        if not preview.get('changed') or not preview.get('diff'):
            raise Refused('changed candidate has no startup diff')
        # Semantic validation lives on validate/commit, rather than schema-only edit.
        api.call('PUT', '/config/dataplane', dict(candidate, workers=2))
        problem = api.call('POST', '/config/validate', want=400)
        assert_problem(problem)
        if api.call('GET', '/config/diff').get('baseRevision') != baseline.get('baseRevision'):
            raise Refused('running revision changed during acceptance')
        if snapshot() != before:
            raise Refused('startup file or VPP restart count changed')
        return {'status': 'API_ACCEPTANCE_PASSED', 'browser_acceptance': 'NOTRUN',
                'runtimeThreads': state['runtimeThreads'], 'diff': preview['diff'],
                'validationPointers': [issue['pointer'] for issue in problem['errors']],
                'startup_unchanged': True, 'NRestarts_unchanged': True}
    finally:
        if edited:
            current_lock = api.call('GET', '/config/lock')
            if (current_lock.get('ownerKeyId'), current_lock.get('lockedAt')) != (owner['ownerKeyId'], owner['lockedAt']):
                raise Refused('candidate lock changed; cleanup refused to preserve other work')
            api.call('POST', '/config/discard')
            if api.call('GET', '/config/diff').get('changes'):
                raise Refused('candidate cleanup failed')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--slot', type=int, required=True)
    args = parser.parse_args()
    try:
        if os.environ.get('NGFW_INTEGRATION') != '1' or os.environ.get('NGFW_DATAPLANE_UI_HOST') != '1':
            raise Refused('NGFW_INTEGRATION=1 and NGFW_DATAPLANE_UI_HOST=1 required')
        api = Api(args.slot, os.environ.get('NGFW_HOST_ACCESS_TOKEN', ''))
        snapshot = lambda: (hashlib.sha256(Path('/etc/vpp/startup.conf').read_bytes()).hexdigest(), restarts())
        with open('/run/lock/ngfw-lab.lock', 'a') as lab, open(f'/run/lock/ngfw-ui-host-w{args.slot}.lock', 'a') as slot_lock:
            fcntl.flock(lab, fcntl.LOCK_SH | fcntl.LOCK_NB)
            fcntl.flock(slot_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            print(json.dumps(acceptance(api, snapshot), indent=2))
        return 0
    except (Refused, OSError, ValueError, subprocess.SubprocessError) as error:
        print(f'dataplane acceptance refused: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
