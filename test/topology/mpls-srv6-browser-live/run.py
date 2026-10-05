#!/usr/bin/env python3
"""Owned slot14 real browser/agent/API acceptance; no shared services changes."""
import os
import json
import signal
import tempfile
from pathlib import Path
import subprocess
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[3]

def browser_acceptance(env, password, request, processes, logs):
    assert os.environ['NGFW_DISPOSABLE_VPP'] == '1'
    # The mount namespace owns /run/vpp. Reject the shared socket before writes.
    assert os.stat('/run/vpp/cli.sock').st_ino != int(os.environ['NGFW_SHARED_CLI_INODE'])
    def native(*args):
        return subprocess.check_output(['/usr/bin/vppctl', *args], timeout=15).decode()
    def receipt(label, value):
        print(label + '=' + json.dumps(value, sort_keys=True), flush=True)
    def poll(check, seconds=30):
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            value = check()
            if value:
                return value
            time.sleep(.1)
        raise AssertionError('actual API/native convergence deadline exceeded')
    baseline = request('config/revisions')['items'][0]['id']
    baseline_config = request('config')
    assert 'fd00:e:ff::1' not in native('show', 'sr', 'localsids')
    # This private GlobalsOwner agent creates and owns table 0 itself.
    interfaces = {'loop1460': {'ipv4': ['10.14.31.1/24'], 'ipv6': ['fd00:e:1::1/64']}, 'loop1461': {}}
    srv6 = {
        'localSids': {
            'fd00:e:ff::1': {'behavior': 'end', 'psp': True, 'vrf': 'default'},
            'fd00:e:ff::2': {'behavior': 'end.x', 'psp': False, 'vrf': 'default', 'interface': 'loop1460', 'nextHop': 'fd00:e:1::2'},
            'fd00:e:ff::a': {'behavior': 'end.dt4', 'psp': False, 'vrf': 'default', 'lookupVrf': 'browser-vrf'},
            'fd00:e:ff::b': {'behavior': 'end.dt6', 'psp': False, 'vrf': 'browser-vrf', 'lookupVrf': 'browser-vrf'},
            'fd00:e:ff::c': {'behavior': 'end.dx2', 'psp': False, 'vrf': 'default', 'interface': 'loop1461'}},
        'policies': {
            'fd00:e:bb::1': {'type': 'default', 'encap': True, 'vrf': 'default', 'encapSource': 'fd00:e::1', 'sidLists': [{'sids': ['fd00:e:ee::1', 'fd00:e:ee::a'], 'weight': 1}, {'sids': ['fd00:e:ee::2', 'fd00:e:ee::a'], 'weight': 3}]},
            'fd00:e:bb::2': {'type': 'spray', 'encap': False, 'vrf': 'browser-vrf', 'sidLists': [{'sids': ['fd00:e:ee::3'], 'weight': 1}]}},
        'steering': [{'type': 'l3', 'prefix': '10.14.160.0/24', 'vrf': 'browser-vrf', 'bsid': 'fd00:e:bb::1'}, {'type': 'l3', 'prefix': 'fd00:e:160::/48', 'vrf': 'browser-vrf', 'bsid': 'fd00:e:bb::2'}, {'type': 'l2', 'interface': 'loop1461', 'bsid': 'fd00:e:bb::1'}]}
    mpls = {
        'tables': {'14001': {}}, 'interfaces': ['loop1460'],
        'labelRoutes': [
            {'table': 0, 'label': 140020, 'eos': False, 'paths': [{'interface': 'browser-t1', 'outLabels': [140021], 'weight': 1}]},
            {'table': 14001, 'label': 140016, 'eos': True, 'paths': [{'nextHop': '10.14.31.2', 'interface': 'loop1460', 'outLabels': [140017, 140018], 'weight': 1}]},
            {'table': 14001, 'label': 140030, 'eos': True, 'paths': [{'vrf': 'browser-vrf', 'weight': 1}]}],
        'tunnels': {'browser-t1': {'paths': [{'nextHop': '10.14.31.2', 'interface': 'loop1460', 'outLabels': [140050], 'weight': 1}], 'l2Only': False}},
        'ipBindings': [{'label': 140040, 'vrf': 'browser-vrf', 'prefix': '10.14.40.0/24'}],
        'sr': {'policies': {'140100': {'segmentLists': [{'labels': [140101, 140102], 'weight': 1}], 'spray': False}}, 'steering': [{'vrf': 'default', 'prefix': '10.14.60.0/24', 'bsid': 140100}]}}
    request('config/vrfs', 'PATCH', {'browser-vrf': {'id': 14060}})
    request('config/interfaces', 'PATCH', interfaces)
    request('config/routing/mpls', 'PATCH', mpls)
    request('config/routing/srv6', 'PATCH', srv6)
    committed = request('config/commit', 'POST', {})
    assert committed['status'] == 'applied', committed
    receipt('CONFIGURED_COMMIT', committed)
    def configured():
        state = request('state/srv6')
        return state if len(state['localSids']) == 5 and len(state['policies']) == 2 and len(state['steering']) == 3 and all(x['configured'] for k in ['localSids','policies','steering'] for x in state[k]) else None
    receipt('SRV6_CONFIGURED_STATE', poll(configured))
    fib = request('state/routing/mpls/fib?table=14001')
    assert fib['tableId'] == 14001
    labels = {item['label']: item for item in fib['items']}
    assert labels[140016]['eos'] and labels[140016]['paths'][0]['outLabels'] == [140017, 140018], fib
    assert labels[140030]['eos'] and labels[140030]['paths'][0]['tableId'] == 14060, fib
    tunnels = request('state/routing/mpls/tunnels')
    assert any(t['name'] == 'browser-t1' and t['owned'] and t['paths'][0]['outLabels'] == [140050] for t in tunnels['items']), tunnels
    receipt('MPLS_CONFIGURED_FIB', fib)
    receipt('MPLS_CONFIGURED_TUNNELS', tunnels)
    before_candidate = request('config')
    request('config/routing/srv6', 'PATCH', {'policies': {'fd00:e:bb::1': {'encapSource': None}}})
    invalid = request('config/commit', 'POST', {}, 400)
    assert any(e['pointer'] == '/routing/srv6/policies/fd00:e:bb::1/encapSource' for e in invalid['errors']), invalid
    receipt('MISSING_ENCAP_SOURCE_EXPECTED_400', invalid)
    assert request('config') == before_candidate
    request('config/discard', 'POST', {})
    receipt('DRIFT_DELETE_NATIVE_RESULT', native('sr', 'localsid', 'del', 'prefix', 'fd00:e:ff::1/128'))
    deleted_native = native('show', 'sr', 'localsids')
    receipt('DRIFT_DELETE_NATIVE_READBACK', deleted_native)
    assert 'fd00:e:ff::1' not in deleted_native
    drift_state = request('state/srv6')
    assert not any(x['sid'] == 'fd00:e:ff::1' for x in drift_state['localSids']), drift_state
    drift = request('state/drift')
    assert any('srv6' in x['pointer'] and 'localSids' in x['pointer'] for x in drift['changes']), drift
    receipt('DRIFT_STATE', drift_state)
    receipt('DRIFT_DETECTED', drift)
    request('config/routing/srv6', 'PATCH', srv6)
    receipt('DRIFT_RECOMMIT', request('config/commit', 'POST', {}))
    poll(lambda: 'fd00:e:ff::1' in native('show', 'sr', 'localsids'))
    receipt('DRIFT_RECOVERED_STATE', poll(configured))
    native_before = {name: native('show', 'sr', name) for name in ['localsids', 'policies', 'steering-policies']}
    log_path = Path('/run/ngfw-test/w14/agent.log')
    log_offset = log_path.stat().st_size
    old = processes[0]
    old.terminate()
    old.wait(timeout=15)
    start = time.monotonic()
    replacement = subprocess.Popen([env['NGFW_ACCEPTANCE_AGENT']], env=env, stdout=logs[0], stderr=logs[0])
    processes[0] = replacement
    assert replacement.pid != old.pid
    def restarted():
        try:
            return configured()
        except (OSError, AssertionError):
            return None
    restored = poll(restarted)
    for name, previous in native_before.items():
        assert native('show', 'sr', name) == previous, 'restart altered native SR objects'
    def restart_log():
        content = log_path.read_bytes()[log_offset:]
        return content if b'resync finished' in content and b'APPLY_STATUS_APPLIED' in content else None
    excerpt = poll(restart_log).decode()
    receipt('OWNED_AGENT_RESTART_RESYNC_LOG', excerpt)
    receipt('OWNED_AGENT_RESTART', {'oldPid': old.pid, 'newPid': replacement.pid, 'seconds': time.monotonic()-start, 'state': restored})
    browser_env = dict(env, NGFW_HTTP_PORT='11400', NGFW_WEB_PORT='15400',
                       NGFW_BROWSER_PASSWORD=password)
    verified = Path(os.environ['NGFW_BROWSER_VERIFIED_WORKTREE'])
    command = [os.environ.get('NGFW_BROWSER_VITE', str(verified / 'apps/web/node_modules/.bin/vite')), 'preview',
               '--outDir', os.environ['NGFW_BROWSER_WEB_DIST'], '--host', '127.0.0.1', '--port', '15400']
    with (Path(os.environ['NGFW_BROWSER_OUTPUT']) / 'web.log').open('wb') as log:
        web = subprocess.Popen(command, cwd=verified / 'apps/web', env=browser_env,
                               stdout=log, stderr=log)
        try:
            for _ in range(100):
                try:
                    with urllib.request.urlopen('http://127.0.0.1:15400', timeout=2) as response:
                        assert response.status == 200
                    break
                except OSError:
                    if web.poll() is not None:
                        raise RuntimeError('owned web preview exited')
                    time.sleep(.1)
            else:
                raise RuntimeError('owned web preview did not become ready')
            with tempfile.TemporaryDirectory(prefix='ngfw-browser-', dir=env['TMPDIR']) as profile:
                browser_env['NGFW_BROWSER_PROFILE'] = profile
                node = subprocess.Popen(['node', str(ROOT / 'test/topology/mpls-srv6-browser-live/browser.mjs')],
                                        env=browser_env, start_new_session=True)
                try:
                    code = node.wait(timeout=600)
                    if code:
                        raise RuntimeError(f'owned browser exited {code}')
                finally:
                    # Only this freshly created child group; survives a Node timeout/error.
                    try:
                        os.killpg(node.pid, signal.SIGTERM)
                    except ProcessLookupError:
                        pass
                    if node.poll() is None:
                        try:
                            node.wait(timeout=5)
                        except subprocess.TimeoutExpired:
                            os.killpg(node.pid, signal.SIGKILL)
                            node.wait()

        finally:
            web.terminate()
            try:
                web.wait(timeout=10)
            except subprocess.TimeoutExpired:
                web.kill(); web.wait()
    rollback = request('config/rollback/' + str(baseline), 'POST', {})
    receipt('BASELINE_REVISION_ROLLBACK', rollback)
    assert rollback['status'] == 'applied'
    state = request('state/srv6')
    assert all(len(state[k]) == 0 for k in ['localSids', 'policies', 'steering']), state
    assert 'fd00:e:ff::' not in native('show', 'sr', 'localsids')
    assert 'fd00:e:bb::' not in native('show', 'sr', 'policies')
    assert request('config') == baseline_config
    assert '140016' not in native('show', 'mpls', 'fib')
    assert not any(x['tableId'] == 0 for x in request('state/routing/mpls/tunnels')['tables'])
    print('MPLS_SRV6_REAL_API_LIFECYCLE=PASS', flush=True)

if __name__ == '__main__':
    assert os.environ.get('NGFW_DISPOSABLE_VPP') == '1'
    assert os.environ.get('NGFW_SLOT') == '14'
    Path(os.environ['NGFW_BROWSER_OUTPUT']).mkdir(parents=True, exist_ok=True)
    original = ROOT / 'test/topology/management-dataplane/acceptance.py'
    source = original.read_text()
    # Checked copy only: preserve all original management/TLS assertions and cleanup.
    # Slot14 addresses/ports replace slot9 constants; original source stays untouched.
    source = source.replace('w9', 'w14').replace("== '9'", "== '14'").replace('3900', '11400').replace('3901', '11401').replace("NGFW_GLOBALS_OWNER='0'", "NGFW_GLOBALS_OWNER='1'")
    anchor = "        for path in ['config', 'config/candidate', 'state/management/tls', 'secrets']:"
    assert source.count(anchor) == 1
    source = source.replace(anchor, '        browser_acceptance(env, password, request, processes, logs)\n' + anchor)
    source = source.replace("        assert status == expected, f", "        if status != expected and path != 'auth/login':\n            print('UNEXPECTED_HTTP_RESPONSE=' + json.dumps(value, sort_keys=True), flush=True)\n        assert status == expected, f")
    source = source.replace('        assert not any(material in corpus for material in materials)',
        '        assert not any(material in corpus for material in materials)\n        assert password.encode() not in corpus\n        assert token.encode() not in corpus')
    def diagnostic_callback(env, password, request, processes, logs):
        try:
            return browser_acceptance(env, password, request, processes, logs)
        except Exception:
            if os.environ.get('NGFW_OWNED_STACK_DIAG') == '1':
                owned = processes[0]
                if owned.poll() is None:
                    # Diagnostic failure only: SIGQUIT this exact freshly owned child.
                    owned.send_signal(signal.SIGQUIT)
                    owned.wait(timeout=15)
            raise
        finally:
            # Preserve only this slot's service logs before original cleanup removes them.
            for name in ['agent.log', 'api.log']:
                path = Path('/run/ngfw-test/w14') / name
                if path.exists():
                    content = path.read_bytes()
                    assert password.encode() not in content
                    assert b'BEGIN PRIVATE KEY' not in content
                    (Path(os.environ['NGFW_BROWSER_OUTPUT']) / name).write_bytes(content)
    namespace = {'__name__': 'owned_browser_fixture', '__file__': str(original),
                 'browser_acceptance': diagnostic_callback}
    exec(compile(source, str(original), 'exec'), namespace)
    try:
        namespace['main']()
    finally:
        prefix = 'ngfw:w14:acceptance:'
        keys = subprocess.run(['valkey-cli', '-n', '14', '--scan', '--pattern', prefix + '*'],
                              check=True, stdout=subprocess.PIPE).stdout.decode().splitlines()
        assert all(key.startswith(prefix) for key in keys)
        for offset in range(0, len(keys), 200):
            subprocess.run(['valkey-cli', '-n', '14', 'UNLINK', *keys[offset:offset+200]],
                           check=True, stdout=subprocess.DEVNULL)
        print('OWNED_VALKEY_KEYS_REMOVED=' + str(len(keys)), flush=True)
