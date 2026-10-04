#!/usr/bin/env python3
"""Live capture acceptance. Requires an isolated slot API/agent and pre-built rig."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import time
import urllib.error
import urllib.request


def request(base, token, method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(base + path, data=data, method=method,
                                 headers={'Authorization': 'Bearer ' + token,
                                          'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=35) as res:
            return res.status, res.read()
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read()


def require_status(result, expected):
    code, data = result
    if code != expected:
        # Never write authentication headers or environment to evidence.
        raise RuntimeError(f'HTTP {code}, expected {expected}: {data[:500]!r}')
    return data


def run(args):
    if args.vpp_socket == '/run/vpp/api.sock' or not args.dedicated_vpp or args.vpp_unit in ('vpp', 'vpp.service'):
        raise ValueError('dispatch capture is banned on shared VPP; dedicated per-slot VPP required')
    token = os.environ['NGFW_CAPTURE_TOKEN']
    prefix = os.environ['NGFW_TEST_PREFIX']
    if not re.fullmatch(r'w(?:[1-9]|1[01]|1[4-9]|2[0-9]|3[0-2])', prefix):
        raise ValueError('worker slot prefix required (CI slot 12 is reserved)')
    if not re.fullmatch(r'host-' + re.escape(prefix) + r'[lw][0-9]+', args.interface):
        raise ValueError('capture interface must use this slot prefix')
    if args.bpf and os.environ.get('NGFW_DF8_GLOBALS') != '1':
        raise ValueError('BPF requires explicit NGFW_DF8_GLOBALS=1 manager window')
    args.output.mkdir(parents=True, exist_ok=True)
    args.output.chmod(0o700)
    events = []
    def command(argv):
        result = subprocess.run(argv, check=True, capture_output=True, text=True, timeout=35)
        events.append({'command': argv, 'stdout': result.stdout, 'stderr': result.stderr})
        return result.stdout.strip()
    def call(method, path, body=None, expected=200):
        result = request(args.api, token, method, path, body)
        events.append({'method': method, 'path': path, 'status': result[0]})
        return require_status(result, expected)
    capture_id = None
    before = {unit: command(['systemctl', 'show', unit, '-p', 'NRestarts', '--value'])
              for unit in ('vpp', args.vpp_unit)}
    try:
        # No packets are emitted until TD-3's real read-only preflight succeeds.
        command([args.preflight, '-socket', args.vpp_socket])
        invalid = json.loads(call('POST', '/api/v1/actions/capture',
            {'interface': args.interface, 'bpf': 'ip; bad'}, 400))
        if '/bpf' not in json.dumps(invalid):
            raise AssertionError('missing /bpf validation pointer')
        body = {'interface': args.interface, 'seconds': 15, 'maxPackets': 1000,
                'snaplen': 128, 'bpf': args.bpf}
        capture_id = json.loads(call('POST', '/api/v1/actions/capture', body, 202))['id']
        if not capture_id.startswith(prefix + '-') or not re.fullmatch(r'[A-Za-z0-9._-]+', capture_id):
            raise AssertionError('unexpected capture owner/id')
        busy = json.loads(call('POST', '/api/v1/actions/capture', body, 409))
        if not busy['type'].endswith('capture-busy'):
            raise AssertionError('missing capture-busy problem')
        call('DELETE', '/api/v1/state/captures/' + capture_id, expected=409)
        # Fixed argv; no arbitrary shell commands or user input reaches a shell.
        command(['ip', 'netns', 'exec', 'ns-' + prefix + '-lan', 'ping', '-c', '3', '-W', '1', args.peer])
        call('POST', '/api/v1/actions/capture/' + capture_id + '/stop', expected=202)
        deadline = time.monotonic() + 35
        while True:
            records = json.loads(call('GET', '/api/v1/state/captures'))['captures']
            record = next(x for x in records if x['id'] == capture_id)
            if record['state'] != 'running':
                break
            if time.monotonic() >= deadline:
                raise TimeoutError('capture did not stop')
            time.sleep(0.2)
        if record['state'] != 'done' or int(record['packets']) < 1:
            raise AssertionError(f'no real captured packets: {record}')
        kept = args.capture_dir / (capture_id + '.pcap')
        info = kept.lstat()
        if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_nlink != 1:
            raise AssertionError('kept pcap is not a regular single-link 0600 file')
        if (Path('/tmp') / (capture_id + '.pcap')).exists():
            raise AssertionError('VPP temporary file remains')
        data = call('GET', '/api/v1/state/captures/' + capture_id + '/file')
        if len(data) != int(record['size']) or hashlib.sha256(data).hexdigest() != record['sha256']:
            raise AssertionError('download metadata mismatch')
        download = args.output / (capture_id + '.pcap')
        with download.open('xb') as file:
            download.chmod(0o600)
            file.write(data)
        command(['tcpdump', '-nn', '-r', str(download)])
        command(['ls', '-l', str(kept)])
        call('DELETE', '/api/v1/state/captures/' + capture_id, expected=204)
        if kept.exists():
            raise AssertionError('DELETE left pcap on disk')
        call('GET', '/api/v1/state/captures/' + capture_id + '/file', expected=404)
        remaining = json.loads(call('GET', '/api/v1/state/captures'))['captures']
        if any(x['id'] == capture_id for x in remaining):
            raise AssertionError('DELETE left metadata')
        capture_id = None
        events.append({'packet_acceptance': 'passed', 'record': record})
    finally:
        if capture_id:
            # Request only this API-owned stream's stop; never stop a foreign capture.
            request(args.api, token, 'POST', '/api/v1/actions/capture/' + capture_id + '/stop')
        after = {unit: command(['systemctl', 'show', unit, '-p', 'NRestarts', '--value'])
                 for unit in ('vpp', args.vpp_unit)}
        events.append({'NRestarts_before': before, 'NRestarts_after': after})
        (args.output / 'evidence.json').write_text(json.dumps(events, indent=2) + '\n')
        if after != before:
            raise RuntimeError('VPP NRestarts changed: stop host runs and investigate')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dedicated-vpp', action='store_true', help='confirm API, agent and rig use dedicated per-slot VPP')
    parser.add_argument('--vpp-socket', required=True)
    parser.add_argument('--vpp-unit', required=True, help='dedicated VPP systemd unit, distinct from shared vpp')
    parser.add_argument('--api', required=True)
    parser.add_argument('--interface', required=True)
    parser.add_argument('--peer', required=True)
    parser.add_argument('--capture-dir', required=True, type=Path)
    parser.add_argument('--preflight', required=True)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--bpf', default='')
    args = parser.parse_args()
    # Serialize host capture tests, protect against VPP restart, lock globals if opted in.
    with open('/run/lock/ngfw-lab.lock', 'a') as lab, open('/run/lock/ngfw-capture-test.lock', 'a') as capture:
        fcntl.flock(lab, fcntl.LOCK_SH)
        fcntl.flock(capture, fcntl.LOCK_EX)
        if args.bpf:
            with open('/run/lock/ngfw-globals.lock', 'a') as globals_lock:
                fcntl.flock(globals_lock, fcntl.LOCK_EX)
                run(args)
        else:
            run(args)

if __name__ == '__main__':
    main()
