#!/usr/bin/env python3
"""Management TLS rotation on an isolated real slot API; no shared host changes."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import socket
import ssl
import subprocess
import tempfile
import time
import urllib.error
import sys
import urllib.request

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from private_http import private_opener
sys.dont_write_bytecode = True

class Refused(RuntimeError):
    pass


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
            response = private_opener().open(request, timeout=30)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            status = response.status
            raw = response.read(1048577)
            content_type = response.headers.get('Content-Type', '')
        if len(raw) > 1048576 or status != want:
            raise Refused(f'{method} {path}: HTTP {status}, expected {want}')
        if want == 204:
            if raw:
                raise Refused('DELETE response unexpectedly contains a body')
            return {}
        result = json.loads(raw)
        if want == 400 and not content_type.startswith('application/problem+json'):
            raise Refused('validation response is not problem+json')
        return result


def handshake(port, certificate, minimum='1.3', maximum='1.3'):
    context = ssl.create_default_context(cafile=str(certificate))
    context.minimum_version = getattr(ssl.TLSVersion, 'TLSv' + minimum.replace('.', '_'))
    context.maximum_version = getattr(ssl.TLSVersion, 'TLSv' + maximum.replace('.', '_'))
    with socket.create_connection(('127.0.0.1', port), timeout=5) as raw:
        with context.wrap_socket(raw, server_hostname='localhost') as channel:
            peer = channel.getpeercert(binary_form=True)
            return {'sha256': hashlib.sha256(peer).hexdigest(), 'version': channel.version()}


def private_material(directory, name):
    certificate, key = directory / (name + '.crt'), directory / (name + '.key')
    subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '2',
        '-subj', '/CN=' + name, '-addext', 'subjectAltName=DNS:localhost,IP:127.0.0.1',
        '-keyout', str(key), '-out', str(certificate)], check=True, timeout=30,
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    key.chmod(0o600)
    return certificate, key


def safe_public(value, keys):
    rendered = json.dumps(value)
    if 'PRIVATE KEY' in rendered or any(key in rendered for key in keys):
        raise Refused('private material appeared in public response/log; content withheld')


def own_lock(api, owner=None):
    state = api.call('GET', '/config/lock')
    if owner is None:
        if state.get('locked'):
            raise Refused('candidate is locked; preserve other work')
        api.call('PATCH', '/config', {})
        state = api.call('GET', '/config/lock')
        if not state.get('ownerKeyId'):
            raise Refused('dedicated API key required')
        return state
    if (state.get('ownerKeyId'), state.get('lockedAt')) != (owner['ownerKeyId'], owner['lockedAt']):
        raise Refused('candidate owner changed; cleanup/edit refused')
    return state


def claim_revision(api, expected):
    owner = own_lock(api)
    difference = api.call('GET', '/config/diff')
    if difference.get('baseRevision') != expected or difference.get('changes'):
        raise Refused('running/candidate changed during lock acquisition; preserve for inspection')
    return owner


def commit_observation(api, response, baseline, owner):
    revision = response.get('revision', {}).get('id')
    difference = api.call('GET', '/config/diff')
    lock = api.call('GET', '/config/lock')
    current = difference.get('baseRevision')
    if current != baseline and current != revision:
        raise Refused('commit outcome ambiguous; preserve new running revision and secrets for inspection')
    if lock.get('locked') and (lock.get('ownerKeyId'), lock.get('lockedAt')) != (owner['ownerKeyId'], owner['lockedAt']):
        raise Refused('commit candidate owner changed; preserve for inspection')
    return (revision if current == revision and current != baseline else None, bool(lock.get('locked')))


def acceptance(api, slot, directory, process_identity):
    before = process_identity()
    if api.call('GET', '/config/diff').get('changes'):
        raise Refused('candidate must be clean')
    baseline_revision = api.call('GET', '/config/diff')['baseRevision']
    original = api.call('GET', '/config')['management']['tls']
    initial = api.call('GET', '/state/management/tls')
    port = initial['listener']['port']
    expected_port = (3000 + slot * 100 if slot < 12 else 10000 + slot * 100) + 1
    if not initial.get('active') or initial.get('error') or port != expected_port:
        raise Refused('slot must have an active baseline TLS listener on HTTP port + 1')
    run_id = f'w{slot}-ui-' + secrets.token_hex(5)
    cert, key = private_material(directory, run_id)
    _, mismatch = private_material(directory, run_id + '-bad')
    key_text, mismatch_text = key.read_text(), mismatch.read_text()
    material = [key_text, mismatch_text]
    uploaded = []
    applied_revision = None
    owner = None
    dirty = False
    preserve = False
    try:
        for kind, name, value in (('cert', run_id, cert.read_text()), ('key', run_id, key_text),
                                  ('key', run_id + '-bad', mismatch_text)):
            output = api.call('POST', '/secrets', {'kind': kind, 'name': name, 'value': value})
            uploaded.append(output['ref'])
            safe_public(output, material)
        owner = claim_revision(api, baseline_revision)
        dirty = True
        own_lock(api, owner)
        api.call('PUT', '/config/management/tls', {'certificateRef': uploaded[0],
            'privateKeyRef': uploaded[1], 'minVersion': '1.3'})
        response = api.call('POST', '/config/commit?comment=management-ui-host')
        try:
            applied_revision, dirty = commit_observation(api, response, baseline_revision, owner)
        except Refused:
            preserve = True
            raise
        if response.get('status') != 'applied' or response.get('notApplied') or applied_revision is None:
            raise Refused('TLS commit not fully applied; observed revision governs cleanup')
        if not dirty:
            owner = None
        deadline = time.monotonic() + 20
        observed = None
        while time.monotonic() < deadline:
            try:
                observed = handshake(port, cert)
                break
            except (ssl.SSLError, OSError):
                time.sleep(0.1)
        if observed is None:
            raise Refused('new certificate handshake unavailable')
        expected = hashlib.sha256(ssl.PEM_cert_to_DER_cert(cert.read_text())).hexdigest()
        if observed != {'sha256': expected, 'version': 'TLSv1.3'}:
            raise Refused('live certificate fingerprint/version mismatch')
        try:
            handshake(port, cert, '1.2', '1.2')
        except ssl.SSLError:
            pass
        else:
            raise Refused('TLS 1.2 accepted after 1.3-only rotation')
        loaded = api.call('GET', '/state/management/tls')
        if loaded.get('loadedRevision') != applied_revision or loaded.get('error'):
            raise Refused('state does not reflect applied certificate revision')
        owner = claim_revision(api, applied_revision)
        dirty = True
        api.call('PUT', '/config/management/tls', {'certificateRef': uploaded[0],
            'privateKeyRef': uploaded[2], 'minVersion': '1.3'})
        problem = api.call('POST', '/config/validate', want=400)
        if not any(issue.get('pointer') == '/management/tls/privateKeyRef' for issue in problem.get('errors', [])):
            raise Refused('mismatched key has no privateKeyRef problem pointer')
        for value in (problem, loaded, api.call('GET', '/config'), api.call('GET', '/secrets'),
                      api.call('GET', '/audit?limit=500')):
            safe_public(value, material)
        if process_identity() != before:
            raise Refused('API process restarted during rotation')
        return {'status': 'TLS_API_ACCEPTANCE_PASSED', 'fingerprintSha256': expected,
            'loadedRevision': applied_revision, 'TLS1.2': 'REFUSED', 'mismatchedKey': '400_WITH_POINTER',
            'private_material_public': False, 'API_process_unchanged': True, 'browser': 'NOTRUN',
            'log_scrub': 'NOTRUN'}
    finally:
        if preserve:
            raise Refused('ambiguous commit: running/candidate/secrets preserved for manager inspection')
        if dirty:
            own_lock(api, owner)
            api.call('POST', '/config/discard')
        if applied_revision is not None:
            if api.call('GET', '/config/diff') != {'baseRevision': applied_revision, 'changes': []}:
                raise Refused('running/candidate changed; rollback refused to preserve concurrent work')
            owner = claim_revision(api, applied_revision)
            result = api.call('POST', f'/config/rollback/{baseline_revision}?comment=management-ui-host-cleanup')
            if result.get('status') not in ('applied', 'unchanged'):
                raise Refused('baseline rollback failed')
            api.call('POST', '/config/discard')
            if api.call('GET', '/config')['management']['tls'] != original:
                raise Refused('baseline TLS configuration not restored')
        for ref in reversed(uploaded):
            api.call('DELETE', '/secrets/' + ref, want=204)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--slot', type=int, required=True)
    args = parser.parse_args()
    try:
        if os.environ.get('NGFW_INTEGRATION') != '1' or os.environ.get('NGFW_MANAGEMENT_UI_HOST') != '1':
            raise Refused('explicit host acceptance opt-ins required')
        api = Api(args.slot, os.environ.get('NGFW_HOST_ACCESS_TOKEN', ''))
        pid = int(os.environ['NGFW_HOST_API_PID'])
        identity = lambda: Path(f'/proc/{pid}/stat').read_text().split(') ', 1)[1].split()[19]
        with open('/run/lock/ngfw-lab.lock', 'a') as lab, open(f'/run/lock/ngfw-ui-host-w{args.slot}.lock', 'a') as slot_lock:
            fcntl.flock(lab, fcntl.LOCK_SH | fcntl.LOCK_NB)
            fcntl.flock(slot_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with tempfile.TemporaryDirectory(prefix='ngfw-management-ui-', dir=os.environ.get('TMPDIR')) as temp:
                print(json.dumps(acceptance(api, args.slot, Path(temp), identity), indent=2))
        return 0
    except (Refused, OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
        print(f'management acceptance refused: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
