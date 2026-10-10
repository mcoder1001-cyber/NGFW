#!/usr/bin/env python3
"""Read-only management HTTPS checks, retaining failure and certificate scope."""
import datetime, hashlib, http.client, json, os, pathlib, re, socket, ssl, subprocess

OUTPUT = pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
result = {'scope': 'management IP destination; verified localhost certificate identity, not IP SAN trust', 'hosts': {}}
try:
    for host in ['172.30.126.37', '172.30.110.211']:
        q = subprocess.run(['ssh', '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes', '-o', 'ConnectTimeout=8', 'root@' + host, 'cat /etc/ngfw/tls/server.crt'], capture_output=True, timeout=15)
        assert q.returncode == 0 and not q.stderr
        ctx = ssl.create_default_context(cadata=q.stdout.decode())
        assert ctx.check_hostname and ctx.verify_mode == ssl.CERT_REQUIRED
        row = {'certificate_SHA': hashlib.sha256(q.stdout).hexdigest(), 'checks': []}
        result['hosts'][host] = row
        def get(path):
            c = http.client.HTTPConnection('localhost', 443, timeout=10)
            c.sock = ctx.wrap_socket(socket.create_connection((host, 443), timeout=10), server_hostname='localhost')
            leaf = hashlib.sha256(c.sock.getpeercert(binary_form=True)).hexdigest()
            c.request('GET', path, headers={'Host': 'localhost'})
            r = c.getresponse()
            body = r.read()
            row['checks'].append({'path': path, 'status': r.status, 'bytes': len(body), 'body_SHA': hashlib.sha256(body).hexdigest(), 'leaf_SHA': leaf, 'TLS_certificate_and_hostname_verified': True})
            c.close()
            return r.status, body
        status, html = get('/')
        assert status == 200
        status, _ = get('/api/v1/state/system')
        assert status == 401
        assets = sorted(set(re.findall(r'(?:src|href)="(/assets/[A-Za-z0-9_.-]+)"', html.decode())))
        assert assets, 'no script or stylesheet assets referenced'
        for asset in assets:
            status, body = get(asset)
            assert status == 200 and len(body) > 100
        row['PASS'] = True
    result['PASS'] = True
except Exception as e:
    result.update(PASS=False, failure=type(e).__name__ + ': ' + str(e))
data = json.dumps(result, indent=2).encode()
name = OUTPUT / ('manager-external-HTTPS-' + datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '.json')
fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
with os.fdopen(fd, 'wb') as stream:
    stream.write(data)
    stream.flush()
    os.fsync(stream.fileno())
fd = os.open(OUTPUT, os.O_DIRECTORY | os.O_NOFOLLOW)
os.fsync(fd)
os.close(fd)
print(json.dumps({'file': str(name), 'SHA': hashlib.sha256(data).hexdigest(), 'bytes': len(data), 'PASS': result['PASS']}))
raise SystemExit(0 if result['PASS'] else 2)
