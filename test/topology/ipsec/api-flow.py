#!/usr/bin/env python3
"""Production API -> encrypted secret delivery -> native VPP profile lifecycle.

Run inside isolated-vpp.py with the reviewed IKEv2 readback plugin, slot env and
NGFW_IPSEC_API_AGENT_BIN pointing to a freshly built product agent. PSKs and the
CLI profile dump stay in memory; persisted evidence contains booleans only.
"""
import base64
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[3]
EVENTS = []


def check(ok, phase):
    if not ok:
        raise RuntimeError(phase)


def wait(predicate, phase, seconds=40):
    until = time.monotonic() + seconds
    while time.monotonic() < until:
        if predicate():
            return
        time.sleep(.2)
    raise RuntimeError(phase)


def run(argv, env=None):
    p = subprocess.run(argv, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    check(p.returncode == 0, 'subprocess failed: ' + Path(argv[0]).name)
    return p.stdout


def stop(p):
    if p is not None and p.poll() is None:
        p.terminate()
        try:
            p.wait(timeout=10)
        except subprocess.TimeoutExpired:
            p.kill()
            p.wait()


def main():
    check(os.environ.get('NGFW_DISPOSABLE_VPP') == '1', 'requires disposable VPP')
    prefix = os.environ['NGFW_TEST_PREFIX']
    slot = int(prefix[1:])
    base = slot * 1000
    run_dir = Path(os.environ.get('NGFW_RUN_DIR', '/run/ngfw-test/' + prefix))
    work = run_dir / 'ipsec-api-flow'
    pg_script_env = dict(os.environ, NGFW_RUN_DIR=str(run_dir.parent))
    check(not work.exists(), 'refusing to reuse existing fixture directory')
    work.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
    work.mkdir(mode=0o700)
    state = work / 'agent-state'
    socket = str(work / 'agent.sock')
    port = os.environ['NGFW_HTTP_PORT']
    url = 'http://127.0.0.1:' + port
    token = ''
    password = secrets.token_urlsafe(32)
    psks = [secrets.token_hex(32), secrets.token_hex(32)]
    markers = [encoded for p in psks for encoded in
               (p.encode(), p.encode().hex().encode(), base64.b64encode(p.encode()))]
    evidence = Path(os.environ['NGFW_IPSEC_API_EVIDENCE'])
    agent = api = None
    api_env = None
    handles = []
    db_created = False

    def request(method, path, body=None, expected=200):
        raw = None if body is None else json.dumps(body).encode()
        headers = {}
        if raw is not None:
            headers['Content-Type'] = 'application/json'
        if token:
            headers['Authorization'] = 'Bearer ' + token
        req = urllib.request.Request(url + '/api/v1' + path, raw, headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=20) as response:
                status, data = response.status, response.read()
        except urllib.error.HTTPError as error:
            status, data = error.code, error.read()
        except (urllib.error.URLError, TimeoutError):
            return None
        # Responses must never echo either actual PSK.
        check(not any(p in data for p in markers), 'secret echoed by API')
        result = json.loads(data) if data else {}
        if expected is None and status in (502, 503):
            return None
        if status != (200 if expected is None else expected):
            # Diagnostic fields are structural only; never print request values.
            EVENTS.append({'phase': 'unexpected-response', 'method': method, 'path': path,
                           'status': status, 'problem': result.get('type'),
                           'detail': result.get('detail'), 'title': result.get('title'),
                           'rules': [e.get('rule') for e in result.get('errors', [])],
                           'pointers': [e.get('pointer') for e in result.get('errors', [])]})
            raise RuntimeError('API status mismatch: ' + method + ' ' + path)
        return result

    def commit(comment):
        result = request('POST', '/config/commit?comment=' + comment)
        check(result and result.get('status') == 'applied', 'commit not applied: ' + comment)
        return result['revision']['id']

    def profile_matches(psk):
        raw = run(['vppctl', '-s', '/run/vpp/cli.sock', 'show', 'ikev2', 'profile'])
        # The VPP formatter emits either ASCII or 0x hex depending on auth.hex.
        return (b'profile ' + (prefix + '-site').encode() in raw and
                (psk.encode() in raw or psk.encode().hex().encode() in raw))

    def bundle_id():
        return json.loads((state / 'agent-state.json').read_text())['secret_bundle']

    def sql(query):
        return run(['psql', '-X', '-qAt', '-c', query], env=pg_env).decode().strip()

    def assert_version(revision, version):
        pinned = json.loads(sql('SELECT secret_versions FROM config_revision WHERE id=' + str(revision)))
        active = int(sql("SELECT version FROM secret WHERE ref='psk/site'"))
        check(pinned.get('psk/site') == version and active == version, 'secret version not pinned/reactivated')

    def native_readback():
        data = request('GET', '/state/ipsec/tunnels?tunnel=site')
        check(data and data.get('daemonVersion') == 'vpp-ikev2', 'wrong IPsec runtime')
        check(len(data.get('tunnels', [])) == 1, 'native profile safe readback missing')
        data = request('GET', '/state/drift')
        check(data and not data.get('changes'), 'unexpected drift after apply/restart')
        # Configuration uses the logical tunnel name; VPP uses the assigned instance.
        running = request('GET', '/config')
        routes = [r for r in running['routing']['static'] if r['prefix'] == '10.6.20.0/24']
        check(len(routes) == 1 and routes[0]['nextHops'][0]['interface'] == 'site', 'API lost logical route interface')
        ctl = os.environ.get('NGFW_IPSEC_API_CTL_BIN', str(ROOT / 'apps/agent/bin/ngfw-agentctl'))
        for selection in ['routing', 'routing,tunnels']:
            retrieved_raw = run([ctl, '-s', socket, 'retrieve', '-subsystems', selection])
            check(not any(marker in retrieved_raw for marker in markers), 'Retrieve leaked secret')
            retrieved = json.loads(retrieved_raw)['desiredState']
            routes = [r for r in retrieved['routing']['static'] if r['prefix'] == '10.6.20.0/24']
            check(len(routes) == 1 and routes[0]['nextHops'][0]['interface'] == 'site',
                  'Retrieve lost logical route interface: ' + selection)
            if 'tunnels' in selection:
                check('site' in retrieved['tunnels']['ipip'], 'Retrieve lost logical IPIP name')
        fib = run(['vppctl', '-s', '/run/vpp/cli.sock', 'show', 'ip', 'fib', 'table', '0', '10.6.20.0/24']).decode()
        check('10.6.20.0/24' in fib and 'ipip' + str(base + 1) in fib, 'VPP route not through runtime IPIP')
        EVENTS.append({'phase': 'logical-route-readback', 'logicalInterface': 'site',
                       'runtimeInterface': 'ipip' + str(base + 1), 'prefix': '10.6.20.0/24',
                       'apiRetrieveVppAgree': True, 'vppFib': fib.strip()})


    def start_agent():
        f = (work / 'agent.log').open('ab')
        handles.append(f)
        p = subprocess.Popen([os.environ['NGFW_IPSEC_API_AGENT_BIN']], env=agent_env, stdout=f, stderr=subprocess.STDOUT)
        wait(lambda: p.poll() is not None or Path(socket).exists(), 'agent socket readiness')
        check(p.poll() is None, 'agent exited during startup')
        return p

    try:
        # Exercise the real entitlement gate with a slot-local signed test licence.
        # This development trust root is confined to this API process and /run.
        licence_keys = work / 'licence-keys'
        licence = work / 'test.ngfwlic'
        serial = prefix + '-ipsec-api-test'
        issuer = str(ROOT / 'tools/license/ngfw-license')
        run([issuer, 'keygen', '--out-dir', str(licence_keys)])
        run([issuer, 'issue', '--key', str(licence_keys / 'ngfw-license-signing.pem'),
             '--customer', 'IPsec API isolated test', '--id', prefix + '-ipsec-api',
             '--days', '1', '--serial', serial, '--features', 'ipsec',
             '--limit', 'ipsecTunnels=1', '--out', str(licence)])
        run([issuer, 'verify', '--pub', str(licence_keys / 'ngfw-license-public.pem'), str(licence)])
        EVENTS.append({'phase': 'test-license', 'feature': 'ipsec', 'limit': 1, 'signed': True, 'isolatedTrustRoot': True})
        run([str(ROOT / 'deploy/dev/pg-test.sh'), 'create', prefix], env=pg_script_env)
        db_created = True
        pg = dict(line.split('=', 1) for line in (run_dir / 'pg.env').read_text().splitlines() if '=' in line)
        pg_env = dict(os.environ, PGHOST=pg['NGFW_PG_HOST'], PGPORT=pg['NGFW_PG_PORT'],
                      PGUSER=pg['NGFW_PG_USER'], PGPASSWORD=pg['NGFW_PG_PASSWORD'], PGDATABASE=pg['NGFW_PG_DATABASE'])
        clean = {k: os.environ[k] for k in ['PATH', 'HOME'] if k in os.environ}
        agent_env = dict(clean, NGFW_AGENT_SOCKET=socket, NGFW_OWNER=prefix,
                         NGFW_GLOBALS_OWNER='0', NGFW_VPP_TABLE_BASE=str(base),
                         NGFW_AGENT_STATE_DIR=str(state), NGFW_METRICS_PORT=os.environ['NGFW_METRICS_PORT'],
                         NGFW_SOCKET_GROUP='root', NGFW_LOG_LEVEL='info')
        api_env = dict(clean, NODE_ENV='production', NGFW_HTTP_HOST='127.0.0.1', NGFW_HTTP_PORT=port,
                       NGFW_LICENSE_FILE=str(licence), NGFW_LICENSE_SERIAL=serial,
                       NGFW_LICENSE_PUBKEY_FILE=str(licence_keys / 'ngfw-license-public.pem'),
                       NGFW_PG_DSN=pg['NGFW_PG_DSN'], NGFW_VALKEY_DB=os.environ['NGFW_VALKEY_DB'],
                       NGFW_VALKEY_PREFIX='ngfw:' + prefix + ':ipsec-api:' + secrets.token_hex(8) + ':',
                       NGFW_AGENT_SOCKET=socket, NGFW_AGENT_OWNER=prefix, NGFW_AGENT_TIMEOUT_MS='15000',
                       NGFW_JWT_SECRET=secrets.token_hex(48), NGFW_SECRET_KEY_FILE=str(work / 'api-secret.key'),
                       NGFW_BOOTSTRAP_ADMIN_PASSWORD=password, NGFW_COOKIE_SECURE='0', NGFW_LOG_LEVEL='warn')
        agent = start_agent()
        f = (work / 'api.log').open('ab')
        handles.append(f)
        api = subprocess.Popen(['node', str(ROOT / 'apps/api/dist/main.js')], env=api_env, stdout=f, stderr=subprocess.STDOUT)
        wait(lambda: api.poll() is not None or request('GET', '/health') is not None, 'API readiness')
        check(api.poll() is None, 'API startup failed')
        token = request('POST', '/auth/login', {'username': 'admin', 'password': password})['accessToken']
        secret = request('POST', '/secrets', {'kind': 'psk', 'name': 'site', 'value': psks[0]})
        check(secret['version'] == 1, 'unexpected initial secret version')
        request('PATCH', '/config', {
            'interfaces': {'loop' + str(base + 2): {'enabled': True, 'ipv4': ['198.18.6.1/30']}},
            'tunnels': {'ipip': {'site': {'instance': base + 1, 'src': '198.18.6.1', 'dst': '198.18.6.2',
                                        'underlayVrf': 'default', 'vrf': 'default', 'ipv4': ['198.18.63.1/30']}}},
            'routing': {'static': [{'prefix': '10.6.20.0/24', 'vrf': 'default',
                                    'nextHops': [{'address': '198.18.63.2', 'interface': 'site'}]}]},
            'vpn': {'ipsec': {
                'proposals': {'test': {'ike': {'encr': 'aes128', 'integ': 'sha256', 'prf': 'prfsha256', 'dh': 'modp2048'},
                                       'esp': {'encr': 'aes128gcm16'}}},
                'tunnels': {'site': {'engine': 'vpp-ikev2', 'localAddr': '198.18.6.1', 'remoteAddr': '198.18.6.2',
                                    'localId': '@local.test', 'remoteId': '@remote.test',
                                    'auth': {'method': 'psk', 'secretRef': 'psk/site'}, 'proposal': 'test',
                                    'routeBased': {'ipipInterface': 'site'}, 'startAction': 'none',
                                    'localTs': ['10.6.10.0/24'], 'remoteTs': ['10.6.20.0/24'],
                                    'underlayVrf': 'default', 'vrf': 'default'}}}}})
        revision1 = commit('native-api-v1')
        check(profile_matches(psks[0]), 'VPP did not install API-delivered PSK v1')
        assert_version(revision1, 1)
        bundle1 = bundle_id()
        native_readback()
        EVENTS.append({'phase': 'commit-v1', 'revision': revision1, 'version': 1, 'nativePskMatches': True})

        stop(agent)
        run(['vppctl', '-s', '/run/vpp/cli.sock', 'ikev2', 'profile', 'del', prefix + '-site'])
        agent = start_agent()
        wait(lambda: request('GET', '/state/ipsec/tunnels?tunnel=site', expected=None) is not None
             and profile_matches(psks[0]), 'API reconnect/profile resync')
        check(profile_matches(psks[0]) and bundle_id() == bundle1, 'restart lost secret/profile identity')
        native_readback()
        EVENTS.append({'phase': 'restart-recreate', 'sameSealedSnapshot': True, 'nativePskMatches': True})

        secret = request('POST', '/secrets?replace=true', {'kind': 'psk', 'name': 'site', 'value': psks[1]})
        check(secret['version'] == 2, 'secret rotation version mismatch')
        request('PATCH', '/config/vpn/ipsec/tunnels/site', {'description': 'rotation-v2'})
        revision2 = commit('native-api-v2')
        check(profile_matches(psks[1]) and bundle_id() != bundle1, 'VPP did not install rotated PSK')
        assert_version(revision2, 2)
        native_readback()
        EVENTS.append({'phase': 'commit-v2', 'revision': revision2, 'version': 2, 'nativePskMatches': True, 'newSealedSnapshot': True})

        reverted = request('POST', '/config/rollback/' + str(revision1) + '?comment=native-api-revert')
        check(reverted.get('status') == 'applied', 'rollback not applied')
        assert_version(reverted['revision']['id'], 1)
        check(profile_matches(psks[0]) and bundle_id() == bundle1, 'rollback did not restore exact original PSK snapshot')
        native_readback()
        EVENTS.append({'phase': 'rollback-v1', 'revision': reverted['revision']['id'], 'version': 1,
                       'originalSealedSnapshot': True, 'nativePskMatches': True})
        # Audit, native state and API/agent logs must never contain raw material.
        audit = request('GET', '/audit?pageSize=100')
        check(not any(p in json.dumps(audit).encode() for p in markers), 'audit leaked secret')
        for p in [work / 'agent.log', work / 'api.log', state / ('secret-cache-' + prefix + '.sealed')]:
            check(not any(secret in p.read_bytes() for secret in markers), 'secret persisted in plaintext')
        EVENTS.append({'phase': 'noecho', 'apiAgentAuditSealedCache': True})
    finally:
        stop(api)
        stop(agent)
        for f in handles:
            f.close()
        shutdown_leak = False
        if EVENTS and EVENTS[-1].get('phase') == 'noecho':
            for p in [work / 'agent.log', work / 'api.log']:
                shutdown_leak = shutdown_leak or any(marker in p.read_bytes() for marker in markers)
            EVENTS.append({'phase': 'noecho-shutdown', 'passed': not shutdown_leak})
        if api_env is not None:
            # Delete only this run's random prefix; never flush a shared slot DB.
            lua = "local c='0'; local n=0; repeat local p=redis.call('SCAN',c,'MATCH',ARGV[1]..'*','COUNT',100); c=p[1]; for _,k in ipairs(p[2]) do n=n+redis.call('DEL',k) end until c=='0'; return n"
            run(['valkey-cli', '-n', os.environ['NGFW_VALKEY_DB'], 'EVAL', lua, '0', api_env['NGFW_VALKEY_PREFIX']])
            EVENTS.append({'phase': 'valkey-cleanup', 'ownPrefixOnly': True})
        if db_created:
            run([str(ROOT / 'deploy/dev/pg-test.sh'), 'drop', prefix], env=pg_script_env)
        evidence.parent.mkdir(parents=True, exist_ok=True)
        evidence.write_text(json.dumps(EVENTS, indent=2) + '\n')
        # Own private runtime only; no test secret or cache key is left behind.
        import shutil
        shutil.rmtree(work)
        check(not shutdown_leak, 'secret leaked in shutdown logs')


if __name__ == '__main__':
    main()
