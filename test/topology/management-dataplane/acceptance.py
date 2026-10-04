#!/usr/bin/env python3
"""Real slot API acceptance; run only beneath hardware-smoke/isolated-vpp.py.

Requires NGFW_ACCEPTANCE_API_MAIN and NGFW_ACCEPTANCE_AGENT pointing to built
current-source artifacts. Generates secret fixtures exclusively on slot tmpfs.
"""
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import time
import urllib.error
import urllib.request


def main():
    assert os.environ.get('NGFW_DISPOSABLE_VPP') == '1', 'private VPP required'
    assert os.environ['NGFW_SLOT'] == '9', 'reserved slot 9 required'
    run = Path('/run/ngfw-test/w9')
    run.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(run, 0o700)
    os.umask(0o077)
    env = dict(os.environ)
    subprocess.run(['deploy/dev/pg-test.sh', 'create', 'w9'], check=True)
    for line in (run / 'pg.env').read_text().splitlines():
        key, value = line.split('=', 1)
        env[key] = value
    password = secrets.token_urlsafe(24)
    env.update(NGFW_JWT_SECRET=secrets.token_urlsafe(48),
               NGFW_BOOTSTRAP_ADMIN_PASSWORD=password, NGFW_COOKIE_SECURE='0',
               NGFW_HTTPS_PORT='3901', NGFW_HTTP_HOST='127.0.0.1',
               NGFW_SECRET_KEY_FILE=str(run / 'secret.key'),
               NGFW_OWNER='w9', NGFW_AGENT_OWNER='w9', NGFW_GLOBALS_OWNER='0',
               NGFW_AGENT_STATE_DIR=str(run / 'agent-state'),
               NGFW_VALKEY_PREFIX='ngfw:w9:acceptance:', NGFW_AGENT_TIMEOUT_MS='15000')
    processes = []
    logs = []
    token = ''
    observed = []
    shared_vpp = subprocess.check_output(['systemctl', 'show', 'vpp', '-p', 'MainPID', '-p', 'NRestarts'])
    print('SHARED_VPP_BEFORE=' + shared_vpp.decode().strip().replace('\n', ','), flush=True)
    def request(path, method='GET', body=None, expected=200):
        headers = {'Content-Type': 'application/json'}
        if token:
            headers['Authorization'] = 'Bearer ' + token
        req = urllib.request.Request('http://127.0.0.1:3900/api/v1/' + path,
                                     data=None if body is None else json.dumps(body).encode(),
                                     headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=40) as response:
                status, raw = response.status, response.read()
        except urllib.error.HTTPError as exc:
            status, raw = exc.code, exc.read()
        value = json.loads(raw) if raw else None
        assert status == expected, f'{method} {path}: status {status}, expected {expected}'
        if path != 'auth/login':
            observed.append(raw)
        return value
    def show(label, value):
        print(label + '=' + json.dumps(value, sort_keys=True), flush=True)
    def invalid(path, body, pointer):
        request(path, 'PATCH', body)
        result = request('config/validate', 'POST', {}, 400)
        assert any(e['pointer'] == pointer for e in result['errors']), result
        show('EXPECTED_400', result)
        request('config/discard', 'POST', {})
    def peer(version):
        result = subprocess.run(['openssl', 's_client', '-connect', '127.0.0.1:3901',
                                 '-servername', 'localhost', version], input=b'',
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=15)
        return result
    def commit_tls(cert, key, minimum):
        request('config/management/tls', 'PATCH',
                {'certificateRef': 'cert/' + cert, 'privateKeyRef': 'key/' + key,
                 'minVersion': minimum})
        show('TLS_COMMIT', request('config/commit', 'POST', {}))
        for _ in range(100):
            state = request('state/management/tls')
            if state.get('listener') and cert in str(state.get('certificateRef', cert)):
                break
            time.sleep(.1)
        return state
    try:
        for name, command in [('agent', [env['NGFW_ACCEPTANCE_AGENT']]),
                              ('api', ['node', env['NGFW_ACCEPTANCE_API_MAIN']])]:
            log = (run / (name + '.log')).open('wb')
            logs.append(log)
            processes.append(subprocess.Popen(command, env=env, stdout=log, stderr=log))
            if name == 'agent':
                for _ in range(100):
                    if Path(env['NGFW_AGENT_SOCKET']).exists():
                        break
                    time.sleep(.1)
        for _ in range(150):
            try:
                request('health')
                break
            except (OSError, AssertionError):
                time.sleep(.2)
        token = request('auth/login', 'POST', {'username': 'admin', 'password': password})['accessToken']
        print('API_PID=' + str(processes[1].pid), flush=True)
        startup = Path('/etc/vpp/startup.conf')
        before = hashlib.sha256(startup.read_bytes()).hexdigest()
        show('DATAPLANE_STATE', request('state/dataplane'))
        request('config/dataplane', 'PATCH', {'workers': 1, 'corelist': [5], 'mainCore': 4})
        preview = request('actions/dataplane/preview', 'POST', {})
        assert preview['restartRequired'] is True and preview['applyAvailable'] is True
        assert hashlib.sha256(preview['rendered'].encode()).hexdigest() == preview['sha256']
        assert 'corelist-workers 5' in preview['rendered']
        show('DATAPLANE_PREVIEW', preview)
        request('config/discard', 'POST', {})
        invalid('config/dataplane', {'workers': 2, 'corelist': [5, 5]}, '/dataplane/corelist/1')
        invalid('config/dataplane', {'workers': 1, 'corelist': [4], 'mainCore': 4}, '/dataplane/mainCore')
        invalid('config/dataplane', {'devices': {'0000:04:00.0': {'rxDesc': 100}}}, '/dataplane/devices/0000:04:00.0/rxDesc')
        assert before == hashlib.sha256(startup.read_bytes()).hexdigest()
        print('STARTUP_UNCHANGED=' + before, flush=True)
        print('DATAPLANE_REAL_API_ACCEPTANCE=PASS', flush=True)
        materials = []
        for name, days in [('w9a', '2'), ('w9b', '2'), ('w9expired', '-1')]:
            key, cert = run / (name + '.key'), run / (name + '.crt')
            csr = run / (name + '.csr')
            subprocess.run(['openssl', 'req', '-new', '-newkey', 'rsa:2048', '-nodes',
                            '-subj', '/CN=' + name, '-keyout', str(key),
                            '-out', str(csr)], check=True, stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL)
            validity = ['-days', days] if days != '-1' else [
                '-not_before', '20200101000000Z', '-not_after', '20210101000000Z']
            subprocess.run(['openssl', 'x509', '-req', '-in', str(csr), '-signkey', str(key),
                            *validity, '-out', str(cert)], check=True,
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            for kind, path in [('key', key), ('cert', cert)]:
                material = path.read_text()
                materials.append(material.encode())
                request('secrets', 'POST', {'kind': kind, 'name': name, 'value': material})
        state = commit_tls('w9a', 'w9a', '1.2')
        show('TLS_STATE_A', state)
        a = peer('-tls1_2')
        assert a.returncode == 0 and b'BEGIN CERTIFICATE' in a.stdout
        public = subprocess.run(['openssl', 'x509', '-noout', '-subject', '-fingerprint', '-sha256'],
                                input=a.stdout, stdout=subprocess.PIPE, check=True).stdout.decode()
        print('PEER_A=' + public.strip(), flush=True)
        state = commit_tls('w9b', 'w9b', '1.3')
        time.sleep(.3)
        b = peer('-tls1_3')
        assert b.returncode == 0 and b'BEGIN CERTIFICATE' in b.stdout
        public = subprocess.run(['openssl', 'x509', '-noout', '-subject', '-fingerprint', '-sha256'],
                                input=b.stdout, stdout=subprocess.PIPE, check=True).stdout.decode()
        assert 'w9b' in public
        print('PEER_B=' + public.strip(), flush=True)
        assert peer('-tls1_2').returncode != 0
        assert processes[1].poll() is None
        print('TLS12_REFUSED_TLS13_ACCEPTED_SAME_PID=' + str(processes[1].pid), flush=True)
        show('TLS_STATE_B', request('state/management/tls'))
        for cert, key, pointer in [('w9b', 'w9a', '/management/tls/privateKeyRef'),
                                   ('w9expired', 'w9expired', '/management/tls/certificateRef')]:
            request('config/management/tls', 'PATCH', {'certificateRef': 'cert/' + cert,
                                                     'privateKeyRef': 'key/' + key})
            result = request('config/commit', 'POST', {}, 400)
            assert any(e['pointer'] == pointer for e in result['errors']), result
            show('TLS_COMMIT_EXPECTED_400', result)
            request('config/discard', 'POST', {})
        for path in ['config', 'config/candidate', 'state/management/tls', 'secrets']:
            request(path)
        audit = subprocess.run(['runuser', '-u', 'postgres', '--', 'psql', '-X', '-qAt',
                                '-d', 'ngfw_w9', '-c', 'select row_to_json(a)::text from audit_log a'],
                               check=True, stdout=subprocess.PIPE).stdout
        corpus = b'\n'.join(observed) + audit + (run / 'api.log').read_bytes()
        assert b'-----BEGIN ' not in corpus
        assert not any(material in corpus for material in materials)
        print('PEM_LEAKS_LOG_AUDIT_GETS=0', flush=True)
        after_vpp = subprocess.check_output(['systemctl', 'show', 'vpp', '-p', 'MainPID', '-p', 'NRestarts'])
        assert after_vpp == shared_vpp
        print('SHARED_VPP_AFTER=' + after_vpp.decode().strip().replace('\n', ','), flush=True)
        assert before == hashlib.sha256(startup.read_bytes()).hexdigest()
        print('MANAGEMENT_REAL_API_ACCEPTANCE=PASS', flush=True)
    finally:
        for process in reversed(processes):
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
        for log in logs:
            log.close()
        subprocess.run(['deploy/dev/pg-test.sh', 'drop', 'w9'], check=True)
        for file in run.glob('*'):
            if file.is_file():
                file.unlink()
        shutil.rmtree(run / 'agent-state', ignore_errors=True)


if __name__ == '__main__':
    main()
