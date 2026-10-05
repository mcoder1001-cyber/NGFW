#!/usr/bin/env python3
"""Owned slot14 real browser/agent/API acceptance; no shared services changes."""
import os
import signal
import tempfile
from pathlib import Path
import subprocess
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[3]

def browser_acceptance(env, password, request):
    loop = 'loop1431'
    request('config/interfaces', 'PATCH', {loop: {'ipv4': ['10.14.2.1/24']}})
    request('config/services/lb', 'PATCH', {'vips': {'browser-web': {
        'prefix': '10.14.250.1/32', 'protocol': 'tcp', 'port': 80,
        'encap': 'gre4', 'newFlowsTableLength': 1024,
        'servers': [{'address': '10.14.2.10'}]}}})
    request('config/commit', 'POST', {})
    live = request('state/lb/vips')
    assert any(v.get('name') == 'browser-web' and v.get('applied') for v in live['items']), 'real LB VIP not applied'
    browser_env = dict(env, NGFW_HTTP_PORT='11400', NGFW_WEB_PORT='15400',
                       NGFW_BROWSER_PASSWORD=password)
    verified = Path(os.environ['NGFW_BROWSER_VERIFIED_WORKTREE'])
    command = [str(verified / 'apps/web/node_modules/.bin/vite'), 'preview',
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
                node = subprocess.Popen(['node', str(ROOT / 'test/topology/browser-closeout/browser.mjs')],
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
    request('config/services/lb', 'PATCH', {'vips': {'browser-web': None}})
    request('config/interfaces', 'PATCH', {loop: None})
    request('config/commit', 'POST', {})

if __name__ == '__main__':
    assert os.environ.get('NGFW_DISPOSABLE_VPP') == '1'
    assert os.environ.get('NGFW_SLOT') == '14'
    Path(os.environ['NGFW_BROWSER_OUTPUT']).mkdir(parents=True, exist_ok=True)
    original = ROOT / 'test/topology/management-dataplane/acceptance.py'
    source = original.read_text()
    # Checked copy only: preserve all original management/TLS assertions and cleanup.
    # Slot14 addresses/ports replace slot9 constants; original source stays untouched.
    source = source.replace('w9', 'w14').replace("== '9'", "== '14'").replace('3900', '11400').replace('3901', '11401')
    anchor = "        for path in ['config', 'config/candidate', 'state/management/tls', 'secrets']:"
    assert source.count(anchor) == 1
    source = source.replace(anchor, '        browser_acceptance(env, password, request)\n' + anchor)
    source = source.replace('        assert not any(material in corpus for material in materials)',
        '        assert not any(material in corpus for material in materials)\n        assert password.encode() not in corpus\n        assert token.encode() not in corpus')
    namespace = {'__name__': 'owned_browser_fixture', '__file__': str(original),
                 'browser_acceptance': browser_acceptance}
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
