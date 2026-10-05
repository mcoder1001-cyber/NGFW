import json
import os
from pathlib import Path
import selectors
import signal
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from driver import API, Refused
from execute import Api, Runner

HERE = Path(__file__).resolve().parent


def alive(pid):
    try:
        return Path(f'/proc/{pid}/stat').read_text().split()[2] != 'Z'
    except FileNotFoundError:
        return False


def ready(process):
    selector = selectors.DefaultSelector()
    try:
        selector.register(process.stdout, selectors.EVENT_READ)
        if not selector.select(5):
            raise RuntimeError('owned test process readiness timeout')
        return json.loads(process.stdout.readline())
    finally:
        selector.close()


class SecurityCleanupChecks(unittest.TestCase):
    def test_slot_api_and_capture_credentials_never_follow_redirect(self):
        sources, sink = [], []
        class Sink(BaseHTTPRequestHandler):
            def do_GET(self):
                sink.append(self.headers.get('Authorization'))
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b'{}')
            def log_message(self, *_):
                pass
        target = ThreadingHTTPServer(('127.0.0.1', 0), Sink)
        class Source(BaseHTTPRequestHandler):
            def do_GET(self):
                sources.append(self.headers.get('Authorization'))
                self.send_response(302)
                self.send_header('Location', f'http://127.0.0.1:{target.server_port}/sink')
                self.end_headers()
            def log_message(self, *_):
                pass
        origin = ThreadingHTTPServer(('127.0.0.1', 0), Source)
        threads = [threading.Thread(target=server.serve_forever, daemon=True) for server in (target, origin)]
        for thread in threads:
            thread.start()
        try:
            api = Api(14, 'NGFW_TEST_PSK_TRAFFIC_C_REDIRECT')
            api.base = f'http://127.0.0.1:{origin.server_port}/api/v1'
            with self.assertRaises(Refused):
                api.call('GET', '/config')
            import urllib.request
            request = urllib.request.Request(api.base + '/state/captures/fixture/file', headers={'Authorization': 'ApiKey NGFW_TEST_PSK_TRAFFIC_C_REDIRECT'})
            with self.assertRaises(Refused):
                api.response(request)
            foundation = API(14, 'NGFW_TEST_PSK_TRAFFIC_C_REDIRECT')
            foundation.base = api.base + '/'
            with self.assertRaises(Refused):
                foundation.request('GET', 'config')
            self.assertEqual(sources, ['ApiKey NGFW_TEST_PSK_TRAFFIC_C_REDIRECT', 'ApiKey NGFW_TEST_PSK_TRAFFIC_C_REDIRECT', 'Bearer NGFW_TEST_PSK_TRAFFIC_C_REDIRECT'])
            self.assertEqual(sink, [])
        finally:
            for server in (origin, target):
                server.shutdown()
                server.server_close()
            for thread in threads:
                thread.join(timeout=5)

    def test_owned_descendant_killed_even_when_leader_exits(self):
        child_source = 'import signal,time;signal.signal(signal.SIGINT,signal.SIG_IGN);signal.signal(signal.SIGTERM,signal.SIG_IGN);time.sleep(60)'
        source = f'import subprocess,sys,json,time;child=subprocess.Popen([sys.executable,"-c",{child_source!r}]);print(json.dumps({{"child":child.pid}}),flush=True);time.sleep(60)'
        leader = subprocess.Popen([sys.executable, '-c', source], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, start_new_session=True)
        try:
            child = ready(leader)['child']
            time.sleep(.1)  # Let the child install its explicit signal ignores.
            Runner.stop(leader)
            end = time.monotonic() + 3
            while alive(child) and time.monotonic() < end:
                time.sleep(.01)
            self.assertFalse(alive(child), 'owned descendant survived after its leader exited')
        finally:
            Runner.stop(leader)
            leader.stdout.close()

    def test_sigterm_enters_finally_cleanup(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / 'cleanup.txt'
            source = f'''import sys,signal,subprocess,time,json
sys.path.insert(0,{str(HERE)!r})
from execute import Runner,interrupt
signal.signal(signal.SIGTERM,interrupt)
child=subprocess.Popen([sys.executable,'-c','import time;time.sleep(60)'],start_new_session=True)
print(json.dumps({{'child':child.pid}}),flush=True)
try:
 time.sleep(60)
finally:
 Runner.stop(child)
 open({str(marker)!r},'w').write('cleanup attempted')
'''
            harness = subprocess.Popen([sys.executable, '-c', source], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, start_new_session=True)
            child = None
            try:
                child = ready(harness)['child']
                harness.send_signal(signal.SIGTERM)
                harness.wait(timeout=10)
                self.assertEqual(marker.read_text(), 'cleanup attempted')
                self.assertFalse(alive(child))
            finally:
                Runner.stop(harness)
                if child is not None:
                    try:
                        os.killpg(child, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                harness.stdout.close()


if __name__ == '__main__':
    unittest.main()
